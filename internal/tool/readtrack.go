package tool

// 写前必须读：记录本轮 run 内 file_read 过的文件，file_edit 前校验，避免凭记忆改文件把内容改坏。
// 作用域是 run（下一轮不继承读态）；无 run 身份时不设限。

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

// maxTrackedReads 读态条数上限：超限整体清空。
// 最坏后果是模型多读一次文件，换来的是进程常驻内存在长会话里不无限增长。
const maxTrackedReads = 4096

var (
	readMu    sync.Mutex
	readFiles = map[string]struct{}{}
)

// MarkFileRead 记录本轮 run 已读取该文件（绝对路径）。
func MarkFileRead(ctx context.Context, absPath string) {
	key := readTrackKey(ctx, absPath)
	if key == "" {
		return
	}
	readMu.Lock()
	defer readMu.Unlock()
	if len(readFiles) >= maxTrackedReads {
		readFiles = map[string]struct{}{}
	}
	readFiles[key] = struct{}{}
}

// FileWasRead 本轮 run 是否读过该文件；无 run 身份返回 true（不设限）。
func FileWasRead(ctx context.Context, absPath string) bool {
	key := readTrackKey(ctx, absPath)
	if key == "" {
		return true
	}
	readMu.Lock()
	defer readMu.Unlock()
	_, ok := readFiles[key]
	return ok
}

// readTrackKey 读态键；无身份 / 空路径返回空串（调用方按「不设限」处理）。
func readTrackKey(ctx context.Context, absPath string) string {
	runID := RunIDFromCtx(ctx)
	if runID == "" || absPath == "" {
		return ""
	}
	// Windows 路径大小写不敏感：统一小写避免同一文件两种写法各记一次
	return runID + "|" + strings.ToLower(filepath.Clean(absPath))
}
