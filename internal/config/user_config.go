// Package config 负责应用配置与首启文件模板。
package config

import (
	"os"
	"path/filepath"

	"WorkBaby/assets"
	"WorkBaby/internal/pkg"
)

var userConfigTemplates = []struct {
	name string
	data []byte
}{
	{name: "model.json", data: assets.ModelConfig},
	{name: "mcp.json", data: assets.MCPConfig},
}

// EnsureUserConfig 为用户目录补齐默认 model.json / mcp.json；已有文件不覆盖。
func EnsureUserConfig(home string) error {
	if home == "" {
		return pkg.New(2004, "home path is required", "")
	}
	for _, template := range userConfigTemplates {
		target := filepath.Join(home, template.name)
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return pkg.Wrap(2004, "stat user config failed", err)
		}

		tmp := target + ".tmp"
		if err := os.WriteFile(tmp, template.data, 0o600); err != nil {
			return pkg.Wrap(2004, "write user config template failed", err)
		}
		if err := os.Rename(tmp, target); err != nil {
			_ = os.Remove(tmp)
			return pkg.Wrap(2004, "install user config template failed", err)
		}
	}
	return nil
}
