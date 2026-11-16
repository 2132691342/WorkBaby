package agent

import (
	"context"
	"errors"
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
	// OnRetry 一次可重试失败后的退避等待：第几次、上限、等待毫秒
	OnRetry func(attempt, max int, delayMs int64)
	// OnRetryEnd 重试等待结束（重发或放弃）
	OnRetryEnd func()
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
	// Thinking 思考档：off 不发，low/medium/high 随请求原样透传
	Thinking string
	// MaxTokens 输出上限；0 表示用模型默认
	MaxTokens int
	// Temperature 采样温度
	Temperature *float64

	// mu 保护队列与状态
	mu sync.Mutex
	// queue 排队中的插话：当前这一轮收尾后注入
	queue []Message
	// followUp 追加队：本轮结束后由上层作为新消息再起一轮
	followUp []Message
	// steps 本次运行实际走的步数
	steps int
	// retries 流内失败已重试次数
	retries int
}

// Enqueue 排一条消息，等当前这一轮结束时注入。
func (r *Runner) Enqueue(msg Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queue = append(r.queue, msg)
}

// EnqueueFollowUp 排一条追加话：本轮收尾后由上层另起一轮投递。
func (r *Runner) EnqueueFollowUp(msg Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.followUp = append(r.followUp, msg)
}

// FollowUpQueued 取追加队并清空它：派发是「取走就发」，队列留在引擎里没有意义。
func (r *Runner) FollowUpQueued() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.followUp
	r.followUp = nil
	return out
}

// ClearQueued 清空两条排队。被清掉的话术就此消失：
// 队列里的消息还没进会话文件，清空不破坏「会话是唯一真相」。
func (r *Runner) ClearQueued() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queue = nil
	r.followUp = nil
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

// FollowUpTexts 取追加队的文本（界面显示用）。
func (r *Runner) FollowUpTexts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.followUp))
	for _, msg := range r.followUp {
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

		message, err := r.streamWithRetry(ctx, h)
		if err != nil {
			// 用户中止不是失败：静默走 aborted 收尾，别弹一句「context canceled」。
			if ctx.Err() == nil {
				notify(h.OnError, err.Error())
			}
			return
		}
		if ctx.Err() != nil {
			// 中止时把已经吐出来的部分落盘：用户看到过这些字，
			// 会话里没有它们就会「重开应用少了一段」。
			r.persistPartial(message, h)
			return
		}
		if message.StopReason == "error" {
			if !r.retryUpstreamError(ctx, h, message.ErrorMessage) {
				notify(h.OnError, message.ErrorMessage)
				return
			}
			continue
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
		if message.StopReason == "length" {
			// 输出被截断时入参是半截 JSON：执行它可能拿残缺参数写坏文件。
			r.failTruncatedCalls(calls, h)
			continue
		}
		r.runTools(ctx, calls, h)
		if ctx.Err() != nil {
			return
		}
		// 工具跑完必须回到循环顶部：下一轮由 history() 把工具结果喂回模型，
		// 模型才知道该说什么。工具结果已落进会话，这里只管把排队中的插话一并落进去。
		r.drainQueue(ctx, h)
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
		Thinking:    r.Thinking,
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
// 每个调用之间捞一次插话：用户说「别改那个文件」要能在下一个工具前生效。
func (r *Runner) runTools(ctx context.Context, calls []Block, h Handler) {
	for _, call := range calls {
		if ctx.Err() != nil {
			// 未执行的调用也要有结果：悬空的 tool_call 会让下一轮被上游整条拒掉。
			r.appendCallResult(call, Result{Text: "操作已中止，这个工具没有执行。", IsError: true}, h)
			continue
		}
		r.drainQueue(ctx, h)
		if h.OnToolStart != nil {
			h.OnToolStart(call.CallId, call.ToolName, call.ToolArgs)
		}
		started := time.Now()
		result := r.Registry.Run(ctx, call.ToolName, call.ToolArgs)
		elapsed := time.Since(started).Milliseconds()
		if h.OnToolEnd != nil {
			h.OnToolEnd(call.CallId, call.ToolName, result, elapsed)
		}
		if _, err := r.appendCallResult(call, result, h); err != nil {
			notify(h.OnError, pkg.As(err).Message)
			return
		}
	}
}

// appendCallResult 把一次工具结果落盘。全文进会话文件，折叠只发生在组装上下文时：
// 落盘就折的话，「展开看全文」在回放后只剩折叠文本，全文永久丢失。
func (r *Runner) appendCallResult(call Block, result Result, h Handler) (domain.SessionEntryDO, error) {
	details := result.Details
	if details == nil {
		details = map[string]any{}
	}
	if _, folded, originalChars := r.Registry.Tidy(result.Text); folded {
		details["tidied"] = true
		details["original_chars"] = originalChars
	}
	reply := Message{
		Role:       RoleTool,
		ToolCallId: call.CallId,
		ToolName:   call.ToolName,
		IsError:    result.IsError,
		Blocks:     []Block{{Kind: KindText, Text: result.Text}},
		// details 跟着结果落盘：待办清单就在里面，回放层靠它复原清单。
		Details:   details,
		CreatedAt: pkg.NowMs(),
	}
	return r.Session.AppendMessage(reply)
}

// failTruncatedCalls 输出被 max_tokens 截断时整批拒绝执行：
// 半截入参喂给写工具，代价可能是写坏一份文件。
func (r *Runner) failTruncatedCalls(calls []Block, h Handler) {
	for _, call := range calls {
		result := Result{Text: "上一条回复被输出上限截断，工具入参不完整，没有执行。请把这次调用重新发一遍。", IsError: true}
		if h.OnToolStart != nil {
			h.OnToolStart(call.CallId, call.ToolName, call.ToolArgs)
		}
		if h.OnToolEnd != nil {
			h.OnToolEnd(call.CallId, call.ToolName, result, 0)
		}
		if _, err := r.appendCallResult(call, result, h); err != nil {
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

// persistPartial 把中止时的半截回复落盘；带工具调用的半截消息一并补齐取消结果。
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
	for _, call := range message.ToolCalls() {
		result := Result{Text: "操作已中止，这个工具没有执行。", IsError: true}
		if _, err := r.appendCallResult(call, result, h); err != nil {
			notify(h.OnError, pkg.As(err).Message)
			return
		}
	}
}

// notify 安全调用回调：回调为 nil 时什么都不做。
func notify(fn func(string), message string) {
	if fn != nil {
		fn(message)
	}
}

// notifyText 安全调用增量回调。空白也是正文：换行与缩进丢了，直播就缺行缺格。
func notifyText(fn func(string), text string) {
	if fn != nil && text != "" {
		fn(text)
	}
}
