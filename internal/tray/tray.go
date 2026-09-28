// Package tray 是系统托盘：关闭到托盘后仍可从这里唤起窗口或退出。
package tray

import (
	"sync"

	"WorkBaby/internal/pkg"

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
	menu Menu
	once sync.Once
	stop chan struct{}
}

// New 构造托盘。
func New(m Menu) *Tray { return &Tray{menu: m, stop: make(chan struct{})} }

// Start 启动托盘事件循环（阻塞，需在 goroutine 里调用）。
func (t *Tray) Start() {
	t.once.Do(func() {
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
	close(t.stop)
	pkg.Warnf("tray: 已退出")
}

// Stop 请求托盘退出。
func (t *Tray) Stop() { systray.Quit() }
