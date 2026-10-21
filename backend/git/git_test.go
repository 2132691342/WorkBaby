// 工作区版本视图链路：真仓库端到端，按坏法分三条——读、写、分支。
// 坏了 = 界面「改了什么 / 提交成没成 / 在哪个分支」与 git 说的不一致。
package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
)

// TestGitChain 每一步都是真实 git 子进程（整套约 9 秒），-short 下跳过：
// 它是唯一值得为速度让步的一条——其余链路都用假进程。
func TestGitChain(t *testing.T) {
	if testing.Short() {
		t.Skip("-short：真 git 子进程链路，提交前跑全量")
	}
	if !Available() {
		t.Skip("这台机器上没有 git")
	}

	// 读链路：状态归类、中文带空格路径还原、行号归属、未跟踪差异，
	// 与读侧护栏（越界拒绝、非仓库给人话而不是错误码）。
	t.Run("读链路：状态、差异与护栏", func(t *testing.T) {
		dir := initRepo(t)
		writeFile(t, dir, "周报 数据.txt", "本周进展顺利\n")
		mustGit(t, dir, "add", ".")
		mustGit(t, dir, "commit", "-q", "-m", "加周报")

		writeFile(t, dir, "a.txt", "one\nTWO\n")
		writeFile(t, dir, "周报 数据.txt", "本周进展顺利\n下周继续\n")
		writeFile(t, dir, "new.txt", "brand\n")

		st, err := Status(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !st.Available || !st.IsRepo || st.Branch == "" {
			t.Fatalf("状态不对：%#v", st)
		}
		got := map[string]domain.GitFileVO{}
		for _, f := range st.Files {
			got[f.Path] = f
		}
		// porcelain 会给非 ASCII 路径加引号并做八进制转义，界面必须拿到还原后的原文。
		if f, ok := got["周报 数据.txt"]; !ok || f.Status != domain.GitChangeModified {
			t.Fatalf("中文带空格路径应被还原并认成改过：%#v（全量 %v）", f, st.Files)
		}
		if got["a.txt"].Status != domain.GitChangeModified ||
			got["a.txt"].Additions != 1 || got["a.txt"].Deletions != 1 {
			t.Fatalf("a.txt 应认成改过且增删各 1 行：%#v", got["a.txt"])
		}
		if got["new.txt"].Status != domain.GitChangeUntracked {
			t.Fatalf("new.txt 应被认成未跟踪：%#v", got["new.txt"])
		}

		// 行号必须精确：错一位，界面上整个文件的行号都会漂移。
		d, err := Diff(dir, "a.txt")
		if err != nil {
			t.Fatal(err)
		}
		if !hasLine(d, domain.GitLineDel, "two") || !hasLine(d, domain.GitLineAdd, "TWO") {
			t.Fatalf("差异内容不对：%#v", d.Lines)
		}
		for _, l := range d.Lines {
			if l.Kind == domain.GitLineDel && l.Text == "two" && l.OldNo != 2 {
				t.Fatalf("删除行号归属不对：%#v", l)
			}
			if l.Kind == domain.GitLineAdd && l.Text == "TWO" && l.NewNo != 2 {
				t.Fatalf("新增行号归属不对：%#v", l)
			}
		}

		// 整个工作区：空 pathspec 会被 git 判非法，这里必须拿到内容而不是静默为空。
		d3, err := Diff(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		if !hasLine(d3, domain.GitLineAdd, "TWO") {
			t.Fatalf("整个工作区的差异为空：%#v", d3.Lines)
		}

		// 未跟踪文件 git diff 给不出内容：必须整篇读作新增行，
		// 否则「新建的文件」在工作区页永远看不到自己改了什么。
		d2, err := Diff(dir, "new.txt")
		if err != nil {
			t.Fatal(err)
		}
		if !hasLine(d2, domain.GitLineAdd, "brand") {
			t.Fatalf("新文件的差异不对：%#v", d2.Lines)
		}

		// 护栏两条同属「界面不能替用户瞎猜」：越界直接拒，非仓库给人话。
		// git 的 128 原文随系统语言变，照原文分流会在中文 Windows 上漏判。
		if _, err := Diff(t.TempDir(), "../outside.txt"); err == nil {
			t.Fatal("工作区之外的路径必须被拒绝")
		}
		plain := t.TempDir()
		st, err = Status(plain)
		if err != nil {
			t.Fatalf("不是仓库不该报错：%v", err)
		}
		if !st.Available || st.IsRepo || st.Reason == "" {
			t.Fatalf("应给出「还不是仓库」的引导：%#v", st)
		}
		if _, err := Diff(plain, ""); err == nil || !strings.Contains(err.Error(), "还不是 Git 仓库") {
			t.Fatalf("差异应给出人话而不是 git 原文：%v", err)
		}
	})

	// 写链路：白名单参数校验 → 勾选入栈 → 提交 → 历史 / 文件历史可读 → 出栈只动索引。
	// 坏了 = 「提交」按钮看起来成功了，但历史里找不到；或没勾选的文件被带进了提交。
	t.Run("写链路：入栈、提交、历史与出栈", func(t *testing.T) {
		dir := initRepo(t)
		// 越界路径在写入时更要命：会把工作区之外的文件收进索引，静默降级成「全部」更不行。
		if _, err := Stage(dir, []string{"../secret.txt"}); err == nil {
			t.Fatal("工作区之外的路径入栈必须被拒")
		}

		writeFile(t, dir, "a.txt", "one\nTWO\n")
		writeFile(t, dir, "b.txt", "brand\n")

		// 只入栈一个：另一个必须仍是未入栈，否则「勾选入栈」会连带把不想提交的带进去。
		st, err := Stage(dir, []string{"a.txt"})
		if err != nil {
			t.Fatal(err)
		}
		byPath := map[string]domain.GitFileVO{}
		for _, f := range st.Files {
			byPath[f.Path] = f
		}
		if !byPath["a.txt"].Staged {
			t.Fatalf("a.txt 应已入栈：%#v", byPath["a.txt"])
		}
		if byPath["b.txt"].Staged {
			t.Fatalf("b.txt 不该被带入栈：%#v", byPath["b.txt"])
		}

		// 空信息必须拒：空提交在历史里无法辨认。
		if _, err := Commit(dir, "   "); err == nil {
			t.Fatal("空提交信息必须被拒绝")
		}
		res, err := Commit(dir, "改 a.txt")
		if err != nil {
			t.Fatal(err)
		}
		if res.Hash == "" {
			t.Fatalf("提交后应带回 hash：%#v", res)
		}
		// 提交后 a.txt 消失、b.txt 以未跟踪留下——这正是「勾选什么提交什么」的证据。
		left := map[string]domain.GitFileVO{}
		for _, f := range res.Status.Files {
			left[f.Path] = f
		}
		if _, ok := left["a.txt"]; ok {
			t.Fatalf("a.txt 已提交，不该还在改动列表：%#v", res.Status.Files)
		}
		if f, ok := left["b.txt"]; !ok || f.Status != domain.GitChangeUntracked {
			t.Fatalf("b.txt 应仍是未入栈的新文件：%#v", left)
		}

		// 历史与单次提交必须马上能读到：读不到就是「提交成功」是假消息。
		history, err := History(dir, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(history) != 2 || history[0].Subject != "改 a.txt" || history[0].Files != 1 {
			t.Fatalf("历史不对：%#v", history)
		}
		detail, err := CommitDetail(dir, history[0].Hash)
		if err != nil {
			t.Fatal(err)
		}
		if len(detail.Files) != 1 || detail.Diff == nil || !hasLine(detail.Diff, domain.GitLineAdd, "TWO") {
			t.Fatalf("提交内容不完整：%#v", detail)
		}

		// 文件历史：没入栈的 b.txt 不该出现在任何提交里；入栈并提交后才该有它。
		fh, err := FileHistory(dir, "b.txt", 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(fh) != 0 {
			t.Fatalf("未入栈的文件不该有提交历史：%#v", fh)
		}
		if _, err := Stage(dir, []string{"b.txt"}); err != nil {
			t.Fatal(err)
		}
		if _, err := Commit(dir, "加上 b.txt"); err != nil {
			t.Fatal(err)
		}
		fh2, err := FileHistory(dir, "b.txt", 10)
		if err != nil || len(fh2) != 1 || fh2[0].Subject != "加上 b.txt" {
			t.Fatalf("入栈提交后文件历史该有它：%#v err=%v", fh2, err)
		}

		// 出栈只动索引：文件内容必须原样留在工作区。
		writeFile(t, dir, "c.txt", "three\n")
		if _, err := Stage(dir, []string{"c.txt"}); err != nil {
			t.Fatal(err)
		}
		if _, err := Unstage(dir, []string{"../secret.txt"}); err == nil {
			t.Fatal("工作区之外的路径出栈必须被拒")
		}
		st2, err := Unstage(dir, []string{"c.txt"})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range st2.Files {
			if f.Path == "c.txt" && f.Staged {
				t.Fatalf("c.txt 应已出栈：%#v", f)
			}
		}
		if raw, rerr := os.ReadFile(filepath.Join(dir, "c.txt")); rerr != nil || string(raw) != "three\n" {
			t.Fatalf("出栈不该动工作区文件：%q err=%v", raw, rerr)
		}
	})

	// 分支链路：名字白名单 → 新建并切过去 → 切回后历史隔离。
	// 坏了 = 面板显示切成功了，实际还在原分支上写东西。
	t.Run("分支链路：白名单、新建、切换与历史隔离", func(t *testing.T) {
		// 以连字符开头会被 git 当成选项；`..`、空格、结尾斜杠、`.lock` 都指向意外的 ref。
		for _, bad := range []string{"-x", "a..b", "a b", "feature/", "x.lock"} {
			if validBranchName(bad) {
				t.Fatalf("分支名 %q 应被拒", bad)
			}
		}
		if !validBranchName("feature/x") || !validBranchName("v1.2.3") {
			t.Fatal("正常的分支名应被接受")
		}

		dir := initRepo(t)
		base, err := Status(dir)
		if err != nil {
			t.Fatal(err)
		}

		st, err := CreateBranch(dir, "feature/x")
		if err != nil || st.Branch != "feature/x" {
			t.Fatalf("新建后应切到新分支：%s err=%v", st.Branch, err)
		}
		writeFile(t, dir, "a.txt", "one\nTWO\n")
		if _, err := Stage(dir, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := Commit(dir, "在新分支上改一笔"); err != nil {
			t.Fatal(err)
		}

		list, err := Branches(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 2 {
			t.Fatalf("应有两条分支（非当前分支被丢掉是解析回归）：%#v", list)
		}
		for _, b := range list {
			if (b.Name == "feature/x") != b.Current {
				t.Fatalf("current 标记不对：%#v", b)
			}
		}

		// 切回基干：新分支上那条提交不该出现在这里。
		st, err = SwitchBranch(dir, base.Branch)
		if err != nil || st.Branch != base.Branch {
			t.Fatalf("没切回 %s：%s err=%v", base.Branch, st.Branch, err)
		}
		if h, err := History(dir, 10); err != nil || len(h) != 1 || h[0].Subject != "init" {
			t.Fatalf("基干上不该看到新分支的提交：%#v err=%v", h, err)
		}
	})
}

// initRepo 造一个已有一次提交的临时仓库：三条链路都要的公共前奏。
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q", "--initial-branch=main")
	mustGit(t, dir, "config", "user.email", "workbaby@local")
	mustGit(t, dir, "config", "user.name", "WorkBaby")
	writeFile(t, dir, "a.txt", "one\ntwo\n")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-q", "-m", "init")
	return dir
}

// mustGit 执行一条 git 命令，失败即停。
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := run(dir, statusTimeout, args...); err != nil {
		t.Fatalf("git %v 失败：%v", args, err)
	}
}

// writeFile 在 dir 下写一个文件。
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hasLine 报告差异里有没有指定种类与内容的一行。
func hasLine(vo *domain.GitDiffVO, kind, text string) bool {
	for _, l := range vo.Lines {
		if l.Kind == kind && l.Text == text {
			return true
		}
	}
	return false
}
