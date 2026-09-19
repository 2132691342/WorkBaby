// Package http 提供通用 HTTP 请求工具。
//
// ADR: 内联快速解析；httpReq 不暴露为领域类型（CLAUDE.md §2.3.1 例外）。
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// maxResponseLen 响应体返回上限。
const maxResponseLen = 50_000

// HTTPTool 通用 HTTP 请求工具（网络风险）。
type HTTPTool struct {
	client *http.Client
}

// New 构造 HTTPTool（默认 30s 超时）。
func New() *HTTPTool {
	return &HTTPTool{client: &http.Client{Timeout: 30 * time.Second}}
}

func (t *HTTPTool) Name() string              { return "http" }
func (t *HTTPTool) RiskLevel() tool.RiskLevel { return tool.RiskNetwork }
func (t *HTTPTool) Description() string {
	return "发送通用 HTTP 请求（仅允许 GET / POST，详见 CLAUDE.md §2.12），返回状态码、响应头与响应体。"
}

func (t *HTTPTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["method", "url"],
			"properties": {
				"method": {"type": "string", "enum": ["GET", "POST"], "description": "HTTP 方法：仅允许 GET / POST"},
				"url": {"type": "string", "description": "请求 URL"},
				"headers": {"type": "object", "description": "请求头 KV"},
				"body": {"type": "string", "description": "请求体（原始字符串，POST 用）"},
				"timeout": {"type": "integer", "description": "超时毫秒，缺省 30000"}
			}
		}`),
	}
}

type httpReq struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Timeout int               `json:"timeout"`
}

// Execute 发送 HTTP 请求并返回结果。
func (t *HTTPTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req httpReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "http args parse failed", err)}
	}
	method, ok := pkg.NormalizeMethod(req.Method)
	if !ok {
		return tool.ToolResult{Err: pkg.New(4004, fmt.Sprintf("http method not allowed (allow=%s)", pkg.AllowedMethodCSV()), req.Method)}
	}
	if req.URL == "" {
		return tool.ToolResult{Err: pkg.New(4004, "http url is required", "")}
	}

	var body io.Reader
	if req.Body != "" {
		body = bytes.NewBufferString(req.Body)
	}
	hreq, err := http.NewRequestWithContext(ctx, method, req.URL, body)
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "build http request failed", err)}
	}
	for k, v := range req.Headers {
		hreq.Header.Set(k, v)
	}
	if req.Headers == nil || hreq.Header.Get("Content-Type") == "" {
		hreq.Header.Set("Content-Type", "application/json")
	}

	client := t.client
	if req.Timeout > 0 {
		client = &http.Client{Timeout: time.Duration(req.Timeout) * time.Millisecond}
	}
	resp, err := client.Do(hreq)
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "http request failed", err)}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseLen+1))
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "read http response failed", err)}
	}
	truncated := len(raw) > maxResponseLen
	if truncated {
		raw = raw[:maxResponseLen]
	}

	out := map[string]any{
		"status":    resp.StatusCode,
		"body":      string(raw),
		"truncated": truncated,
	}
	// 若响应是 JSON，结构化返回便于 LLM 解析
	var v any
	if json.Valid(raw) && json.Unmarshal(raw, &v) == nil {
		out["json"] = v
	}
	bs, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4006, "marshal http result failed", err)}
	}
	return tool.ToolResult{Content: string(bs), Data: out}
}
