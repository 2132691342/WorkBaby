package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// maxSteps 是一轮对话里最多允许的「模型调用 → 工具执行」次数。
// 不设上限的话，一个反复试错的模型能把一次对话变成无限循环，烧光额度。
const maxSteps = 24

// Handler 是一次运行时的事件出口。引擎只认这些词汇，翻译成 SSE 是上层的事。
type Handler struct {
	// OnText 正文增量
	OnText func(delta string)
	// OnThinking 思考增量
	OnThinking func(delta string)
	// OnStepStart 第几步开始（从 1 数）
	OnStepStart func(step int)
	// OnToolStart 工具开始执行
	OnToolStart func(callId, name string, args string)
	// OnToolEnd 工具执行结束
	OnToolEnd func(callId, name string, result Result, durationMs int64)
	// OnAssistant 一条助手消息已落盘
	OnAssistant func(msg Message)
	// OnUsage 累计用量
	OnUsage func(usage Usage)
	// OnError 失败原话（已经是可以直接显示给用户的中文）
	OnError func(message string)
	// OnDone 运行收尾；aborted 表示被用户中止
	OnDone func(aborted bool)
}

// Runner 是一次对话运行的执行器。
type Runner struct {
	// Streamer 上游适配器
	Streamer Streamer
	// Registry 工具注册表
	Registry *Registry
	// Session 会话（真相落盘在这里）
	Session *Session
	// Model 本次使用的模型与凭据
	Model ModelRef
	// MaxTokens 输出上限；0 表示用模型默认
	MaxTokens int
	// Temperature 采样温度
	Temperature *float64

	// mu 保护队列与状态
	mu sync.Mutex
	// queue 排队中的插话：当前这一轮收尾后注入
	queue []Message
	// steps 本次运行实际走的步数
	steps int
}

// Enqueue 排一条消息，等当前这一轮结束时注入。
func (r *Runner) Enqueue(msg Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queue = append(r.queue, msg)
}

// Queued 取排队中的文本（界面显示用）。
func (r *Runner) Queued() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.queue))
	for _, msg := range r.queue {
		out = append(out, msg.Text())
	}
	return out
}

// RemoveQueued 摘掉排队里的一条。
// 队列是本进程内的切片，所以这是真正的「只删这一条」——
// 不需要「整队取出再重投」那套会丢话术的绕法。
func (r *Runner) RemoveQueued(index int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.queue) {
		return pkg.Err(pkg.CodeBadRequest, "要移除的位置不在队列里")
	}
	r.queue = append(r.queue[:index], r.queue[index+1:]...)
	return nil
}

// Steps 本次运行已走的步数。
func (r *Runner) Steps() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.steps
}

// Run 跑完一整轮：追加用户消息 → 反复「调模型 / 跑工具」直到模型不再要工具。
func (r *Runner) Run(ctx context.Context, userMessage Message, h Handler) {
	defer func() {
		if h.OnDone != nil {
			h.OnDone(errors.Is(ctx.Err(), context.Canceled))
		}
	}()
	if _, err := r.Session.AppendMessage(userMessage); err != nil {
		notify(h.OnError, pkg.As(err).Message)
		return
	}
	r.loop(ctx, h)
}

// loop 是轮次循环本体。
func (r *Runner) loop(ctx context.Context, h Handler) {
	for step := 1; step <= maxSteps; step++ {
		if ctx.Err() != nil {
			return
		}
		r.mu.Lock()
		r.steps = step
		r.mu.Unlock()
		if h.OnStepStart != nil {
			h.OnStepStart(step)
		}

		message, err := r.streamOnce(ctx, h)
		if err != nil {
			notify(h.OnError, err.Error())
			return
		}
		if ctx.Err() != nil {
			// 中止时把已经吐出来的部分落盘：用户看到过这些字，
			// 会话里没有它们就会「重开应用少了一段」。
			r.persistPartial(message, h)
			return
		}
		if message.StopReason == "error" {
			notify(h.OnError, message.ErrorMessage)
			return
		}
		if len(message.Blocks) > 0 {
			if _, appendErr := r.Session.AppendMessage(message); appendErr != nil {
				notify(h.OnError, pkg.As(appendErr).Message)
				return
			}
			if h.OnAssistant != nil {
				h.OnAssistant(message)
			}
		}
		if message.Usage != nil && h.OnUsage != nil {
			h.OnUsage(*message.Usage)
		}

		calls := message.ToolCalls()
		if len(calls) == 0 {
			// 模型不再要工具：注入排队中的插话，没有就收尾。
			if !r.drainQueue(ctx, h) {
				return
			}
			continue
		}
		r.runTools(ctx, calls, h)
		if ctx.Err() != nil {
			return
		}
		if !r.drainQueue(ctx, h) {
			return
		}
	}
	// 步数用满：明确告诉用户停下了，而不是静默截断。
	notify(h.OnError, "这一步来回次数太多，已停下。把任务拆小一些再试。")
}

// streamOnce 发起一次模型调用并累积结果。
func (r *Runner) streamOnce(ctx context.Context, h Handler) (Message, error) {
	system := SystemPrompt(r.Registry, r.Session.Cwd())
	request := Request{
		Model:       r.Model,
		System:      system,
		Messages:    r.history(),
		Tools:       r.Registry.Specs(),
		MaxTokens:   r.MaxTokens,
		Temperature: r.Temperature,
	}
	// 上下文超窗时先裁历史：上游会直接报错，那时用户看到的是
	// 一句与「你想做什么」毫无关系的报错。
	request.Messages = trimToWindow(request.Messages, r.Model.ContextWindow, len(system))

	stream, err := r.Streamer.Stream(ctx, request)
	if err != nil {
		return Message{}, err
	}
	builder := &assistantBuilder{}
	for ev := range stream {
		builder.apply(ev)
		switch ev.Kind {
		case EventText:
			notifyText(h.OnText, ev.Text)
		case EventThinking:
			notifyText(h.OnThinking, ev.Text)
		}
	}
	message := builder.message(r.Model)
	if builder.errMsg != "" {
		message.StopReason = "error"
		message.ErrorMessage = builder.errMsg
	}
	return message, nil
}

// runTools 顺序执行一条助手消息里的全部工具调用。
// 顺序而不是并发：工具会改同一批文件，并发写同一份文件的结果不可预测。
func (r *Runner) runTools(ctx context.Context, calls []Block, h Handler) {
	for _, call := range calls {
		if ctx.Err() != nil {
			return
		}
		if h.OnToolStart != nil {
			h.OnToolStart(call.CallId, call.ToolName, call.ToolArgs)
		}
		started := time.Now()
		result := r.Registry.Run(ctx, call.ToolName, call.ToolArgs)
		elapsed := time.Since(started).Milliseconds()
		// 界面看到的是完整输出，模型看到的是折叠后的。
		// 两个都要：界面要能翻全文，模型不该为几万字日志付出上下文代价。
		contextText, truncated, originalChars := r.Registry.Tidy(result.Text)
		if h.OnToolEnd != nil {
			h.OnToolEnd(call.CallId, call.ToolName, result, elapsed)
		}
		details := result.Details
		if details == nil {
			details = map[string]any{}
		}
		if truncated {
			// 折叠的事实要跟着落盘：回放时能知道这段被折过，
			// 界面也能提示「已折叠，展开看全文」。
			details["tidied"] = true
			details["original_chars"] = originalChars
		}
		reply := Message{
			Role:       RoleTool,
			ToolCallId: call.CallId,
			ToolName:   call.ToolName,
			IsError:    result.IsError,
			// 进上下文的是折叠后的正文。
			Blocks: []Block{{Kind: KindText, Text: contextText}},
			// details 跟着结果落盘：待办清单就在里面，
			// 回放层靠它把清单复原出来——只记正文的话界面还原不出清单。
			Details:   details,
			CreatedAt: pkg.NowMs(),
		}
		if _, err := r.Session.AppendMessage(reply); err != nil {
			notify(h.OnError, pkg.As(err).Message)
			return
		}
	}
}

// drainQueue 把排队中的插话落进会话；返回是否还能继续跑。
// 没有排队就返回 false——没有新输入时循环必须停，
// 否则模型会对着同一段上下文反复自问自答。
func (r *Runner) drainQueue(ctx context.Context, h Handler) bool {
	r.mu.Lock()
	pending := r.queue
	r.queue = nil
	r.mu.Unlock()
	if len(pending) == 0 {
		return false
	}
	for _, msg := range pending {
		if ctx.Err() != nil {
			return false
		}
		if _, err := r.Session.AppendMessage(msg); err != nil {
			notify(h.OnError, pkg.As(err).Message)
			return false
		}
	}
	return true
}

// persistPartial 把中止时的半截回复落盘。
func (r *Runner) persistPartial(message Message, h Handler) {
	if len(message.Blocks) == 0 && message.Text() == "" {
		return
	}
	message.StopReason = "aborted"
	if _, err := r.Session.AppendMessage(message); err != nil {
		notify(h.OnError, pkg.As(err).Message)
		return
	}
	if h.OnAssistant != nil {
		h.OnAssistant(message)
	}
}

// history 把会话条目里的消息按顺序取出来，组成模型上下文。
func (r *Runner) history() []Message {
	entries := r.Session.Entries()
	out := make([]Message, 0, len(entries))
	for _, entry := range entries {
		if entry.EntryType != domain.SessionEntryMessage || entry.EntryMessage == nil {
			continue
		}
		if entry.EntryMessage.MessageRole == string(RoleSystem) {
			continue
		}
		out = append(out, MessageFromDO(entry.EntryMessage))
	}
	return out
}

// trimToWindow 在上下文超出模型窗口时丢掉最早的消息。
// 保留开头一条用户消息：它多半写着「这次要做什么」，
// 全丢光之后模型就不知道自己在干什么了。
func trimToWindow(messages []Message, contextWindow int, systemChars int) []Message {
	if contextWindow <= 0 || len(messages) == 0 {
		return messages
	}
	budget := contextWindow - systemChars/3 - 1024
	if budget <= 0 {
		budget = contextWindow / 2
	}
	total := 0
	for _, msg := range messages {
		total += estimateTokens(msg)
	}
	if total <= budget {
		return messages
	}
	// 从最早的消息开始丢，但始终留着最后一条（刚刚说的那句话）。
	cut := 0
	for cut < len(messages)-1 && total > budget {
		total -= estimateTokens(messages[cut])
		cut++
	}
	return messages[cut:]
}

// estimateTokens 粗估一条消息的 token 数。
// 精确计数要按模型分词器来，这里只需要一个「是否超窗」的判断：
// 中文约 1.5 字符/token，英文约 4 字符/token，取中间偏保守的 3。
func estimateTokens(msg Message) int {
	chars := 0
	for _, block := range msg.Blocks {
		chars += len([]rune(block.Text))
		chars += len(block.ToolArgs)
	}
	return chars/3 + 8
}

// notify 安全调用回调：回调为 nil 时什么都不做。
func notify(fn func(string), message string) {
	if fn != nil {
		fn(message)
	}
}

// notifyText 安全调用增量回调。
func notifyText(fn func(string), text string) {
	if fn != nil && strings.TrimSpace(text) != "" {
		fn(text)
	}
}
