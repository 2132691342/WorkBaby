package service

import (
	"os"

	"WorkBaby/internal/pkg"
)

// writeConfigFile 原子写用户配置文件：先备份旧文件为 {file}.bak，再写 tmp 后 rename。
// 返回 rollback：调用方在后续业务失败时恢复旧内容（文件原本不存在则删除新文件）。
func writeConfigFile(path, content string) (rollback func(), err error) {
	var old []byte
	hasOld := false
	if data, rerr := os.ReadFile(path); rerr == nil {
		old = data
		hasOld = true
		if werr := os.WriteFile(path+".bak", data, 0o600); werr != nil {
			return nil, pkg.Wrap(2017, "backup config file failed", werr)
		}
	}
	tmp := path + ".tmp"
	if werr := os.WriteFile(tmp, []byte(content), 0o600); werr != nil {
		return nil, pkg.Wrap(2017, "write config file failed", werr)
	}
	if rerr := os.Rename(tmp, path); rerr != nil {
		_ = os.Remove(tmp)
		return nil, pkg.Wrap(2017, "replace config file failed", rerr)
	}
	return func() {
		if hasOld {
			_ = os.WriteFile(path, old, 0o600)
			return
		}
		_ = os.Remove(path)
	}, nil
}
