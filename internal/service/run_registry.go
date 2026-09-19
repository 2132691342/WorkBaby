package service

import (
	"context"
	"sync"
)

// runRegistry 记录当前活动的 run（按 sessionID），提供取消能力。
// 同 sessionID 重复注册会取消旧 ctx 防泄漏；取消后立即移除，重复取消幂等。
type runRegistry struct {
	mu   sync.Mutex
	runs map[string]runEntry
}

// runEntry 活动 run：cancel 函数 + runID（注入/事件路由需要）。
type runEntry struct {
	runID  string
	cancel context.CancelFunc
}

func newRunRegistry() *runRegistry { return &runRegistry{runs: map[string]runEntry{}} }

// set 注册 sessionID 的 run；覆盖前会 cancel 旧 ctx（兜底防泄漏）。
func (r *runRegistry) set(sessionID, runID string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev, ok := r.runs[sessionID]; ok {
		prev.cancel()
	}
	r.runs[sessionID] = runEntry{runID: runID, cancel: cancel}
}

// delete 注销（run 自然结束时调用）。
func (r *runRegistry) delete(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.runs, sessionID)
}

// deleteIf 仅当当前注册的 runID 与期望一致时才注销。
// 目标模式自动续跑会在旧 run 尾部直接拉起新 run：旧 goroutine 的 defer
// 若无条件 delete，会把新 run 的取消句柄一起删掉（停止按钮失灵）。
func (r *runRegistry) deleteIf(sessionID, runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.runs[sessionID]; ok && e.runID == runID {
		delete(r.runs, sessionID)
	}
}

// lookup 返回该会话当前活动 run 的 runID；无活动 run 返回 false。
func (r *runRegistry) lookup(sessionID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.runs[sessionID]
	if !ok || e.runID == "" {
		return "", false
	}
	return e.runID, true
}

// cancel 取消指定 session 的 run；返回是否找到。找不到（已结束/不存在）不报错。
func (r *runRegistry) cancel(sessionID string) bool {
	r.mu.Lock()
	e, ok := r.runs[sessionID]
	if ok {
		delete(r.runs, sessionID)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	e.cancel()
	return true
}
