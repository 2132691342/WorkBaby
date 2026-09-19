// Package config 封装 Viper；读 config.yaml + ENV 覆盖；运行时配置走 KV（system_settings 表）。
package config

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"sync"

	"WorkBaby/internal/pkg"
	"github.com/spf13/viper"
)

// Config 强类型视图：固化在结构体里，避免散落在业务代码里强类型断言。
type Config struct {
	App       AppCfg       `mapstructure:"app"`
	Logging   LoggingCfg   `mapstructure:"logging"`
	Database  DatabaseCfg  `mapstructure:"database"`
	Assistant AssistantCfg `mapstructure:"assistant"`
	Security  SecurityCfg  `mapstructure:"security"`
}

// SecurityCfg 安全相关：主密钥（API Key 等加密用）。
type SecurityCfg struct {
	MasterKeyB64 string `mapstructure:"masterKeyB64"` // 32 字节 base64；首启自动生成并写回文件
}

type AppCfg struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
	Env     string `mapstructure:"env"`  // dev | prod
	Port    int    `mapstructure:"port"` // 预留（不打 HTTP server；给将来调试用）
}

type LoggingCfg struct {
	Level string `mapstructure:"level"` // debug | info | warn | error
}

type DatabaseCfg struct {
	Path          string `mapstructure:"path"`
	WAL           bool   `mapstructure:"wal"`
	BusyTimeoutMs int    `mapstructure:"busyTimeoutMs"`
	FTS           bool   `mapstructure:"fts"`
}

type AssistantCfg struct {
	DefaultTemperature   float64 `mapstructure:"defaultTemperature"`
	DefaultThinking      string  `mapstructure:"defaultThinking"` // off | low | medium | high
	DefaultContextWindow int     `mapstructure:"defaultContextWindow"`
	CompressionRatio     float64 `mapstructure:"compressionRatio"`
}

var (
	mu     sync.RWMutex
	loaded *Config
)

// Init 读取配置；config.yaml 不存在则用默认值写出（首次启动的友好路径）。
// 同时保证 security.masterKeyB64 不空：空就随机生成并落盘。
func Init(path string) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("WORKBABY")
	v.AutomaticEnv()
	setDefaults(v)

	if _, err := os.Stat(path); err == nil {
		v.SetConfigFile(path)
		if rerr := v.ReadInConfig(); rerr != nil {
			return nil, pkg.Wrap(2002, "read config failed", rerr)
		}
	} else if !os.IsNotExist(err) {
		return nil, pkg.Wrap(2002, "stat config failed", err)
	} else {
		// 文件不存在：写一份默认出去，方便用户编辑
		if werr := v.WriteConfigAs(path); werr != nil {
			return nil, pkg.Wrap(2003, "write default config failed", werr)
		}
	}

	// 主密钥：确保 config.yaml 中至少有一份（首启生成 + 落盘）。
	if _, err := ensureMasterKey(v, path); err != nil {
		return nil, err
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, pkg.Wrap(2002, "unmarshal config failed", err)
	}
	mu.Lock()
	loaded = &c
	mu.Unlock()
	pkg.L.Info("config loaded", "path", path, "env", c.App.Env)
	return &c, nil
}

func ensureMasterKey(v *viper.Viper, path string) (string, error) {
	mk := v.GetString("security.masterKeyB64")
	if mk == "" {
		mk = os.Getenv("WORKBABY_SECURITY_MASTERKEYB64")
	}
	if mk == "" {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return "", pkg.Wrap(2020, "generate master key", err)
		}
		mk = base64.StdEncoding.EncodeToString(key)
		v.Set("security.masterKeyB64", mk)
		if err := v.WriteConfigAs(path); err != nil {
			return "", pkg.Wrap(2021, "persist master key", err)
		}
	}
	return mk, nil
}

// EnsureMasterKey 导出别名，给 handler 单独调（CLI 测试）。
func EnsureMasterKey(v *viper.Viper, path string) (string, error) { return ensureMasterKey(v, path) }

// Get 返回当前快照；用于 service 内部读取，避免持锁。
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return loaded
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "WorkBaby")
	v.SetDefault("app.version", "0.1.0")
	v.SetDefault("app.env", "dev")
	v.SetDefault("app.port", 0)
	v.SetDefault("logging.level", "info")
	v.SetDefault("database.path", "")
	v.SetDefault("database.wal", true)
	v.SetDefault("database.busyTimeoutMs", 5000)
	v.SetDefault("database.fts", true)
	v.SetDefault("assistant.defaultTemperature", 0.25)
	v.SetDefault("assistant.defaultThinking", "medium")
	v.SetDefault("assistant.defaultContextWindow", 128000)
	v.SetDefault("assistant.compressionRatio", 0.75)
	v.SetDefault("security.masterKeyB64", "")
}
