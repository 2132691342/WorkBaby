//go:build !windows

package capability

// shellHint 非 Windows 平台无 .cmd 外壳与 shell 内建命令。
func shellHint() string {
	return "exec 工具直接创建进程、不经过 shell；需要管道或重定向时显式调用 sh -c。"
}
