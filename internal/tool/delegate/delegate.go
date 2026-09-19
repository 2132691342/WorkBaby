// Package delegate 提供「子 Agent 委派」工具。
//
// 父 Agent 把可独立完成的子任务交给专用 Agent（coding/research/writer）：
// 子 run 上下文隔离、预算独立、只回传摘要，父上下文不被子任务的中间过程撑爆。
// 委派能力经 ctx 注入（tool.Delegator），本包不感知 harness。
package delegate

import (
	"context"
	"encoding/json"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// Tool 子 Agent 委派工具。
type Tool struct{}

// New 构造委派工具。
func New() *Tool { return &Tool{} }

var paramsSchema = json.RawMessage(`{
	"type": "object",
	"required": ["task"],
	"properties": {
		"agent": {"type": "string", "description": "子 Agent 名：内置 coding（工程）/ research（检索）/ writer（写作）/ default（通用），或设置页定义的自定义子智能体名；未知回退 default"},
		"task":  {"type": "string", "description": "交给子 Agent 的完整任务描述，需自包含（子 Agent 看不到当前对话）"}
	}
}`)

func (t *Tool) Name() string { return "delegate_task" }
func (t *Tool) Description() string {
	return "把可独立完成的大块任务委派给子 Agent（内置 coding/research/writer，或设置页定义的自定义子智能体）。子任务独立执行且只回传摘要，适合调研、批量检索、独立编码等不污染主上下文的工作。"
}
func (t *Tool) Schema() tool.ToolSchema {
	return tool.ToolSchema{Name: t.Name(), Description: t.Description(), Parameters: paramsSchema}
}

// RiskLevel 委派按 exec 级对待：子 Agent 会真实调用工具（可能写文件/跑命令）。
func (t *Tool) RiskLevel() tool.RiskLevel { return tool.RiskExec }

// Execute 执行委派；委派能力未注入时返回可见错误（模型据此改道自己干）。
func (t *Tool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "delegate_task args parse failed", err)}
	}
	req.Task = strings.TrimSpace(req.Task)
	if req.Task == "" {
		return tool.ToolResult{Err: pkg.New(4004, "delegate_task 需要非空 task", "")}
	}
	d := tool.DelegatorFromCtx(ctx)
	if d == nil {
		return tool.ToolResult{Content: "delegate_task 不可用：当前运行环境未开启委派，请自己完成该任务。",
			Err: pkg.New(5008, "委派能力未注入", "")}
	}
	summary, err := d.Delegate(ctx, req.Agent, req.Task)
	if err != nil {
		return tool.ToolResult{Content: "委派失败：" + err.Error(), Err: err}
	}
	return tool.ToolResult{
		Content: "子 Agent（" + orDefault(req.Agent) + "）完成，摘要如下：\n" + summary,
		Meta:    map[string]string{"agent": orDefault(req.Agent)},
	}
}

// orDefault 缺省 Agent 名（与 harness.Agent 的回退语义保持一致）。
func orDefault(name string) string {
	if strings.TrimSpace(name) == "" {
		return "default"
	}
	return name
}
