package shell

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// baseName 取路径末段。
func baseName(dir string) string { return filepath.Base(dir) }

// parentDir 取父目录。
func parentDir(dir string) string { return filepath.Dir(dir) }

// displayScale 取主屏缩放比（100% → 1，200% → 2）。
// 取不到时给 1：宁可窗口偏小，也不要按错误的比放大一倍。
func displayScale() int {
	user32 := syscall.NewLazyDLL("user32.dll")
	gdi32 := syscall.NewLazyDLL("gdi32.dll")
	getDC := user32.NewProc("GetDC")
	releaseDC := user32.NewProc("ReleaseDC")
	getDeviceCaps := gdi32.NewProc("GetDeviceCaps")
	logPixelsX := uintptr(88)

	hwnd, _, _ := getDC.Call(0)
	if hwnd == 0 {
		return 1
	}
	defer releaseDC.Call(0, hwnd)
	dpi, _, _ := getDeviceCaps.Call(hwnd, logPixelsX)
	if dpi == 0 {
		return 1
	}
	scale := int(dpi) / 96
	if scale < 1 {
		return 1
	}
	if scale > 3 {
		return 3
	}
	return scale
}

// fatalDialog 在「什么都没有」的时刻把话说出去：没有窗口、没有日志目录，
// 发布版也没有控制台，只剩一个系统消息框可用。
func fatalDialog(title, message, detail string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	text := message
	if detail != "" {
		text += "\n\n" + detail
	}
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)
	const mbIconError = 0x00000010
	messageBox.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), mbIconError)
}

// ensure layout 的 dir 存在（装配早期把几个关键目录建出来）。
func ensureLayoutDirs(dataDir string) {
	for _, dir := range []string{dataDir} {
		if dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
	}
}
