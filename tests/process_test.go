// 外部进程：长驻子进程的收尾纪律、子进程环境、git 冲突的三种结局。
// 三件事的共同点是本机真实进程——收尾挂死会让界面卡在「运行中」，
// 环境被遮蔽会改坏用户工具链的可见性，冲突口径写反是不可逆的数据损失。
package tests

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/pkg"
)

// persistentShell 造一条本机必然存在的短命命令行：cmd /c exit 0。
// Windows 用 cmd；其他平台退到 sh，测试本身不该绑死在一个 OS 上。
func persistentShell() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "exit 0"}
	}
	return "sh", []string{"-c", "exit 0"}
}

// stderrShell 造一条会往 stderr 写一行再退出的命令行：用来验证遗言能被完整读到。
// 标记用 ASCII：cmd 的 echo 走系统代码页（GBK），中文标记会变成乱码字节。
func stderrShell() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo PROC_STDERR_MARK 1>&2"}
	}
	return "sh", []string{"-c", "echo PROC_STDERR_MARK 1>&2"}
}

// runWithBudget 在预算内跑 f，超时即失败并带出卡住的现场。
func runWithBudget(t *testing.T, budget time.Duration, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(budget):
		t.Fatalf("%s：超过 %v 仍未返回", what, budget)
	}
}

// findGit 找系统装好的 git；没有就跳过（运行时不随包分发）。
// 探测走 pkg.LookPath：自己扫 PATH，不拉控制台窗口。
func findGit(t *testing.T) string {
	t.Helper()
	found := pkg.LookPath("git")
	if found == "" {
		t.Skip("找不到可用的 git，跳过冲突解决测试")
	}
	return found
}

// runGit 跑一条 git 命令；失败即测试失败（故意冲突的 merge 不走这里）。
// 出口对齐 pkg.RunProc：argv 直传、窗口隐藏、超时必杀。
func runGit(t *testing.T, gitExe, dir string, args ...string) {
	t.Helper()
	result, err := pkg.RunProc(gitExe, append([]string{"-C", dir}, args...), pkg.ProcOptions{Dir: dir, TimeoutMs: 20000})
	if err != nil {
		t.Fatalf("git %s 启动失败：%v", strings.Join(args, " "), err)
	}
	if result.Code != 0 {
		t.Fatalf("git %s 失败（退出码 %d）：%s%s", strings.Join(args, " "), result.Code, result.Stdout, result.Stderr)
	}
}

func TestProcess(t *testing.T) {
	// 长驻进程收尾：Kill 必回、幂等、已退出时立刻返回、遗言仍可读，
	// 每条都有时间预算兜着。
	t.Run("长驻进程收尾", func(t *testing.T) {
		t.Run("自然退出后 Kill 立刻返回", func(t *testing.T) {
			exe, args := persistentShell()
			proc, err := pkg.StartPersistent(exe, args, "", nil)
			if err != nil {
				t.Fatalf("启动长驻进程失败：%v", err)
			}
			runWithBudget(t, 3*time.Second, "进程自然退出后 Kill", proc.Kill)
			if !proc.Exited() {
				t.Fatal("Kill 返回后应处于已退出状态")
			}
		})

		t.Run("进程还活着时 Kill 也返回", func(t *testing.T) {
			exe, args := persistentShell()
			proc, err := pkg.StartPersistent(exe, args, "", nil)
			if err != nil {
				t.Fatalf("启动长驻进程失败：%v", err)
			}
			// 杀树 + 等 Wait 收尾那条路不能把调用方挂死。
			runWithBudget(t, 3*time.Second, "进程存活时 Kill", proc.Kill)
		})

		t.Run("连续两次 Kill 幂等", func(t *testing.T) {
			exe, args := persistentShell()
			proc, err := pkg.StartPersistent(exe, args, "", nil)
			if err != nil {
				t.Fatalf("启动长驻进程失败：%v", err)
			}
			// 调用方在 defer 与显式收尾各杀一次是常态。
			runWithBudget(t, 3*time.Second, "第一次 Kill", proc.Kill)
			runWithBudget(t, 3*time.Second, "第二次 Kill", proc.Kill)
		})

		t.Run("收尾后错误输出仍可读", func(t *testing.T) {
			exe, args := stderrShell()
			proc, err := pkg.StartPersistent(exe, args, "", nil)
			if err != nil {
				t.Fatalf("启动长驻进程失败：%v", err)
			}
			runWithBudget(t, 3*time.Second, "收尾", proc.Kill)
			// 已退出时读到完整遗言，不能因为并发被读空或读到半截。
			if got := proc.StderrText(); !strings.Contains(got, "PROC_STDERR_MARK") {
				t.Fatalf("收尾后应读到完整遗言，实际 %q", got)
			}
		})
	})

	// 子进程环境：只追加必须项，不遮蔽用户的 HOME。
	t.Run("子进程环境", func(t *testing.T) {
		layout := pkg.ResolveLayout(t.TempDir(), "", true)
		values := map[string]string{}
		for _, item := range layout.ChildEnv(nil) {
			if idx := strings.IndexByte(item, '='); idx > 0 {
				values[item[:idx]] = item[idx+1:]
			}
		}
		if got, ok := values["HOME"]; ok && got == layout.DataDir {
			t.Fatalf("HOME 不该被改写到壳的数据目录：%q", got)
		}
		if got := values["NO_COLOR"]; got != "1" {
			t.Fatalf("子进程应禁掉颜色码：NO_COLOR=%q", got)
		}
		if got := values["GIT_TERMINAL_PROMPT"]; got != "0" {
			t.Fatalf("子进程不该卡在 git 的交互提问上：GIT_TERMINAL_PROMPT=%q", got)
		}
	})

	// 冲突解决：ours / theirs 各自把文件落成什么，空路径与未知口径直接拒。
	// --ours/--theirs 写反了，用户点完按钮会静默丢掉另一边的代码。
	t.Run("冲突解决", func(t *testing.T) {
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
				_, _ = pkg.RunProc(gitExe, []string{"-C", dir, "merge", "feature"}, pkg.ProcOptions{Dir: dir, TimeoutMs: 20000})

				if err := h.store.Save(map[string]string{"workspace.current": dir}); err != nil {
					t.Fatalf("写工作区设置失败：%v", err)
				}

				out, err := h.app.Git.ResolveConflict(t.Context(), "conflict.txt", tc.side)
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
			_, err := h.app.Git.ResolveConflict(t.Context(), "  ", "ours")
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
			_, err := h.app.Git.ResolveConflict(t.Context(), "conflict.txt", "theirs-typo")
			if err == nil {
				t.Fatal("不认识的口径应被拒绝")
			}
			if pkg.As(err).Code != pkg.CodeBadRequest {
				t.Fatalf("未知口径应回 %q，实际 %q", pkg.CodeBadRequest, pkg.As(err).Code)
			}
		})
	})
}
