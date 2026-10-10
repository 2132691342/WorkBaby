// 应用生命周期链路：装配等待判据、关闭去向判定（收托盘 / 真退出）。
// 坏了的表现：慢启动被误报失败、界面永远等待，或关闭后窗口消失、进程退不掉。
package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"WorkBaby/backend/api"
	"WorkBaby/backend/db"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/repo"
)

func TestAppLifecycle(t *testing.T) {
	t.Run("等待判据：慢装配不误判 / 失败如实报 / 卡死有上限", func(t *testing.T) {
		// 慢装配（首次启动要解压运行时）：必须等到装配真的完成，不能一秒钟就报失败
		slow := NewApp()
		go func() {
			time.Sleep(120 * time.Millisecond)
			slow.startOK.Store(true)
			close(slow.startDone)
		}()
		start := time.Now()
		if !slow.awaitStartup() {
			t.Fatal("装配最终成功后必须判定为成功，不能误报失败")
		}
		if d := time.Since(start); d < 100*time.Millisecond {
			t.Fatalf("没有真正等待装配完成，只用了 %v", d)
		}

		failed := NewApp()
		close(failed.startDone)
		if failed.awaitStartup() {
			t.Fatal("装配失败必须判定为失败")
		}

		// 超时上限不能被忽略，否则装配真卡死时界面会永远等下去
		stuck := &App{handler: api.New("test"), startDone: make(chan struct{}), startWait: 80 * time.Millisecond}
		if stuck.awaitStartup() {
			t.Fatal("装配卡住必须超时返回")
		}
	})

	t.Run("关闭去向：托盘开关与退出流程决定收托盘还是真退出", func(t *testing.T) {
		fresh := NewApp()
		if !fresh.closeToTray() {
			t.Fatal("未装配时按默认行为收进托盘")
		}
		// 托盘没就绪时拦截关闭，等于把窗口藏进一个还不存在的托盘里，用户再也找不回来
		if fresh.beforeClose(context.Background()) {
			t.Fatal("托盘没就绪时不能拦截关闭")
		}
		// 未进入退出流程时拦截关闭（收进托盘）
		if fresh.closeAllowed() {
			t.Fatal("普通关闭必须被拦截，否则窗口直接消失")
		}
		// 退出流程中必须放行，否则托盘「退出」是空操作，进程留在后台占着 exe
		fresh.quitting.Store(true)
		if !fresh.closeAllowed() {
			t.Fatal("退出流程中必须放行关闭")
		}

		// 用户关掉「后台驻留」后，关闭必须真的退出；这条读持久化设置，需要一个真库
		database, err := db.Open(filepath.Join(t.TempDir(), "app.db"))
		if err != nil {
			t.Fatal(err)
		}
		r := repo.New(database)
		t.Cleanup(func() { _ = r.Close() })
		if err := r.SetSetting(domain.SettingMinimizeToTray, "false"); err != nil {
			t.Fatal(err)
		}
		off := &App{handler: &api.Handler{Repo: r}}
		if off.closeToTray() {
			t.Fatal("用户关掉后台驻留后，关闭必须真的退出")
		}
	})
}
