package pkg

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcResult 是一次子进程执行的完整结果；三个输出流分开给，调用方按需取。
type ProcResult struct {
	// Code 退出码；被超时杀掉时是 -1
	Code int
	// Stdout 标准输出全文
	Stdout string
	// Stderr 标准错误全文
	Stderr string
	// TimedOut 是否因超时被终止
	TimedOut bool
}

// ProcOptions 是一次子进程执行的配置。
type ProcOptions struct {
	// Dir 工作目录；空表示继承当前进程
	Dir string
	// Env 完整环境变量；nil 表示继承当前进程
	Env []string
	// TimeoutMs 超时毫秒；超时必杀整棵进程树
	TimeoutMs int
	// OnLine 逐行回调（stdout + stderr 合并），用于把 npm 输出推给界面
	OnLine func(line string)
}

// RunProc 跑一个子进程：argv 数组直传、不进 shell、窗口隐藏、超时杀进程树。
// 本函数的输出始终被缓冲，即使给了 OnLine——两条路径共用一份读循环，避免半行错位。
func RunProc(exe string, args []string, opt ProcOptions) (*ProcResult, error) {
	timeout := opt.TimeoutMs
	if timeout <= 0 {
		timeout = 60_000
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Millisecond)
	defer cancel()

	cmd := exec.Command(exe, args...)
	cmd.Dir = opt.Dir
	if opt.Env != nil {
		cmd.Env = opt.Env
	}
	// windowsHide 的等价物：不闪控制台窗口。
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	var stdout, stderr bytes.Buffer
	var mu sync.Mutex
	outWriter := &lineWriter{buf: &stdout, mu: &mu, onLine: opt.OnLine}
	errWriter := &lineWriter{buf: &stderr, mu: &mu, onLine: opt.OnLine}
	cmd.Stdout = outWriter
	cmd.Stderr = errWriter

	if err := cmd.Start(); err != nil {
		return nil, WrapErr(CodeProvision, "子进程启动失败："+exe, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	result := &ProcResult{}
	select {
	case err := <-done:
		result.Code = exitCode(cmd, err)
	case <-ctx.Done():
		result.TimedOut = true
		KillTree(cmd.Process.Pid)
		<-done
		result.Code = -1
	}
	outWriter.Flush()
	errWriter.Flush()
	mu.Lock()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	mu.Unlock()
	if result.TimedOut {
		return result, Err(CodeProvision, "子进程超时被终止", strings.Join(append([]string{exe}, args...), " "))
	}
	return result, nil
}

// PersistentProc 是一个长驻的行协议子进程：stdin 写请求、stdout 按 LF 收应答。
// MCP 的 stdio 传输与将来的 RPC 子进程共用它——外观纪律与超时必杀不该每个调用方各写一遍。
type PersistentProc struct {
	// cmd 进程对象；Wait 之前必须先写完 stdin
	cmd *exec.Cmd
	// stdin 请求出口
	stdin io.WriteCloser
	// stdout 请求出口
	stdout io.ReadCloser
	// stderr 失败时服务端几乎总在这里说了原因
	stderr io.ReadCloser
	// done 装 Wait 的返回值；缓冲 1，Wait 的 goroutine 写一次不阻塞、不会泄漏
	done chan error
	// exited 是收尾广播：Wait 之后关闭。供 Exited / StderrText / Kill 反复查看，
	// 与 done 分开是因为通道收一次就空了，而这三处都要能重复判断。
	exited chan struct{}
	// killOnce 幂等：重复 Kill 只杀一次树
	killOnce sync.Once
	// closeOnce 幂等：管道只关一次，重复 Kill 不 panic
	closeOnce sync.Once
	// stderrMu 保护 stderrBuf：收 stderr 的 goroutine 与读者会并发
	stderrMu sync.Mutex
	// stderrBuf 错误输出缓冲
	stderrBuf strings.Builder
}

// killSettleBudget 是 Kill 等 Wait 收尾的上限：超过就放行，
// 让「杀不干净」变成可观察的失败，而不是把调用方一起挂死。
const killSettleBudget = 3 * time.Second

// StartPersistent 拉起一个长驻子进程：argv 直传、不进 shell、窗口隐藏。
// 环境给 nil 时继承当前进程；调用方负责用完调 Kill。
func StartPersistent(exe string, args []string, dir string, env []string) (*PersistentProc, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	cmd.SysProcAttr = HiddenProcAttr()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, WrapErr(CodeProvision, "stdin 打开失败："+exe, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, WrapErr(CodeProvision, "stdout 打开失败："+exe, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, WrapErr(CodeProvision, "stderr 打开失败："+exe, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, WrapErr(CodeProvision, "子进程启动失败："+exe, err)
	}
	p := &PersistentProc{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		done:   make(chan error, 1),
		exited: make(chan struct{}),
	}
	// StderrPipe 的读必须发生在 Wait 之前，否则可能死锁；后台收着，失败时翻人话用。
	// 先等 stderr 收完再广播 exited：这样 StderrText 在 exited 之后读到的是完整遗言。
	stderrDrained := make(chan struct{})
	go func() {
		defer close(stderrDrained)
		_, _ = io.Copy(&lockedWriter{buf: &p.stderrBuf, mu: &p.stderrMu}, stderr)
	}()
	// Wait 必须有人调：否则僵尸进程留着，且 exited 永远不关，Kill 只能等到超时。
	go func() {
		err := cmd.Wait()
		<-stderrDrained
		p.done <- err
		close(p.exited)
	}()
	return p, nil
}

// lockedWriter 把写入串到一把锁上：收 stderr 的 goroutine 与 StderrText 并发。
type lockedWriter struct {
	buf *strings.Builder
	mu  *sync.Mutex
}

// Write 加锁后落进缓冲。
func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// Stdin 请求出口。
func (p *PersistentProc) Stdin() io.Writer { return p.stdin }

// Stdout 应答来源。
func (p *PersistentProc) Stdout() io.Reader { return p.stdout }

// WriteLine 写一行请求：JSON 一行 + LF，正是行分隔帧协议要的形状。
func (p *PersistentProc) WriteLine(line string) error {
	if _, err := io.WriteString(p.stdin, line+"\n"); err != nil {
		return WrapErr(CodeMcpServer, "请求写不进服务进程", err)
	}
	return nil
}

// ReadLine 按读循环取一行应答；进程退出或流断开即停止。
func (p *PersistentProc) ReadLine(reader io.Reader) <-chan string {
	return LineChan(reader)
}

// Exited 进程是否已经退出。
func (p *PersistentProc) Exited() bool {
	select {
	case <-p.exited:
		return true
	default:
		return false
	}
}

// StderrText 到目前为止的错误输出（一行）；没有给空串。
// 进程退出后读到的就是完整遗言，还在跑时是已吐出的部分。
func (p *PersistentProc) StderrText() string {
	full := func() string {
		p.stderrMu.Lock()
		defer p.stderrMu.Unlock()
		return strings.TrimSpace(p.stderrBuf.String())
	}
	if p.Exited() {
		return full()
	}
	parts := strings.Split(full(), "\n")
	return strings.TrimSpace(parts[len(parts)-1])
}

// Kill 杀掉整棵进程树并收尾：长驻进程对 stdin 关闭的反应不可控，直接杀树不留孤儿。
// 幂等且必回：已自然退出时 exited 早已关闭，这里立刻返回；
// 等不到收尾也只是放行，绝不死锁。
func (p *PersistentProc) Kill() {
	p.killOnce.Do(func() {
		_ = p.stdin.Close()
		if p.cmd.Process != nil && !p.Exited() {
			KillTree(p.cmd.Process.Pid)
		}
	})
	select {
	case <-p.exited:
	case <-time.After(killSettleBudget):
	}
	p.closeOnce.Do(func() {
		_ = p.stdout.Close()
		_ = p.stderr.Close()
	})
}

// LineChan 把一个字节流按 LF 切成行推进通道；读尽或出错即关闭。
// 不用 bufio.Scanner 默认行为：它会把 U+2028/2029 也当分隔符，而它们在 JSON 里是合法字符。
// 长驻 RPC 子进程与一次性探测共用这一套切帧。
func LineChan(reader io.Reader) <-chan string {
	lines := make(chan string, 32)
	go func() {
		defer close(lines)
		buf := make([]byte, 32*1024)
		rest := ""
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				rest += string(buf[:n])
				for {
					idx := strings.IndexByte(rest, '\n')
					if idx < 0 {
						break
					}
					lines <- strings.TrimSuffix(rest[:idx], "\r")
					rest = rest[idx+1:]
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return lines
}

// exitCode 从 Wait 的返回里取退出码；拿不到给 -1。
func exitCode(cmd *exec.Cmd, err error) int {
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

// HiddenProcAttr 是隐藏窗口 + 无控制台的 SysProcAttr：RunProc 与长驻子进程（RPC / 探测）共用一套外观纪律。
func HiddenProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// KillTree 杀掉整棵进程树：npm 会拉起子进程，只杀父进程会留下一堆孤儿。
// taskkill 自己带超时：它卡住的话，调用方的超时保证会跟着失效——
// 而「超时必杀」是子进程纪律里最要紧的一条。
func KillTree(pid int) {
	if pid <= 0 {
		return
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	kill.SysProcAttr = HiddenProcAttr()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = kill.Run()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
}

// lineWriter 一边把字节攒进缓冲，一边按 LF 回调整行。
type lineWriter struct {
	buf    *bytes.Buffer
	mu     *sync.Mutex
	onLine func(string)
	rest   strings.Builder
}

// Write 实现 io.Writer。
func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if w.onLine == nil {
		return len(p), nil
	}
	// 只按 LF 切：readline 类的实现会把 U+2028 / U+2029 也当分隔符，
	// 而它们在 JSON 字符串里是合法内容，切错会让日志断成两截。
	w.rest.Write(p)
	text := w.rest.String()
	w.rest.Reset()
	for {
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			w.rest.WriteString(text)
			break
		}
		line := strings.TrimSuffix(text[:idx], "\r")
		if strings.TrimSpace(line) != "" {
			w.onLine(line)
		}
		text = text[idx+1:]
	}
	return len(p), nil
}

// Flush 把最后一行没有换行结尾的内容也回调出去。
func (w *lineWriter) Flush() {
	if w.onLine == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.rest.Len() == 0 {
		return
	}
	line := strings.TrimSpace(w.rest.String())
	w.rest.Reset()
	if line != "" {
		w.onLine(line)
	}
}
