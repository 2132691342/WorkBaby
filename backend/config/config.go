// Package config 负责应用配置：config.yaml 由 Viper 读写，密钥主密钥首启生成。
package config

import (
	"errors"
	"os"

	"WorkBaby/backend/pkg"

	"github.com/spf13/viper"
)

// Config 是应用级配置，只有不适合放进数据库的东西才放这里。
type Config struct {
	MasterKey string
	Workspace string
	LogLevel  string
	v         *viper.Viper
}

// Load 读取配置；不存在则生成主密钥后写入。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetDefault("log_level", "info")
	v.SetDefault("workspace", "")
	v.SetDefault("master_key", "")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			if !isNotExist(err) {
				return nil, pkg.Wrap(2001, "读取配置文件失败", err)
			}
		}
	}

	c := &Config{
		MasterKey: v.GetString("master_key"),
		Workspace: v.GetString("workspace"),
		LogLevel:  v.GetString("log_level"),
		v:         v,
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.MasterKey == "" {
		key, err := pkg.NewMasterKey()
		if err != nil {
			return nil, err
		}
		c.MasterKey = key
		if err := c.Save(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Save 落盘配置。
func (c *Config) Save() error {
	c.v.Set("master_key", c.MasterKey)
	c.v.Set("workspace", c.Workspace)
	c.v.Set("log_level", c.LogLevel)
	if err := c.v.WriteConfig(); err != nil {
		return pkg.Wrap(2001, "写入配置文件失败", err)
	}
	return nil
}

// SetWorkspace 更新工作目录并落盘。
func (c *Config) SetWorkspace(dir string) error {
	c.Workspace = dir
	return c.Save()
}

// isNotExist 判断「配置文件还没建」这种正常首启场景。
// 指定了具体文件路径时 Viper 返回 *fs.PathError 而不是 ConfigFileNotFoundError，两种都要认。
func isNotExist(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
		return true
	}
	return errors.Is(err, os.ErrNotExist)
}
