//go:build windows

package capability

// shellHint Windows 下 exec 直接 CreateProcess，不经 shell。
func shellHint() string {
	return "exec 工具直接创建进程、不经过 shell，不支持管道 / 重定向 / 命令串联语法。\n" +
		"需要 shell 能力（|、>、&&）或使用 dir / type / echo / copy 等内建命令时，" +
		"command 传 \"cmd\"、args 传 [\"/c\", \"<完整命令行>\"]；" +
		"需要 PowerShell 时用 \"powershell\" + [\"-Command\", \"...\"]。\n" +
		"npm / npx / uvx 等以 .cmd 形态存在的命令可直接传命令名，执行侧自动包 cmd /c。"
}
