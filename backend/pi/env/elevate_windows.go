//go:build windows

// 提权执行：以 UAC 重跑本程序完成系统级 PATH 的写入，等子进程退出拿结果。
// 提权进程无界面：改完注册表、广播环境变更后立即退出，结果只经退出码回传。
package env

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"WorkBaby/backend/pkg"
)

// pathAdminArg 是提权子进程的命令行标志：main 在单实例守卫之前拦截它。
const pathAdminArg = "--path-admin"

// shellExecuteInfo 按 SHELLEXECUTEINFOW 的 C 布局逐字段对应；cbSize 与 fMask 是 4 字节 ULONG，
// 其后 hwnd 按指针对齐落在偏移 8——Go 的对齐规则给出同一布局。
type shellExecuteInfo struct {
	cbSize, fMask uint32
	hwnd          uintptr
	lpVerb        *uint16
	lpFile        *uint16
	lpParameters  *uint16
	lpDirectory   *uint16
	nShow         int32
	hInstApp      uintptr
	lpIDList      uintptr
	lpClass       *uint16
	hkeyClass     uintptr
	dwHotKey      uint32
	hIcon         uintptr
	hProcess      uintptr
}

// elevatePathAdmin 以管理员身份重跑本程序，把一组目录在两级 PATH 间挪动。
// 用户在 UAC 弹窗点「否」时返回 9723 且不改变任何东西。
func elevatePathAdmin(level string, dirs []string) error {
	exe, err := os.Executable()
	if err != nil {
		return pkg.Wrap("9723", "定位本程序失败，没法请求管理员授权", err)
	}
	if len(dirs) == 0 {
		return pkg.New("9726", "没有要挪动的目录", "")
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	// 多个目录用一个参数传，分隔符与 PATH 相同：含空格的目录因此只需要一对普通引号，
	// 不能用 %q——Go 的转义会把反斜杠写成两个，提权进程收到的路径就不是原来那条了。
	params, _ := syscall.UTF16PtrFromString(fmt.Sprintf(`%s %s "%s"`, pathAdminArg, level, strings.Join(dirs, string(os.PathListSeparator))))

	const (
		seeMaskNoCloseProcess = 0x0040
		swHide                = 0
		// UAC 输密码可能很慢，但要赶在前端 HTTP 超时（120s）之前给出结果。
		waitTimeoutMs = 110000
	)
	info := shellExecuteInfo{
		cbSize:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		nShow:        swHide,
	}
	proc := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteExW")
	// SeeMaskNoCloseProcess 要求结构体本身留在栈上不动直到调用返回。
	ret, _, err := proc.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		if isErrorCode(err, 1223) { // ERROR_CANCELLED
			return pkg.New("9723", "没有授权管理员权限，PATH 保持原样", "")
		}
		return pkg.Wrap("9723", "请求管理员授权失败", err)
	}
	if info.hProcess == 0 {
		return pkg.New("9723", "管理员进程没有返回句柄", level)
	}
	wait := syscall.NewLazyDLL("kernel32.dll").NewProc("WaitForSingleObject")
	state, _, _ := wait.Call(info.hProcess, waitTimeoutMs)
	if state == 0xFFFFFFFF { // WAIT_FAILED；超时（0x102）按仍在跑处理，注册表由子进程最终写入
		return pkg.New("9723", "等待管理员进程失败", level)
	}
	var code uint32
	getCode := syscall.NewLazyDLL("kernel32.dll").NewProc("GetExitCodeProcess")
	okRet, _, _ := getCode.Call(info.hProcess, uintptr(unsafe.Pointer(&code)))
	if okRet == 0 {
		return pkg.New("9723", "读不到管理员进程的结果", level)
	}
	switch code {
	case 0:
		pkg.Infof("env: 运行时目录已切到 %s 级 PATH：%s", level, strings.Join(dirs, "；"))
		return nil
	case 2:
		return pkg.New("9723", "写不进系统级 PATH（检查管理员权限）", strings.Join(dirs, "；"))
	case 3:
		return pkg.New("9723", "写不进用户 PATH（检查注册表权限）", strings.Join(dirs, "；"))
	case 4:
		return pkg.New("9723", "提权进程收到的参数不对", level)
	default:
		return pkg.New("9723", "PATH 切换没有完成", fmt.Sprintf("exit=%d", code))
	}
}

// isErrorCode 判断 syscall 错误是不是指定的 GetLastError 值。
func isErrorCode(err error, code uintptr) bool {
	if errno, ok := err.(syscall.Errno); ok {
		return uintptr(errno) == code
	}
	return false
}

// RunPathAdminMode 是提权子进程的入口：main 在一切装配之前调它，命中标志则改完 PATH 即退出。
// 退出码：0 成功，2 系统级写入失败，3 用户级写入失败，4 参数不对；半途失败宁可留在 both 也不留在 none。
func RunPathAdminMode(args []string) (int, bool) {
	if len(args) < 4 || args[1] != pathAdminArg {
		return 0, false
	}
	level, raw := args[2], args[3]
	if level != "system" && level != "user" && level != "none" {
		return 4, true
	}
	dirs := []string{}
	for _, dir := range strings.Split(raw, string(os.PathListSeparator)) {
		if dir = strings.TrimSpace(dir); dir != "" {
			dirs = append(dirs, dir)
		}
	}
	if len(dirs) == 0 {
		return 4, true
	}
	for _, dir := range dirs {
		if code := applyPathLevel(level, dir); code != 0 {
			return code, true
		}
	}
	return 0, true
}

// applyPathLevel 在提权进程里落地单个目录的级别切换。
func applyPathLevel(level, dir string) int {
	switch level {
	case "system":
		if err := appendMachinePathEntry(dir); err != nil {
			return 2
		}
		if err := removeUserPathEntry(dir); err != nil {
			return 3
		}
	case "user":
		if err := prependUserPath(dir); err != nil {
			return 3
		}
		if err := removeMachinePathEntry(dir); err != nil {
			return 2
		}
	default: // none：两级都摘掉
		if err := removeMachinePathEntry(dir); err != nil {
			return 2
		}
		if err := removeUserPathEntry(dir); err != nil {
			return 3
		}
	}
	return 0
}
