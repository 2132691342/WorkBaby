// 覆盖 OnStartup 与 OnDomReady 的并发关系。
// Wails 把 OnStartup 放在独立 goroutine 里跑，OnDomReady 由 WebView2 导航回调触发，
// 两者没有顺序保证。曾经用「Svc 是不是 nil」判断启动失败，于是首次启动
// （要解压内置 Python，约两秒）必然被误判成失败，界面永远停在启动页。

package main

import (
	"testing"
	"time"

	"WorkBaby/internal/api"
)

func TestAwaitStartupWaitsForSlowStartup(t *testing.T) {
	a := NewApp()
	go func() {
		time.Sleep(120 * time.Millisecond)
		a.startOK.Store(true)
		close(a.startDone)
	}()

	start := time.Now()
	ok := a.awaitStartup()
	if !ok {
		t.Fatal("装配最终成功后必须判定为成功，不能误报失败")
	}
	if d := time.Since(start); d < 100*time.Millisecond {
		t.Fatalf("没有真正等待装配完成，只用了 %v", d)
	}
}

func TestAwaitStartupReportsRealFailure(t *testing.T) {
	a := NewApp()
	close(a.startDone)
	if a.awaitStartup() {
		t.Fatal("装配失败必须判定为失败")
	}
}

// 超时上限不能被忽略：否则装配真卡死时界面会永远等下去。
func TestAwaitStartupTimesOut(t *testing.T) {
	a := &App{Handler: api.New("test"), startDone: make(chan struct{}), startWait: 80 * time.Millisecond}
	if a.awaitStartup() {
		t.Fatal("装配卡住必须超时返回")
	}
}

func TestBeforeCloseHidesUnlessQuitting(t *testing.T) {
	a := NewApp()
	// 未进入退出流程：拦截关闭窗口（收进托盘）。
	if a.closeAllowed() {
		t.Fatal("普通关闭必须被拦截，否则窗口直接消失")
	}
	// 已进入退出流程：必须放行，否则「退出」按钮是空操作、进程留在后台。
	a.quitting.Store(true)
	if !a.closeAllowed() {
		t.Fatal("退出流程中必须放行关闭")
	}
}
