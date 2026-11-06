package platform

import (
	"fmt"
	"strings"

	"WorkBaby/backend/pkg"
)

// 开机自启落在 HKCU：装态是本用户的，不需要管理员权限。
const (
	// autoStartRunKey 用户级自启项所在键
	autoStartRunKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	// autoStartValue 自启项名
	autoStartValue = "WorkBaby"
)

// AutoStartEnabled 读注册表判断是否已设为开机自启。
func AutoStartEnabled() bool {
	out, err := regQuery()
	if err != nil {
		return false
	}
	return strings.Contains(out, autoStartValue)
}

// SetAutoStart 打开或关闭开机自启；exePath 是可执行文件绝对路径。
func SetAutoStart(enabled bool, exePath string) error {
	if !enabled {
		return runReg([]string{"delete", autoStartRunKey, "/v", autoStartValue, "/f"})
	}
	// --hidden：开机自启只拉起托盘，不往屏幕上弹一个空窗口。
	value := fmt.Sprintf(`"%s" --hidden`, exePath)
	if err := runReg([]string{"add", autoStartRunKey, "/v", autoStartValue, "/t", "REG_SZ", "/d", value, "/f"}); err != nil {
		return pkg.WrapErr(pkg.CodeInternal, "开机自启设置失败", err)
	}
	return nil
}

// regQuery 读自启项原文。
func regQuery() (string, error) {
	res, err := pkg.RunProc("reg", []string{"query", autoStartRunKey, "/v", autoStartValue}, pkg.ProcOptions{TimeoutMs: 10_000})
	if err != nil {
		return "", err
	}
	return res.Stdout, nil
}

// runReg 跑一条 reg 命令。
// 与 regQuery 走同一条 RunProc：注册表被锁或被杀软拦截时，
// 裸 CombinedOutput 会一直挂着，而「超时必杀」是子进程纪律里最要紧的一条。
func runReg(args []string) error {
	res, err := pkg.RunProc("reg", args, pkg.ProcOptions{TimeoutMs: 10_000})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("reg 退出码 %d: %s", res.Code, strings.TrimSpace(res.Stderr))
	}
	return nil
}
