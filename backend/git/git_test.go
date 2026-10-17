// 工作区版本视图链路：真仓库端到端（状态归类 / 中文带空格路径 / 行号归属 / 未跟踪差异 / 整区差异）
// + 路径护栏。坏了 = 界面「改了什么」与 git 说的不一致。
package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/domain"
)

// TestGitChain 真仓库端到端：init → 提交 → 改动 → status 与 diff 全链路。没有 git 的机器跳过。
// 每一步都是真实 git 子进程（整套约 9 秒），因此 -short 下跳过：它是唯一值得为速度让步的一条。
func TestGitChain(t *testing.T) {
	if testing.Short() {
		t.Skip("-short：真 git 子进程链路，提交前跑全量")
	}
	if !Available() {
		t.Skip("这台机器上没有 git")
	}

	t.Run("状态与差异全链路", func(t *testing.T) {
		dir := t.TempDir()
		mustGit(t, dir, "init", "-q")
		mustGit(t, dir, "config", "user.email", "workbaby@local")
		mustGit(t, dir, "config", "user.name", "WorkBaby")
		writeFile(t, dir, "a.txt", "one\ntwo\n")
		writeFile(t, dir, "周报 数据.txt", "本周进展顺利\n")
		mustGit(t, dir, "add", ".")
		mustGit(t, dir, "commit", "-q", "-m", "init")

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

		// 未跟踪文件 git 给不出差异：整篇读作新增行。
		d2, err := Diff(dir, "new.txt")
		if err != nil {
			t.Fatal(err)
		}
		if !hasLine(d2, domain.GitLineAdd, "brand") {
			t.Fatalf("新文件的差异不对：%#v", d2.Lines)
		}

		// 整个工作区：空 pathspec 会被 git 判非法，这里必须拿到内容而不是静默为空。
		d3, err := Diff(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		if !hasLine(d3, domain.GitLineAdd, "TWO") {
			t.Fatalf("整个工作区的差异为空：%#v", d3.Lines)
		}
	})

	// 护栏两条同属「界面不能替用户瞎猜」：越界路径直接拒，非仓库目录给人话。
	// git 的 128 原文随系统语言变，照原文分流会在中文 Windows 上漏判。
	t.Run("护栏：越界拒绝与非仓库引导", func(t *testing.T) {
		if _, err := Diff(t.TempDir(), "../outside.txt"); err == nil {
			t.Fatal("工作区之外的路径必须被拒绝")
		}
		dir := t.TempDir()
		st, err := Status(dir)
		if err != nil {
			t.Fatalf("不是仓库不该报错：%v", err)
		}
		if !st.Available || st.IsRepo || st.Reason == "" {
			t.Fatalf("应给出「还不是仓库」的引导：%#v", st)
		}
		if _, err := Diff(dir, ""); err == nil || !strings.Contains(err.Error(), "还不是 Git 仓库") {
			t.Fatalf("差异应给出人话而不是 git 原文：%v", err)
		}
	})
}

// mustGit 执行一条 git 写命令，失败即停。
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
