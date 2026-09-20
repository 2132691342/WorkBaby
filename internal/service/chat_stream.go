package service

// 本文件：core.Event → 前端协议（chat:*）/ 消息块 / tool 消息的唯一映射点。
// 内核只产领域事件（12 种），落块、落 tool 消息、发事件全部在这里收口。
//
// 子 Agent 委派的生命周期事件走独立 chat:subagent-* 通道（复用父 run 的 chat:done 会让
// 前端提前关闭 SSE）；子工具事件推 chat:tool*（带 agent 标签），但不写父 run 的块与 tool 历史。

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"WorkBaby/internal/capability"
	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// coreEventMapper 一次 run 的事件映射器。
type coreEventMapper struct {
	svc            *ChatService
	ctx            context.Context
	ses            *domain.ChatSessionDO
	runID          string
	assistantMsgID string
	runState       *capability.RunState // 能力装配态：技能命中详情在 RunStart 时落事件/块
	runStart       time.Time

	blockSeq  int64          // message_blocks 单调序号（同一事件对应同一 seq）
	toolCalls []llm.ToolCall // 父 run 收集的工具调用（run 结束写 assistant.tool_calls）
	doneEvent any            // 终态事件载荷（domain.ChatDoneEvent）：延迟到 assistant 落库后由调用方发出

	// textSeg 自上次落块以来累积的正文增量：在工具调用之前与轮次/run 结束时落块，
	// 使块的 seq 与模型真实输出顺序一致（叙述 → 工具 → 叙述）。
	textSeg strings.Builder
}

// newCoreEventMapper 构造一次 run 的事件映射器。
func newCoreEventMapper(svc *ChatService, ctx context.Context, ses *domain.ChatSessionDO,
	runID, assistantMsgID string, runState *capability.RunState) *coreEventMapper {
	return &coreEventMapper{
		svc: svc, ctx: ctx, ses: ses, runID: runID, assistantMsgID: assistantMsgID,
		runState: runState, runStart: time.Now(),
	}
}

// persistBlock 过程块落库（thinking / tool_call / tool_result / artifact / skill），
// 仅父 run 落块；刷新后历史消息据此完整回放执行过程。
func (m *coreEventMapper) persistBlock(e core.Event, kind domain.MessageBlockKind, payload map[string]any) {
	if m.svc.blocks == nil || m.assistantMsgID == "" || e.RunID != m.runID {
		return
	}
	m.blockSeq++
	bs, err := json.Marshal(payload)
	if err != nil {
		return
	}
	row := domain.MessageBlockDO{
		ID:        pkg.NewID(domain.IDMessageBlock),
		MessageID: m.assistantMsgID,
		SessionID: m.ses.ID,
		Seq:       m.blockSeq,
		Kind:      kind,
		Payload:   string(bs),
	}
	if err := m.svc.blocks.Create(m.ctx, &row); err != nil {
		pkg.L.Warn("persist message block failed", "runID", m.runID, "err", err.Error())
	}
}

// flushText 把累积的正文片段落成一个 text 块（空/纯空白不落）。
// 落块时机 = 工具调用之前 与 轮次/run 结束：这样块序列里的正文与工具调用
// 就保持了模型真实的输出顺序，而不是「过程全在前、正文全在后」。
func (m *coreEventMapper) flushText(e core.Event) {
	if m.textSeg.Len() == 0 {
		return
	}
	text := m.textSeg.String()
	m.textSeg.Reset()
	if strings.TrimSpace(text) == "" {
		return
	}
	m.persistBlock(e, domain.BlockText, map[string]any{"text": text})
}

// toolResultContentOf 组装落库的 tool 消息正文：错误信息并入正文，
// 下次 run 重建上下文时模型能看到「上次为什么失败」。
func toolResultContentOf(p core.ToolResultPayload) string {
	if p.Err != "" {
		return "error: " + p.Err + "\n" + p.Content
	}
	return p.Content
}

// handle 实现 core.Sink：单事件映射入口。
func (m *coreEventMapper) handle(e core.Event) {
	s := m.svc
	runID, ses := m.runID, m.ses
	isChild := e.RunID != m.runID
	switch e.Kind {
	case core.EventRunStart:
		if isChild {
			s.emit(runID, ses.ID, "chat:subagent-start", map[string]any{"sub_run_id": e.RunID, "agent": e.Agent})
			return
		}
		// 技能命中先于首帧正文：时间线叙事顺序为「技能命中 → 工具 → 回答」
		if st := m.runState; st != nil && st.SkillName != "" {
			s.emit(runID, ses.ID, "chat:skill", skillBlockPayload(st))
			m.persistBlock(e, domain.BlockSkill, skillBlockPayload(st))
		}
		s.emit(runID, ses.ID, "chat:stream.start", map[string]any{"model": ses.Model})
	case core.EventTurnStart:
		// 长任务的轮次推进需要用户可见：前端据此标注「第 N 轮」并按轮分段时间线。
		if isChild {
			return
		}
		s.emit(runID, ses.ID, "chat:turn-start", map[string]any{"turn": e.Turn})
	case core.EventCheckpoint:
		// 检查点位点：崩溃/中断后能续跑到哪，续跑提示据此说明从哪一轮接着做。
		if isChild {
			return
		}
		s.emit(runID, ses.ID, "chat:checkpoint", map[string]any{"turn": e.Turn})
	case core.EventTurnDelta:
		if isChild {
			return
		}
		if p, ok := e.Payload.(core.DeltaPayload); ok && p.Kind == "content" {
			s.emit(runID, ses.ID, "chat:stream", map[string]any{"delta": p.Text})
			m.textSeg.WriteString(p.Text)
		}
	case core.EventTurnThinking:
		if isChild {
			return
		}
		if p, ok := e.Payload.(core.DeltaPayload); ok && p.Kind == "thinking" {
			s.emit(runID, ses.ID, "chat:thinking", map[string]any{"delta": p.Text})
		}
	case core.EventTurnEnd:
		if isChild {
			return
		}
		// 轮次结束推本轮用量：前端 streamingStats 实时驱动上下文进度与消息用量行。
		if p, ok := e.Payload.(core.TurnEndPayload); ok {
			s.emit(runID, ses.ID, "chat:stats", map[string]any{
				"turn":                  e.Turn,
				"input_tokens":          p.Usage.Input,
				"output_tokens":         p.Usage.Output,
				"cache_read_tokens":     p.Usage.CacheRead,
				"cache_creation_tokens": p.Usage.CacheWrite,
				"total_tokens":          p.Usage.Total,
				"latency_ms":            p.LatencyMs,
			})
		}
		// 本轮叙述收尾：这一轮若以正文结束（没有后续工具调用），在这里落块。
		m.flushText(e)
	case core.EventToolCall:
		if p, ok := e.Payload.(core.ToolCallPayload); ok {
			if !isChild {
				// 先落正文再落工具调用：块的 seq 顺序即用户看到的执行顺序。
				m.flushText(e)
				m.toolCalls = append(m.toolCalls, llm.ToolCall{
					ID:   p.ID,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      p.Name,
						Arguments: p.Arguments,
					},
				})
			}
			s.emit(runID, ses.ID, "chat:tool", map[string]any{
				"id": p.ID, "name": p.Name, "arguments": p.Arguments,
				"activity": p.Activity, "agent": e.Agent,
			})
			if !isChild {
				m.persistBlock(e, domain.BlockToolCall, map[string]any{
					"id": p.ID, "name": p.Name, "arguments": p.Arguments, "activity": p.Activity,
				})
			}
		}
	case core.EventToolStart:
		if _, ok := e.Payload.(core.ToolCallPayload); ok {
			s.emit(runID, ses.ID, "chat:tool-start", map[string]any{"agent": e.Agent, "turn": e.Turn})
		}
	case core.EventToolResult:
		p, ok := e.Payload.(core.ToolResultPayload)
		if !ok {
			return
		}
		s.emit(runID, ses.ID, "chat:tool-result", map[string]any{
			"id": p.ToolCallID, "name": p.Name,
			"content": p.Content, "error": p.Err, "duration_ms": p.DurationMs,
			"agent": e.Agent, "ui_hint": p.UIHint, "data": p.Data,
			"refused": p.Refused, "refused_reason": p.RefusedReason,
		})
		if isChild {
			return
		}
		// 计划快照独立成事件：前端进度卡直接消费，无需解析工具文本。
		if p.Name == todoToolName {
			if st, ok := p.Data["session_todo"].(domain.TodoStateRESP); ok {
				s.emit(runID, ses.ID, "chat:todo", map[string]any{"state": st})
			}
		}
		// 结果 + 产物落块：审批拒绝同样落块，历史可复现完整过程。
		m.persistBlock(e, domain.BlockToolResult, map[string]any{
			"tool_call_id": p.ToolCallID, "name": p.Name,
			"content": p.Content, "error": p.Err,
			"duration_ms": p.DurationMs, "refused": p.Refused,
			"ui_hint": p.UIHint, "data": p.Data,
		})
		if len(p.Data) > 0 {
			m.persistBlock(e, domain.BlockArtifact, map[string]any{"name": p.Name, "data": p.Data})
		}
		// 工具结果落库（role=tool），供后续轮次/下次 run 重建上下文。
		// 序号统一走分配器：与插话注入消息共享水位，避免撞号。
		seq, serr := s.allocSeq(m.ctx, ses.ID, 1)
		if serr != nil {
			pkg.L.Warn("alloc tool message seq failed", "err", serr)
		}
		now := time.Now().UnixMilli()
		toolMsg := &domain.MessageDO{
			ID:         pkg.NewID(domain.IDMessage),
			SessionID:  ses.ID,
			Seq:        seq,
			RunID:      m.runID,
			Role:       domain.MessageRoleTool,
			Content:    toolResultContentOf(p),
			ToolCallID: p.ToolCallID,
			Status:     domain.MessageStatusCompleted,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		_ = s.messages.Insert(m.ctx, toolMsg)
	case core.EventCompressed:
		// 压缩证据：用户只会看到「前面的聊天不见了」，不提示就等于静默吞上下文。
		p, ok := e.Payload.(core.CompressedPayload)
		if !ok || isChild {
			return
		}
		pkg.L.Info("context compressed", "runID", runID, "removed", p.Removed, "truncated", p.Truncated)
		s.persistCompressBoundary(m.ctx, ses, p)
		s.emit(runID, ses.ID, "chat:compressed", map[string]any{
			"removed_messages": p.Removed,
			"summary":          p.Summary,
			"truncated":        p.Truncated,
		})
	case core.EventError:
		p, ok := e.Payload.(core.ErrorPayload)
		if !ok {
			return
		}
		if isChild {
			s.emit(runID, ses.ID, "chat:subagent-error", map[string]any{
				"sub_run_id": e.RunID, "agent": e.Agent, "message": p.Message,
			})
			return
		}
		s.emit(runID, ses.ID, "chat:error", map[string]any{"code": p.Code, "message": p.Message})
	case core.EventRetry:
		// 建流瞬时错误退避重试：用户可见进度，模型不可见。
		if isChild {
			return
		}
		if p, ok := e.Payload.(core.RetryPayload); ok {
			s.emit(runID, ses.ID, "chat:retry", map[string]any{
				"attempt":  p.Attempt,
				"delay_ms": p.DelayMs,
			})
		}
	case core.EventRunDone:
		p, ok := e.Payload.(core.RunDonePayload)
		if !ok {
			return
		}
		if isChild {
			s.emit(runID, ses.ID, "chat:subagent-done", map[string]any{
				"sub_run_id": e.RunID, "agent": e.Agent,
				"reason": string(domain.MapHarnessReason(p.Reason)),
			})
			return
		}
		// 收尾前把最后一段正文落块（正常路径已在 TurnEnd 落过，这里是兜底）。
		m.flushText(e)
		m.doneEvent = domain.ChatDoneEvent{
			Status:     "completed",
			Reason:     string(domain.MapHarnessReason(p.Reason)),
			StopReason: p.StopReason,
			MessageID:  m.assistantMsgID,
			Usage: &domain.ChatDoneUsage{
				InputTokens:  p.Usage.Input,
				OutputTokens: p.Usage.Output,
				CacheRead:    p.Usage.CacheRead,
				CacheWrite:   p.Usage.CacheWrite,
				Total:        p.Usage.Total,
			},
		}
	}
}
