package service

import (
	"context"
	"encoding/json"

	"WorkBaby/internal/core"
	"WorkBaby/internal/event"
)

// Emitter 事件统一出口：注入归属、分配 seq、写重放缓冲后广播。全应用唯一出口。
type Emitter struct {
	bus *event.Bus
	log *event.RunEventLog
}

// NewEmitter 构造事件出口；log 可为 nil（退化为无重放）。
func NewEmitter(bus *event.Bus, log *event.RunEventLog) *Emitter {
	return &Emitter{bus: bus, log: log}
}

// Emit 发布事件；payload 支持 map（高频增量，零转换）或领域结构体（JSON 归一）。
// runID 为空 = 会话级事件（目标状态等）：仍广播并带 session_id，由订阅方按会话过滤。
func (e *Emitter) Emit(runID, sessionID, name string, payload any) {
	if e == nil || e.bus == nil {
		return
	}
	m := payloadMap(payload)
	m["run_id"] = runID
	m["session_id"] = sessionID
	if e.log != nil && runID != "" {
		e.log.Append(runID, name, m)
	}
	e.bus.Publish(name, m)
}

// EmitCtx 从 ctx 提取 run/session 归属后发布（工具、审批、文件变更、工件等能力域使用）。
func (e *Emitter) EmitCtx(ctx context.Context, name string, payload any) {
	e.Emit(core.RunIDFromCtx(ctx), core.SessionIDFromCtx(ctx), name, payload)
}

// payloadMap 归一化为事件载荷 map：map 直接使用；结构体经 JSON 往返（低频事件可接受）。
func payloadMap(payload any) map[string]any {
	switch p := payload.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return p
	default:
		bs, err := json.Marshal(p)
		if err != nil {
			return map[string]any{}
		}
		var m map[string]any
		if json.Unmarshal(bs, &m) != nil {
			return map[string]any{}
		}
		return m
	}
}
