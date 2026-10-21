// 工作区版本视图链路：真仓库端到端，按坏法分五条；场景尽量共用同一个仓库（仓库数 = 子进程数 = 耗时）。
// 坏了 = 界面「改了什么 / 提交成没成 / 在哪个分支 / 冲突到哪一步」与 git 说的不一致。
package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// TestGitChain 每一步都是真实 git 子进程（整套约 20 秒），-short 下跳过：
// 它是唯一值得为速度让步的一条——其余链路都用假进程。
func TestGitChain(t *testing.T) {
	if testing.Short() {
		t.Skip("-short：真 git 子进程链路，提交前跑全量")
	}
	if !Available() {
		t.Skip("这台机器上没有 git")
	}

	// 读链路：路径换算是这条链的地基——工作区相对与仓库根相对换算错，
	// 子目录工作区里暂存与差异必然失败；状态归类与差异还原错了，界面对账就失去意义。
	t.Run("读链路：路径换算、状态归类、差异与护栏", func(t *testing.T) {
		root := initRepo(t)
		sub := filepath.Join(root, "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, sub, "周报 数据.txt", "本周进展顺利\n")
		mustGit(t, root, "add", ".")
		mustGit(t, root, "commit", "-q", "-m", "加周报")

		// 在子目录工作区里改文件：status 必须报工作区相对路径（不带 sub/ 前缀），
		// 且这个路径能直接喂回 Stage / Diff——那是界面的真实动作序列。
		writeFile(t, sub, "周报 数据.txt", "本周进展顺利\n下周继续\n")
		writeFile(t, sub, "a.txt", "one\nTWO\n")
		writeFile(t, sub, "new.txt", "brand\n")
		writeFile(t, root, "outside.txt", "o\n")

		st, err := Status(sub)
		if err != nil {
			t.Fatal(err)
		}
		if !st.Available || !st.IsRepo || st.Branch == "" {
			t.Fatalf("状态不对：%#v", st)
		}
		if st.RepoRoot == sub {
			t.Fatalf("RepoRoot 应是仓库根而不是工作目录：%q", st.RepoRoot)
		}
		got := map[string]domain.GitFileVO{}
		for _, f := range st.Files {
			if strings.HasPrefix(f.Path, "sub/") || f.Path == "outside.txt" {
				t.Fatalf("子目录工作区应只报工作区相对路径，不该看到 %q（全量 %v）", f.Path, st.Files)
			}
			got[f.Path] = f
		}
		// porcelain 会给非 ASCII 路径加引号并做八进制转义，界面必须拿到还原后的原文。
		// 增删行数对已提交过的文件才准（未跟踪文件不在 numstat 里，按 0 计）。
		if f, ok := got["周报 数据.txt"]; !ok || f.Status != domain.GitChangeModified {
			t.Fatalf("中文带空格路径应被还原并认成改过：%#v（全量 %v）", f, st.Files)
		}
		if got["new.txt"].Status != domain.GitChangeUntracked {
			t.Fatalf("new.txt 应被认成未跟踪：%#v", got["new.txt"])
		}

		// 用 status 交出来的路径直接暂存：换算错就是「pathspec did not match」。
		if _, err := Stage(sub, []string{"a.txt"}); err != nil {
			t.Fatalf("子目录工作区暂存失败：%v", err)
		}
		staged, err := Status(sub)
		if err != nil {
			t.Fatal(err)
		}
		if !findFile(staged, "a.txt").Staged {
			t.Fatalf("a.txt 应已入栈：%#v", staged.Files)
		}

		// 单文件差异：行号精确到个位，错一位整块行号漂移。
		// a.txt 在这个仓库里是新文件，diff 给的是整篇新增（@@ -0,0 +1,2 @@）。
		d, err := Diff(sub, "a.txt")
		if err != nil {
			t.Fatal(err)
		}
		if !hasLine(d, domain.GitLineAdd, "one") || !hasLine(d, domain.GitLineAdd, "TWO") {
			t.Fatalf("差异内容不对：%#v", d.Lines)
		}
		for _, l := range d.Lines {
			if l.Kind == domain.GitLineAdd && l.Text == "TWO" && l.NewNo != 2 {
				t.Fatalf("新增行号归属不对：%#v", l)
			}
		}
		// 文件头必须是工作区相对：与 status 上报的路径同一套写法。
		for _, l := range d.Lines {
			if l.Kind == domain.GitLineFile && strings.HasPrefix(l.Text, "sub/") {
				t.Fatalf("差异文件头带了仓库前缀：%q", l.Text)
			}
		}

		// 整个工作区：空 pathspec 会被 git 判非法；未跟踪文件不在索引里，
		// git diff 给不出内容，必须整篇按新增行补读出来。
		d3, err := Diff(sub, "")
		if err != nil {
			t.Fatal(err)
		}
		if !hasLine(d3, domain.GitLineAdd, "TWO") || !hasLine(d3, domain.GitLineAdd, "brand") {
			t.Fatalf("整篇差异应含已跟踪与未跟踪文件的改动：%#v", d3.Lines)
		}

		// 护栏：越界直接拒；不是仓库给人话而不是 git 的 128 原文（原文随系统语言变）。
		if _, err := Diff(sub, "../outside.txt"); err == nil {
			t.Fatal("工作区之外的路径必须被拒绝")
		}
		plain := t.TempDir()
		ps, err := Status(plain)
		if err != nil || !ps.Available || ps.IsRepo || ps.Reason == "" {
			t.Fatalf("不是仓库应给引导而不是错误：%#v err=%v", ps, err)
		}
		if _, err := Diff(plain, ""); err == nil || !strings.Contains(err.Error(), "还不是 Git 仓库") {
			t.Fatalf("差异应给出人话：%v", err)
		}
	})

	// 提交链路：勾选什么提交什么是界面对用户的承诺；失败归因决定用户能不能自己走出下一步。
	// 坏了 = 没勾选的文件被带进提交、提交失败只报 exit status 1、出栈动了工作区文件。
	t.Run("提交链路：勾选入栈、失败归因、历史与出栈", func(t *testing.T) {
		dir := initRepo(t)
		// 越界路径在写入时更要命：会把工作区之外的文件收进索引。
		if _, err := Stage(dir, []string{"../secret.txt"}); err == nil {
			t.Fatal("工作区之外的路径入栈必须被拒")
		}

		writeFile(t, dir, "a.txt", "one\nTWO\n")
		writeFile(t, dir, "b.txt", "brand\n")
		st, err := Stage(dir, []string{"a.txt"})
		if err != nil {
			t.Fatal(err)
		}
		if !findFile(st, "a.txt").Staged || findFile(st, "b.txt").Staged {
			t.Fatalf("只勾选 a.txt 时 b.txt 不该被带入栈：%#v", st.Files)
		}

		// 空信息必须拒：空提交在历史里无法辨认。
		if _, err := Commit(dir, "   ", false); pkgCode(err) != "1016" {
			t.Fatalf("空提交信息必须被拒，实际：%v", err)
		}
		res, err := Commit(dir, "改 a.txt", false)
		if err != nil {
			t.Fatal(err)
		}
		if res.Hash == "" {
			t.Fatalf("提交后应带回 hash：%#v", res)
		}
		// 提交后 a.txt 消失、b.txt 以未跟踪留下——这正是「勾选什么提交什么」的证据。
		if findFile(res.Status, "a.txt").Path != "" ||
			findFile(res.Status, "b.txt").Status != domain.GitChangeUntracked {
			t.Fatalf("提交后的状态不对：%#v", res.Status.Files)
		}

		// 失败归因：git 把失败原因写在 stdout，归因丢了用户就只剩一句 exit status 1。
		if _, err := Commit(dir, "空提交", false); pkgCode(err) != "1017" {
			t.Fatalf("无改动提交应报 1017，实际：%v", err)
		}
		noIdent := t.TempDir()
		mustGit(t, noIdent, "init", "-q", "--initial-branch=main")
		writeFile(t, noIdent, "a.txt", "a\n")
		if _, err := Stage(noIdent, nil); err != nil {
			t.Fatal(err)
		}
		emptyCfg := filepath.Join(noIdent, "empty.gitconfig")
		if err := os.WriteFile(emptyCfg, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		// 只在这一条里屏蔽全局身份配置，避免污染同包其它链路的 git 行为。
		oldGlobal, hadGlobal := os.LookupEnv("GIT_CONFIG_GLOBAL")
		os.Setenv("GIT_CONFIG_GLOBAL", emptyCfg)
		_, err = Commit(noIdent, "无身份", false)
		if hadGlobal {
			os.Setenv("GIT_CONFIG_GLOBAL", oldGlobal)
		} else {
			os.Unsetenv("GIT_CONFIG_GLOBAL")
		}
		if pkgCode(err) != "1020" {
			t.Fatalf("缺身份提交应报 1020，实际：%v", err)
		}

		// 历史与单次提交必须马上能读到：读不到就是「提交成功」是假消息。
		// 行数统计对已跟踪文件的改动才准（a.txt 是新文件，整篇都是新增）。
		history, err := History(dir, 10)
		if err != nil || len(history) != 2 || history[0].Subject != "改 a.txt" {
			t.Fatalf("历史不对：%#v err=%v", history, err)
		}
		detail, err := CommitDetail(dir, history[0].Hash)
		if err != nil || len(detail.Files) != 1 || detail.Diff == nil ||
			!hasLine(detail.Diff, domain.GitLineAdd, "TWO") {
			t.Fatalf("提交内容不完整：%#v err=%v", detail, err)
		}
		// 提交抽屉里的文件头是仓库根相对（与仓库全景一致），能定位到文件即可。
		headerOK := false
		for _, l := range detail.Diff.Lines {
			if l.Kind == domain.GitLineFile && strings.HasSuffix(l.Text, "a.txt") {
				headerOK = true
			}
		}
		if !headerOK {
			t.Fatalf("提交差异没有文件头：%#v", detail.Diff.Lines)
		}

		// 出栈只动索引：文件内容必须原样留在工作区。
		writeFile(t, dir, "c.txt", "three\n")
		if _, err := Stage(dir, []string{"c.txt"}); err != nil {
			t.Fatal(err)
		}
		st2, err := Unstage(dir, []string{"c.txt"})
		if err != nil {
			t.Fatal(err)
		}
		if findFile(st2, "c.txt").Staged {
			t.Fatalf("c.txt 应已出栈：%#v", st2.Files)
		}
		if raw, rerr := os.ReadFile(filepath.Join(dir, "c.txt")); rerr != nil ||
			strings.ReplaceAll(string(raw), "\r\n", "\n") != "three\n" {
			t.Fatalf("出栈不该动工作区文件：%q err=%v", raw, rerr)
		}

		// 丢弃：空 paths 等于一次不可逆清空，必须被拒；显式路径时已跟踪回 HEAD、未跟踪删除。
		if _, err := Discard(dir, nil); pkgCode(err) != "1018" {
			t.Fatalf("不指定文件的丢弃必须被拒，实际：%v", err)
		}
		writeFile(t, dir, "a.txt", "one\nCHANGED\n")
		writeFile(t, dir, "junk.txt", "j\n")
		st3, err := Discard(dir, []string{"a.txt", "junk.txt"})
		if err != nil {
			t.Fatalf("丢弃失败：%v", err)
		}
		// a.txt 与 junk.txt 被丢弃；b.txt / c.txt 从未提交过，仍以未跟踪留下——这正是只动指定文件的证据。
		if findFile(st3, "a.txt").Path != "" || findFile(st3, "junk.txt").Path != "" ||
			findFile(st3, "b.txt").Status != domain.GitChangeUntracked ||
			findFile(st3, "c.txt").Status != domain.GitChangeUntracked {
			t.Fatalf("丢弃只应动指定文件：%#v", st3.Files)
		}
		if raw, rerr := os.ReadFile(filepath.Join(dir, "a.txt")); rerr != nil ||
			strings.ReplaceAll(string(raw), "\r\n", "\n") != "one\nTWO\n" {
			t.Fatalf("已跟踪文件应回到 HEAD 的内容：%q err=%v", raw, rerr)
		}
		if _, serr := os.Stat(filepath.Join(dir, "junk.txt")); !os.IsNotExist(serr) {
			t.Fatalf("未跟踪文件应被删除：err=%v", serr)
		}

		// 改写：历史条数不变、信息变成新的——多了或少了都是改错了提交。
		before, _ := History(dir, 5)
		writeFile(t, dir, "amend.txt", "am\n")
		if _, err := Stage(dir, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := Commit(dir, "第一次写的", false); err != nil {
			t.Fatal(err)
		}
		if _, err := Commit(dir, "改好的摘要", true); err != nil {
			t.Fatalf("amend 失败：%v", err)
		}
		after, _ := History(dir, 5)
		if len(after) != len(before)+1 || after[0].Subject != "改好的摘要" {
			t.Fatalf("amend 后历史不对：before=%d after=%d", len(before), len(after))
		}
	})

	// 推送链路：首次推送建上游、远端被别人推进后各档推送的表现。
	// 坏了 = 界面说推送成功其实没推上去，或者一下点就把远端别人的提交覆盖掉。
	t.Run("推送链路：自动建上游、被拒与分级强制推送", func(t *testing.T) {
		root, dir := initRepoWithRemote(t)
		if _, err := Push(dir, domain.GitPushREQ{}); err != nil {
			t.Fatalf("首次推送应自动建立上游：%v", err)
		}
		st, err := Status(dir)
		if err != nil || st.Upstream == "" || len(st.Remotes) != 1 {
			t.Fatalf("推送后应带上上游与远端：upstream=%q remotes=%#v err=%v", st.Upstream, st.Remotes, err)
		}
		// 远端地址里不该混进 remote -v 的 `(fetch)` 标记。
		if strings.Contains(st.Remotes[0].URL, "(fetch)") {
			t.Fatalf("远端地址被 remote -v 的标记污染：%#v", st.Remotes[0])
		}
		if _, err := Push(dir, domain.GitPushREQ{}); err != nil {
			t.Fatalf("二次推送应直接成功：%v", err)
		}
		// 纯 --force 必须输分支名确认，否则相当于「点一下就覆盖远端」。
		if _, err := Push(dir, domain.GitPushREQ{Force: domain.GitForceTrue}); pkgCode(err) != "1018" {
			t.Fatalf("未确认的纯强制推送必须被拒，实际：%v", err)
		}

		// 远端被别人的提交推进：普通推送必须被拒并给出「先拉取」的下一步。
		other := filepath.Join(root, "other")
		mustGit(t, root, "clone", "-q", filepath.Join(root, "remote.git"), other)
		mustGit(t, other, "config", "user.email", "other@local")
		mustGit(t, other, "config", "user.name", "Other")
		writeFile(t, other, "other.txt", "o\n")
		mustGit(t, other, "add", ".")
		mustGit(t, other, "commit", "-q", "-m", "other")
		mustGit(t, other, "push", "-q", "origin", "main")

		writeFile(t, dir, "mine.txt", "m\n")
		if _, err := Stage(dir, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := Commit(dir, "mine", false); err != nil {
			t.Fatal(err)
		}
		if _, err := Push(dir, domain.GitPushREQ{}); pkgCode(err) != "1024" {
			t.Fatalf("远端领先时普通推送应报 1024，实际：%v", err)
		}
		// 带租约的强制推送必须拒：本地跟踪引用还停在上次同步的提交，而远端已经变了——
		// 租约正是靠这个差距保护别人的提交；先「取回远端」再推，租约与远端一致，才允许覆盖。
		if _, err := Push(dir, domain.GitPushREQ{Force: domain.GitForceLease}); pkgCode(err) != "1027" {
			t.Fatalf("远端领先时带租约的强制推送应报 1027，实际：%v", err)
		}
		if _, err := Fetch(dir, ""); err != nil {
			t.Fatalf("取回远端状态失败：%v", err)
		}
		if _, err := Push(dir, domain.GitPushREQ{Force: domain.GitForceLease}); err != nil {
			t.Fatalf("已同步远端后带租约的强制推送应成功：%v", err)
		}
	})

	// 合并冲突是「界面说成了但其实没成」的最高发区：冲突必须落地为可继续的状态，
	// 每一步（未解决就提交 / 未解决就继续）都要被点名拦住，解决后继续、中途都能中止。
	t.Run("合并冲突：落地为状态、逐文件解决、继续与中止", func(t *testing.T) {
		dir := initRepo(t)
		mustGit(t, dir, "checkout", "-q", "-b", "feature")
		writeFile(t, dir, "a.txt", "feature-side\n")
		mustGit(t, dir, "add", ".")
		mustGit(t, dir, "commit", "-q", "-m", "feat")
		mustGit(t, dir, "checkout", "-q", "main")
		writeFile(t, dir, "a.txt", "main-side\n")
		mustGit(t, dir, "add", ".")
		mustGit(t, dir, "commit", "-q", "-m", "main2")

		cst, err := Merge(dir, "feature", false)
		if err != nil {
			t.Fatalf("冲突应作为可继续的状态返回，而不是错误：%v", err)
		}
		if cst.State != domain.GitStateMerge || !cst.Conflict {
			t.Fatalf("冲突后状态应标记 merge 且 conflict=true：%#v", cst)
		}
		cf := findFile(cst, "a.txt")
		if !cf.Conflict || cf.Status != domain.GitChangeConflict {
			t.Fatalf("冲突文件应带 conflict 标记：%#v", cf)
		}
		if _, err := Commit(dir, "试提交", false); pkgCode(err) != "1021" {
			t.Fatalf("冲突未解决时提交应报 1021，实际：%v", err)
		}
		if _, err := UpdateState(dir, domain.GitStateContinue); pkgCode(err) != "1021" {
			t.Fatalf("冲突未解决时继续应报 1021，实际：%v", err)
		}

		writeFile(t, dir, "a.txt", "resolved\n")
		rst, err := ResolveConflict(dir, []string{"a.txt"})
		if err != nil || findFile(rst, "a.txt").Conflict {
			t.Fatalf("标记已解决后不该再有冲突标记：%#v err=%v", rst.Files, err)
		}
		done, err := UpdateState(dir, domain.GitStateContinue)
		if err != nil || done.State != domain.GitStateNone || done.Conflict {
			t.Fatalf("解决冲突后应能继续并回到普通状态：%#v err=%v", done, err)
		}
		if h, herr := History(dir, 3); herr != nil || len(h) < 1 || !strings.Contains(h[0].Subject, "feature") {
			t.Fatalf("合并后历史首条应是合并提交：%#v err=%v", h, herr)
		}

		// 中止：仓库必须回到开始前的状态（干净、无中间状态）。
		mustGit(t, dir, "checkout", "-q", "-B", "conf2", "HEAD~1")
		writeFile(t, dir, "a.txt", "conf2-side\n")
		mustGit(t, dir, "add", ".")
		mustGit(t, dir, "commit", "-q", "-m", "conf2")
		mustGit(t, dir, "checkout", "-q", "main")
		if _, err := Merge(dir, "conf2", false); err != nil {
			t.Fatalf("第二次合并冲突也该返回状态：%v", err)
		}
		aborted, err := UpdateState(dir, domain.GitStateAbort)
		if err != nil || aborted.State != domain.GitStateNone || aborted.Conflict || len(aborted.Files) != 0 {
			t.Fatalf("中止后应回到干净状态：%#v err=%v", aborted, err)
		}
	})

	// 分支链路：白名单是注入面（以 - 开头会被 git 当成选项），生命周期是界面动作序列。
	// 坏了 = 面板显示切成功了实际还在原分支上写东西，或者删分支删掉了未合并的提交。
	t.Run("分支链路：名字白名单、新建切换合并删除", func(t *testing.T) {
		// 白名单只各验一个代表：以 - 开头（选项注入）与 --upload-pack 形态（命令注入）。
		if validBranchName("-x") || validRemoteName("-x") || validRemoteURL("--upload-pack=evil") {
			t.Fatal("以 - 开头的名字与地址必须被拒（会被 git 当成选项）")
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
		if _, err := Commit(dir, "在新分支上改一笔", false); err != nil {
			t.Fatal(err)
		}

		list, err := Branches(dir)
		if err != nil || len(list) != 2 {
			t.Fatalf("应有两条本地分支（非当前分支被丢掉是解析回归）：%#v err=%v", list, err)
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

		// 合进基干后提交应在基干历史里；已合并的分支可以删，删完清单里不该还有它。
		if _, err := Merge(dir, "feature/x", false); err != nil {
			t.Fatalf("合并已提交的分支应成功：%v", err)
		}
		if _, err := DeleteBranch(dir, "feature/x"); err != nil {
			t.Fatalf("删除已合并分支失败：%v", err)
		}
		for _, b := range mustBranches(t, dir) {
			if b.Name == "feature/x" {
				t.Fatalf("分支应已删除：%#v", mustBranches(t, dir))
			}
		}
	})
}

// initRepo 造一个已有一次提交的临时仓库：各条链路都要的公共前奏。
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

// initRepoWithRemote 造一个裸仓库 + 一个已提交的本地仓库，返回两者路径。
func initRepoWithRemote(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	mustGit(t, root, "init", "-q", "--bare", "--initial-branch=main", bare)
	dir := filepath.Join(root, "work")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "init", "-q", "--initial-branch=main")
	mustGit(t, dir, "config", "user.email", "workbaby@local")
	mustGit(t, dir, "config", "user.name", "WorkBaby")
	mustGit(t, dir, "remote", "add", "origin", filepath.ToSlash(bare))
	writeFile(t, dir, "a.txt", "one\ntwo\n")
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-q", "-m", "init")
	return root, dir
}

// mustGit 执行一条 git 命令，失败即停。
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := run(dir, statusTimeout, args...); err != nil {
		t.Fatalf("git %v 失败：%v", args, err)
	}
}

// mustBranches 取分支清单，失败即停。
func mustBranches(t *testing.T, dir string) []domain.GitBranchVO {
	t.Helper()
	list, err := Branches(dir)
	if err != nil {
		t.Fatalf("取分支清单失败：%v", err)
	}
	return list
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

// findFile 在状态里按路径找一条；找不到返回零值（零值的 Path 为空）。
func findFile(st *domain.GitStatusVO, path string) domain.GitFileVO {
	if st == nil {
		return domain.GitFileVO{}
	}
	for _, f := range st.Files {
		if f.Path == path {
			return f
		}
	}
	return domain.GitFileVO{}
}

// pkgCode 取错误的业务码：断言「界面拿到的分流码」而不是错误文案。
func pkgCode(err error) string { return pkg.CodeOf(err) }
