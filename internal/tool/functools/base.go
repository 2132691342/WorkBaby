// Package functools 纯函数工具集合：无 IO、无副作用，
// 输入 JSON 参数 → 输出 JSON 结果，全部可用标准库实现（math_eval 复用 expr）。
//
// ADR: 通用 FuncTool 骨架 + 内联 schema，避免 30+ 个工具各写一遍 Tool 接口样板。
package functools

import (
	"context"
	"encoding/json"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// execFunc 执行函数：解析 args 返回任意结果（自动 JSON 序列化）。
type execFunc func(ctx context.Context, raw json.RawMessage) (any, error)

// FuncTool 纯函数工具骨架：schema 自描述 + 执行函数。
type FuncTool struct {
	name   string
	desc   string
	risk   tool.RiskLevel
	schema string
	fn     execFunc
}

// New 构造纯函数工具。
func New(name, desc string, risk tool.RiskLevel, schema string, fn execFunc) *FuncTool {
	return &FuncTool{name: name, desc: desc, risk: risk, schema: schema, fn: fn}
}

func (t *FuncTool) Name() string              { return t.name }
func (t *FuncTool) Description() string       { return t.desc }
func (t *FuncTool) RiskLevel() tool.RiskLevel { return t.risk }

func (t *FuncTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{Name: t.name, Description: t.desc, Parameters: json.RawMessage(t.schema)}
}

// Meta 打 functools 分组标签，让 ToolService 能在「未配置启停」时按分组默认禁用。
func (t *FuncTool) Meta() tool.ToolMeta {
	return tool.ToolMeta{Group: tool.GroupFunctools}
}

// Execute 调用执行函数并把结果序列化为 JSON 文本（ToolResult.Content）。
// 字符串结果原样返回（LLM 直接读文本），其余类型 JSON 序列化。
func (t *FuncTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	v, err := t.fn(ctx, args)
	if err != nil {
		return tool.ToolResult{Err: err}
	}
	if s, ok := v.(string); ok {
		return tool.ToolResult{Content: s, Data: map[string]any{"value": s}}
	}
	bs, err := json.Marshal(v)
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "marshal tool result failed", err)}
	}
	return tool.ToolResult{Content: string(bs), Data: map[string]any{"value": v}}
}

// All 返回全部已注册纯函数工具（handler 启动装配时批量注册）。
func All() []tool.Tool {
	return []tool.Tool{
		mathEvalTool(),
		mathStatsTool(),
		currentTimeTool(),
		dateAddTool(),
		dateDiffTool(),
		textReplaceTool(),
		textCountTool(),
		textExtractTool(),
		regexMatchTool(),
		regexExtractTool(),
		regexReplaceTool(),
		jsonParseTool(),
		jsonGetTool(),
		jsonValidateTool(),
		csvReadTool(),
		csvWriteTool(),
		hashMd5Tool(),
		hashSha256Tool(),
		hashHmacTool(),
		base64EncodeTool(),
		base64DecodeTool(),
		urlEncodeTool(),
		urlDecodeTool(),
		randomUUIDTool(),
		randomStringTool(),
		randomNumberTool(),
		dataCleanTool(),
		dataAggregateTool(),
		dataValidateTool(),
		ipLookupTool(),
	}
}
