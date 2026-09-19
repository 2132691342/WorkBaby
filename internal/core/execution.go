package core

import (
	"sort"
	"sync"
	"time"
)

// Scope 一次执行的入口作用域。
// 所有 Agent 轮次必须归属一个 run：chat 每轮登记一个，子 Agent 的 parent_run_id
// 指向其父 run，形成统一执行拓扑。
type Scope string

const (
	ScopeChatTurn Scope = "chat_turn"
	ScopeTask     Scope = "task"
	ScopeToolOnly Scope = "tool_only"
	ScopeDelegate Scope = "delegate"
)

// ExecutionState 执行生命周期状态。
type ExecutionState string

const (
	StatePlanning     ExecutionState = "planning"
	StateRunning      ExecutionState = "running"
	StatePaused       ExecutionState = "paused"
	StateWaitingInput ExecutionState = "waiting_input"
	StateCompleted    ExecutionState = "completed"
	StateFailed       ExecutionState = "failed"
	StateCancelled    ExecutionState = "cancelled"
)

// ExecutionRun 一次执行的登记信息。由调用方铸造 runID（RUN_ 前缀 + ULID），
// 注册到 ExecutionRegistry 后即可被查询/关联。
type ExecutionRun struct {
	RunID       string         `json:"run_id"`
	ParentRunID string         `json:"parent_run_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	AgentName   string         `json:"agent_name,omitempty"`
	Scope       Scope          `json:"scope"`
	State       ExecutionState `json:"state"`
	StartedAt   int64          `json:"started_at"`
	UpdatedAt   int64          `json:"updated_at"`
}

// ExecutionRegistry 单进程执行注册表：登记 / 查询 / 终态 / 活跃列表。
// 桌面单进程语义：进程内可见即可，跨重启不恢复（续跑由 SQL 检查点承担）。
type ExecutionRegistry struct {
	mu    sync.RWMutex
	runs  map[string]*ExecutionRun
	order map[string]int64 // runID → 注册序号（淘汰用确定性依据，避免同毫秒竞态）
	clock int64
	max   int // 最多跟踪的 run 数；超限淘汰最老已终态 run
}

// NewExecutionRegistry 构造注册表。
func NewExecutionRegistry(max int) *ExecutionRegistry {
	if max <= 0 {
		max = 64
	}
	return &ExecutionRegistry{
		runs:  make(map[string]*ExecutionRun, max),
		order: make(map[string]int64, max),
		max:   max,
	}
}

// Register 登记一个已铸造的 run（runID 由调用方生成，因为需要先落库再跑）。
// 重复注册同 runID 覆盖旧记录（幂等）。
func (r *ExecutionRegistry) Register(run *ExecutionRun) {
	if run == nil || run.RunID == "" {
		return
	}
	now := time.Now().UnixMilli()
	run.StartedAt = now
	run.UpdatedAt = now
	if run.State == "" {
		run.State = StateRunning
	}
	if run.Scope == "" {
		run.Scope = ScopeChatTurn
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.runs) >= r.max {
		r.evictLocked()
	}
	if _, exists := r.order[run.RunID]; !exists {
		r.clock++
		r.order[run.RunID] = r.clock
	}
	r.runs[run.RunID] = run
}

// Finish 标记终态；run 不存在则忽略。
func (r *ExecutionRegistry) Finish(runID string, state ExecutionState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run, ok := r.runs[runID]; ok {
		run.State = state
		run.UpdatedAt = time.Now().UnixMilli()
	}
}

// Get 查单个 run。
func (r *ExecutionRegistry) Get(runID string) (*ExecutionRun, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[runID]
	return run, ok
}

// Active 返回仍在运行的 run（最新在前）。
func (r *ExecutionRegistry) Active() []ExecutionRun {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ExecutionRun, 0, len(r.runs))
	for _, run := range r.runs {
		if run.State == StateRunning || run.State == StatePlanning ||
			run.State == StatePaused || run.State == StateWaitingInput {
			out = append(out, *run)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt > out[j].StartedAt })
	return out
}

// evictLocked 超限淘汰：优先淘汰注册最早的已终态 run，否则注册最早的在跑 run。
// 淘汰依据注册序号（逻辑时钟），同毫秒创建也保持确定性。
func (r *ExecutionRegistry) evictLocked() {
	target := ""
	var oldest int64 = int64(1<<63 - 1)
	for id, run := range r.runs {
		if isTerminal(run.State) && r.order[id] < oldest {
			oldest, target = r.order[id], id
		}
	}
	if target == "" {
		for id := range r.runs {
			if r.order[id] < oldest {
				oldest, target = r.order[id], id
			}
		}
	}
	if target != "" {
		delete(r.runs, target)
		delete(r.order, target)
	}
}

// isTerminal 是否已进入终态。
func isTerminal(state ExecutionState) bool {
	return state == StateCompleted || state == StateFailed || state == StateCancelled
}
