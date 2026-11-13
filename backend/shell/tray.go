package shell

import (
	"context"
	_ "embed"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// trayRecentMax 是托盘菜单里「最近工作目录」最多铺几条：
// 托盘菜单是一个弹出层，长到要滚动就算不上快捷方式了。
const trayRecentMax = 5

//go:embed res/icon.ico
var trayIcon []byte

// buildTrayMenu 组装托盘菜单。
// 「最近工作目录」这一支随设置变化，所以整棵树可重建：
// 直接换掉菜单比就地改某几项更省心（v3 的菜单项没有稳定的可变句柄）。
func (a *App) buildTrayMenu(wailsApp *application.App, win *application.WebviewWindow) {
	menu := wailsApp.NewMenu()
	menu.Add("显示窗口").OnClick(func(*application.Context) { a.showWindow() })

	recent := menu.AddSubmenu("最近工作目录")
	a.fillRecentWorkspaces(recent)

	menu.Add("打开数据目录").OnClick(func(*application.Context) {
		if a.svc != nil {
			if err := a.svc.OpenDataDir(); err != nil {
				pkg.Warn("打不开数据目录：%v", err)
			}
		}
	})

	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) { a.requestQuit() })
	a.tray.SetMenu(menu)
	// 托盘图标显式给品牌图标：默认走 exe 图标，但 16px 下的多尺寸适配
	// 由这里的 ico 自己保证，不依赖壳的默认回退。
	a.tray.SetIcon(trayIcon)
	a.tray.SetTooltip("WorkBaby")
	// 单击图标 = 把窗口带出来；菜单用右键。
	a.tray.OnClick(func() { a.showWindow() })
	_ = win
}

// fillRecentWorkspaces 往子菜单里铺最近用过的目录。
// 空列表时留一条禁用的说明项而不是空菜单：一个点开什么都没有的菜单会让人以为功能坏了。
func (a *App) fillRecentWorkspaces(sub *application.Menu) {
	rows := a.recentWorkspaces()
	if len(rows) == 0 {
		item := sub.Add("还没有用过工作目录")
		item.SetEnabled(false)
		return
	}
	for i, row := range rows {
		if i >= trayRecentMax {
			break
		}
		dir := row.WorkspacePath
		item := sub.Add(shortPath(dir))
		item.SetTooltip(dir)
		item.OnClick(func(*application.Context) {
			// 切目录是业务层的动作，让它照同一条路走：
			// 界面与托盘对「当前目录」的理解必须一致。
			if a.svc == nil {
				return
			}
			if err := a.svc.SetWorkspace(dir); err != nil {
				pkg.Warn("托盘切工作目录失败：%v", err)
				return
			}
			a.showWindow()
			a.refreshTrayMenu()
		})
	}
}

// refreshTrayMenu 重建托盘菜单。工作目录或最近列表变化后调用。
func (a *App) refreshTrayMenu() {
	if a.tray == nil || a.app == nil || a.win == nil {
		return
	}
	a.buildTrayMenu(a.app, a.win)
}

// recentWorkspaces 取最近工作目录。
func (a *App) recentWorkspaces() []domain.WorkspaceRowVO {
	if a.svc == nil {
		return nil
	}
	return a.svc.WorkspaceRows()
}

// shortPath 取最后两级路径，够看清是哪个项目。
func shortPath(dir string) string {
	tail := baseName(dir)
	parent := baseName(parentDir(dir))
	if parent == "" || parent == "." || parent == dir {
		return tail
	}
	return parent + `\` + tail
}

// contextWithTimeout 造一个带超时的上下文。
func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
