//go:build windows

package pet

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"WorkBaby/internal/domain"
)

// 命中区域上限：矩形过多会让 CreateRectRgn/CombineRgn 的调用次数与耗时线性上涨，
// 而桌宠可见元素远少于这个量级，超出即视为异常输入直接截断。
const maxHitRects = 64

var (
	ctUser32 = syscall.NewLazyDLL("user32.dll")
	ctGdi32  = syscall.NewLazyDLL("gdi32.dll")

	procEnumWindows              = ctUser32.NewProc("EnumWindows")
	procIsWindowVisible          = ctUser32.NewProc("IsWindowVisible")
	procGetWindowTextLen         = ctUser32.NewProc("GetWindowTextLengthW")
	procSetWindowRgn             = ctUser32.NewProc("SetWindowRgn")
	procGetWindowThreadProcessID = ctUser32.NewProc("GetWindowThreadProcessId")

	procCreateRectRgn = ctGdi32.NewProc("CreateRectRgn")
	procCombineRgn    = ctGdi32.NewProc("CombineRgn")
	procDeleteObject  = ctGdi32.NewProc("DeleteObject")
)

const rgnOr = 2 // CombineRgn 的 RGN_OR

// ApplyHitRegion 用命中矩形并集设置窗口可见区域（区域外像素不接收点击，落到下层窗口）。
// rects 为空时清空区域（恢复整窗可交互），避免把用户锁死在「无法点击」的状态。
func ApplyHitRegion(rects []domain.PetHitRect) error {
	if len(rects) == 0 {
		return ClearHitRegion()
	}
	hwnd := mainWindowHandle()
	if hwnd == 0 {
		return nil // 窗口尚未就绪：静默跳过
	}
	if len(rects) > maxHitRects {
		rects = rects[:maxHitRects]
	}
	dst, _, _ := procCreateRectRgn.Call(0, 0, 0, 0)
	if dst == 0 {
		return nil
	}
	for _, r := range rects {
		if r.W <= 0 || r.H <= 0 {
			continue
		}
		src, _, _ := procCreateRectRgn.Call(
			uintptr(int32(r.X)), uintptr(int32(r.Y)),
			uintptr(int32(r.X+r.W)), uintptr(int32(r.Y+r.H)),
		)
		if src == 0 {
			continue
		}
		procCombineRgn.Call(dst, dst, src, uintptr(rgnOr))
		procDeleteObject.Call(src)
	}
	// SetWindowRgn 成功后区域所有权移交系统，不能再 DeleteObject。
	procSetWindowRgn.Call(hwnd, dst, 1)
	return nil
}

// ClearHitRegion 清空窗口区域（恢复默认整窗）。
func ClearHitRegion() error {
	hwnd := mainWindowHandle()
	if hwnd == 0 {
		return nil
	}
	procSetWindowRgn.Call(hwnd, 0, 1)
	return nil
}

// mainWindowHandle 取当前进程的主窗口句柄：可见 + 有标题的顶层窗口（托盘消息窗为 HWND_MESSAGE，不计入）。
func mainWindowHandle() uintptr {
	pid := uint32(os.Getpid())
	var found uintptr
	cb := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		var wpid uint32
		procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&wpid)))
		if wpid != pid {
			return 1 // 继续枚举
		}
		if vis, _, _ := procIsWindowVisible.Call(hwnd); vis == 0 {
			return 1
		}
		if n, _, _ := procGetWindowTextLen.Call(hwnd); n == 0 {
			return 1
		}
		found = hwnd
		return 0 // 命中即停止
	})
	procEnumWindows.Call(cb, 0)
	return found
}
