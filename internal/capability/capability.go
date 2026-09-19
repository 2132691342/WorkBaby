// Package capability 把各能力域以统一契约接入 Agent。
//
// 一个能力对 Agent 暴露三条通道：Preload（每轮自动注入上下文）/ Tools（模型主动调用）/
// Capture（run 结束后自动沉淀）。注册表按 order 串联 Preload，
// 新增能力只需实现 Capability 并注册，装配方无需改动。
package capability

// capability 包核心抽象。

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// Capability 能力接入契约。
type Capability interface {
	ID() string
	// Preload 每轮 run 前装配上下文；返回空表示本轮不注入。
	Preload(ctx context.Context, c *PreloadCtx) ([]core.Section, error)
	// Tools 暴露给模型的工具；无返回 nil。
	Tools() []tool.Tool
	// Capture run 结束后的自动沉淀；由注册表异步调用。
	Capture(ctx context.Context, c *CaptureCtx) error
}

// RunState 一次 run 的装配运行态；能力间经此交换结果（如激活 Skill 的工具白名单）。
type RunState struct {
	SkillTools []string // 激活 Skill 声明的工具白名单；空 = 不限制
	// Skill 命中详情：SkillName 空表示本轮未命中；装配方据此推 chat:skill 让前端时间线可见。
	SkillName        string
	SkillSource      string // builtin / download / custom
	SkillVersion     string
	SkillDescription string
	SkillInjectedLen int // 注入正文字符数
}

// PreloadCtx 上下文装配输入。
type PreloadCtx struct {
	SessionID string
	RunID     string
	UserInput string
	Session   *domain.ChatSessionDO
	Def       core.Definition
	State     *RunState
}

// CaptureCtx 沉淀输入。
type CaptureCtx struct {
	SessionID  string
	RunID      string
	UserInput  string
	Reply      string
	Transcript []llm.Message
	Def        core.Definition
	// 本 run 使用的 provider / model：终局抽取类能力（收件箱）据此调模型。
	ProviderID string
	Model      string
}

// 注入顺序：环境（含人格）→ 工作区 → 会话变量 → 记忆 → 知识库 → Skill。
const (
	OrderEnvironment = 15
	OrderWorkspace   = 20
	OrderSessionVar  = 28
	OrderMemory      = 30
	OrderKnowledge   = 40
	OrderSkill       = 50
)

type entry struct {
	cap   Capability
	order int
}

// Registry 能力注册表；Preload 按 order 升序串联。
type Registry struct {
	mu   sync.RWMutex
	ents []entry
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry { return &Registry{} }

// Register 注册能力；order 小者先注入。ID 重复返回错误。
func (r *Registry) Register(c Capability, order int) error {
	if c == nil {
		return pkg.New(5021, "能力为空", "")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.ents {
		if e.cap.ID() == c.ID() {
			return pkg.New(5022, "能力重复注册", c.ID())
		}
	}
	r.ents = append(r.ents, entry{cap: c, order: order})
	sort.SliceStable(r.ents, func(i, j int) bool { return r.ents[i].order < r.ents[j].order })
	return nil
}

// All 按注入顺序返回全部能力；nil 注册表返回空。
func (r *Registry) All() []Capability {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, 0, len(r.ents))
	for _, e := range r.ents {
		out = append(out, e.cap)
	}
	return out
}

// Tools 汇总全部能力暴露的工具（按注入顺序）。
func (r *Registry) Tools() []tool.Tool {
	out := make([]tool.Tool, 0)
	for _, c := range r.All() {
		out = append(out, c.Tools()...)
	}
	return out
}

// PreloadAll 串联全部能力的 Preload；单个能力失败仅告警并跳过，不阻断 run。
// nil 注册表返回空并初始化运行态，调用方无需判空。
func (r *Registry) PreloadAll(ctx context.Context, c *PreloadCtx) []core.Section {
	if c.State == nil {
		c.State = &RunState{}
	}
	if r == nil {
		return nil
	}
	r.mu.RLock()
	ents := append([]entry(nil), r.ents...)
	r.mu.RUnlock()

	out := make([]core.Section, 0, 4)
	for _, e := range ents {
		pieces, err := preloadOne(e.cap, ctx, c)
		if err != nil {
			pkg.L.Warn("capability preload failed", "cap", e.cap.ID(), "run", c.RunID, "err", err.Error())
			continue
		}
		// 未显式声明拼接序的段沿用能力的注册 order：注册序即注入序。
		for i := range pieces {
			if pieces[i].Order == 0 {
				pieces[i].Order = e.order
			}
		}
		out = append(out, pieces...)
	}
	return out
}

// preloadOne 执行单个 Preload 并兜住 panic：能力域异常不应拖垮整次 run。
func preloadOne(cp Capability, ctx context.Context, c *PreloadCtx) (p []core.Section, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			p, err = nil, fmt.Errorf("capability panic: %v", rec)
		}
	}()
	return cp.Preload(ctx, c)
}

// CaptureAll 异步沉淀：脱离 run ctx（run 结束时它已取消），每能力独立超时。
// nil 注册表为空操作，调用方无需判空。
func (r *Registry) CaptureAll(c *CaptureCtx, timeout time.Duration) {
	if r == nil || c == nil {
		return
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	for _, cp := range r.All() {
		go func(cp Capability) {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if err := captureOne(cp, ctx, c); err != nil {
				pkg.L.Warn("capability capture failed", "cap", cp.ID(), "session", c.SessionID, "err", err.Error())
			}
		}(cp)
	}
}

func captureOne(cp Capability, ctx context.Context, c *CaptureCtx) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("capability panic: %v", rec)
		}
	}()
	return cp.Capture(ctx, c)
}
