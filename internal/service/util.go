package service

import (
	"encoding/json"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
)

func nowMillis() int64 { return time.Now().UnixMilli() }

// toLLMMessages 把会话链还原出的条目转成内核消息。
func toLLMMessages(entries []domain.EntryDO) []llm.Message {
	out := make([]llm.Message, 0, len(entries))
	for _, e := range entries {
		var p domain.MessagePayload
		_ = json.Unmarshal([]byte(e.PayloadJSON), &p)
		out = append(out, llm.Message{
			Role: e.Role, Content: p.Content, Thinking: p.Thinking,
			ToolCallID: p.ToolCallID, IsError: p.IsError,
			ToolCalls:  toLLMToolCalls(p.ToolCalls),
		})
	}
	return out
}

// toMessageVO 把条目转成前端渲染用的消息。
func toMessageVO(e *domain.EntryDO) domain.MessageVO {
	var p domain.MessagePayload
	_ = json.Unmarshal([]byte(e.PayloadJSON), &p)
	var u domain.UsageVO
	_ = json.Unmarshal([]byte(e.UsageJSON), &u)
	vo := domain.MessageVO{
		ID: e.ID, Role: e.Role, Type: e.Type, Thinking: p.Thinking,
		Content: p.Content, ToolCallID: p.ToolCallID,
		ToolName: p.ToolName, IsError: p.IsError, StopReason: p.StopReason,
		LatencyMs: p.LatencyMs, CreatedAt: e.CreatedAt,
	}
	if len(p.ToolCalls) > 0 {
		vo.ToolCalls = p.ToolCalls
	}
	if u.Total > 0 || u.Input > 0 || u.Output > 0 {
		cp := u
		vo.Usage = &cp
	}
	return vo
}

// payloadOf 把内核消息序列化成条目 payload。
func payloadOf(m llm.Message, stopReason string, latencyMs int64, toolName ...string) string {
	p := domain.MessagePayload{
		Thinking: m.Thinking, Content: m.Content, ToolCallID: m.ToolCallID,
		IsError: m.IsError, StopReason: stopReason, LatencyMs: latencyMs,
		ToolCalls: toDomainToolCalls(m.ToolCalls),
	}
	if len(toolName) > 0 {
		p.ToolName = toolName[0]
	}
	raw, _ := json.Marshal(p)
	return string(raw)
}

func toLLMToolCalls(in []domain.ToolCall) []llm.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(in))
	for _, c := range in {
		out = append(out, llm.ToolCall{ID: c.ID, Name: c.Name, Label: c.Label, Args: c.Args})
	}
	return out
}

func toDomainToolCalls(in []llm.ToolCall) []domain.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.ToolCall, 0, len(in))
	for _, c := range in {
		out = append(out, domain.ToolCall{ID: c.ID, Name: c.Name, Label: c.Label, Args: c.Args})
	}
	return out
}
