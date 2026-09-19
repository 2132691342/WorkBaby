package service

// 本文件：子 Agent 委派（core.Loop 实现）。
//
// 四重隔离：子 run 只看到「人设 + 任务」、独立预算、工具集只能收缩不能升权、
// 正文流不进父回答（只回传摘要）。并发相同 (agent, task) 共享一次执行。

import (
	"context"
	"strings"
	"sync"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// 委派预算：父 run 的资源不能被单个委派吃光。
const (
	delegateMaxTurns    = 12
	delegateWallTime    = 3 * time.Minute
	delegateToolTimeout = 2 * time.Minute
	delegateSummaryMax  = 4000 // 摘要回传上限（rune）
)

// delegateFlight 同参委派的去重句柄。
type delegateFlight struct {
	done    chan struct{}
	summary string
	err     error
}

// coreDelegator 用 core.Loop 跑子任务，只把摘要回传给父 Agent。
type coreDelegator struct {
	svc         *ChatService
	ses         *domain.ChatSessionDO
	parentRunID string
	parentSink  core.Sink // 父 run 的事件出口：子事件经此转发（带 Agent 标签）
	model       string
	toolNames   []string

	mu      sync.Mutex
	flights map[string]*delegateFlight
}

// newCoreDelegator 构造委派器。
func (s *ChatService) newCoreDelegator(ses *domain.ChatSessionDO, parentRunID string,
	parentSink core.Sink, model string, toolNames []string) *coreDelegator {
	return &coreDelegator{
		svc: s, ses: ses, parentRunID: parentRunID, parentSink: parentSink,
		model: model, toolNames: toolNames, flights: map[string]*delegateFlight{},
	}
}

// Delegate 实现 tool.Delegator。
func (d *coreDelegator) Delegate(ctx context.Context, agentName, task string) (string, error) {
	if d.svc.reg == nil {
		return "", pkg.New(5008, "委派不可用：当前运行环境未配置模型", "")
	}
	task = strings.TrimSpace(task)
	if task == "" {
		return "", pkg.New(5008, "委派失败：任务描述为空", "")
	}
	def := core.Agent(agentName) // 未知名回退 default

	// 并发同参去重：命中在飞委派则等待其结果，不重复扇出。
	key := def.Name + "|" + task
	d.mu.Lock()
	if f, ok := d.flights[key]; ok {
		d.mu.Unlock()
		select {
		case <-f.done:
			return f.summary, f.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	flight := &delegateFlight{done: make(chan struct{})}
	d.flights[key] = flight
	d.mu.Unlock()
	defer func() {
		close(flight.done)
		d.mu.Lock()
		delete(d.flights, key)
		d.mu.Unlock()
	}()

	childRunID := pkg.NewID("RUN")
	if d.svc.execs != nil {
		d.svc.execs.Register(&core.ExecutionRun{
			RunID:       childRunID,
			ParentRunID: d.parentRunID,
			SessionID:   d.ses.ID,
			AgentName:   def.Name,
			Scope:       core.ScopeDelegate,
			State:       core.StateRunning,
		})
		defer d.svc.execs.Finish(childRunID, core.StateCompleted)
	}

	prov, err := d.svc.reg.Get(d.ses.ProviderID)
	if err != nil {
		flight.err = pkg.Wrap(5008, "子任务 Provider 不可用", err)
		return "", flight.err
	}

	// 子 run 事件只转发工具层与生命周期：正文/思考不转发，避免混进父回答。
	sink := core.FuncSink(func(e core.Event) {
		switch e.Kind {
		case core.EventToolCall, core.EventToolStart, core.EventToolResult,
			core.EventError, core.EventRunStart, core.EventRunDone:
			e.ParentRunID = d.parentRunID
			e.Agent = def.Name
			d.parentSink.Emit(e)
		}
	})

	// 工具集只能收缩：在父 run 已暴露的工具里按子 Agent 策略再过滤。
	childTools := filterToolNames(d.toolNames, def)

	childModel := def.EffectiveModel(d.model)
	cfg := core.Config{
		Model:    childModel,
		System:   def.Persona,
		MaxTurns: delegateMaxTurns,
		Exec:     core.ExecOptions{Timeout: delegateToolTimeout},
	}
	// 子 run 上下文天然短小，无需压缩。
	loop := core.New(prov, d.svc.tools.Registry(), cfg).
		WithSink(sink).
		WithMeta(core.Meta{RunID: childRunID, SessionID: d.ses.ID, ParentRunID: d.parentRunID, Agent: def.Name}).
		Expose(childTools).
		WithGuard(d.svc.guardsFor(coreLoopSpec{Ses: d.ses, RunID: childRunID, Def: def})...)

	childCtx, cancel := context.WithTimeout(core.WithRunContext(ctx, childRunID, d.ses.ID), delegateWallTime)
	defer cancel()

	out, runErr := loop.Run(childCtx, []*llm.Message{llm.UserMessage(task)})
	if runErr != nil {
		flight.err = pkg.Wrap(5008, "子任务执行失败", runErr)
		return "", flight.err
	}
	// 子 run 用量单独落库：委派可达十几轮 + 几十次工具调用，
	// 不落库会让 token_usages 系统性漏计整个委派。Turn 记 -1 与父对话轮次区分。
	if d.svc.usages != nil {
		for _, t := range out.PerTurn {
			d.svc.persistUsageRowFrom(ctx, d.ses, d.parentRunID, "", -1,
				domain.UsageSourceDelegate, def.Name, childModel, t.Usage)
		}
	}
	flight.summary = pkg.TruncateRunes(strings.TrimSpace(out.Content), delegateSummaryMax)
	return flight.summary, nil
}

// filterToolNames 子 Agent 工具集：父 run 已暴露的工具 ∩ 子 Agent 策略。
// 只能收缩不能升权——子 Agent 拿不到父 run 没暴露的工具。
func filterToolNames(parent []string, def core.Definition) []string {
	if len(def.Tools.Allow) == 0 && len(def.Tools.Deny) == 0 && def.Tools.MaxTools <= 0 {
		return parent
	}
	defs := make([]llm.ToolDefinition, 0, len(parent))
	for _, n := range parent {
		defs = append(defs, llm.ToolDefinition{Name: n})
	}
	filtered := def.FilterTools(defs)
	out := make([]string, 0, len(filtered))
	for _, d := range filtered {
		out = append(out, d.Name)
	}
	return out
}

// ensure tool 包被引用：Delegator 接口的编译期断言。
var _ tool.Delegator = (*coreDelegator)(nil)
