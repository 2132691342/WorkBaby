// 工作区级配置目录：统一叫 .workbaby；旧 .pi 目录存在时一次性搬过来。
package service

import (
	"os"
	"path/filepath"

	"WorkBaby/backend/pkg"
)

// workspaceConfigDir 返回工作区的配置目录并确保迁移：
// 旧目录存在且新目录不存在时整体改名（一次性）；两边都在时以新目录为准，旧目录不动。
func workspaceConfigDir(workspace string) string {
	next := filepath.Join(workspace, ".workbaby")
	legacy := filepath.Join(workspace, ".pi")
	if !pkg.Exists(next) && pkg.IsDir(legacy) {
		if err := os.Rename(legacy, next); err != nil {
			pkg.Warn("工作区配置目录迁移失败（沿用旧目录）：%v", err)
			return legacy
		}
		pkg.Info("工作区配置目录已迁移：%s → %s", legacy, next)
	}
	return next
}