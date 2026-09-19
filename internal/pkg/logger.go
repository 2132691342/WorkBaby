// 文件：结构化日志——log/slog 封装，按等级分流落文件 + 按大小轮转。

package pkg

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// LogFileSize 单文件最大字节（10 MiB）；超出后轮转。
const LogFileSize = 10 * 1024 * 1024

// LogKeepCount 保留的旧日志份数（含当前）。
const LogKeepCount = 5

// L 全局日志入口，只用 Info / Warn / Error 三个等级：Info 生命周期、Warn 可自愈异常、
// Error 需人介入的失败。Init 之前调用退化为 stderr。
var L *slog.Logger = fallback()

// Init 初始化日志：workbaby.log 全量、workbaby-warn.log 仅 warn、workbaby-error.log 仅 error；
// 控制台只输出 warn 及以上。
func Init(logDir string) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return Wrap(1011, "create log dir failed", err)
	}
	main, err := openRotating(filepath.Join(logDir, "workbaby.log"))
	if err != nil {
		return Wrap(1012, "open log file failed", err)
	}
	warn, err := openRotating(filepath.Join(logDir, "workbaby-warn.log"))
	if err != nil {
		return Wrap(1012, "open warn log file failed", err)
	}
	errFile, err := openRotating(filepath.Join(logDir, "workbaby-error.log"))
	if err != nil {
		return Wrap(1012, "open error log file failed", err)
	}
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	L = slog.New(&levelSplit{
		main:    slog.NewJSONHandler(main, opts),
		warn:    slog.NewJSONHandler(warn, opts),
		err:     slog.NewJSONHandler(errFile, opts),
		console: slog.NewTextHandler(os.Stderr, opts),
	})
	return nil
}

// levelSplit 按等级分流：全量落主文件，warn/error 另落独立文件，warn 以上同步控制台。
type levelSplit struct {
	main    slog.Handler
	warn    slog.Handler
	err     slog.Handler
	console slog.Handler
}

// Enabled 屏蔽 Debug 及以下（本项目只用三个等级）。
func (h *levelSplit) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo && h.main.Enabled(ctx, level)
}

// Handle 主线写完再写旁路；旁路失败不影响主链路（日志不得反向阻断业务）。
func (h *levelSplit) Handle(ctx context.Context, r slog.Record) error {
	if err := h.main.Handle(ctx, r); err != nil {
		return err
	}
	if r.Level >= slog.LevelError {
		_ = h.err.Handle(ctx, r)
	} else if r.Level >= slog.LevelWarn {
		_ = h.warn.Handle(ctx, r)
	}
	if r.Level >= slog.LevelWarn {
		_ = h.console.Handle(ctx, r)
	}
	return nil
}

// WithAttrs 四个子处理器同步附加属性。
func (h *levelSplit) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &levelSplit{
		main:    h.main.WithAttrs(attrs),
		warn:    h.warn.WithAttrs(attrs),
		err:     h.err.WithAttrs(attrs),
		console: h.console.WithAttrs(attrs),
	}
}

// WithGroup 四个子处理器同步分组。
func (h *levelSplit) WithGroup(name string) slog.Handler {
	return &levelSplit{
		main:    h.main.WithGroup(name),
		warn:    h.warn.WithGroup(name),
		err:     h.err.WithGroup(name),
		console: h.console.WithGroup(name),
	}
}

// fallback Init 之前的临时 logger：仅 stderr，保证早期失败仍有输出。
func fallback() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

// openRotating 打开日志文件（按大小轮转）。
func openRotating(path string) (*rotatingFile, error) {
	return newRotating(path, LogFileSize, LogKeepCount)
}

// rotatingFile 按尺寸轮转的写入目标；内部持锁，可并发写。
type rotatingFile struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	keep    int
	f       *os.File
	size    int64
}

// newRotating 追加模式打开，保留旧引用。
func newRotating(path string, max int64, keep int) (*rotatingFile, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	st, _ := f.Stat()
	return &rotatingFile{path: path, maxSize: max, keep: keep, f: f, size: st.Size()}, nil
}

// Write 写入并在超过阈值时轮转；单条 slog 行不会超过 maxSize。
func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := r.f.Write(p)
	r.size += int64(n)
	if r.size >= r.maxSize {
		r.rotate()
	}
	return n, err
}

// rotate 把 .1 .2 ... 顺次后移；当前文件重命名为 .1。
func (r *rotatingFile) rotate() {
	_ = r.f.Close()
	for i := r.keep - 1; i >= 1; i-- {
		_ = os.Rename(r.path+"."+strconv.Itoa(i), r.path+"."+strconv.Itoa(i+1))
	}
	_ = os.Rename(r.path, r.path+".1")
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		r.f = f
		r.size = 0
	}
}
