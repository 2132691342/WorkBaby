package agent

import (
	"context"
	"strings"

	"WorkBaby/backend/pkg"
)

// mcpTool 把一个远端 MCP 工具包成本地 Tool。
// 外壳薄到只有转接：声明转发 schema，执行转发调用，翻译统一走 McpClient。
type mcpTool struct {
	// client 服务会话
	client *McpClient
	// serverName 服务名（拼工具名用）
	serverName string
	// toolName 服务侧工具名
	toolName string
	// description 服务给的一句话说明
	description string
	// schema 入参 JSON Schema，原样透传
	schema map[string]any
}

// Spec 工具声明。名字 mcp_{server}_{tool}，服务名里允许的字符与这里拼出的形状一致。
func (t *mcpTool) Spec() ToolSpec {
	return ToolSpec{
		Name:        McpToolName(t.serverName, t.toolName),
		Description: mcpToolDescription(t.description, t.serverName),
		Schema:      t.schema,
	}
}

// mcpToolDescription 描述带上服务名：模型要能从一句话里看出这个工具属于谁。
func mcpToolDescription(desc, server string) string {
	if strings.TrimSpace(desc) == "" {
		return "MCP 工具（服务：" + server + "）"
	}
	return desc
}

// Run 执行一次远端调用。MCP 工具按「写工具」处理：未登记按写处理是仓库不变量，
// 这里不向白名单自荐——计划模式拦下它们是预期行为，不是缺陷。
func (t *mcpTool) Run(ctx context.Context, reg *Registry, args map[string]any) Result {
	if t.client.Exited() {
		return errorf("MCP 服务 %s 已退出（%s）：工具调用失败，可先确认服务进程再重试",
			t.serverName, pkg.CodeMcpServer)
	}
	text, err := t.client.Call(ctx, t.toolName, args)
	if err != nil {
		ae := pkg.As(err)
		return errorf("MCP 工具调用失败（%s）：%s", ae.Code, ae.Message)
	}
	return Result{Text: text, Details: map[string]any{"mcp_server": t.serverName, "mcp_tool": t.toolName}}
}

// LoadMcpTools 对一个服务完成握手与拉清单，把每个工具包成 Tool。
// 返回 nil 表示这个服务这次没有可用的工具段（失败或没工具），
// 上层记 Warn 跳过即可：单个服务起不来不拦其他服务，更不拦引擎。
func LoadMcpTools(def McpServerDef, envBase []string) ([]Tool, *McpClient, error) {
	client, err := DialMcp(def, envBase)
	if err != nil {
		return nil, nil, err
	}
	specs, err := client.ListTools()
	if err != nil {
		client.Close()
		return nil, nil, err
	}
	tools := make([]Tool, 0, len(specs))
	for _, spec := range specs {
		tools = append(tools, &mcpTool{
			client:      client,
			serverName:  def.ServerName,
			toolName:    spec.Name,
			description: spec.Description,
			schema:      spec.Schema,
		})
	}
	return tools, client, nil
}
