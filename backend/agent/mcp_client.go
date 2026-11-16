package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/pkg"
)

// McpServerDef 是一条 stdio MCP 服务定义：从 mcp.json 的一个条目解出来。
// 引擎不认 mcp.json 的 map 形状——service 层解好喂进来，字段语义两边对齐。
type McpServerDef struct {
	// ServerName 服务名（mcp.json 的键）
	ServerName string
	// Command 可执行程序
	Command string
	// Args 参数数组
	Args []string
	// Cwd 工作目录；空串继承当前进程
	Cwd string
	// Env 追加的环境变量
	Env map[string]string
	// TimeoutMs 每次请求的超时毫秒；0 用缺省
	TimeoutMs int
}

// mcpRequestBudget 是一次请求的缺省预算；握手另给更宽的一份。
const mcpRequestBudget = 30 * time.Second

// mcpHandshakeBudget 是握手的缺省预算：npx 类包装器冷启动要下载包，比常速慢得多。
const mcpHandshakeBudget = 60 * time.Second

// McpToolName MCP 工具的全名：与现有内置工具的下划线风格一致。
func McpToolName(server, tool string) string {
	return "mcp_" + server + "_" + tool
}

// McpToolSpec 是从 tools/list 摘出来的一条工具声明。
type McpToolSpec struct {
	// Name 服务侧工具名
	Name string
	// Description 服务给的一句话说明
	Description string
	// Schema 入参 JSON Schema，原样透传给模型
	Schema map[string]any
}

// McpClient 是一个 stdio MCP 服务的会话：握手、拉工具清单、按名调用。
type McpClient struct {
	// def 服务定义
	def McpServerDef
	// proc 长驻子进程
	proc *pkg.PersistentProc
	// lines 应答行通道
	lines <-chan string
	// nextId 请求号，单调递增
	nextId int
	// mu 串行化一次请求：应答只有一条读循环，两个并发请求会互相偷走对方的应答，
	// 而读到不匹配 id 的行只能丢弃——那一次调用就只能等到超时。
	mu sync.Mutex
}

// DialMcp 拉起服务进程并完成 initialize 握手。
// 拉不起来或握手不成就当场收摊：半死的服务留着只会让第一次调用挂到超时。
func DialMcp(def McpServerDef, envBase []string) (*McpClient, error) {
	proc, err := pkg.StartPersistent(def.Command, def.Args, def.Cwd, mcpEnvOf(def, envBase))
	if err != nil {
		return nil, err
	}
	client := &McpClient{def: def, proc: proc, lines: proc.ReadLine(proc.Stdout())}
	if err := client.handshake(); err != nil {
		proc.Kill()
		return nil, err
	}
	return client, nil
}

// mcpEnvOf 子进程环境：包一层必须项，再追加用户配的变量。
func mcpEnvOf(def McpServerDef, base []string) []string {
	if len(def.Env) == 0 {
		return base
	}
	env := make([]string, 0, len(base)+len(def.Env))
	env = append(env, base...)
	for key, value := range def.Env {
		env = append(env, key+"="+value)
	}
	return env
}

// handshake 走一遍 initialize → notifications/initialized。
func (c *McpClient) handshake() error {
	result, err := c.request(context.Background(), "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "WorkBaby", "version": "1.0"},
	}, mcpHandshakeBudget)
	if err != nil {
		return err
	}
	if _, ok := result["serverInfo"]; ok {
		// 服务端说不说 capabilities 都行，拿 serverInfo 当握手成的旁证足够了。
		_ = c.proc.WriteLine(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	}
	return nil
}

// ListTools 拉工具清单。
func (c *McpClient) ListTools() ([]McpToolSpec, error) {
	result, err := c.request(context.Background(), "tools/list", map[string]any{}, c.requestBudget())
	if err != nil {
		return nil, err
	}
	tools, _ := result["tools"].([]any)
	out := make([]McpToolSpec, 0, len(tools))
	for _, item := range tools {
		rec, _ := item.(map[string]any)
		if rec == nil {
			continue
		}
		name, _ := rec["name"].(string)
		if name == "" {
			continue
		}
		desc, _ := rec["description"].(string)
		schema, _ := rec["inputSchema"].(map[string]any)
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, McpToolSpec{Name: name, Description: desc, Schema: schema})
	}
	return out, nil
}

// Call 调一个工具。模型给的 arguments 已是解析后的 map，原样进 params。
func (c *McpClient) Call(ctx context.Context, tool string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	params := map[string]any{"name": tool, "arguments": args}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", pkg.Err(pkg.CodeMcpCall, "调用已取消："+ctxErr.Error())
	}
	result, err := c.request(ctx, "tools/call", params, c.requestBudget())
	if err != nil {
		return "", err
	}
	if isError, _ := result["isError"].(bool); isError {
		return "", pkg.Err(pkg.CodeMcpCall, "MCP 工具报错："+mcpContentText(result["content"]))
	}
	text := mcpContentText(result["content"])
	if text == "" {
		// content 缺省或为空：把结构化结果折成 JSON 给模型，总比空手强。
		if raw, err := pkg.MarshalJSONLine(result); err == nil {
			text = raw
		}
	}
	return text, nil
}

// Close 收摊：杀树不留孤儿。
func (c *McpClient) Close() {
	if c.proc != nil {
		c.proc.Kill()
	}
}

// StderrText 服务端已吐出的错误输出，失败时进 Detail。
func (c *McpClient) StderrText() string { return c.proc.StderrText() }

// Exited 服务进程是否已退出。
func (c *McpClient) Exited() bool { return c.proc.Exited() }

// requestBudget 条目自带的超时（毫秒）；没配用缺省。
func (c *McpClient) requestBudget() time.Duration {
	if c.def.TimeoutMs > 0 {
		return time.Duration(c.def.TimeoutMs) * time.Millisecond
	}
	return mcpRequestBudget
}

// request 发一条请求并等回包。请求号单调递增：并发调用各等各的 id，串号也不至于张冠李戴。
func (c *McpClient) request(ctx context.Context, method string, params map[string]any, budget time.Duration) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextId++
	id := c.nextId
	payload, err := pkg.MarshalJSONLine(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	})
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeMcpCall, "请求编码失败", err)
	}
	if c.proc.Exited() {
		return nil, pkg.Err(pkg.CodeMcpServer, "MCP 服务进程已退出："+c.def.ServerName)
	}
	if err := c.proc.WriteLine(payload); err != nil {
		return nil, err
	}
	rec := mcpWaitFor(ctx, c.lines, id, budget)
	if rec == nil {
		if c.proc.Exited() {
			msg := "MCP 服务进程在等待应答时退出：" + c.def.ServerName
			if detail := c.proc.StderrText(); detail != "" {
				msg += "（" + pkg.Truncate(detail, 200) + "）"
			}
			return nil, pkg.Err(pkg.CodeMcpServer, msg)
		}
		return nil, pkg.Err(pkg.CodeMcpTimeout,
			"MCP 服务在超时内没有应答："+c.def.ServerName+" 的 "+method)
	}
	if errObj, ok := rec["error"].(map[string]any); ok {
		msg, _ := errObj["message"].(string)
		if msg == "" {
			msg = "服务端返回了错误"
		}
		return nil, pkg.Err(pkg.CodeMcpCall, "MCP 调用失败："+msg)
	}
	result, _ := rec["result"].(map[string]any)
	if result == nil {
		return nil, pkg.Err(pkg.CodeMcpCall, "MCP 应答缺 result 字段："+c.def.ServerName)
	}
	return result, nil
}

// mcpWaitFor 在预算内等指定 id 的应答；超时、取消或流断开给 nil。
func mcpWaitFor(ctx context.Context, lines <-chan string, id int, budget time.Duration) map[string]any {
	if budget <= 0 {
		budget = mcpRequestBudget
	}
	deadline := time.After(budget)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return nil
			}
			var rec map[string]any
			if pkg.UnmarshalJSON([]byte(line), &rec) != nil {
				continue
			}
			if got, _ := rec["id"].(float64); int(got) == id {
				return rec
			}
		case <-deadline:
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

// mcpContentText 把 MCP 的 content 数组折成纯文本：text 块直接拼，其余块报类型。
func mcpContentText(raw any) string {
	blocks, ok := raw.([]any)
	if !ok {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, item := range blocks {
		block, _ := item.(map[string]any)
		if block == nil {
			continue
		}
		if text, ok := block["text"].(string); ok {
			parts = append(parts, text)
			continue
		}
		kind, _ := block["type"].(string)
		if kind != "" {
			parts = append(parts, fmt.Sprintf("[%s 内容，文本客户端不显示]", kind))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
