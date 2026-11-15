// 长驻子进程的收尾纪律：Kill 必回、幂等、已退出时立刻返回、遗言仍可读。
// 收尾路径一旦挂死，界面就会卡在「运行中」，所以这里每条都有时间预算兜着。
package tests

import (
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

// TestPersistentProcLifecycle 断言长驻子进程的收尾四条纪律。
func TestPersistentProcLifecycle(t *testing.T) {
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
}
