// 覆盖「全新环境下整条启动装配链必须成功」。
// 曾经整条链在干净机器上失败过一次：界面空着、每个接口都 404，真实原因只在日志里。

package api

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStartupOnCleanEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WORKBABY_HOME", home)

	h := New("test")
	if err := h.Startup(context.Background()); err != nil {
		t.Fatalf("首次启动必须成功: %v", err)
	}
	t.Cleanup(h.Shutdown)

	if h.Svc == nil {
		t.Fatal("服务容器没有装配，界面会空着且接口全 404")
	}
	if h.Cfg.MasterKey == "" {
		t.Fatal("配置没有生成主密钥")
	}
	if h.Repo == nil || h.Registry == nil {
		t.Fatal("仓储或工具注册表没有装配")
	}
	// 11 个内置工具少一个都会让模型突然找不到东西，属于跨模块断裂。
	if got := len(h.Registry.All()); got != 11 {
		t.Fatalf("内置工具数量不对: %d", got)
	}
	if h.Port() != 0 {
		t.Fatal("端口应由 domReady 阶段写入，Startup 阶段应为 0")
	}
	if _, found := h.Registry.Get("read"); !found {
		t.Fatal("read 工具没有注册")
	}
	if h.Skills == nil {
		t.Fatal("技能注册表没有装配")
	}
	if len(h.Skills.List()) == 0 {
		t.Fatal("内置技能没有加载")
	}
	if filepath.Base(h.Paths.DBPath) != "workbaby.db" {
		t.Fatalf("数据库路径不对: %s", h.Paths.DBPath)
	}
}
