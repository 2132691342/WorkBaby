package pkg

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	maxLogBytes = 8 << 20 // 单文件 8MB 后轮转
	keepLogs    = 3
)

// Logger 持有两个落盘目标：全量日志与 warn/error 专档。
type Logger struct {
	dir     string
	all     *rotateFile
	problem *rotateFile
}

var (
	logMu   sync.Mutex
	current *Logger
)

// InitLog 初始化全局 slog：控制台只出 info，warn 与 error 额外落专档。
func InitLog(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, Wrap(1001, "创建日志目录失败", err)
	}
	all, err := newRotateFile(filepath.Join(dir, "app.log"))
	if err != nil {
		return nil, err
	}
	problem, err := newRotateFile(filepath.Join(dir, "warn.log"))
	if err != nil {
		_ = all.Close()
		return nil, err
	}

	l := &Logger{dir: dir, all: all, problem: problem}
	level := &slog.LevelVar{}
	level.Set(slog.LevelInfo)

	slog.SetDefault(slog.New(slog.NewTextHandler(all, &slog.HandlerOptions{Level: level})))
	logMu.Lock()
	current = l
	logMu.Unlock()
	return l, nil
}

// Infof 记一条 info。启动链路的关键节点必须走这里：
// 走 stderr 的日志在 GUI 程序里没人看得见，排查等于盲猜。
func Infof(format string, args ...any) {
	slog.Info(fmtSprintf(format, args...))
}

// Warnf 记一条 warn，同时进全量与专档。
func Warnf(format string, args ...any) {
	slog.Warn(fmtSprintf(format, args...))
	appendProblem("WARN", format, args...)
}

// Errorf 记一条 error，同时进全量与专档。
func Errorf(format string, args ...any) {
	slog.Error(fmtSprintf(format, args...))
	appendProblem("ERROR", format, args...)
}

func appendProblem(level, format string, args ...any) {
	logMu.Lock()
	l := current
	logMu.Unlock()
	if l == nil || l.problem == nil {
		return
	}
	line := level + " " + fmtSprintf(format, args...) + "\n"
	_, _ = l.problem.Write([]byte(line))
}

// Close 关闭日志文件句柄。
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	_ = l.all.Close()
	return l.problem.Close()
}

// Dir 返回日志目录。
func (l *Logger) Dir() string { return l.dir }

// rotateFile 是带大小轮转的写入目标；轮转在同一次 Write 内完成，避免并发重命名竞态。
type rotateFile struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
}

func newRotateFile(path string) (*rotateFile, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, Wrap(1002, "打开日志文件失败", err)
	}
	st, _ := f.Stat()
	return &rotateFile{path: path, f: f, size: st.Size()}, nil
}

func (r *rotateFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size > maxLogBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotateFile) rotate() error {
	_ = r.f.Close()
	_ = os.Remove(r.path + "." + itoa(keepLogs))
	for i := keepLogs - 1; i >= 1; i-- {
		_ = os.Rename(r.path+"."+itoa(i), r.path+"."+itoa(i+1))
	}
	_ = os.Rename(r.path, r.path+".1")
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return Wrap(1002, "轮转后重开日志失败", err)
	}
	r.f = f
	r.size = 0
	return nil
}

func (r *rotateFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}
