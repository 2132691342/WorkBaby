// Package mcp 是 MCP（Model Context Protocol）客户端：
// JSON-RPC 2.0 over 子进程 stdin/stdout；v1 只实现 tools 子集（initialize / tools/list / tools/call）。
//
// 边界：不依赖 harness / api / service / wails；进程管理与协议解析在本包内闭环。
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"WorkBaby/internal/pkg"
)

const (
	// protocolVersion 与 MCP spec 2025-06-18 对齐。
	protocolVersion = "2025-06-18"
	// maxLineBytes stdout 单行上限，防日志炸弹撑爆内存。
	maxLineBytes = 1 << 20
	// defaultCallTimeout 单次 RPC 超时兜底（调用方 ctx 无 deadline 时生效）。
	defaultCallTimeout = 60 * time.Second
	// stderrTailBytes 保留子进程 stderr 尾部字节数，用于启动失败时定位原因。
	stderrTailBytes = 4096
)

// ToolDef MCP tools/list 返回的工具定义。
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Content MCP 工具结果的内容块（v1 只处理 text）。
// Content MCP 返回的内容块。v1 常见三类：text / image / resource。
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"` // image 块的媒体类型
	Data     string `json:"data,omitempty"`     // image 块的 base64 数据
	URI      string `json:"uri,omitempty"`      // resource 块的 URI
}

// Result MCP tools/call 的结果。
type Result struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError"`
}

// ServerInfo initialize 返回的服务端信息。
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Client MCP 客户端契约；StdioClient 是 v1 唯一实现，接口便于测试替换与 v2 加 HTTP。
type Client interface {
	Initialize(ctx context.Context) (*ServerInfo, error)
	ListTools(ctx context.Context) ([]ToolDef, error)
	CallTool(ctx context.Context, name string, args json.RawMessage) (*Result, error)
	Close() error
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("mcp rpc error %d: %s", e.Code, e.Message) }

// StdioClient 通过子进程 stdio 与单个 MCP server 通信；写请求串行化，读在后台分发。
type StdioClient struct {
	name      string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    *tailWriter
	done      chan struct{} // Close 触发；用于 Close 同步等待 cmd 退出
	death     chan struct{} // markDead / Close 都关闭；Manager 监听 server 死亡
	writeMu   sync.Mutex
	pendingMu sync.Mutex
	pending   map[int64]chan *rpcResponse
	nextID    int64
	dead      atomicError
	closeOnce sync.Once // 关闭 stdin / kill 进程 / 等 cmd 退出
	deathOnce sync.Once // 关闭 death 通道
}

// Done 返回「子进程已退出」信号的 channel：Manager 据此自动注销已死亡 server 的工具，
// 避免模型反复调用一个不再存在的远端工具（连不上 → 8003 → Stagnation 熔断）。
func (c *StdioClient) Done() <-chan struct{} { return c.death }

// DialStdio 启动 MCP server 子进程并完成 stdio 接线；进程退出由后台 goroutine 接管。
// 调用方负责 Close，否则子进程会残留。
//
// pathDirs 可选：非空时把 dirs 前置到子进程 PATH（如内置 node/python 解压根），
// 让 npx / uvx 等命令在系统 PATH 上缺失时也能找到。
func DialStdio(name, command string, args, env []string, pathDirs func() []string) (*StdioClient, error) {
	if command == "" {
		return nil, pkg.New(8003, "mcp command is empty", name)
	}
	// 平台侧解析：Windows 上 .cmd 外壳需包 cmd /c 才能 CreateProcess
	exe, prefix := resolveCommand(command)
	cmd := exec.Command(exe, append(prefix, args...)...)
	cmd.Env = append(cmd.Environ(), env...)
	if pathDirs != nil {
		if dirs := pathDirs(); len(dirs) > 0 {
			cmd.Env = envWithPath(cmd.Env, dirs)
		}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // Windows 不弹控制台

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, pkg.Wrap(8003, "mcp stdin pipe failed", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, pkg.Wrap(8003, "mcp stdout pipe failed", err)
	}
	tail := newTailWriter(stderrTailBytes)
	cmd.Stderr = tail
	if err := cmd.Start(); err != nil {
		return nil, pkg.Wrap(8003, "mcp server start failed", fmt.Errorf("%s: %w", command, err))
	}

	c := &StdioClient{
		name:    name,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  tail,
		done:    make(chan struct{}),
		death:   make(chan struct{}),
		pending: make(map[int64]chan *rpcResponse),
	}
	go c.readLoop()
	go func() {
		waitErr := cmd.Wait()
		if waitErr == nil {
			waitErr = errors.New("exit status 0")
		}
		c.markDead(fmt.Errorf("mcp server exited: %w", waitErr))
	}()
	return c, nil
}

// Name 返回本客户端对应的 server 名。
func (c *StdioClient) Name() string { return c.name }

// StderrTail 返回子进程 stderr 尾部内容（启动失败定位用）。
func (c *StdioClient) StderrTail() string { return c.stderr.String() }

// Initialize 完成 MCP 握手并发送 initialized 通知。
func (c *StdioClient) Initialize(ctx context.Context) (*ServerInfo, error) {
	var out struct {
		ProtocolVersion string     `json:"protocolVersion"`
		ServerInfo      ServerInfo `json:"serverInfo"`
	}
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "WorkBaby", "version": "0.1.0"},
	}
	if err := c.call(ctx, "initialize", params, &out); err != nil {
		return nil, wrapMCP(8004, "mcp initialize failed", err)
	}
	_ = c.notify("notifications/initialized", map[string]any{})
	return &out.ServerInfo, nil
}

// ListTools 拉取服务端工具清单（分页跟进 nextCursor 至空）。
//
// MCP spec 2025-06-18 的 tools/list 是分页接口；不跟 nextCursor 会让工具超一页的 server
// 静默丢失工具却仍报 Ready=true。
func (c *StdioClient) ListTools(ctx context.Context) ([]ToolDef, error) {
	out := make([]ToolDef, 0, 8)
	var cursor string
	for {
		var page struct {
			Tools      []ToolDef `json:"tools"`
			NextCursor string    `json:"nextCursor,omitempty"`
		}
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.call(ctx, "tools/list", params, &page); err != nil {
			return nil, wrapMCP(8006, "mcp tools/list failed", err)
		}
		out = append(out, page.Tools...)
		if page.NextCursor == "" {
			return out, nil
		}
		cursor = page.NextCursor
	}
}

// CallTool 调用远端工具；args 为 LLM 产出的原始 JSON。
func (c *StdioClient) CallTool(ctx context.Context, name string, args json.RawMessage) (*Result, error) {
	var arguments any = map[string]any{}
	if len(bytes.TrimSpace(args)) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return nil, pkg.Wrap(8005, "mcp tool args is not valid json", err)
		}
	}
	var out Result
	params := map[string]any{"name": name, "arguments": arguments}
	if err := c.call(ctx, "tools/call", params, &out); err != nil {
		return nil, wrapMCP(8005, "mcp tool call failed", err)
	}
	return &out, nil
}

// Close 关闭 stdin 并终止子进程；可重复调用。
func (c *StdioClient) Close() error {
	var killErr error
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		if c.cmd.Process != nil {
			killErr = c.cmd.Process.Kill()
		}
		select {
		case <-c.done:
		case <-time.After(3 * time.Second):
		}
	})
	c.deathOnce.Do(func() { close(c.death) })
	return killErr
}

// call 发起一次 JSON-RPC 请求并等待同 id 响应。
func (c *StdioClient) call(ctx context.Context, method string, params any, out any) error {
	if err := c.dead.Load(); err != nil {
		return pkg.Wrap(8003, "mcp server is not running", err)
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultCallTimeout)
		defer cancel()
	}

	c.pendingMu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan *rpcResponse, 1)
	c.pending[id] = ch
	c.pendingMu.Unlock()
	defer c.dropPending(id)

	payload, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
	if err != nil {
		return pkg.Wrap(8000, "marshal mcp request failed", err)
	}
	c.writeMu.Lock()
	_, werr := c.stdin.Write(append(payload, '\n'))
	c.writeMu.Unlock()
	if werr != nil {
		return pkg.Wrap(8003, "write to mcp server failed", werr)
	}

	select {
	case <-ctx.Done():
		return pkg.Wrap(8000, "mcp call cancelled or timed out", ctx.Err())
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if out != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, out); err != nil {
				return pkg.Wrap(8000, "unmarshal mcp result failed", err)
			}
		}
		return nil
	}
}

// notify 发送不需要响应的通知（无 id 字段）。
func (c *StdioClient) notify(method string, params any) error {
	payload, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: method, Params: params})
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.stdin.Write(append(payload, '\n'))
	return err
}

// readLoop 按行读取 stdout 并按 id 分发；非 JSON 行（server 调试输出）忽略。
// 进程结束或读失败时唤醒全部等待者，避免调用方永久挂起。
func (c *StdioClient) readLoop() {
	defer close(c.done)

	scanner := bufio.NewScanner(c.stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}
		if resp.ID == nil { // 服务端通知，v1 不处理
			continue
		}
		c.pendingMu.Lock()
		ch := c.pending[*resp.ID]
		delete(c.pending, *resp.ID)
		c.pendingMu.Unlock()
		if ch != nil {
			ch <- &resp // 缓冲 1，不阻塞
		}
	}
	if err := scanner.Err(); err != nil {
		c.markDead(fmt.Errorf("read mcp stdout: %w", err))
		return
	}
	c.markDead(errors.New("mcp stdout closed"))
}

// markDead 记录死亡原因并唤醒所有等待中的请求；同时关闭 death 通道通知 Manager 注销工具。
func (c *StdioClient) markDead(reason error) {
	if reason == nil {
		reason = errors.New("unknown reason")
	}
	if c.dead.Load() == nil {
		c.dead.Store(reason)
	}
	c.pendingMu.Lock()
	waiting := make([]chan *rpcResponse, 0, len(c.pending))
	for id, ch := range c.pending {
		waiting = append(waiting, ch)
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
	for _, ch := range waiting {
		ch <- &rpcResponse{Error: &rpcError{Code: -32000, Message: reason.Error()}}
	}
	c.deathOnce.Do(func() { close(c.death) })
}

func (c *StdioClient) dropPending(id int64) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

// wrapMCP 包 AppError；已是 AppError 的（如进程已死 8003）保留原始码，避免二次包装丢语义。
func wrapMCP(code int, message string, err error) error {
	if ae, ok := pkg.As(err); ok {
		return ae
	}
	return pkg.Wrap(code, message, err)
}

// atomicError 互斥保护的错误槽（进程死亡原因只写一次，读频繁）。
type atomicError struct {
	mu  sync.Mutex
	err error
}

func (a *atomicError) Load() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

func (a *atomicError) Store(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.err = err
}

// tailWriter 只保留最近 size 字节的 io.Writer（子进程 stderr 诊断用）。
type tailWriter struct {
	mu   sync.Mutex
	buf  []byte
	size int
}

func newTailWriter(size int) *tailWriter { return &tailWriter{buf: make([]byte, 0, size), size: size} }

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.size {
		w.buf = append([]byte(nil), w.buf[len(w.buf)-w.size:]...)
	}
	return len(p), nil
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}
