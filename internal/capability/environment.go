package capability

import (
	"context"
	"runtime"
	"time"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/tool"
)

// environmentCap 注入运行环境事实（系统 / 架构 / 时间 / exec 调用形态）与 Agent 人设：
// 都是与本次 run 强相关、本会话内不变的静态上下文。
// exec 直接创建进程、不过 shell，命令能不能跑通只能由环境段告知，靠模型猜必然失败。
type environmentCap struct{}

func NewEnvironment() Capability { return &environmentCap{} }

func (c *environmentCap) ID() string { return "environment" }

func (c *environmentCap) Tools() []tool.Tool { return nil }

func (c *environmentCap) Capture(_ context.Context, _ *CaptureCtx) error { return nil }

// Preload 同时输出人设段与环境段；人设空时跳过该段（BuildSystem 自然丢弃空 Body）。
func (c *environmentCap) Preload(_ context.Context, p *PreloadCtx) ([]agent.Section, error) {
	out := make([]agent.Section, 0, 2)
	if persona := p.Def.Persona; persona != "" {
		out = append(out, agent.Section{Key: "persona", Title: "角色", Body: persona, Order: agent.OrderPersona, Priority: agent.PriorityEssential})
	}
	out = append(out, agent.Section{
		Key:      "environment",
		Title:    "运行环境",
		Body:     "操作系统：" + runtime.GOOS + " / " + runtime.GOARCH + "\n当前时间：" + time.Now().Format("2006-01-02 15:04:05") + "\n命令执行：" + shellHint(),
		Order:    agent.OrderEnv,
		Priority: agent.PriorityEssential,
	})
	return out, nil
}