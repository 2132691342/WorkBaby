// Package chat 是对话编排：一个工作区一个引擎子进程，事件翻译成界面词汇。
package chat

import (
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/sse"
)

// usageWire 是引擎事件里的用量形状。
type usageWire struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
	Cost       *struct {
		Total float64 `json:"total"`
	} `json:"cost"`
}

// Stream 是一次运行的实时翻译器：维护块表，把引擎增量事件转成界面帧。
// 生命周期：chat:start → patch/turn → turn_end → settled/done。
// 块表是「界面现在该显示什么」的唯一答案；message_end 覆盖它，增量只为早看到字。
type Stream struct {
	mu sync.Mutex

	// RunId 本次运行 ID
	RunId string
	// Blocks 实时块表
	Blocks []domain.ChatBlockVO
	// UserMessage 本次运行的用户话术
	UserMessage string
	// Streaming 是否正在流式输出
	Streaming bool

	usage *usageWire
	hub   *sse.Hub
}

// NewStream 造一个翻译器。
func NewStream(hub *sse.Hub) *Stream {
	return &Stream{hub: hub}
}

// Begin 登记一次运行并广播 chat:start。
func (s *Stream) Begin(message string) string {
	s.mu.Lock()
	s.RunId = pkg.NewID(pkg.PrefixRun)
	s.Blocks = nil
	s.UserMessage = message
	s.usage = nil
	s.Streaming = true
	runId := s.RunId
	s.mu.Unlock()
	s.emitRun("chat:start", map[string]any{"started_at": pkg.NowMs()})
	return runId
}

// Active 是否正在流式输出。
func (s *Stream) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Streaming
}

// Idle 把流式态收起来（绑定失败或运行被拒时用）。
func (s *Stream) Idle() {
	s.mu.Lock()
	s.Streaming = false
	s.mu.Unlock()
}

// OnEvent 是一个引擎会话事件的入口。
func (s *Stream) OnEvent(ev Record) {
	switch stringOf(ev["type"]) {
	case "message_update":
		s.onMessageUpdate(ev)
	case "message_end":
		s.onMessageEnd(ev)
	case "tool_execution_start":
		s.onToolStart(ev)
	case "tool_execution_update":
		s.onToolUpdate(ev)
	case "tool_execution_end":
		s.onToolEnd(ev)
	case "turn_end":
		s.onTurnEnd(ev)
	case "agent_settled":
		s.onSettled(ev)
	case "queue_update":
		s.hub.Broadcast("chat:queue", sse.EngineTag(map[string]any{
			"steering":  asStrings(ev["steering"]),
			"follow_up": asStrings(ev["followUp"]),
		}))
	case "compaction_start":
		s.hub.Broadcast("chat:compaction", sse.EngineTag(map[string]any{"state": "start"}))
	case "compaction_end":
		s.hub.Broadcast("chat:compaction", sse.EngineTag(map[string]any{"state": "done"}))
	case "auto_retry_start":
		s.hub.Broadcast("chat:retry", sse.EngineTag(map[string]any{
			"attempt":  ev["attempt"],
			"max":      ev["maxAttempts"],
			"delay_ms": ev["delayMs"],
		}))
	case "auto_retry_end":
		s.hub.Broadcast("chat:retry_end", sse.EngineTag(nil))
	case "extension_error":
		s.hub.Broadcast("chat:notify", sse.EngineTag(map[string]any{
			"level":   "error",
			"message": "扩展出错：" + stringOf(ev["error"]),
		}))
	case "session_info_changed":
		s.hub.Broadcast("session:title", sse.EngineTag(map[string]any{"title": stringOf(ev["name"])}))
	}
}

// onMessageUpdate 处理增量：正文、思考、工具调用的开始与结束。
func (s *Stream) onMessageUpdate(ev Record) {
	if raw, ok := ev["usage"].(map[string]any); ok {
		s.setUsage(raw)
	}
	inner, ok := ev["assistantMessageEvent"].(map[string]any)
	if !ok {
		return
	}
	delta := stringOf(inner["delta"])
	index := intOf(inner["contentIndex"], -1)
	if index < 0 {
		index = s.blockCount()
	}
	switch stringOf(inner["type"]) {
	case "text_delta":
		if delta != "" {
			s.appendDelta(index, domain.BlockText, delta)
		}
	case "thinking_delta":
		if delta != "" {
			s.appendDelta(index, domain.BlockThinking, delta)
		}
	case "toolcall_start":
		block := domain.ChatBlockVO{
			BlockKind:  domain.BlockTool,
			ToolCallId: stringOf(inner["id"]),
			ToolName:   stringOf(inner["toolName"]),
			ToolStatus: "pending",
		}
		s.putBlock(index, block)
		s.emitPatch("block", index, domain.BlockTool, "", &block)
	case "toolcall_end":
		call, ok := inner["toolCall"].(map[string]any)
		if !ok {
			return
		}
		s.mu.Lock()
		if index < len(s.Blocks) {
			block := &s.Blocks[index]
			if id := stringOf(call["id"]); id != "" {
				block.ToolCallId = id
			}
			if name := stringOf(call["name"]); name != "" {
				block.ToolName = name
			}
			block.ToolArgs = call["arguments"]
		}
		snapshot := s.blocksLocked()
		s.mu.Unlock()
		s.emitPatch("block", index, domain.BlockTool, "", &snapshot[index])
	}
}

// onMessageEnd 用完整内容重建块表：这是权威快照。
func (s *Stream) onMessageEnd(ev Record) {
	msg, ok := ev["message"].(map[string]any)
	if !ok {
		return
	}
	role := stringOf(msg["role"])
	if role == "user" {
		s.mu.Lock()
		s.UserMessage = textOf(msg["content"])
		s.mu.Unlock()
		return
	}
	if role != "assistant" {
		return
	}
	s.mu.Lock()
	content, _ := msg["content"].([]any)
	next := make([]domain.ChatBlockVO, 0, len(content))
	for _, item := range content {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch stringOf(block["type"]) {
		case "text":
			next = append(next, domain.ChatBlockVO{BlockKind: domain.BlockText, BlockText: stringOf(block["text"])})
		case "thinking":
			next = append(next, domain.ChatBlockVO{BlockKind: domain.BlockThinking, BlockText: stringOf(block["thinking"])})
		case "toolCall":
			id := stringOf(block["id"])
			if existing := findTool(s.Blocks, id); existing != nil {
				next = append(next, *existing)
				continue
			}
			next = append(next, domain.ChatBlockVO{
				BlockKind:  domain.BlockTool,
				ToolCallId: id,
				ToolName:   stringOf(block["name"]),
				ToolArgs:   block["arguments"],
				ToolStatus: "pending",
			})
		}
	}
	s.Blocks = next
	snapshot := s.blocksLocked()
	stopReason := stringOf(msg["stopReason"])
	errorMessage := stringOf(msg["errorMessage"])
	if stopReason == "error" {
		s.Streaming = false
	}
	s.mu.Unlock()

	s.emitRun("chat:blocks", map[string]any{"blocks": snapshot})
	if stopReason == "error" {
		if errorMessage == "" {
			errorMessage = "模型调用失败"
		}
		s.emitRun("chat:error", map[string]any{"message": errorMessage})
	}
}

// onToolStart 工具开始执行。
func (s *Stream) onToolStart(ev Record) {
	toolCallId := stringOf(ev["toolCallId"])
	index, block := s.mutateTool(toolCallId, func(b *domain.ChatBlockVO) {
		b.ToolStatus = "running"
		if args, ok := ev["args"]; ok && args != nil {
			b.ToolArgs = args
		}
	})
	if block == nil {
		return
	}
	s.emitPatch("block", index, domain.BlockTool, "", block)
	s.emitRun("chat:live", map[string]any{
		"phase":         "tool",
		"tool_name":     stringOf(ev["toolName"]),
		"tool_call_id":  toolCallId,
		"output_tokens": s.outputTokens(),
	})
}

// onToolUpdate 工具流式输出。
func (s *Stream) onToolUpdate(ev Record) {
	partial, _ := ev["partialResult"].(map[string]any)
	text := contentText(partial)
	if text == "" {
		return
	}
	index, block := s.mutateTool(stringOf(ev["toolCallId"]), func(b *domain.ChatBlockVO) {
		b.ToolOutput = text
	})
	if block != nil {
		s.emitPatch("block", index, domain.BlockTool, "", block)
	}
}

// onToolEnd 工具结束：输出、错误标记、耗时、差异行、被改动的文件。
func (s *Stream) onToolEnd(ev Record) {
	result, _ := ev["result"].(map[string]any)
	isError, _ := ev["isError"].(bool)
	durationMs := intOf(ev["durationMs"], 0)
	index, block := s.mutateTool(stringOf(ev["toolCallId"]), func(b *domain.ChatBlockVO) {
		b.ToolOutput = contentText(result)
		b.ToolIsError = isError
		if isError {
			b.ToolStatus = "error"
		} else {
			b.ToolStatus = "done"
		}
		b.DurationMs = durationMs
		if details, ok := result["details"].(map[string]any); ok {
			if diff := stringOf(details["diff"]); diff != "" {
				b.DiffLines = parseDiffLines(diff)
			}
		}
		if path := filePathOf(b.ToolArgs, result); path != "" {
			b.FilePath = path
		}
	})
	if block != nil {
		s.emitPatch("block", index, domain.BlockTool, "", block)
	}
}

// onTurnEnd 一轮结束：广播回合快照与用量。
func (s *Stream) onTurnEnd(ev Record) {
	if msg, ok := ev["message"].(map[string]any); ok {
		if raw, ok := msg["usage"].(map[string]any); ok {
			s.setUsage(raw)
		}
	}
	s.mu.Lock()
	blocks := s.blocksLocked()
	userMessage := s.UserMessage
	usage := s.usage
	s.mu.Unlock()

	s.emitRun("chat:turn", map[string]any{"user_message": userMessage, "blocks": blocks})
	usageVO := domain.TurnUsageVO{}
	if usage != nil {
		usageVO = domain.TurnUsageVO{
			InputTokens:      usage.Input,
			OutputTokens:     usage.Output,
			CacheReadTokens:  usage.CacheRead,
			CacheWriteTokens: usage.CacheWrite,
		}
		if usage.Cost != nil {
			usageVO.CostUsd = usage.Cost.Total
		}
	}
	s.emitRun("chat:turn_end", map[string]any{"usage": usageVO})
}

// onSettled 引擎确认不会再有自动续跑。
func (s *Stream) onSettled(ev Record) {
	s.mu.Lock()
	s.Streaming = false
	s.mu.Unlock()
	aborted, _ := ev["aborted"].(bool)
	s.emitRun("chat:settled", map[string]any{"aborted": aborted})
	s.emitRun("chat:done", nil)
}

// OnUiRequest 处理非对话框类的引擎 UI 请求（通知 / 状态条 / 小组件 / 预填输入框）。
func (s *Stream) OnUiRequest(rec Record) {
	switch stringOf(rec["method"]) {
	case "notify":
		s.hub.Broadcast("chat:notify", sse.EngineTag(map[string]any{
			"level":   defaultString(stringOf(rec["notifyType"]), "info"),
			"message": stringOf(rec["message"]),
		}))
	case "setStatus":
		s.hub.Broadcast("chat:status", sse.EngineTag(map[string]any{
			"key":  stringOf(rec["statusKey"]),
			"text": stringOf(rec["statusText"]),
		}))
	case "setWidget":
		s.hub.Broadcast("chat:widget", sse.EngineTag(map[string]any{
			"key":   stringOf(rec["widgetKey"]),
			"lines": asStrings(rec["widgetLines"]),
		}))
	case "setTitle":
		s.hub.Broadcast("session:title", sse.EngineTag(map[string]any{"title": stringOf(rec["title"])}))
	case "set_editor_text":
		s.hub.Broadcast("chat:editor_text", sse.EngineTag(map[string]any{"text": stringOf(rec["text"])}))
	}
}

// appendDelta 追加一段增量到指定块。
func (s *Stream) appendDelta(index int, kind domain.BlockKind, delta string) {
	s.mu.Lock()
	for len(s.Blocks) <= index {
		s.Blocks = append(s.Blocks, domain.ChatBlockVO{})
	}
	block := &s.Blocks[index]
	if block.BlockKind != kind {
		*block = domain.ChatBlockVO{BlockKind: kind}
	}
	block.BlockText += delta
	s.mu.Unlock()
	s.emitPatch("delta", index, kind, delta, nil)
	if kind == domain.BlockText {
		s.emitRun("chat:live", map[string]any{"phase": "text", "output_tokens": s.outputTokens()})
	} else {
		s.emitRun("chat:live", map[string]any{"phase": "thinking", "output_tokens": s.outputTokens()})
	}
}

// putBlock 整块替换指定下标。
func (s *Stream) putBlock(index int, block domain.ChatBlockVO) {
	s.mu.Lock()
	for len(s.Blocks) <= index {
		s.Blocks = append(s.Blocks, domain.ChatBlockVO{})
	}
	s.Blocks[index] = block
	s.mu.Unlock()
}

// mutateTool 按工具调用 ID 找块并就地改；返回下标与改后的副本。
func (s *Stream) mutateTool(toolCallId string, mutate func(*domain.ChatBlockVO)) (int, *domain.ChatBlockVO) {
	if toolCallId == "" {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Blocks {
		if s.Blocks[i].BlockKind == domain.BlockTool && s.Blocks[i].ToolCallId == toolCallId {
			mutate(&s.Blocks[i])
			copyBlock := s.Blocks[i]
			return i, &copyBlock
		}
	}
	return 0, nil
}

// blockCount 当前块数。
func (s *Stream) blockCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Blocks)
}

// blocksLocked 取块表副本；调用方需持有锁。
func (s *Stream) blocksLocked() []domain.ChatBlockVO {
	out := make([]domain.ChatBlockVO, len(s.Blocks))
	copy(out, s.Blocks)
	return out
}

// setUsage 记下最新用量读数。
func (s *Stream) setUsage(raw map[string]any) {
	usage := &usageWire{
		Input:      intOf(raw["input"], 0),
		Output:     intOf(raw["output"], 0),
		CacheRead:  intOf(raw["cacheRead"], 0),
		CacheWrite: intOf(raw["cacheWrite"], 0),
	}
	if cost, ok := raw["cost"].(map[string]any); ok {
		usage.Cost = &struct {
			Total float64 `json:"total"`
		}{Total: floatOf(cost["total"])}
	}
	s.mu.Lock()
	s.usage = usage
	s.mu.Unlock()
}

// outputTokens 当前输出 token 读数。
func (s *Stream) outputTokens() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.usage == nil {
		return 0
	}
	return s.usage.Output
}

// emitPatch 广播一帧块增量 / 整块替换。
func (s *Stream) emitPatch(mode string, index int, kind domain.BlockKind, delta string, block *domain.ChatBlockVO) {
	payload := map[string]any{"mode": mode, "index": index, "kind": string(kind)}
	if delta != "" {
		payload["delta"] = delta
	}
	if block != nil {
		payload["block"] = block
	}
	s.emitRun("chat:patch", payload)
}

// emitRun 广播一帧带 run_id 与引擎标记的聊天事件。
func (s *Stream) emitRun(event string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	s.mu.Lock()
	data["run_id"] = s.RunId
	s.mu.Unlock()
	s.hub.Broadcast(event, sse.EngineTag(data))
}

// findTool 在块表里按调用 ID 找工具块。
func findTool(blocks []domain.ChatBlockVO, toolCallId string) *domain.ChatBlockVO {
	if toolCallId == "" {
		return nil
	}
	for i := range blocks {
		if blocks[i].BlockKind == domain.BlockTool && blocks[i].ToolCallId == toolCallId {
			return &blocks[i]
		}
	}
	return nil
}
