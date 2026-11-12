// 冲突解决：`git add` 只是冲突的第三种结局，另外两种是「保留我方」与「保留对方」。
// 两个 --ours/--theirs 写反了，用户点完界面上的按钮会静默丢掉另一边的代码，那是不可逆的数据损失。
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/pkg"
)

// TestConflictResolve 断言三种冲突结局各自把文件内容落成什么。
func TestConflictResolve(t *testing.T) {
	gitExe := findGit(t)

	// 每个口径单独造一次冲突：解决是不可逆的，一次仓库只能验证一个结局。
	cases := []struct {
		name     string
		side     string
		wantBody string
		notBody  string
	}{
		{name: "保留我方", side: "ours", wantBody: "main 版本", notBody: "feature 版本"},
		{name: "保留对方", side: "theirs", wantBody: "feature 版本", notBody: "main 版本"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			if !withBuiltinGit(t) {
				t.Skip("本机没有 git，跳过")
			}
			dir := t.TempDir()
			runGit(t, gitExe, dir, "init", "-b", "main")
			runGit(t, gitExe, dir, "config", "user.email", "t@example.com")
			runGit(t, gitExe, dir, "config", "user.name", "t")

			// 主分支写「main 版本」，特性分支写「feature 版本」，同一个文件同一行。
			writeFile(t, filepath.Join(dir, "conflict.txt"), "main 版本\n")
			runGit(t, gitExe, dir, "add", ".")
			runGit(t, gitExe, dir, "commit", "-m", "main")
			runGit(t, gitExe, dir, "checkout", "-b", "feature")
			writeFile(t, filepath.Join(dir, "conflict.txt"), "feature 版本\n")
			runGit(t, gitExe, dir, "commit", "-am", "feature")
			runGit(t, gitExe, dir, "checkout", "main")
			writeFile(t, filepath.Join(dir, "conflict.txt"), "main 版本改\n")
			runGit(t, gitExe, dir, "commit", "-am", "main2")

			// 合并必然冲突：不检查退出码，冲突就是这一步的预期结果。
			_ = exec.Command(gitExe, "-C", dir, "merge", "feature").Run()

			if err := h.store.Save(map[string]string{"workspace.current": dir}); err != nil {
				t.Fatalf("写工作区设置失败：%v", err)
			}

			out, err := h.app.Git.ResolveConflict("conflict.txt", tc.side)
			if err != nil {
				t.Fatalf("按 %q 解决冲突失败：%v", tc.side, err)
			}
			if out.HasConflict {
				t.Errorf("解决后不该还报冲突：%+v", out.ChangedFiles)
			}

			raw, err := os.ReadFile(filepath.Join(dir, "conflict.txt"))
			if err != nil {
				t.Fatalf("读解决后的文件失败：%v", err)
			}
			body := string(raw)
			if !strings.Contains(body, tc.wantBody) {
				t.Errorf("按 %q 解决后应包含 %q，实际 %q", tc.side, tc.wantBody, body)
			}
			if strings.Contains(body, tc.notBody) {
				t.Errorf("按 %q 解决后不该还留着 %q，实际 %q", tc.side, tc.notBody, body)
			}
			// 冲突标记不清掉，提交进去就是一堆 <<<<<<< 进历史。
			if strings.Contains(body, "<<<<<<<") || strings.Contains(body, ">>>>>>>") {
				t.Errorf("解决后不该残留冲突标记，实际 %q", body)
			}
			// 已暂存：这一条是提交的前置，没暂存等于没解决。
			for _, file := range out.ChangedFiles {
				if file.FilePath == "conflict.txt" && !file.IsStaged {
					t.Errorf("解决后该文件应已暂存：%+v", file)
				}
			}
		})
	}

	// ---- 路径空串：不猜，直接拒 ----
	t.Run("空路径拒绝", func(t *testing.T) {
		h := newHarness(t)
		_, err := h.app.Git.ResolveConflict("  ", "ours")
		if err == nil {
			t.Fatal("空路径应被拒绝")
		}
		if pkg.As(err).Code != pkg.CodeBadRequest {
			t.Fatalf("空路径应回 %q，实际 %q", pkg.CodeBadRequest, pkg.As(err).Code)
		}
	})

	// ---- 不认识的口径：不猜，直接拒 ----
	t.Run("未知口径拒绝", func(t *testing.T) {
		h := newHarness(t)
		_, err := h.app.Git.ResolveConflict("conflict.txt", "theirs-typo")
		if err == nil {
			t.Fatal("不认识的口径应被拒绝")
		}
		if pkg.As(err).Code != pkg.CodeBadRequest {
			t.Fatalf("未知口径应回 %q，实际 %q", pkg.CodeBadRequest, pkg.As(err).Code)
		}
	})
}

// withBuiltinGit 确认本机有可用的 git。
// 壳用系统装的那个，没有就跳过需要它的用例。
func withBuiltinGit(t *testing.T) bool {
	t.Helper()
	_, err := exec.LookPath("git")
	return err == nil
}

// findGit 找可用的 git；没有就跳过。
func findGit(t *testing.T) string {
	t.Helper()
	if found, err := exec.LookPath("git"); err == nil {
		return found
	}
	t.Skip("找不到可用的 git，跳过冲突解决测试")
	return ""
}

// runGit 跑一条 git 命令；失败即测试失败（除了故意冲突的 merge）。
func runGit(t *testing.T, gitExe, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(gitExe, append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s 失败：%v\n%s", strings.Join(args, " "), err, out)
	}
}

// writeFile 写一个文件，父目录不存在就建。
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := pkg.EnsureDir(filepath.Dir(path)); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}
}
