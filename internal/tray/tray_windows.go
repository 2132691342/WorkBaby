//go:build windows

// Windows 系统托盘：Shell_NotifyIcon + 隐藏消息窗 + 弹出菜单自研；
// 消息泵跑在独立 goroutine，与 Wails 主消息循环互不阻塞。
package tray

import (
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmApp              = 0x8000 // WM_APP：托盘回调消息
	wmLeftButtonUp     = 0x0202
	wmRightButtonUp    = 0x0205
	wmLeftButtonDblclk = 0x0203
	wmCommand          = 0x0111

	nimAdd    = 0x00000000
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mfString    = 0x00000000
	mfSeparator = 0x00000800

	tpmLeftAlign   = 0x0000
	tpmTopAlign    = 0x0000
	tpmRightButton = 0x0002

	idiApplication = 32512

	hwndMessage uintptr = ^uintptr(2) // HWND_MESSAGE = -3
)

// 托盘菜单命令 ID。
const (
	trayCmdShow = 1000
	trayCmdPet  = 1001
	trayCmdHide = 1002
	trayCmdQuit = 1003
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procRegisterClassW      = user32.NewProc("RegisterClassW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
)

// msg 与 Win32 MSG 对齐。
type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct {
		x int32
		y int32
	}
}

// point 与 Win32 POINT 对齐。
type point struct {
	x int32
	y int32
}

// notifyIconData 精简版 NOTIFYICONDATAW（含到 szTip 为止；只用 message/icon/tip）。
type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
}

// wndClass WNDCLASSW。
type wndClass struct {
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
}

// 消息回调里的全局态（单托盘实例）。
var (
	trayHWnd   uintptr
	trayCmdFn  func(cmd uint32)
	wndProcPtr uintptr // 保活：GC 不能回收 callback
)

// Run 创建托盘并进入消息循环（阻塞，直到进程退出）；锁定 OS 线程，
// 保证 Win32 消息回调与消息泵同线程，否则窗口过程会在错误线程收消息引发竞争与闪烁。
func Run(actions Actions) {
	runtime.LockOSThread()
	defer func() {
		// 正常路径通过主流程调 wruntime.Quit 触发 WM_QUIT 退出消息循环。
		_ = recover()
	}()
	wndProcPtr = windows.NewCallback(func(hwnd uintptr, uMsg uint32, wParam, lParam uintptr) uintptr {
		switch uMsg {
		case wmApp:
			switch lParam {
			case wmRightButtonUp, wmLeftButtonUp:
				showTrayMenu()
				return 0
			case wmLeftButtonDblclk:
				if trayCmdFn != nil {
					trayCmdFn(trayCmdShow)
				}
				return 0
			}
			return 0
		case wmCommand:
			cmd := uint32(wParam & 0xffff)
			if cmd >= trayCmdShow && cmd <= trayCmdQuit && trayCmdFn != nil {
				trayCmdFn(cmd)
			}
			return 0
		}
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(uMsg), wParam, lParam)
		return r
	})

	className, _ := syscall.UTF16PtrFromString("WorkBabyTrayWindow")
	hInst := getModuleHandle()
	wc := wndClass{
		lpfnWndProc:   wndProcPtr,
		hInstance:     hInst,
		lpszClassName: className,
	}
	procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))

	// 消息窗口：不可见、无任务栏（HWND_MESSAGE = -3）。
	hWnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0, 0, 0, 0, 0,
		hwndMessage,
		0, hInst, 0,
	)
	if hWnd == 0 {
		return
	}
	trayHWnd = hWnd

	hIcon, _, _ := procLoadIconW.Call(0, idiApplication)
	if !addTrayIcon(hWnd, hIcon) {
		return
	}

	trayCmdFn = func(cmd uint32) {
		switch cmd {
		case trayCmdShow:
			actions.ShowMain()
		case trayCmdPet:
			actions.Pet()
		case trayCmdHide:
			actions.Hide()
		case trayCmdQuit:
			actions.Quit()
		}
	}

	// 消息循环（阻塞本 goroutine）。
	for {
		var m msg
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 || ret == ^uintptr(0) { // WM_QUIT 或出错
			break
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	_ = removeTrayIcon(hWnd)
	_, _, _ = procDestroyWindow.Call(hWnd)
}

// showTrayMenu 在托盘图标位置弹出右键菜单。
func showTrayMenu() {
	if trayHWnd == 0 {
		return
	}
	_, _, _ = procSetForegroundWindow.Call(trayHWnd)
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)
	_ = appendMenuString(hMenu, trayCmdShow, "打开主窗口")
	_ = appendMenuString(hMenu, trayCmdPet, "召唤 / 收起桌宠")
	_ = appendMenuString(hMenu, trayCmdHide, "隐藏主窗口")
	appendMenuSeparator(hMenu)
	_ = appendMenuString(hMenu, trayCmdQuit, "退出 WorkBaby")

	var p point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	_, _, _ = procTrackPopupMenu.Call(
		hMenu,
		tpmLeftAlign|tpmTopAlign|tpmRightButton,
		uintptr(uint32(p.x)), uintptr(uint32(p.y)), 0, trayHWnd, 0,
	)
}

// addTrayIcon Shell_NotifyIcon(NIM_ADD)。
func addTrayIcon(hWnd, hIcon uintptr) bool {
	var nid notifyIconData
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hWnd
	nid.uID = 1
	nid.uFlags = nifMessage | nifIcon | nifTip
	nid.uCallbackMessage = wmApp
	nid.hIcon = hIcon
	copy(nid.szTip[:], syscall.StringToUTF16("WorkBaby — 随时召唤桌宠"))
	r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	return r != 0
}

// removeTrayIcon Shell_NotifyIcon(NIM_DELETE)。
func removeTrayIcon(hWnd uintptr) bool {
	var nid notifyIconData
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	nid.hWnd = hWnd
	nid.uID = 1
	r, _, _ := procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	return r != 0
}

// appendMenuString 追加一条菜单项。
func appendMenuString(hMenu uintptr, id uintptr, text string) error {
	p, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	_, _, _ = procAppendMenuW.Call(hMenu, mfString, id, uintptr(unsafe.Pointer(p)))
	return nil
}

// appendMenuSeparator 追加分隔线。
func appendMenuSeparator(hMenu uintptr) {
	_, _, _ = procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
}

// getModuleHandle 取当前进程模块句柄。
func getModuleHandle() uintptr {
	r, _, _ := procGetModuleHandleW.Call(0)
	return r
}
