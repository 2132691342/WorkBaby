// 应用生命周期链路：启动等待（慢装配不误判 / 失败 / 超时）与关闭放行判据。
// Wails 把 OnStartup 放在独立 goroutine 里跑，与 OnDomReady 没有顺序保证；
// 启动失败误判的表现是首次启动（解压内置 Python 约两秒）界面永远停在启动页。
package main

import (
	"context"
	"testing"
	"time"

	"WorkBaby/internal/api"
	"WorkBaby/internal/domain"
)

func TestAppLifecycle(t *testing.T) {
	t.Run("等待慢装配后判成功", func(t *testing.T) {
		a := NewApp()
		go func() {
			time.Sleep(120 * time.Millisecond)
			a.startOK.Store(true)
			close(a.startDone)
		}()
		start := time.Now()
		if !a.awaitStartup() {
			t.Fatal("装配最终成功后必须判定为成功，不能误报失败")
		}
		if d := time.Since(start); d < 100*time.Millisecond {
			t.Fatalf("没有真正等待装配完成，只用了 %v", d)
		}
	})

	t.Run("装配失败要判失败", func(t *testing.T) {
		a := NewApp()
		close(a.startDone)
		if a.awaitStartup() {
			t.Fatal("装配失败必须判定为失败")
		}
	})

	// 超时上限不能被忽略，否则装配真卡死时界面会永远等下去。
	t.Run("卡死要超时返回", func(t *testing.T) {
		a := &App{Handler: api.New("test"), startDone: make(chan struct{}), startWait: 80 * time.Millisecond}
		if a.awaitStartup() {
			t.Fatal("装配卡住必须超时返回")
		}
	})

	// 未进入退出流程时拦截关闭（收进托盘）；已进入必须放行，
	// 否则托盘「退出」是空操作，进程留在后台并占住 exe。
	t.Run("关闭拦截与退出放行", func(t *testing.T) {
		a := NewApp()
		if a.closeAllowed() {
			t.Fatal("普通关闭必须被拦截，否则窗口直接消失")
		}
		a.quitting.Store(true)
		if !a.closeAllowed() {
			t.Fatal("退出流程中必须放行关闭")
		}
	})

	// 「关闭到托盘」是可关的开关：关掉后关闭就是真退出。
	// 关掉后台驻留却发现程序怎么都退不掉，只能去任务管理器，属于设计缺失。
	t.Run("关闭到托盘尊重用户选择", func(t *testing.T) {
		a := NewApp()
		if !a.closeToTray() {
			t.Fatal("未装配时按默认行为收进托盘")
		}
		dir := t.TempDir()
		t.Setenv("WORKBABY_HOME", dir)
		if err := a.Handler.Startup(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(a.Handler.Shutdown)
		if err := a.Handler.Repo.SetSetting(domain.SettingMinimizeToTray, "false"); err != nil {
			t.Fatal(err)
		}
		if a.closeToTray() {
			t.Fatal("用户关掉后台驻留后，关闭必须真的退出")
		}
	})
}
