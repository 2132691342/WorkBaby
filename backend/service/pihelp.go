// 服务层内部小工具：写文件、读文件、整数解析。
package service

import (
	"os"
	"strconv"
	"strings"

	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
)

// pkgWarn 记一条警告（封装一层，避免各处重复写格式化）。
func pkgWarn(msg string, err error) {
	pkg.Warnf("%s: %v", msg, err)
}

// writeFileAtomic 原子替换写入。
func writeFileAtomic(path string, data []byte) error {
	return pienv.WriteFileAtomic(path, data, 0o600)
}

// readFileOrEmpty 读文件；不存在时返回空串。
func readFileOrEmpty(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", pkg.Wrap("9713", "读配置文件失败", err)
	}
	return string(raw), nil
}

// parseInt 解析一个整数设置值。
func parseInt(s string) (int, error) { return strconv.Atoi(strings.TrimSpace(s)) }
