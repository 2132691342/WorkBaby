package pkg

import (
	"bytes"
	"context"
	"os"
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
		return nil, WrapErr(CodeRuntimeDown, "子进程启动失败："+exe, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	result := &ProcResult{}
	select {
	case err := <-done:
		result.Code = exitCode(cmd, err)
	case <-ctx.Done():
		result.TimedOut = true
		killTree(cmd.Process.Pid)
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
		return result, Err(CodeRuntimeDown, "子进程超时被终止", strings.Join(append([]string{exe}, args...), " "))
	}
	return result, nil
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

// killTree 杀掉整棵进程树：npm 会拉起子进程，只杀父进程会留下一堆孤儿。
func killTree(pid int) {
	if pid <= 0 {
		return
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	_ = kill.Run()
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
	w.buf.Write(p)
	w.mu.Unlock()
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
	if w.onLine == nil || w.rest.Len() == 0 {
		return
	}
	line := strings.TrimSpace(w.rest.String())
	w.rest.Reset()
	if line != "" {
		w.onLine(line)
	}
}

// CurrentEnv 复制当前进程的环境变量，供需要增删若干项的调用方改造。
func CurrentEnv() []string {
	return append([]string(nil), os.Environ()...)
}
