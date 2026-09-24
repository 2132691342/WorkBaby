package service

// 本文件：一次 run 的「运行前装配」步骤集合（技能目录 / Agent 覆盖 / 用户钩子 /
// 历史 / system 拼装 / 工具暴露 / 采样与预算）。executeAgent 只负责按序调用并运行内核。

import (
	"context"
	"strings"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// syncWorkspaceSkills 按会话工作区叠加技能目录（未变化时零开销）；失败不阻断 run。
func (s *ChatService) syncWorkspaceSkills(ctx context.Context, ses *domain.ChatSessionDO) {
	if s.skillSync == nil {
		return
	}
	if err := s.skillSync(ctx, ses.WorkspacePath); err != nil {
		pkg.L.Warn("sync workspace skills failed", "sessionID", ses.ID, "err", err.Error())
	}
}

// applyAgentOverrides 应用 Agent 定义的运行期覆盖，返回实际使用的模型与采样参数。
// 模型：Agent 定义优先于会话模型（优先级差必须让用户看见）；推理强度：请求级优先。
func (s *ChatService) applyAgentOverrides(ctx context.Context, ses *domain.ChatSessionDO, runID string, def core.Definition, params core.RequestParams) (string, core.RequestParams) {
	runModel := def.EffectiveModel(ses.Model)
	if runModel != ses.Model {
		pkg.L.Info("agent overrides session model",
			"sessionID", ses.ID, "agent", def.Name, "sessionModel", ses.Model, "model", runModel)
		s.emit(runID, ses.ID, "chat:warn", domain.ChatWarnEvent{
			Kind:         "agent_model_override",
			Agent:        def.Name,
			SessionModel: ses.Model,
			Model:        runModel,
			Message:      "本次运行由子智能体定义指定了模型 " + runModel + "（会话模型 " + ses.Model + " 已被覆盖）",
		})
	}
	if params.Thinking == nil && def.Thinking != "" {
		params.Thinking = llm.ThinkingFromEffort(def.Thinking)
	}
	return runModel, params
}

// promptHookContext 触发 UserPromptSubmit 用户钩子：返回可并入 system 的补充上下文；
// 被钩子阻断时返回错误。续跑不重复触发——该事件属于「用户提交」这一次动作。
func (s *ChatService) promptHookContext(ctx context.Context, ses *domain.ChatSessionDO, runID, userInput string, resume bool) (string, error) {
	if s.hookRunner == nil || resume {
		return "", nil
	}
	blocked, reason, extra := s.hookRunner.UserPromptSubmit(ctx, ses.ID, runID, userInput, ses.WorkspacePath, ses.PermissionMode)
	if blocked {
		return "", pkg.New(8610, "请求被用户钩子阻断", reason)
	}
	return extra, nil
}

// loadRunHistory 拉会话历史并转成 LLM 消息（含工具调用上下文与图片附件）。
// 辅助对话（侧链路）把主会话历史按预算前置拼接，追问不必重复交代背景。
func (s *ChatService) loadRunHistory(ctx context.Context, ses *domain.ChatSessionDO) ([]*llm.Message, error) {
	hists, err := s.messages.ListBySession(ctx, ses.ID, 0, 0)
	if err != nil {
		return nil, err
	}
	vision := s.providerVision(ctx, ses.ProviderID)
	llmMsgs, err := s.toLLMMessagesWithVision(ctx, hists, vision)
	if err != nil {
		return nil, err
	}
	parentMsgs, err := s.sideParentMessages(ctx, ses, vision)
	if err != nil {
		return nil, err
	}
	if len(parentMsgs) > 0 {
		llmMsgs = append(parentMsgs, llmMsgs...)
		pkg.L.Debug("side parent prefix", "sessionID", ses.ID, "prefixMsgs", len(parentMsgs))
	}
	return llmMsgs, nil
}

// systemWithHookContext 把用户钩子的补充段并入 system 尾部（SessionStart 仅首轮 + UserPromptSubmit），
// 返回最终 system 正文。追加而非新开 system 消息：多 system 段在部分上游被拒。
func (s *ChatService) systemWithHookContext(ctx context.Context, ses *domain.ChatSessionDO, runID string, sys *llm.Message, promptCtx string, resume bool) string {
	if s.hookRunner != nil {
		var parts []string
		if !resume {
			if txt := s.hookRunner.SessionStart(ctx, ses.ID, runID, "startup", ses.WorkspacePath, ses.PermissionMode); txt != "" {
				parts = append(parts, txt)
			}
		}
		if promptCtx != "" {
			parts = append(parts, promptCtx)
		}
		if len(parts) > 0 {
			extra := "## 用户钩子上下文\n\n" + strings.Join(parts, "\n\n")
			if sys != nil {
				sys.Content = sys.Content + "\n\n" + extra
			} else {
				sys = &llm.Message{Role: llm.RoleSystem, Content: extra}
			}
		}
	}
	if sys == nil {
		return ""
	}
	return sys.Content
}

// exposedToolDefs 工具暴露的唯一入口：启用工具 → Skill 白名单（命中技能时）→ Agent 工具策略。
// 上下文占用透视走同一入口（skillTools 传 nil），保证「看到的工具」等于「实发的工具」。
func (s *ChatService) exposedToolDefs(ctx context.Context, def core.Definition, skillTools []string) []llm.ToolDefinition {
	return def.FilterTools(s.tools.LLMDefinitionsFiltered(ctx, skillTools))
}

// exposedToolNames 暴露工具名清单（内核 Expose 用；空清单表示不限制）。
func (s *ChatService) exposedToolNames(ctx context.Context, def core.Definition, skillTools []string) []string {
	defs := s.exposedToolDefs(ctx, def, skillTools)
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	return names
}

// sampleParams 采样参数两层合并：请求级 > 全局默认（Provider 级不参与）。
func (s *ChatService) sampleParams(ctx context.Context, params core.RequestParams) (*float64, *llm.ThinkingConfig) {
	dl := s.defaults(ctx)
	temperature := params.Temperature
	if temperature == nil {
		t := dl.Temperature
		temperature = &t
	}
	thinking := params.Thinking
	if thinking == nil {
		thinking = dl.Thinking
	}
	return temperature, thinking
}

// toolCallTimeout 单次工具执行超时：Agent 预算优先，否则内核默认 5 分钟。
func toolCallTimeout(def core.Definition) time.Duration {
	if def.Budget.ToolCallTimeout > 0 {
		return def.Budget.ToolCallTimeout
	}
	return 5 * time.Minute
}

// failOutcome run 启动期失败的统一出口：落库 + 发 chat:error + 返回错误终态。
func (s *ChatService) failOutcome(ctx context.Context, ses *domain.ChatSessionDO, runID, assistantMsgID string, err error) *core.Outcome {
	s.failRun(ctx, runID, ses.ID, assistantMsgID, err)
	return &core.Outcome{Reason: core.ReasonError, Err: err}
}
