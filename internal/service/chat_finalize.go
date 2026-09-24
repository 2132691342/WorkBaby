package service

// 本文件：一次 run 的「收尾三件事」——终态落库、反幻觉核验、run 后沉淀。
// executeAgent 在此之后只剩「返回结果」这一个动作。

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"WorkBaby/internal/capability"
	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// finalizeRun 写终态：用量落库 → 运行历史 → 授权回滚 → assistant 消息状态与费用。
func (s *ChatService) finalizeRun(ctx context.Context, ses *domain.ChatSessionDO, runID, assistantMsgID, runModel string, res *core.Outcome, mapper *coreEventMapper) {
	s.finishRunRecord(ctx, runID, res)

	nowMs := time.Now().UnixMilli()
	// 终止原因统一口径：内核枚举 → 领域 stop_reason，
	// max_turns / budget_exceeded 不再被硬写成 completed，前端可差异化收尾。
	stopReason := domain.MapHarnessReason(res.Reason)
	// 权限来源回滚：以 error/cancelled 收尾的 run 不留下本次扩出的免审授权
	//（未验证的工作不保留「本会话允许」），成功/主动收尾的 run 授权保留。
	if s.approval != nil && (res.Reason == core.ReasonCancelled || res.Reason == core.ReasonError) {
		s.approval.RollbackRun(runID)
	}
	toolCallsJSON := ""
	if len(mapper.toolCalls) > 0 {
		if bs, err := json.Marshal(mapper.toolCalls); err == nil {
			toolCallsJSON = string(bs)
		}
	}
	// 消息级费用估算：按累计用量 × 单价（未配置单价为空串，前端不显示费用）。
	costUSD := s.modelPricing(ctx, runModel).
		CostUSD(int64(res.Usage.InputTokens), int64(res.Usage.OutputTokens), int64(res.Usage.CacheReadTokens))
	costStr := ""
	if costUSD > 0 {
		costStr = fmt.Sprintf("$%.4f", costUSD)
	}

	if res.Err != nil {
		s.finishExec(runID, core.StateFailed)
		_ = s.messages.UpdateStatus(ctx, assistantMsgID, map[string]any{
			"content":       res.Content,
			"thinking":      res.Thinking,
			"tool_calls":    toolCallsJSON,
			"status":        domain.MessageStatusFailed,
			"stop_reason":   stopReason,
			"output_tokens": res.Usage.OutputTokens,
			"cost":          costStr,
			"updated_at":    nowMs,
		})
		return
	}
	s.finishExec(runID, execState(res.Reason))
	_ = s.messages.UpdateStatus(ctx, assistantMsgID, map[string]any{
		"content":       res.Content,
		"thinking":      res.Thinking,
		"tool_calls":    toolCallsJSON,
		"status":        domain.MessageStatusCompleted,
		"stop_reason":   stopReason,
		"input_tokens":  res.Usage.InputTokens,
		"output_tokens": res.Usage.OutputTokens,
		"cache_read":    res.Usage.CacheReadTokens,
		"total_tokens":  res.Usage.TotalTokens,
		"cost":          costStr,
		"latency_ms":    nowMs - ses.LastMessageAt,
		"updated_at":    nowMs,
	})
}

// verifyArtifactClaims 反幻觉核验：声称已产出文件，但本 run 没有对应的 file_changes
//（拒绝/失败的不算）→ 强提示揭露。证据级别是「声明路径与本 run 产物的精确比对」——
// 只跑 exec / 搜索后泛指「已生成 report」也算幻觉。
func (s *ChatService) verifyArtifactClaims(ctx context.Context, ses *domain.ChatSessionDO, runID, runModel string, res *core.Outcome, mapper *coreEventMapper) {
	if !claimsArtifact(res.Content) || s.changeSvc == nil {
		return
	}
	changeRows, _ := s.changeSvc.ListByRun(ctx, runID, 200)
	changes := make([]changeEvidence, 0, len(changeRows))
	for _, r := range changeRows {
		changes = append(changes, changeEvidence{Path: r.RelPath})
	}
	if evidenceForClaim(claimedPaths(res.Content), mapper.toolCalls, changes) {
		return
	}
	pkg.L.Warn("unbacked artifact claim (no matching file_changes in run)",
		"runID", runID, "sessionID", ses.ID, "model", runModel,
		"claimed", fmt.Sprint(claimedPaths(res.Content)), "changes", len(changeRows))
	s.emit(runID, ses.ID, "chat:warn", domain.ChatWarnEvent{
		Kind:    "unbacked_claim",
		Message: "本条回复声称已产出文件，但本 run 没有任何对应的写文件变更记录——相关文件并不存在，请让模型实际写入后再确认。",
	})
}

// afterRun 收尾后的沉淀与续跑：记忆形成等（异步、独立超时、不阻塞响应）+ 目标模式续跑判定。
func (s *ChatService) afterRun(ctx context.Context, ses *domain.ChatSessionDO, def core.Definition, runID, runModel, userInput string, sent []*llm.Message, res *core.Outcome) {
	// 各能力按自身策略决定是否沉淀（如 Agent 定义关闭 Formation 时记忆能力直接跳过）
	s.caps.CaptureAll(&capability.CaptureCtx{
		SessionID:  ses.ID,
		RunID:      runID,
		UserInput:  userInput,
		Reply:      res.Content,
		Transcript: captureTranscript(sent, userInput, res.Content),
		Def:        def,
		ProviderID: ses.ProviderID,
		Model:      runModel,
	}, memoryCaptureTimeout)
	// 目标模式：活动目标在 run 正常收尾后自动校验，未达标携带下一步动作续跑
	s.maybeContinueGoal(ctx, ses, runID, res)
}

// logRunResult run 结果日志（成功与失败两条口径；字段是排障时唯一稳定的入口）。
func logRunResult(ses *domain.ChatSessionDO, runID, runModel string, res *core.Outcome, toolCalls int, elapsed int64, resume bool) {
	if res.Err != nil {
		pkg.L.Error("chat run failed",
			"runID", runID, "sessionID", ses.ID, "model", runModel, "reason", res.Reason,
			"latencyMs", elapsed, "err", res.Err.Error())
		return
	}
	pkg.L.Info("chat run done",
		"runID", runID, "sessionID", ses.ID, "model", runModel, "reason", res.Reason,
		"stopReason", res.StopReason, "turns", res.Turns, "toolCalls", toolCalls,
		"latencyMs", elapsed, "resume", resume,
		"input", res.Usage.InputTokens, "output", res.Usage.OutputTokens,
		"cacheRead", res.Usage.CacheReadTokens, "total", res.Usage.TotalTokens)
}
