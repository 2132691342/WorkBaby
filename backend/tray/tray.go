// Package tray 是系统托盘：关闭到托盘后仍可从这里唤起窗口或退出。
package tray

import (
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"WorkBaby/backend/pkg"

	"github.com/getlantern/systray"
)

// Menu 定义托盘菜单项与回调。
type Menu struct {
	Title    string
	ShowText string
	QuitText string
	OnShow   func()
	OnNew    func()
	OnQuit   func()
}

// Tray 持有托盘生命周期；Start 只应被调用一次。
type Tray struct {
	menu     Menu
	once     sync.Once
	exitOnce sync.Once
	done     chan struct{}
}

// New 构造托盘。
func New(m Menu) *Tray { return &Tray{menu: m, done: make(chan struct{})} }

// Start 启动托盘事件循环（阻塞，需在 goroutine 里调用）。
// 托盘窗口的创建与消息循环必须在同一个 OS 线程，否则菜单点击与退出回调一起失灵。
func (t *Tray) Start() {
	t.once.Do(func() {
		runtime.LockOSThread()
		systray.Run(t.onReady, t.onExit)
	})
}

func (t *Tray) onReady() {
	// Windows 托盘缺省没有任何图标，任务栏里只是一块透明占位，用户根本找不到；
	// 必须显式注入与应用一致的那枚图标，再给悬停提示。
	systray.SetIcon(iconICO)
	systray.SetTooltip(t.menu.Title)
	systray.SetTitle(t.menu.Title)
	if t.menu.ShowText != "" {
		show := systray.AddMenuItem(t.menu.ShowText, t.menu.ShowText)
		go func() {
			for range show.ClickedCh {
				safeCall(t.menu.OnShow)
			}
		}()
	}
	if t.menu.OnNew != nil {
		item := systray.AddMenuItem("新建对话", "新建对话")
		go func() {
			for range item.ClickedCh {
				safeCall(t.menu.OnNew)
			}
		}()
	}
	if t.menu.QuitText != "" {
		quit := systray.AddMenuItem(t.menu.QuitText, t.menu.QuitText)
		go func() {
			for range quit.ClickedCh {
				safeCall(t.menu.OnQuit)
			}
		}()
	}
}

// safeCall 兜住菜单回调的 panic：托盘是进程级组件，回调里的一个 bug 应当只留下日志与
// 堆栈，而不是把整进程连同正在跑的对话一起带走。
func safeCall(f func()) {
	if f == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			pkg.Errorf("tray: 菜单回调崩溃: %v\n%s", rec, debug.Stack())
		}
	}()
	f()
}

func (t *Tray) onExit() {
	t.exitOnce.Do(func() { close(t.done) })
	// 正常退出不是告警：warn.log 要留给真正的异常，否则每次退出都刷一条，
	// 出事时没人会去看它。
	pkg.Infof("tray: 消息循环已收尾")
}

// Stop 请求托盘退出（异步：消息循环何时收完尾见 WaitForExit）。
func (t *Tray) Stop() { systray.Quit() }

// WaitForExit 等消息循环收尾，最多 d。收尾里包含把图标从通知区摘掉，
// 进程抢在它前面退出会留下一个只能靠鼠标划过才消失的「幽灵图标」。
func (t *Tray) WaitForExit(d time.Duration) bool {
	select {
	case <-t.done:
		return true
	case <-time.After(d):
		return false
	}
}
