package chat

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/pkg"
)

// Record 是一条引擎记录（会话事件与 UI 请求共用）。
type Record map[string]any

// Handlers 是 RPC 的三个出口。
type Handlers struct {
	// OnEvent 会话事件（message_update / tool_execution_* / turn_end ...）
	OnEvent func(rec Record)
	// OnUiRequest 引擎推上来的 UI 交互请求
	OnUiRequest func(rec Record)
	// OnExit 子进程退出
	OnExit func(code int)
}

// PiRpc 是一个工作区一个的引擎子进程。
// 请求按 id 关联应答，事件原样上抛；stdout 严格按 LF 切帧。
type PiRpc struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	pending map[string]*pendingCall
	seq     int
	rest    string
	exited  bool
	stopOne sync.Once

	cwd     string
	env     []string
	entry   string
	nodeExe string
	handler Handlers

	// StartedAt 子进程启动时刻（毫秒）
	StartedAt int64
	// CallCount 已发出的命令数
	CallCount int
	// LastError 最近一次失败原因
	LastError string
}

type pendingCall struct {
	command string
	done    chan struct{}
	data    any
	err     error
}

// requestTimeout 是单条命令的等待上限：压缩一轮长上下文可能要好几分钟。
const requestTimeout = 10 * time.Minute

// NewPiRpc 造一个未启动的 RPC 客户端。
func NewPiRpc(nodeExe, entry, cwd string, env []string, h Handlers) *PiRpc {
	return &PiRpc{
		pending: map[string]*pendingCall{},
		cwd:     cwd,
		env:     env,
		entry:   entry,
		nodeExe: nodeExe,
		handler: h,
	}
}

// Start 拉起子进程并开始泵 stdout。
func (r *PiRpc) Start() error {
	if !pkg.Exists(r.entry) {
		return pkg.Err(pkg.CodeRpcDown, "pi 尚未安装，请先在应用页安装")
	}
	cmd := exec.Command(r.nodeExe, r.entry, "--mode", "rpc")
	cmd.Dir = r.cwd
	cmd.Env = r.env
	// windowsHide + CREATE_NO_WINDOW：不闪控制台。
	cmd.SysProcAttr = pkg.HiddenProcAttr()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return pkg.WrapErr(pkg.CodeRpcDown, "引擎 stdin 打开失败", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return pkg.WrapErr(pkg.CodeRpcDown, "引擎 stdout 打开失败", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return pkg.WrapErr(pkg.CodeRpcDown, "引擎 stderr 打开失败", err)
	}
	if err := cmd.Start(); err != nil {
		return pkg.WrapErr(pkg.CodeRpcDown, "引擎启动失败", err)
	}
	r.mu.Lock()
	r.cmd = cmd
	r.stdin = stdin
	r.StartedAt = pkg.NowMs()
	r.mu.Unlock()

	go r.pumpStdout(stdout)
	go r.pumpStderr(stderr)
	go func() {
		waitErr := cmd.Wait()
		code := -1
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		} else if waitErr == nil {
			code = 0
		}
		r.mu.Lock()
		r.exited = true
		r.mu.Unlock()
		r.failAll(pkg.CodeRpcDown, "pi 子进程已退出")
		if r.handler.OnExit != nil {
			r.handler.OnExit(code)
		}
	}()
	return nil
}

// Alive 子进程是否还活着。
func (r *PiRpc) Alive() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cmd != nil && !r.exited
}

// Pid 子进程 PID；没起给 0。
func (r *PiRpc) Pid() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil || r.cmd.Process == nil {
		return 0
	}
	return r.cmd.Process.Pid
}

// pumpStdout 读 stdout 并按 LF 切帧。不用 bufio.Scanner：
// 它按行切的同时会撞上 U+2028 / U+2029，而那两个字符在 JSON 字符串里合法。
func (r *PiRpc) pumpStdout(reader io.Reader) {
	buf := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			r.consume(string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

// pumpStderr 读 stderr 只记日志：它不是协议数据，解析它等于给自己造 bug。
func (r *PiRpc) pumpStderr(reader io.Reader) {
	scanner := bufio.NewReader(reader)
	for {
		line, err := scanner.ReadString('\n')
		if text := strings.TrimSpace(line); text != "" {
			pkg.Warn("pi: %s", pkg.Truncate(text, 300))
		}
		if err != nil {
			return
		}
	}
}

// consume 把 chunk 攒进缓冲，凑够一整行就派发。
func (r *PiRpc) consume(chunk string) {
	r.mu.Lock()
	r.rest += chunk
	var lines []string
	for {
		idx := strings.IndexByte(r.rest, '\n')
		if idx < 0 {
			break
		}
		lines = append(lines, strings.TrimSuffix(r.rest[:idx], "\r"))
		r.rest = r.rest[idx+1:]
	}
	r.mu.Unlock()
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			r.dispatch(line)
		}
	}
}

// dispatch 分派一条记录：应答 / UI 请求 / 会话事件。
func (r *PiRpc) dispatch(line string) {
	var rec Record
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		pkg.Warn("pi 非 JSON 输出：%s", pkg.Truncate(line, 200))
		return
	}
	kind, _ := rec["type"].(string)
	switch kind {
	case "response":
		id, _ := rec["id"].(string)
		r.mu.Lock()
		call, ok := r.pending[id]
		delete(r.pending, id)
		r.mu.Unlock()
		if !ok {
			return
		}
		if success, _ := rec["success"].(bool); !success {
			msg, _ := rec["error"].(string)
			if msg == "" {
				msg = "pi 命令失败"
			}
			call.err = pkg.Err(pkg.CodeRpcCommand, msg)
		} else {
			call.data = rec["data"]
		}
		close(call.done)
	case "extension_ui_request":
		if r.handler.OnUiRequest != nil {
			r.handler.OnUiRequest(rec)
		}
	default:
		if r.handler.OnEvent != nil {
			r.handler.OnEvent(rec)
		}
	}
}

// Request 发一条命令并等它的应答。超时返回 9702。
func (r *PiRpc) Request(command Record, timeout time.Duration) (any, error) {
	if !r.Alive() {
		return nil, pkg.Err(pkg.CodeRpcDown, "pi 未在运行")
	}
	if timeout <= 0 {
		timeout = requestTimeout
	}
	r.mu.Lock()
	r.seq++
	id := "wb-" + strconv.Itoa(r.seq)
	call := &pendingCall{command: stringOf(command["type"]), done: make(chan struct{})}
	r.pending[id] = call
	r.CallCount++
	r.mu.Unlock()

	wire := Record{"id": id}
	for k, v := range command {
		wire[k] = v
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		r.removePending(id)
		return nil, pkg.WrapErr(pkg.CodeRpcCommand, "命令序列化失败", err)
	}
	if err := r.writeLine(string(payload)); err != nil {
		r.removePending(id)
		return nil, err
	}
	select {
	case <-call.done:
		return call.data, call.err
	case <-time.After(timeout):
		r.removePending(id)
		return nil, pkg.Err(pkg.CodeRpcTimeout, "pi 命令超时："+call.command)
	}
}

// Send 发一条无需应答的记录（extension_ui_response 用）。
func (r *PiRpc) Send(rec Record) error {
	payload, err := json.Marshal(rec)
	if err != nil {
		return pkg.WrapErr(pkg.CodeRpcCommand, "应答序列化失败", err)
	}
	return r.writeLine(string(payload))
}

// writeLine 写一行 JSONL；写失败即视为连接已断。
func (r *PiRpc) writeLine(line string) error {
	r.mu.Lock()
	stdin := r.stdin
	alive := r.cmd != nil && !r.exited
	r.mu.Unlock()
	if !alive || stdin == nil {
		return pkg.Err(pkg.CodeRpcDown, "pi 未在运行")
	}
	if _, err := io.WriteString(stdin, line+"\n"); err != nil {
		return pkg.WrapErr(pkg.CodeRpcDown, "向 pi 写命令失败", err)
	}
	return nil
}

// Stop 关停：关 stdin 触发引擎优雅退出，等它真退了才返回（上限 3 秒，到点强杀）。
// 调用方接下来往往要删包目录——Windows 上进程还活着就删不了，睡 200ms 赌运气不够。
func (r *PiRpc) Stop() {
	r.stopOne.Do(func() {
		r.failAll(pkg.CodeRpcDown, "pi 正在关停")
		r.mu.Lock()
		stdin, cmd := r.stdin, r.cmd
		r.mu.Unlock()
		if stdin != nil {
			_ = stdin.Close()
		}
		if cmd == nil || cmd.Process == nil {
			return
		}
		pid := cmd.Process.Pid
		time.AfterFunc(5*time.Second, func() {
			if r.Alive() {
				pkg.KillTree(pid)
			}
		})
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !r.Alive() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// removePending 摘掉一条等待中的请求。
func (r *PiRpc) removePending(id string) {
	r.mu.Lock()
	delete(r.pending, id)
	r.mu.Unlock()
}

// failAll 把所有等待中的请求一次性拒掉：进程已经没了，等下去只会更久。
func (r *PiRpc) failAll(code, message string) {
	r.mu.Lock()
	waiting := r.pending
	r.pending = map[string]*pendingCall{}
	r.mu.Unlock()
	for _, call := range waiting {
		call.err = pkg.Err(code, message)
		close(call.done)
	}
}
