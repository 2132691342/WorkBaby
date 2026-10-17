// 知识库的 MCP 出口：把 FTS5 检索暴露成 PI 能调的工具（stdio JSON-RPC）。
// 内核是外部 PI 进程，索引不在它进程里，所以走 PI 的标准扩展通路而不是内置工具。
package knowledge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// mcpToolName 是 PI 侧看到的工具名；带前缀的是 PI 的命名规则（mcp__{server}__{tool}）。
const (
	mcpToolSearch = "knowledge_search"
	// mcpProtocolVersion 是兜底版本：客户端没在 initialize 里带版本时按这个答。
	mcpProtocolVersion = "2024-11-05"
	// mcpHitRunes 是单条命中回给模型的正文上限：一次检索命中几万字会把上下文窗口灌满。
	mcpHitRunes = 1200
	mcpMaxLimit = 10
)

// bom 是 UTF-8 字节序标记，出现在帧首时必须剥掉。
var bom = []byte{0xEF, 0xBB, 0xBF}

// MCPServer 是知识库的 stdio MCP 服务：一次进程一条 stdin 通道，串行处理请求。
type MCPServer struct {
	svc *Service
	in  *bufio.Scanner
	out io.Writer
}

// NewMCPServer 绑定一个知识库服务与 stdio 两端。
func NewMCPServer(svc *Service, in io.Reader, out io.Writer) *MCPServer {
	sc := bufio.NewScanner(in)
	// 单条 JSON-RPC 请求可能带着长会话文本，默认 64KB 上限会把它截断成坏行。
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	return &MCPServer{svc: svc, in: sc, out: out}
}

// rpcRequest 是一条 JSON-RPC 请求；id 缺失就是通知（不需要回）。
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve 读一行答一行，直到 stdin 关闭。
// 它绝不向 out 写非协议内容：stdout 就是协议通道，一行日志就能让 PI 解析失败。
func (m *MCPServer) Serve() error {
	for m.in.Scan() {
		line := m.in.Bytes()
		// BOM 不是 JSON 的一部分：带着它的客户端确实存在（Windows 侧文本管道），
		// 不在这里剥掉，第一帧 initialize 就会解析失败，握手直接谈崩。
		line = bytes.TrimPrefix(line, bom)
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			m.reply(req.ID, nil, &rpcError{Code: -32700, Message: "parse error"})
			continue
		}
		// 通知没有 id：按协议不能回，回了对方会把这行当成错位的响应。
		if len(req.ID) == 0 {
			continue
		}
		result, rerr := m.handle(req)
		m.reply(req.ID, result, rerr)
	}
	return m.in.Err()
}

// handle 分发协议方法。未知方法按 JSON-RPC 报错而不是忽略——
// PI 需要看见「这个 server 不认识这个动作」，静默不回会表现成握手超时。
func (m *MCPServer) handle(req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return m.initialize(req.Params), nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return m.toolsList(), nil
	case "tools/call":
		return m.toolsCall(req.Params)
	default:
		return nil, &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
}

func (m *MCPServer) initialize(raw json.RawMessage) any {
	version := mcpProtocolVersion
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	// 客户端带了版本就按它答：版本协商是它说了算，硬回自己的会让它直接断开。
	if json.Unmarshal(raw, &p) == nil && p.ProtocolVersion != "" {
		version = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": "workbaby-knowledge", "version": "1"},
	}
}

func (m *MCPServer) toolsList() any {
	return map[string]any{
		"tools": []map[string]any{{
			"name":        mcpToolSearch,
			"description": "在用户导入的本地资料（文档 / 表格 / 演示文稿）里检索，返回命中的原文片段。想知道「公司那份规范怎么说」时先用它。",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "要查的内容，一句关键词或一个问题"},
					"limit": map[string]any{"type": "integer", "description": "最多返回几条（1-10，默认 5）"},
				},
				"required": []string{"query"},
			},
		}},
	}
}

// textBlock 是 MCP 的 content 条目形态。
type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (m *MCPServer) toolsCall(raw json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string `json:"name"`
		Arguments struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "invalid params"}
	}
	if p.Name != mcpToolSearch {
		return toolResult("这个服务只提供 "+mcpToolSearch, true), nil
	}
	if p.Arguments.Query == "" {
		return toolResult("查询不能为空", true), nil
	}
	// 上限在这里夹住：工具声明承诺了 1-10，就不能让一次调用把整个窗口灌满。
	limit := min(p.Arguments.Limit, mcpMaxLimit)
	hits, err := m.svc.Search(context.Background(), p.Arguments.Query, limit)
	if err != nil {
		// 工具内部的失败按协议回 isError：模型看得见才会改问法，抛协议错只会让调用整个断掉。
		return toolResult("检索失败："+err.Error(), true), nil
	}
	return toolResult(renderHits(hits), false), nil
}

// renderHits 把命中渲染成模型好引用的文本块。
// 零命中给一句话而不是空串：空结果会让模型以为工具坏了，转而凭记忆编。
func renderHits(hits []domain.SearchHitVO) string {
	if len(hits) == 0 {
		return "知识库里没有相关内容。"
	}
	var b strings.Builder
	for i, h := range hits {
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(". ")
		b.WriteString(h.Title)
		b.WriteString("（")
		b.WriteString(h.Path)
		b.WriteString("）\n")
		b.WriteString(pkg.Cut(h.Content, mcpHitRunes))
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func toolResult(text string, isError bool) any {
	return map[string]any{
		"content": []textBlock{{Type: "text", Text: text}},
		"isError": isError,
	}
}

func (m *MCPServer) reply(id json.RawMessage, result any, e *rpcError) {
	resp := rpcResp{JSONRPC: "2.0", ID: id, Result: result, Error: e}
	raw, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = m.out.Write(append(raw, '\n'))
}
