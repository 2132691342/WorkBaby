package pkg

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// 真实路径解析：路径守卫必须按「文件最终落在哪」判断，而不是按字符串形状。
// 符号链接与 junction 都不带 ModeSymlink 标记（junction 连 os.Lstat 都看不出来），
// 所以只能问系统：打开句柄拿最终路径。这条是工作区边界的最后一道保险。

// kernel32 的最终路径查询；只取 DOS 卷形态，避免 \\?\ 前缀带进业务路径。
var (
	kernel32DLL                   = syscall.NewLazyDLL("kernel32.dll")
	procGetFinalPathNameByHandleW = kernel32DLL.NewProc("GetFinalPathNameByHandleW")
)

// volumeNameDos 让返回值形如 C:\dir\file（而不是 \\?\ 卷 GUID 形态）。
const volumeNameDos = 0x0

// RealPath 求一个路径的真实位置：存在的部分解开符号链接与 junction，
// 尚不存在的尾部按字面拼回去（「写到工作区里的新文件」也要能校验）。
func RealPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if resolved, ok := finalPathName(abs); ok {
		return resolved
	}
	// 路径还不存在：找到最长的存在祖先，解开它，再把剩下的相对段拼回去。
	dir := abs
	tail := make([]string, 0, 4)
	for {
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs
		}
		tail = append([]string{filepath.Base(dir)}, tail...)
		dir = parent
		if resolved, ok := finalPathName(dir); ok {
			return filepath.Join(append([]string{resolved}, tail...)...)
		}
	}
}

// finalPathName 打开一个路径的句柄并取它的最终路径；取不到给 false。
// 用 FILE_FLAG_BACKUP_SEMANTICS 才能对目录开句柄——目录正是链接最常出现的位置。
func finalPathName(path string) (string, bool) {
	pointer, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", false
	}
	handle, err := syscall.CreateFile(
		pointer,
		0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return "", false
	}
	defer syscall.CloseHandle(handle)
	for size := 512; size <= 32*1024; size *= 2 {
		buffer := make([]uint16, size)
		written, _, _ := procGetFinalPathNameByHandleW.Call(
			uintptr(handle),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(size),
			uintptr(volumeNameDos),
		)
		if written == 0 {
			return "", false
		}
		// 返回值是所需长度；比缓冲大说明这次没写下，换更大的缓冲重来。
		if int(written) >= size {
			continue
		}
		return trimVolumePrefix(syscall.UTF16ToString(buffer[:written])), true
	}
	return "", false
}

// trimVolumePrefix 去掉 \\?\ 与 \\?\UNC\ 两种前缀。
// 带前缀的路径能参与字符串比较，但拼进业务数据后会被界面原样显示。
func trimVolumePrefix(path string) string {
	if rest, ok := strings.CutPrefix(path, `\\?\UNC\`); ok {
		return `\\` + rest
	}
	if rest, ok := strings.CutPrefix(path, `\\?\`); ok {
		return rest
	}
	return path
}
