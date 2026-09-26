package agent

import "sync"

// MapSteps 步骤记忆：已完成（成功）的工具调用按 `name|args` 记结果。
// Load 只命中检查点恢复的记录——续跑命中即复用，不重放副作用；
// 本 run 内新记的结果仅随检查点持久化（Snapshot），不在本 run 内复用，
// 避免「写后重读」拿到旧值。跨进程经检查点的 Steps 字段承载。
//
// 与 LoopGuard 的区别：LoopGuard 记「重复调用」用于熔断，这里记「已完成结果」用于复用。
type MapSteps struct {
	mu       sync.Mutex
	m        map[string]string
	restored map[string]bool // 检查点恢复的键：Load 的唯一命中来源
}

// NewMapSteps 构造；init 为检查点恢复的既有记录（可为 nil），恢复键可复用。
func NewMapSteps(init map[string]string) *MapSteps {
	s := &MapSteps{
		m:        make(map[string]string, len(init)),
		restored: make(map[string]bool, len(init)),
	}
	for k, v := range init {
		s.m[k] = v
		s.restored[k] = true
	}
	return s
}

// Load 取可复用的已完成调用结果（仅检查点恢复的键）。
func (s *MapSteps) Load(key string) (string, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.restored[key] {
		return "", false
	}
	v, ok := s.m[key]
	return v, ok
}

// Store 记录一次成功调用的结果：仅随检查点持久化，本 run 内不复用。
func (s *MapSteps) Store(key, content string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = content
}

// Snapshot 导出全量快照：写入检查点，供跨进程续跑复用。
func (s *MapSteps) Snapshot() map[string]string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.m))
	for k, v := range s.m {
		out[k] = v
	}
	return out
}
