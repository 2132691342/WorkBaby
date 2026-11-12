// Package service 是业务编排层：设置、会话、对话、模型、技能、MCP、文件、git。
// 它只依赖 repo / pkg / domain / agent，不感知 HTTP 与 Wails。
package service

import (
	"encoding/json"
	"sync"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/sse"
)

// Stream 是一次运行的实时翻译器：把引擎事件翻成界面词汇。
// 块表是「界面现在该显示什么」的唯一答案；助手消息落盘时整表覆盖，
// 增量只为让用户早一点看到字。
type Stream struct {
	// mu 保护块表与状态
	mu sync.Mutex
	// RunId 本次运行 ID
	RunId string
	// Blocks 实时块表
	Blocks []domain.ChatBlockVO
	// UserMessage 本次运行的用户话术
	UserMessage string
	// Streaming 是否正在流式输出
	Streaming bool
	// toolIndex 工具调用 ID → 块下标
	toolIndex map[string]int
	// usage 累计用量
	usage agent.Usage
	// hub 事件广播
	hub *sse.Hub
}

// NewStream 造一个翻译器。
func NewStream(hub *sse.Hub) *Stream {
	return &Stream{hub: hub, toolIndex: map[string]int{}}
}

// Begin 登记一次运行并广播 chat:start。
func (s *Stream) Begin(message string) string {
	s.mu.Lock()
	s.RunId = pkg.NewID(pkg.PrefixRun)
	s.Blocks = nil
	s.UserMessage = message
	s.Streaming = true
	s.toolIndex = map[string]int{}
	s.usage = agent.Usage{}
	runId := s.RunId
	s.mu.Unlock()
	s.emit("chat:start", map[string]any{"started_at": pkg.NowMs()})
	return runId
}

// Active 是否正在流式输出。
func (s *Stream) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Streaming
}

// Idle 收起流式态。
func (s *Stream) Idle() {
	s.mu.Lock()
	s.Streaming = false
	s.mu.Unlock()
}

// TextDelta 正文增量。
func (s *Stream) TextDelta(delta string) {
	s.appendText(domain.BlockText, delta)
	s.emit("chat:live", map[string]any{"phase": "text", "output_tokens": s.outputTokens()})
}

// ThinkingDelta 思考增量。
func (s *Stream) ThinkingDelta(delta string) {
	s.appendText(domain.BlockThinking, delta)
	s.emit("chat:live", map[string]any{"phase": "thinking", "output_tokens": s.outputTokens()})
}

// appendText 追加文本到末尾的同类块；换种就另开一块。
func (s *Stream) appendText(kind domain.BlockKind, delta string) {
	if delta == "" {
		return
	}
	s.mu.Lock()
	if n := len(s.Blocks); n > 0 && s.Blocks[n-1].BlockKind == kind {
		// 流式期只保留尾部：几万字的思考全塞进 DOM 会把主线程压死，
		// 而用户看的是最后几行。
		s.Blocks[n-1].BlockText = tail(s.Blocks[n-1].BlockText+delta, maxLiveBlock)
		index := n - 1
		s.mu.Unlock()
		s.emitPatch("delta", index, kind, delta, nil)
		return
	}
	s.Blocks = append(s.Blocks, domain.ChatBlockVO{BlockKind: kind, BlockText: delta})
	index := len(s.Blocks) - 1
	block := s.Blocks[index]
	s.mu.Unlock()
	s.emitPatch("block", index, kind, "", &block)
}

// maxLiveBlock 是流式期单个文本块的保留上限。
const maxLiveBlock = 8000

// tail 取字符串尾部的 n 个字符。
func tail(text string, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	return string(runes[len(runes)-n:])
}

// ToolStart 工具开始执行。
func (s *Stream) ToolStart(callId, name, args string) {
	block := domain.ChatBlockVO{
		BlockKind:  domain.BlockTool,
		ToolCallId: callId,
		ToolName:   name,
		ToolArgs:   decodeAny(args),
		ToolStatus: "running",
	}
	s.mu.Lock()
	s.Blocks = append(s.Blocks, block)
	s.toolIndex[callId] = len(s.Blocks) - 1
	index := len(s.Blocks) - 1
	s.mu.Unlock()
	s.emitPatch("block", index, domain.BlockTool, "", &block)
	s.emit("chat:live", map[string]any{
		"phase":         "tool",
		"tool_name":     name,
		"tool_call_id":  callId,
		"output_tokens": s.outputTokens(),
	})
}

// ToolEnd 工具结束：输出、错误标记、耗时、差异行。
func (s *Stream) ToolEnd(callId, name string, result agent.Result, durationMs int64) {
	s.mu.Lock()
	index, ok := s.toolIndex[callId]
	if !ok {
		s.mu.Unlock()
		return
	}
	block := &s.Blocks[index]
	block.ToolOutput = result.Text
	block.ToolIsError = result.IsError
	block.DurationMs = int(durationMs)
	block.ToolStatus = "done"
	if result.IsError {
		block.ToolStatus = "error"
	}
	if diff, ok := result.Details["diff"].(string); ok && diff != "" {
		block.DiffLines = ParseDiffLines(diff)
	}
	if path, ok := result.Details["file_path"].(string); ok {
		block.FilePath = path
	}
	snapshot := *block
	s.mu.Unlock()
	s.emitPatch("block", index, domain.BlockTool, "", &snapshot)
	// 待办清单变了就推一帧：面板要跟着跑动实时更新，
	// 等下一轮刷新才看到，用户会以为清单没生效。
	s.emitTodos(name, result)
}

// emitTodos 待办工具结束后广播新清单。
// 只认 todo 工具：别的工具不该碰这份清单。
func (s *Stream) emitTodos(name string, result agent.Result) {
	if name != "todo" {
		return
	}
	raw, ok := result.Details["tasks"].([]any)
	if !ok {
		return
	}
	todos := make([]domain.TodoVO, 0, len(raw))
	for _, item := range raw {
		task, ok := item.(map[string]any)
		if !ok {
			continue
		}
		subject, _ := task["subject"].(string)
		status, _ := task["status"].(string)
		if subject == "" || status == "" {
			continue
		}
		id := 0
		if n, ok := task["id"].(float64); ok {
			id = int(n)
		}
		activeForm, _ := task["activeForm"].(string)
		todos = append(todos, domain.TodoVO{
			TodoId:         id,
			TodoSubject:    subject,
			TodoStatus:     status,
			TodoActiveForm: activeForm,
		})
	}
	s.emit("chat:todos", map[string]any{"todos": todos})
}

// AssistantMessage 一条助手消息已落盘：用完整内容覆盖块表。
func (s *Stream) AssistantMessage(msg agent.Message) {
	blocks := make([]domain.ChatBlockVO, 0, len(msg.Blocks))
	s.mu.Lock()
	for _, block := range msg.Blocks {
		switch block.Kind {
		case agent.KindText:
			if block.Text == "" {
				continue
			}
			blocks = append(blocks, domain.ChatBlockVO{BlockKind: domain.BlockText, BlockText: block.Text})
		case agent.KindThinking:
			if block.Text == "" {
				continue
			}
			blocks = append(blocks, domain.ChatBlockVO{BlockKind: domain.BlockThinking, BlockText: block.Text})
		case agent.KindToolCall:
			// 工具块沿用实时阶段已经填好的输出与状态：落盘的消息里只有入参。
			if existing := s.toolBlock(block.CallId); existing != nil {
				blocks = append(blocks, *existing)
				continue
			}
			blocks = append(blocks, domain.ChatBlockVO{
				BlockKind:  domain.BlockTool,
				ToolCallId: block.CallId,
				ToolName:   block.ToolName,
				ToolArgs:   decodeAny(block.ToolArgs),
				ToolStatus: "done",
			})
		}
	}
	s.Blocks = blocks
	s.mu.Unlock()
	s.emit("chat:turn", map[string]any{"user_message": s.userText(), "turn_blocks": blocks})

	usage := domain.TurnUsageVO{}
	if msg.Usage != nil {
		usage = turnUsage(*msg.Usage)
	}
	s.emit("chat:turn_end", map[string]any{"turn_usage": usage})
}

// toolBlock 按调用 ID 取已有的工具块副本；调用方需持有锁。
func (s *Stream) toolBlock(callId string) *domain.ChatBlockVO {
	if callId == "" {
		return nil
	}
	for i := range s.Blocks {
		if s.Blocks[i].BlockKind == domain.BlockTool && s.Blocks[i].ToolCallId == callId {
			copyBlock := s.Blocks[i]
			return &copyBlock
		}
	}
	return nil
}

// Usage 记下累计用量。
func (s *Stream) Usage(usage agent.Usage) {
	s.mu.Lock()
	s.usage = usage
	s.mu.Unlock()
}

// Fail 一次运行失败。
func (s *Stream) Fail(message string) {
	s.mu.Lock()
	s.Streaming = false
	s.mu.Unlock()
	s.emit("chat:error", map[string]any{"message": pkg.CleanText(message)})
}

// Finish 运行收尾。
func (s *Stream) Finish(aborted bool) {
	s.mu.Lock()
	s.Streaming = false
	s.mu.Unlock()
	s.emit("chat:settled", map[string]any{"aborted": aborted})
	s.emit("chat:done", map[string]any{"aborted": aborted})
}

// userText 取本次运行的用户话术。
func (s *Stream) userText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.UserMessage
}

// outputTokens 当前输出 token 读数。
func (s *Stream) outputTokens() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage.OutputTokens
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
	s.emit("chat:patch", payload)
}

// emit 广播一帧带 run_id 的聊天事件。
func (s *Stream) emit(event string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	s.mu.Lock()
	data["run_id"] = s.RunId
	s.mu.Unlock()
	s.hub.Broadcast(event, sse.EngineTag(data))
}

// turnUsage 把引擎用量转成界面读数。
func turnUsage(usage agent.Usage) domain.TurnUsageVO {
	return domain.TurnUsageVO{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
	}
}

// decodeAny 把入参原文解成通用形状；解不开原样给字符串。
func decodeAny(raw string) any {
	if raw == "" {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}
