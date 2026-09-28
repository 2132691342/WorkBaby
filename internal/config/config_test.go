// 覆盖「全新机器第一次启动必须能起来」这条不变量。
// 它曾经坏过：指定配置文件路径时 Viper 返回 *fs.PathError 而不是 ConfigFileNotFoundError，
// 漏判导致首次启动直接失败——界面空着、接口全 404，而真实原因只在日志里。

package config

import (
	"path/filepath"
	"testing"
)

func TestLoadCreatesConfigOnFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	c, err := Load(path)
	if err != nil {
		t.Fatalf("首次启动不应失败: %v", err)
	}
	if c.MasterKey == "" {
		t.Fatal("首次启动必须生成主密钥")
	}
	if c.LogLevel != "info" {
		t.Fatalf("日志级别应取缺省值，实际 %q", c.LogLevel)
	}

	// 落盘后重启必须读回同一把密钥，否则已加密的 API Key 全部解不开。
	again, err := Load(path)
	if err != nil {
		t.Fatalf("二次启动失败: %v", err)
	}
	if again.MasterKey != c.MasterKey {
		t.Fatal("主密钥没有持久化")
	}
}

func TestSetWorkspacePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := c.SetWorkspace(dir); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if again.Workspace != dir {
		t.Fatalf("工作目录没有持久化: %q", again.Workspace)
	}
}
