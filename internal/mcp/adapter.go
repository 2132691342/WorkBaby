package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// MCPToolPrefix MCP 工具全局命名前缀：mcp_{server}_{tool}，与内置工具、其他 server 天然隔离。
const MCPToolPrefix = "mcp_"

// mcpMaxResultChars 单次远端返回的最大字符数：超过则补截断标记后回填 LLM，避免撑爆上下文。
// 与 internal/tool/exec 的 MaxToolResultLen 同源；不依赖 tool.MetaOf 是因为 MCPAdapter
// 默认 RiskLevel=Network（即便有 MetaProvider 也会被兜底为 ReadOnly，截断字段被忽略）。
const mcpMaxResultChars = 50_000

// fallbackSchema 远端 inputSchema 缺失或非法 JSON 时的兜底：不信任外部输入，但不让工具彻底不可用。
var fallbackSchema = json.RawMessage(`{"type":"object","properties":{}}`)

// invalidToolNameChars 工具名合法字符集外的字符 → 替换为下划线。
//
// MCP 允许任意字符串作工具名，部分会含 `/` `:` `.` ` ` 等（fs:read、github.create_issue）；
// 上游 LLM 的函数名只接受 [A-Za-z0-9_-]，未清洗的非法字符会让整次请求 400。
var invalidToolNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// sanitizeToolName 把任意字符串收敛到 LLM 函数名合法字符集。
func sanitizeToolName(s string) string {
	if s == "" {
		return "tool"
	}
	return invalidToolNameChars.ReplaceAllString(s, "_")
}

// MCPAdapter 把 MCP 远端工具适配成本地 tool.Tool。
type MCPAdapter struct {
	ServerName string
	Client     Client
	Tool       ToolDef
}

// Name 全局唯一工具名。ServerName 已在 service/mcp.go 校验为 [A-Za-z0-9_-]{1,32}；
// 远端 Tool.Name 自由格式，这里 sanitize 防 `/` `:` 等字符让整次 LLM 请求 400。
func (a *MCPAdapter) Name() string {
	return MCPToolPrefix + a.ServerName + "_" + sanitizeToolName(a.Tool.Name)
}

// Description 远端无描述时给出可读兜底。
func (a *MCPAdapter) Description() string {
	if a.Tool.Description != "" {
		return a.Tool.Description
	}
	return "MCP 工具（来自 server " + a.ServerName + "）"
}

// Schema 暴露给 LLM 的参数定义。
func (a *MCPAdapter) Schema() tool.ToolSchema {
	params := a.Tool.InputSchema
	if len(bytes.TrimSpace(params)) == 0 || !json.Valid(params) {
		params = fallbackSchema
	}
	return tool.ToolSchema{Name: a.Name(), Description: a.Description(), Parameters: params}
}

// RiskLevel 外部进程能力不可控，默认按网络级。
func (a *MCPAdapter) RiskLevel() tool.RiskLevel { return tool.RiskNetwork }

// Meta 显式声明 Group=exec（网络/远程调用），避免被 MetaOf 兜底为 ReadOnly=true
// 参与只读并发执行——MCP 写类工具（发邮件 / 下订单）有副作用，必须串行。
func (a *MCPAdapter) Meta() tool.ToolMeta { return tool.ToolMeta{Group: tool.GroupExec} }

// Execute 转发到远端；远端返回 isError 时内容仍然回填给 LLM，同时置 Err 便于前端提示。
// 大返回（> mcpMaxResultChars）截断，避免读文件类工具一次灌进几十万字符撑爆上下文。
//
// 非文本块降级为占位说明：文本模型读不了像素/二进制，但「这里有一张图/一个资源」
// 的信号本身有价值——静默丢弃会让模型以为工具坏了（返回空）。
func (a *MCPAdapter) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	res, err := a.Client.CallTool(ctx, a.Tool.Name, args)
	if err != nil {
		return tool.ToolResult{Err: err}
	}
	var sb strings.Builder
	for _, c := range res.Content {
		switch c.Type {
		case "text":
			sb.WriteString(c.Text)
		case "image":
			sb.WriteString(fmt.Sprintf("[image: %s, %d bytes base64 — 文本模型无法直接读取，如需查看请让用户保存后人工确认]",
				c.MimeType, len(c.Data)))
		case "resource":
			if c.Text != "" {
				sb.WriteString("[resource: " + c.URI + "]\n" + c.Text)
			} else {
				sb.WriteString("[resource: " + c.URI + "（二进制内容未内联）]")
			}
		default:
			sb.WriteString("[unknown content block: " + c.Type + "]")
		}
		sb.WriteString("\n")
	}
	content := strings.TrimSpace(sb.String())
	if runes := []rune(content); len(runes) > mcpMaxResultChars {
		content = string(runes[:mcpMaxResultChars]) + "\n... (truncated)"
	}
	out := tool.ToolResult{Content: content}
	if res.IsError {
		out.Err = pkg.New(8005, "mcp tool returned an error result", a.Tool.Name)
	}
	return out
}
