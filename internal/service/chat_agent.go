package service

// 本文件：把 core.Loop 装配成一次 chat run 的执行内核。
//
// 与旧装配的分工：core 只提供「主循环 + 中间件插槽」，本文件负责把业务侧的
// 权限模式、目录信任、人工审批、检查点、委派与用量落库接到插槽上。
// 所有横切关注点都以中间件形式接入，主循环里没有任何 if 分支。

import (
	"context"
	"strings"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/tool"
)

// coreLoopSpec 一次 run 的装配输入。
type coreLoopSpec struct {
	Ses            *domain.ChatSessionDO
	RunID          string
	AssistantMsgID string
	Def            core.Definition
	Provider       llm.Provider
	Model          string
	System         string
	ToolNames      []string
	Sink           core.Sink
	Temperature    *float64
	Thinking       *llm.ThinkingConfig
	MaxTokens      *int
	MaxInput       int
	MaxRunTokens   int
	ExecTimeout    time.Duration
	Hooks          core.Hooks
	// Approver 覆盖默认审批门。默认 coreApprover 阻塞等用户决策（聊天 run）；
	// 后台任务等无人值守 run 注入 taskApprover（只吃免审授权，不弹窗）。
	Approver core.Approver
}

// newCoreLoop 构造一次 run 的内核实例。
func (s *ChatService) newCoreLoop(spec coreLoopSpec) *core.Loop {
	cfg := core.Config{
		Model:        spec.Model,
		System:       spec.System,
		MaxInput:     spec.MaxInput,
		MaxRunTokens: spec.MaxRunTokens,
		Temperature:  spec.Temperature,
		MaxTokens:    spec.MaxTokens,
		Thinking:     spec.Thinking,
		Exec:         core.ExecOptions{Timeout: spec.ExecTimeout},
	}
	spec.Def.Budget.Apply(&cfg)

	loop := core.New(spec.Provider, s.tools.Registry(), cfg).
		WithSink(spec.Sink).
		WithMeta(core.Meta{RunID: spec.RunID, SessionID: spec.Ses.ID, Agent: spec.Def.Name}).
		WithAssistantMessage(spec.AssistantMsgID).
		WithCompressor(core.MicroCompressor{}).
		WithHooks(spec.Hooks).
		Expose(spec.ToolNames).
		WithGuard(s.guardsFor(spec)...).
		// 建流瞬时错误重试：退避前进度透出为 agent.retry 事件，
		// 编排层（service/chat_service）桥接为前端 chat:retry 横幅。
		WithRetry(llm.DefaultRetryPolicy(), func(attempt int, delay time.Duration) {
			spec.Sink.Emit(core.Event{
				Kind: core.EventRetry, RunID: spec.RunID, SessionID: spec.Ses.ID, Agent: spec.Def.Name,
				Payload: core.RetryPayload{Attempt: attempt, DelayMs: delay.Milliseconds()},
			})
		})

	if s.checkpoints != nil {
		loop.WithCheckpoints(s.checkpoints)
	}
	return loop
}

// guardsFor 组装护栏中间件链（顺序即语义，外到内，任一环拒绝即短路）：
// ExposeGuard → SchemaGuard → PolicyGuard → hookGuard → AdaptiveLoopGuard → RepeatGuard。
func (s *ChatService) guardsFor(spec coreLoopSpec) []core.Middleware {
	var exposed map[string]bool
	if len(spec.ToolNames) > 0 {
		exposed = make(map[string]bool, len(spec.ToolNames))
		for _, n := range spec.ToolNames {
			exposed[n] = true
		}
	}

	// 审批门：spec.Approver 优先（后台任务等无人值守 run），否则阻塞等用户决策。
	approver := core.Approver(coreApprover{svc: s.approval})
	if spec.Approver != nil {
		approver = spec.Approver
	}

	ms := []core.Middleware{
		core.ExposeGuard(exposed),
		core.SchemaGuard(),
		// PolicyGuard 现在统一收口安全门：注入/路径/计划/审批四件事。
		// 注入检测无条件生效；路径预检与审批门按需挂入。
		core.PolicyGuard(
			s.coreMode(context.Background(), spec.Ses.PermissionMode),
			allowRules{},
			approver,
			s.pathPolicy(),
		),
	}
	// 用户钩子在策略门之后：内置护栏先裁决，钩子只处理「本机护栏已放行、但用户另有规矩」的场景。
	ms = append(ms, s.hookGuard(spec.Ses, spec.RunID))
	// 失败改道闸门：在 RepeatGuard 之前计数失败次数，达标后给模型注入改道提示；
	// 不影响 RepeatGuard 的同参熔断（两者维度互补：tool-name 维度 vs 同参 key 维度）。
	ms = append(ms, core.AdaptiveLoopGuard(3))
	// 循环与停滞熔断放最后：前面的拒绝（未暴露/越权）不算「重复调用」。
	return append(ms, core.RepeatGuard(3, nil))
}

// hookGuard 用户钩子闸门（子进程协议）：PreToolUse 可放行 / 升级确认 / 拦截，
// PostToolUse 的附加上下文并入回执。钩子自身故障不阻断（内部已兜底放行）。
func (s *ChatService) hookGuard(ses *domain.ChatSessionDO, runID string) core.Middleware {
	if s.hookRunner == nil {
		return func(next core.Handler) core.Handler { return next }
	}
	return func(next core.Handler) core.Handler {
		return func(ctx context.Context, call core.Call) tool.ToolResult {
			pre := s.hookRunner.PreToolUse(ctx, ses.ID, runID, call.Name, call.ID,
				ses.WorkspacePath, ses.PermissionMode, call.Args)
			switch pre.Decision {
			case "deny":
				reason := strings.TrimSpace(pre.Reason)
				if reason == "" {
					reason = "用户钩子拦截了本次调用"
				}
				return refusal(core.RefusePolicy, reason)
			case "ask":
				if s.approval == nil || !s.approval.Approve(ctx, approvalCommand(call), tool.RiskApprovalNeeds) {
					return refusal(core.RefuseApproval, "用户钩子要求人工确认，已被拒绝")
				}
			}

			out := next(ctx, call)
			base := []string{out.Content}
			if post := s.hookRunner.PostToolUse(ctx, ses.ID, runID, call.Name, call.ID,
				ses.WorkspacePath, ses.PermissionMode, call.Args, out.Content); strings.TrimSpace(post) != "" {
				base = append(base, "[hook] "+post)
			}
			if out.Err != nil {
				if extra := s.hookRunner.PostToolUseFailure(ctx, ses.ID, runID, call.Name, call.ID,
					ses.WorkspacePath, ses.PermissionMode, call.Args, out.Err.Error()); strings.TrimSpace(extra) != "" {
					base = append(base, "[hook] "+extra)
				}
			}
			out.Content = strings.Join(base, "\n\n")
			return out
		}
	}
}

// pathPolicy 路径/计划模式预检，返回 (ok, reason)，由 PolicyGuard 在审批前判定。
// 只对目标目录类工具（exec 的 cwd）生效：file_* 已被工作区沙箱约束，再问一次只会噪声。
func (s *ChatService) pathPolicy() core.PathPolicy {
	return func(ctx context.Context, call core.Call) (bool, string) {
		if s.planStore != nil {
			if reason := s.planStore.Guard(core.SessionIDFromCtx(ctx), call.Name); reason != "" {
				return false, "计划模式拒绝: " + reason
			}
		}
		if s.trust == nil {
			return true, ""
		}
		dir, related := extractTargetDir(call.Name, call.Args)
		if !related || strings.TrimSpace(dir) == "" {
			return true, ""
		}
		if allowed, reason := s.trust.Ensure(ctx, dir); !allowed {
			return false, "路径不受信任: " + dir + "（" + reason + "）"
		}
		return true, ""
	}
}

// coreMode 会话权限模式映射。无 restricted 档：它与 default 的差异只在「ask 还是直接 deny」，
// 而 deny 会让模型无法改道，收益低于代价。
func (s *ChatService) coreMode(ctx context.Context, sessionMode string) core.Mode {
	mode := tool.SessionModeDefault
	if v := strings.TrimSpace(sessionMode); v != "" {
		mode = tool.ParseSessionMode(v)
	} else if s.setRepo != nil {
		if row, err := s.setRepo.Get(ctx, domain.SettingKeyAgentSessionMode); err == nil && row != nil {
			mode = tool.ParseSessionMode(row.V)
		}
	}
	switch mode {
	case tool.SessionModeAutoEdit:
		return core.ModeAutoEdit
	case tool.SessionModeYolo:
		return core.ModeYolo
	default:
		return core.ModeDefault
	}
}

// allowRules 显式放行清单（default 模式下本会走 ask 的那些）。
// 放行不等于无护栏：命令级裁决仍在工具内部生效（exec 的白名单与危险正则）。
type allowRules map[string]bool

// Match 实现 core.Rules。
func (r allowRules) Match(name string) (core.Decision, bool) {
	if r[name] || gateAllowSet[name] {
		return core.DecisionAllow, true
	}
	return "", false
}

// gateAllowSet 由 gateInternalAllowTools 派生的集合（含通配写法）。
var gateAllowSet = func() map[string]bool {
	out := make(map[string]bool, len(gateInternalAllowTools))
	for _, n := range gateInternalAllowTools {
		out[n] = true
	}
	return out
}()

// coreApprover 把人工审批门适配到内核接口。
type coreApprover struct{ svc *ApprovalService }

// Approve 阻塞等待用户决策；命令描述用于审批卡片展示与「本会话允许」的免审记忆。
func (a coreApprover) Approve(ctx context.Context, call core.Call, risk string) bool {
	return a.svc.Approve(ctx, approvalCommand(call), risk)
}

// approvalCommand 生成一次调用的可读描述。
// 优先用工具自述（RiskClassifier），退化为「工具名 + 参数」。
func approvalCommand(call core.Call) string {
	if call.Tool != nil {
		if rc, ok := call.Tool.(tool.RiskClassifier); ok {
			if desc, _ := rc.ClassifyArgs(call.Args); desc != "" {
				return desc
			}
		}
	}
	if args := strings.TrimSpace(string(call.Args)); args != "" && args != "{}" {
		return call.Name + " " + args
	}
	return call.Name
}

// refusal 构造结构化拒绝回执；拒绝不是故障，模型可据此改道续跑。
func refusal(reason, hint string) tool.ToolResult {
	if strings.TrimSpace(hint) == "" {
		hint = "已被护栏拒绝"
	}
	return tool.ToolResult{Content: hint, Refused: true, Meta: map[string]string{"refused_reason": reason}}
}
