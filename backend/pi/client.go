// Package pi 监督外部 PI 内核进程：拉起、JSONL 协议编解码、事件翻译与生命周期。
// 它不实现 Agent，只负责运行、观测并控制 Agent；上层通过 Client 的命令与事件接口使用它。
package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"WorkBaby/backend/pkg"
)

// 错误码段位 9700-9799：PI 子进程 / 协议 / 环境引导。
var (
	ErrProcessDown   = pkg.New("9701", "助手进程已经退出，正在重新拉起", "")
	ErrCommandFailed = pkg.New("9702", "助手没有接受这条指令", "")
	ErrNotReady      = pkg.New("9704", "助手环境还没准备好", "")
)

// commandTimeout 是单条命令的默认等待上限。prompt 只等"已接受"，很快；
// abort 要等会话真正空闲，所以它单独给长超时。
const (
	commandTimeout = 30 * time.Second
	abortTimeout   = 90 * time.Second
	stopGrace      = 5 * time.Second
)

// Config 是一次 spawn 的全部参数。
type Config struct {
	// Exe 是 PI 的入口（node.exe，脚本在 Args[0]）；测试里也可以是假 PI 的可执行文件。
	Exe string
	// Workspace 是子进程工作目录，即 PI 的 cwd。
	Workspace string
	// Env 是完整环境（含注入的 PATH），nil 表示继承当前进程。
	Env []string
	// Args 是入口脚本与附加参数（不含 --mode rpc，由客户端补，且必须排在它前面）。
	Args []string
	// NoSession 为 true 时不落 PI 的会话文件。
	NoSession bool
}

// Handler 接收翻译前的原始事件；由 service 层的翻译器实现。
// 显式调用而不是回调解耦：事件必须按到达顺序同步处理，异步回调会打乱先后。
type Handler interface {
	// OnEvent 收到一条会话事件。
	OnEvent(ev Event)
	// OnExit 在子进程退出时调用一次；err 为 nil 表示有序退出。
	OnExit(err error)
}

// Client 是一个 PI 子进程的协议客户端。
type Client struct {
	cfg Config
	h   Handler

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	mu      sync.Mutex
	waits   map[string]chan response
	pending map[string]struct{}
	seq     atomic.Uint64
	dead    atomic.Bool
	closed  atomic.Bool

	// done 在读写协程都结束后关闭，供 Stop 等待。
	done chan struct{}
	// exited 保护 OnExit 只调一次。
	exited sync.Once
	// stderrTail 保留 stderr 的最后若干行，用于报错时给人看。
	stderrTail *tailBuffer
}

// response 是一条命令响应。
type response struct {
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

// Start 拉起子进程并开始收记录。返回时进程已启动，但 PI 可能尚未就绪。
func Start(cfg Config, h Handler) (*Client, error) {
	if cfg.Exe == "" {
		return nil, ErrNotReady
	}
	// cfg.Args 是入口脚本（或扩展参数），必须排在 --mode 之前：node 把第一个非选项
	// 参数当脚本，之后的原样交给它；反过来 node 会自己去解析 --mode，报 "bad option"。
	args := make([]string, 0, len(cfg.Args)+3)
	args = append(args, cfg.Args...)
	args = append(args, "--mode", "rpc")
	if cfg.NoSession {
		args = append(args, "--no-session")
	}

	cmd := exec.Command(cfg.Exe, args...)
	cmd.Dir = cfg.Workspace
	if len(cfg.Env) > 0 {
		cmd.Env = cfg.Env
	}
	hideWindow(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, pkg.Wrap("9705", "无法建立助手输入通道", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, pkg.Wrap("9705", "无法建立助手输出通道", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, pkg.Wrap("9705", "无法建立助手诊断通道", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, pkg.Wrap("9705", "拉不起助手进程（PI）", err)
	}

	c := &Client{
		cfg: cfg, h: h, cmd: cmd,
		stdin: stdin, stdout: stdout, stderr: stderr,
		waits:      map[string]chan response{},
		pending:    map[string]struct{}{},
		done:       make(chan struct{}),
		stderrTail: newTailBuffer(64),
	}
	go c.readStdout()
	go c.readStderr()
	return c, nil
}

// Alive 报告子进程是否仍在运行。
func (c *Client) Alive() bool { return !c.dead.Load() && !c.closed.Load() }

// PID 返回子进程号（排障用）。
func (c *Client) PID() int {
	if c.cmd == nil || c.cmd.Process == nil {
		return 0
	}
	return c.cmd.Process.Pid
}

// Stderr 返回最近的诊断输出，供错误信息带上真实原因。
func (c *Client) Stderr() string { return c.stderrTail.String() }

// Call 发一条命令并等它的响应。
func (c *Client) Call(ctx context.Context, name string, payload any) (json.RawMessage, error) {
	return c.call(ctx, commandTimeout, name, payload)
}

// CallLong 用于 abort 这类要等会话真正空闲的命令。
func (c *Client) CallLong(ctx context.Context, name string, payload any) (json.RawMessage, error) {
	return c.call(ctx, abortTimeout, name, payload)
}

func (c *Client) call(ctx context.Context, timeout time.Duration, name string, payload any) (json.RawMessage, error) {
	if !c.Alive() {
		return nil, ErrProcessDown
	}
	id := fmt.Sprintf("wb-%d", c.seq.Add(1))

	body := map[string]any{"id": id, "type": name}
	if payload != nil {
		switch v := payload.(type) {
		case map[string]any:
			for k, val := range v {
				body[k] = val
			}
		default:
			raw, err := json.Marshal(payload)
			if err != nil {
				return nil, pkg.Wrap("9702", "指令参数无法编码", err)
			}
			var asMap map[string]any
			if err := json.Unmarshal(raw, &asMap); err != nil {
				return nil, pkg.Wrap("9702", "指令参数形态不对", err)
			}
			for k, val := range asMap {
				body[k] = val
			}
		}
	}

	ch := make(chan response, 1)
	c.mu.Lock()
	c.waits[id] = ch
	c.mu.Unlock()

	if err := c.write(body); err != nil {
		c.clearWait(id)
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, ErrProcessDown
		}
		if !resp.Success {
			msg := resp.Error
			if msg == "" {
				msg = "助手拒绝了这个操作"
			}
			return nil, pkg.New("9702", msg, resp.Command)
		}
		return resp.Data, nil
	case <-ctx.Done():
		c.clearWait(id)
		return nil, pkg.Wrap("9703", "操作被取消", ctx.Err())
	case <-timer.C:
		c.clearWait(id)
		return nil, pkg.New("9703", "助手一直没有回应", name)
	}
}

// WriteResponse 回一条扩展 UI 响应（审批决策的唯一出口）。
func (c *Client) WriteResponse(payload any) error {
	if !c.Alive() {
		return ErrProcessDown
	}
	return c.write(payload)
}

// write 写一条 JSONL 记录并刷出。
func (c *Client) write(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return pkg.Wrap("9702", "指令无法编码", err)
	}
	// LF 是唯一记录分隔符；\n 在 JSON 字符串里只会以 \n 转义形式出现，不会误切。
	raw = append(raw, '\n')

	c.mu.Lock()
	stdin := c.stdin
	c.mu.Unlock()
	if stdin == nil {
		return ErrProcessDown
	}
	n, err := stdin.Write(raw)
	if err != nil {
		c.markDead(err)
		return pkg.Wrap("9701", "助手进程已经不接受指令了", err)
	}
	if n != len(raw) {
		// 半条 JSON 写进去会让 PI 的解析器吞掉后续指令，表现为"所有命令都超时"。
		c.markDead(io.ErrShortWrite)
		return pkg.Wrap("9701", "指令没有完整送达助手", io.ErrShortWrite)
	}
	return nil
}

// readStdout 按 LF 切分记录并分发。禁止用会按 U+2028/U+2029 切分的行读取器——
// 那两个字符在 JSON 字符串里合法，按它们切会截断记录。
func (c *Client) readStdout() {
	defer c.settle()
	r := bufio.NewReaderSize(c.stdout, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			c.dispatch(trimEOL(line))
		}
		if err != nil {
			return
		}
	}
}

// readStderr 只收集诊断，绝不解析为协议。
func (c *Client) readStderr() {
	r := bufio.NewReaderSize(c.stderr, 1<<16)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			c.stderrTail.Add(strings.TrimRight(string(trimEOL(line)), "\r"))
		}
		if err != nil {
			return
		}
	}
}

// dispatch 分类一条记录：响应按 id 唤醒，事件交给 handler，其余忽略。
func (c *Client) dispatch(line []byte) {
	if len(line) == 0 {
		return
	}
	var probe struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Method    string          `json:"method"`
		Success   bool            `json:"success"`
		Command   string          `json:"command"`
		Error     string          `json:"error"`
		Data      json.RawMessage `json:"data"`
		Timestamp int64           `json:"-"`
	}
	// 拷一份给事件解析用：Event 需要额外字段，先在这里只做分类。
	raw := make([]byte, len(line))
	copy(raw, line)
	if err := json.Unmarshal(line, &probe); err != nil {
		// 无法解析的行不致命：PI 的日志偶发混入，忽略即可，但不能让它中断读循环。
		return
	}
	switch probe.Type {
	case "response":
		c.mu.Lock()
		ch := c.waits[probe.ID]
		delete(c.waits, probe.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- response{Command: probe.Command, Success: probe.Success, Error: probe.Error, Data: probe.Data}
		}
	case "extension_ui_request":
		ev, ok := parseEvent(raw)
		if !ok {
			return
		}
		c.h.OnEvent(ev)
	default:
		ev, ok := parseEvent(raw)
		if !ok {
			// 未知记录必须忽略而不是报错：装了扩展的环境首帧就是 setStatus/setWidget，
			// 当错误处理会让每个装扩展的环境都起不来。
			return
		}
		c.h.OnEvent(ev)
	}
}

// settle 在读写协程退出后统一收尾：唤醒所有等待者、通知 handler、关 done。
func (c *Client) settle() {
	c.exited.Do(func() {
		err := c.cmd.Wait()
		c.markDead(err)

		c.mu.Lock()
		for id, ch := range c.waits {
			close(ch)
			delete(c.waits, id)
		}
		c.mu.Unlock()

		if c.h != nil {
			c.h.OnExit(err)
		}
	})
}

// markDead 标记进程不可用并关掉输入通道。
func (c *Client) markDead(err error) {
	if c.dead.Swap(true) {
		return
	}
	c.mu.Lock()
	stdin := c.stdin
	c.stdin = nil
	c.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if c.done != nil {
		select {
		case <-c.done:
		default:
			close(c.done)
		}
	}
	_ = err
}

// clearWait 摘掉一个等待条目。
func (c *Client) clearWait(id string) {
	c.mu.Lock()
	delete(c.waits, id)
	c.mu.Unlock()
}

// Stop 有序关闭：关 stdin（PI 的关闭信号），等它自己收尾，超时再强杀。
func (c *Client) Stop() {
	if c.closed.Swap(true) {
		return
	}
	c.mu.Lock()
	stdin := c.stdin
	c.stdin = nil
	c.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}

	select {
	case <-c.done:
		return
	case <-time.After(stopGrace):
	}

	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
	}
}

// Wait 等待子进程结束。
func (c *Client) Wait() { <-c.done }

// trimEOL 去掉行尾的 LF 与可选 CR。
func trimEOL(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	if n := len(b); n > 0 && b[n-1] == '\r' {
		b = b[:n-1]
	}
	return b
}

// tailBuffer 保留最后 N 行，避免 stderr 无限增长。
type tailBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newTailBuffer(max int) *tailBuffer { return &tailBuffer{max: max, lines: make([]string, 0, max)} }

// Add 追加一行并维持环形上限。
func (t *tailBuffer) Add(s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines = append(t.lines, s)
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
}

// String 按写入顺序拼出缓冲内容。

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.Join(t.lines, "\n")
}
