package shell

import (
	"fmt"
	"time"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

// Ready 由前端在首屏渲染完后调用，用来把隐藏的窗口显示出来。
//
// 不能在装配完成时显示：v3 把建窗推迟到主循环，ServiceStartup 执行时窗口还不存在，
// 那时 Show() 只会建窗仍隐藏，随后的 Focus() 会踩到未初始化的 WebView2 controller。
func (a *App) Ready() {
	// 前端每次热重载都会再调一次。
	if a.revealed.Swap(true) {
		return
	}
	a.refreshTrayMenu()
	if a.startHidden {
		// 开机自启只上托盘，不往屏幕上弹一个空窗口。
		return
	}
	// 建窗时尺寸按物理像素声明、可能被屏幕裁掉，这里用**逻辑值**再设一次：
	// 这条路径 Wails 会自己按 DPI 换算。
	if a.win != nil {
		a.win.SetSize(windowLogicalWidth, windowLogicalHeight)
		a.centerOnPrimaryScreen()
	}
	a.showWindow()
	pkg.Info("首屏就绪，窗口已显示")
}

// centerOnPrimaryScreen 把窗口放到主屏工作区正中。
// 不用 Wails 的 Center()：它用客户区尺寸配监视器，对无边框窗口会偏。
func (a *App) centerOnPrimaryScreen() {
	if a.win == nil || a.app == nil {
		return
	}
	screen := a.app.Screen.GetPrimary()
	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		a.win.Center()
		return
	}
	width, height := a.win.Size()
	area := screen.WorkArea
	a.win.SetPosition(area.X+(area.Width-width)/2, area.Y+(area.Height-height)/2)
}

// showWindow 把窗口带到前台：显示 + 还原 + 聚焦。
// 三步缺一不可：窗口可能在隐藏、最小化、最大化三种状态中的任意一种。
func (a *App) showWindow() {
	if a.win == nil {
		return
	}
	a.win.Show()
	a.win.Restore()
	a.win.Focus()
}

// toTray 收托盘：只隐藏窗口，进程与引擎继续活着。
func (a *App) toTray() {
	if a.win != nil {
		a.win.Hide()
	}
}

// askCloseDecision 请求界面就「关闭时做什么」做一次选择。
// 走事件而不是系统通知：窗口已要隐藏，在隐藏窗口上画弹层用户看不到，
// 所以先不藏窗口，发一帧事件让界面弹选择框，选完再执行。
func (a *App) askCloseDecision() {
	if a.hub == nil {
		a.toTray()
		return
	}
	a.hub.Broadcast("app:close_requested", map[string]any{"close_action": a.closeAction()})
}

// ApplyCloseDecision 执行界面对「关窗时做什么」的选择。
func (a *App) ApplyCloseDecision(action string, remember bool) error {
	if action != service.CloseActionQuit {
		action = service.CloseActionTray
	}
	if a.svc != nil {
		if remember {
			if err := a.svc.SetCloseAction(action); err != nil {
				return err
			}
		}
		a.svc.MarkTrayHintShown()
	}
	if action == service.CloseActionQuit {
		a.requestQuit()
		return nil
	}
	a.toTray()
	return nil
}

// requestQuit 退出应用。
// 不能在菜单回调里直接调 app.Quit()：那一刻 TrackPopupMenuEx 还没返回，
// 在它栈上销毁托盘会让消息循环在已释放的菜单上继续处理命令。所以先置标志再异步做。
func (a *App) requestQuit() {
	if a.quitting.Swap(true) {
		return
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		if a.app != nil {
			a.app.Quit()
		}
	}()
}

// Quit 退出应用。
func (a *App) Quit() { a.requestQuit() }

// WindowMinimize 最小化。
func (a *App) WindowMinimize() {
	if a.win != nil {
		a.win.Minimise()
	}
}

// WindowToggleMaximize 最大化 / 还原。
func (a *App) WindowToggleMaximize() {
	if a.win == nil {
		return
	}
	if a.win.IsMaximised() {
		a.win.Restore()
		return
	}
	a.win.Maximise()
}

// WindowClose 关窗（按设置收托盘或退出）。
func (a *App) WindowClose() {
	if a.closeAction() == service.CloseActionQuit {
		a.requestQuit()
		return
	}
	a.toTray()
}

// OpenExternal 用系统默认浏览器打开链接。
func (a *App) OpenExternal(url string) error {
	if a.app == nil {
		return fmt.Errorf("窗口还没就绪")
	}
	a.app.Browser.OpenURL(url)
	return nil
}

// PickFolder 打开系统目录选择框；取消返回空串（空串就是用户没选，不是失败）。
func (a *App) PickFolder(defaultDir string) (string, error) {
	if a.app == nil {
		return "", fmt.Errorf("窗口还没就绪")
	}
	return a.app.Dialog.OpenFile().
		SetTitle("选择工作目录").
		SetDirectory(defaultDir).
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		PromptForSingleSelection()
}

// PickFiles 打开文件选择框，可多选；取消返回空列表。
func (a *App) PickFiles(title string) ([]string, error) {
	if a.app == nil {
		return nil, fmt.Errorf("窗口还没就绪")
	}
	if title == "" {
		title = "选择文件"
	}
	picked, err := a.app.Dialog.OpenFile().
		SetTitle(title).
		CanChooseDirectories(false).
		CanChooseFiles(true).
		PromptForMultipleSelection()
	if err != nil {
		// 取消在部分环境下会以错误形式返回；当成「没选」而不是失败。
		return nil, nil
	}
	return picked, nil
}

// shutdown 退出前的收尾，跑在**主线程**上：
// 每多停一毫秒，用户看到的就是「点了退出但窗口还在」。所以引擎停靠并发 + 短超时。
func (a *App) shutdown() {
	if a.svc != nil {
		done := make(chan struct{})
		go func() {
			defer close(done)
			a.svc.Shutdown()
		}()
		select {
		case <-done:
		case <-time.After(quitGrace):
			pkg.Warn("引擎未在超时内停完，直接退出")
		}
	}
	if a.srv != nil {
		// SSE 长连接不会自己断：给一个短超时，别让它拖住退出。
		ctx, cancel := contextWithTimeout(800 * time.Millisecond)
		_ = a.srv.Stop(ctx)
		cancel()
	}
	if a.store != nil {
		_ = a.store.Close()
	}
	pkg.Info("已退出")
}
