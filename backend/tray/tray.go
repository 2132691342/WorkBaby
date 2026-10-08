// Package tray 是系统托盘：关闭到托盘后仍可从这里唤起窗口或退出。
package tray

import (
	"runtime"
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
// Windows 上托盘窗口的创建与消息循环必须落在同一个 OS 线程，否则
// GetMessage 收不到 CreateWindowEx 那个线程的消息，菜单点击与退出回调一起失灵。
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
				if t.menu.OnShow != nil {
					t.menu.OnShow()
				}
			}
		}()
	}
	if t.menu.OnNew != nil {
		item := systray.AddMenuItem("新建对话", "新建对话")
		go func() {
			for range item.ClickedCh {
				t.menu.OnNew()
			}
		}()
	}
	if t.menu.QuitText != "" {
		quit := systray.AddMenuItem(t.menu.QuitText, t.menu.QuitText)
		go func() {
			for range quit.ClickedCh {
				if t.menu.OnQuit != nil {
					t.menu.OnQuit()
				}
			}
		}()
	}
}

func (t *Tray) onExit() {
	t.exitOnce.Do(func() { close(t.done) })
	pkg.Warnf("tray: 已退出")
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
