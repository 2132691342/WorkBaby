package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// 终止原因：统一出口，前端据此决定是否给「继续」入口。
const (
	ReasonEndTurn   = "end_turn"
	ReasonMaxTurns  = "max_turns"
	ReasonCancelled = "cancelled"
	ReasonBudget    = "budget_exceeded"
	ReasonError     = "error"
	ReasonStopped   = "stopped" // 钩子要求停止
)

// defaultMaxRetries 建流瞬时错误默认重试次数（不含首调）。
const defaultMaxRetries = 5

// DefaultMaxTurns 默认轮次上限。需要多步工具调用 + 中间纠错的任务，20 轮会在终态
// 收束前被砍断；100 轮给足余量，可由 Agent 定义或会话设置覆盖。
const DefaultMaxTurns = 100

// DefaultMaxRunTokens 默认 run 累计 token 预算（成本熔断）；<=0 表示不限。
// 桌面单用户场景里 10M tokens 远超一次"读 PPT 源文档→生成→审稿"的合理用量，
// 主要拦"模型进入死循环反复重发同一工具"的极端情况。
const DefaultMaxRunTokens = 10_000_000

// Config run 级配置；零值字段在 New 里补默认。
type Config struct {
	Model        string
	System       string
	MaxTurns     int // 轮次上限；0 取默认 DefaultMaxTurns
	Parallel     int // 只读工具并行度；0 取默认 4
	MaxInput     int // 上下文 token 预算（压缩触发线）；0 表示不压缩
	MaxRunTokens int // run 累计 token 预算（成本熔断）；<=0 取默认 DefaultMaxRunTokens
	MaxRetries   int // 建流瞬时错误最大重试次数；<=0 取默认 5
	Exec         ExecOptions
	Temperature  *float64
	MaxTokens    *int
	Thinking     *llm.ThinkingConfig
}

func (c Config) withDefaults() Config {
	if c.MaxTurns <= 0 {
		c.MaxTurns = DefaultMaxTurns
	}
	if c.Parallel <= 0 {
		c.Parallel = 4
	}
	if c.MaxRunTokens <= 0 {
		c.MaxRunTokens = DefaultMaxRunTokens
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = defaultMaxRetries
	}
	return c
}

// RequestParams 单次 run 的请求级采样参数，优先级高于全局默认。
type RequestParams struct {
	Temperature *float64
	Thinking    *llm.ThinkingConfig
}

// Meta run 身份信息，注入每个事件。
type Meta struct {
	RunID       string
	SessionID   string
	ParentRunID string
	Agent       string
}

// Hooks 可选扩展点。全部为 nil 时行为不变——扩展不得侵入主循环。
type Hooks struct {
	// BeforeTurn 每轮请求前改写消息序列（上下文裁剪、临时注入）。
	// 旧字段：与 TransformContext 二选一（TransformContext 优先）。
	BeforeTurn func(ctx context.Context, turn int, msgs []*llm.Message) []*llm.Message
	// AfterToolCall 工具执行后回调（落块、记忆抽取、审计）。
	AfterToolCall func(ctx context.Context, call Call, res tool.ToolResult)
	// ShouldStop 轮结束后询问是否提前终止。
	ShouldStop func(ctx context.Context, turn int) bool
	// Steering 轮间插话：跑过工具之后、下一轮之前注入（长任务中途纠偏）。
	// 旧字段：新代码改用 Loop.WithSteeringQueue。
	Steering func(ctx context.Context) []*llm.Message
	// FollowUp 收尾续接：本轮无工具调用、run 即将收尾时注入（自动接下一波）。
	// 旧字段：新代码改用 Loop.WithFollowUpQueue。
	FollowUp func(ctx context.Context) []*llm.Message
	// TransformContext（PI Phase 3）每轮请求前裁剪上下文（删空占位 / 孤儿 tool / 折叠历史）。
	// 失败 → finish(error)。
	TransformContext func(ctx context.Context, turn int, msgs []*llm.Message) ([]*llm.Message, error)
	// ConvertToLlm（PI Phase 3）协议边界归一：AgentMessage → LlmMessage。
	// 默认实现按 role 过滤 user/assistant/toolResult；未设置时走默认。
	ConvertToLlm func(ctx context.Context, msgs []*llm.Message) ([]*llm.Message, error)
	// GetAPIKey（PI Phase 3）每次请求前取 API key，支持过期刷新。
	// 未设置时用 provider 内置 key。
	GetAPIKey func(ctx context.Context, provider string) (string, error)
}

// Compressor 上下文压缩器：预算内返回原序列，超预算返回折叠后的序列。
type Compressor interface {
	Compress(msgs []*llm.Message, budget int) ([]*llm.Message, CompressInfo)
}

// Checkpoint 与 CheckpointStore 定义见 checkpoint.go。

// Outcome run 结果与新增消息；调用方负责落库。
type Outcome struct {
	Reason     string
	StopReason string
	Content    string         // 累积正文（多轮拼接）
	Thinking   string         // 累积推理
	Messages   []*llm.Message // 本 run 新增（assistant + tool），不含输入历史
	Usage      llm.TokenUsage // run 累计用量
	Turns      int
	PerTurn    []TurnUsage // 每轮用量明细：token_usages 逐调用落库的数据源
	Err        error       // 失败原因（Reason == ReasonError 时非 nil）
	Timings    Timings     // 分段耗时归因：回答「这次 run 慢在哪」
}

// Timings 分段耗时（毫秒）。三段互斥且近似穷举：等模型、跑工具、压缩上下文。
// 剩余差额是本机调度与落库开销——低到不值得单独归因。
type Timings struct {
	LLMMs      int64
	ToolsMs    int64
	CompressMs int64
}

// TurnUsage 单轮用量。
type TurnUsage struct {
	Turn      int
	Usage     llm.TokenUsage
	LatencyMs int64 // 本轮「建流→流结束」墙钟耗时
}

// === PI Phase 3 新增类型 ===

// NextTurnUpdate 下一轮准备位点：PrepareNextTurn 回调的返回值。
// 零值字段表示「保持上一轮设置」——调用方按需填充。
type NextTurnUpdate struct {
	Model         string               // 空 = 保持
	Thinking      *llm.ThinkingConfig  // nil = 保持
	ExtraMessages []*llm.Message       // 注入消息（与 steering/follow-up 互补）
	CompressInfo  *CompressInfo        // 压缩证据（与 chat:compressed 事件同源）
}

// LastTurnContext PrepareNextTurn 的入参：上一轮的快照。
type LastTurnContext struct {
	Turn       int
	StopReason string
	Content    string
	Thinking   string
	Usage      llm.TokenUsage
}

// QueueMode 队列消费模式（PI Phase 3）。
type QueueMode string

const (
	QueueOneAtATime QueueMode = "one-at-a-time" // 默认：每次 drain 取 1 条
	QueueAll        QueueMode = "all"            // 批量入队（多用户同步场景）
)

// MessageQueue 通用消息队列接口；steering / follow-up 共用。
type MessageQueue interface {
	Drain() []*llm.Message
	Enqueue(*llm.Message)
	HasItems() bool
	Mode() QueueMode
}

// SteeringQueue 轮间插话队列：内层每轮开调 Drain。
type FollowUpQueue MessageQueue

// queueAdapter 把函数式接口（Hooks.Steering / Hooks.FollowUp）包装成 MessageQueue，
// 保留旧用法零迁移成本。
type queueAdapter struct {
	mode QueueMode
	hook func(ctx context.Context) []*llm.Message
	buf  []*llm.Message
}

func (q *queueAdapter) Drain() []*llm.Message {
	if q.hook == nil {
		return nil
	}
	switch q.mode {
	case QueueAll:
		// 兼容旧 hook 直返消息：一次性取出
		msgs := q.hook(context.Background())
		return msgs
	default:
		if len(q.buf) == 0 {
			q.buf = q.hook(context.Background())
		}
		if len(q.buf) == 0 {
			return nil
		}
		head := q.buf[0]
		q.buf = q.buf[1:]
		return []*llm.Message{head}
	}
}

func (q *queueAdapter) Enqueue(m *llm.Message) {
	q.buf = append(q.buf, m)
}

func (q *queueAdapter) HasItems() bool {
	if len(q.buf) > 0 {
		return true
	}
	return q.hook != nil
}

func (q *queueAdapter) Mode() QueueMode { return q.mode }

// Loop ReAct 执行内核：PI Phase 3 拆 2 层循环（外 wait follow-up / 内 tool+steering）。
type Loop struct {
	provider llm.Provider
	registry *tool.Registry
	exec     Handler
	sink     Sink
	cfg      Config
	meta     Meta
	hooks    Hooks
	exposed  map[string]bool // nil = 全部已注册工具
	compress Compressor
	cp       CheckpointStore

	assistantMsgID string // 续跑落回同一条 assistant 消息，不新开
	steps          *MapSteps

	// retryPolicy 与 onRetry 共同决定建流重试：两者都不为 nil 时生效；
	// nil 时沿用旧行为（建流瞬时错误直接失败），兼容既有测试与未注入重试的场景。
	retryPolicy llm.RetryPolicy
	onRetry     func(attempt int, delay time.Duration)

	// PI Phase 3 新增
	prepareNextTurn func(ctx context.Context, last LastTurnContext) (NextTurnUpdate, error)
	steeringQueue   MessageQueue // 旧 Hooks.Steering 通过 queueAdapter 包装
	followUpQueue   FollowUpQueue // 旧 Hooks.FollowUp 同上
	getAPIKey       func(ctx context.Context, provider string) (string, error)
}

// New 构造 Loop；护栏链必须经 WithGuard 装配，未装配时 Run/Resume 直接失败（见 runLoop）。
func New(provider llm.Provider, reg *tool.Registry, cfg Config) *Loop {
	if reg == nil {
		reg = tool.NewRegistry()
	}
	return &Loop{provider: provider, registry: reg, cfg: cfg.withDefaults(), sink: NopSink{}}
}

// WithSink 设置事件出口。
func (l *Loop) WithSink(s Sink) *Loop {
	if s != nil {
		l.sink = s
	}
	return l
}

// WithMeta 设置 run 身份。
func (l *Loop) WithMeta(m Meta) *Loop { l.meta = m; return l }

// WithHooks 设置扩展点。
func (l *Loop) WithHooks(h Hooks) *Loop { l.hooks = h; return l }

// WithCompressor 设置压缩器。
func (l *Loop) WithCompressor(c Compressor) *Loop { l.compress = c; return l }

// WithCheckpoints 设置检查点存储。
func (l *Loop) WithCheckpoints(s CheckpointStore) *Loop { l.cp = s; return l }

// WithAssistantMessage 指定 assistant 消息 id：检查点据此落位，续跑不新开消息。
func (l *Loop) WithAssistantMessage(id string) *Loop { l.assistantMsgID = id; return l }

// WithSteps 注入步骤记忆（续跑复用已完成调用，不重放副作用）。
func (l *Loop) WithSteps(s *MapSteps) *Loop { l.steps = s; return l }

// StepsStore 返回步骤记忆的 StepStore 视图，供护栏（RepeatGuard）消费。
// Resume 会整体重建 l.steps，适配器每次调用解引用，始终看到当前实例。
func (l *Loop) StepsStore() StepStore { return loopSteps{l} }

// loopSteps StepStore 适配器：转发到 l.steps 当前实例（MapSteps 方法 nil 安全）。
type loopSteps struct{ l *Loop }

func (s loopSteps) Load(key string) (string, bool) { return s.l.steps.Load(key) }
func (s loopSteps) Store(key, content string)      { s.l.steps.Store(key, content) }

// WithRetry 注入建流重试策略与回调；policy 为零值或 onRetry 为 nil 时不重试。
// 每次进入下一次重试前会调 onRetry(attempt, delay)，编排层可借此发 chat:retry。
func (l *Loop) WithRetry(policy llm.RetryPolicy, onRetry func(attempt int, delay time.Duration)) *Loop {
	if onRetry == nil {
		return l
	}
	l.retryPolicy = policy
	l.onRetry = onRetry
	return l
}

// WithPrepareNextTurn（PI Phase 3）注入下一轮准备回调：每轮结束后触发，
// 返回 Model/Thinking/ExtraMessages/CompressInfo；零值字段表示「保持上一轮」。
// 同时挂载压缩 + 模型/思考档切换的统一位点。
func (l *Loop) WithPrepareNextTurn(h func(ctx context.Context, last LastTurnContext) (NextTurnUpdate, error)) *Loop {
	l.prepareNextTurn = h
	return l
}

// WithGetAPIKey（PI Phase 3）注入 API key 回调，每次请求前调用以支持过期刷新。
func (l *Loop) WithGetAPIKey(h func(ctx context.Context, provider string) (string, error)) *Loop {
	l.getAPIKey = h
	return l
}

// WithSteeringQueue（PI Phase 3）注入轮间插话队列。默认模式 QueueOneAtATime。
// 旧 Hooks.Steering 仍生效，但与本队列互斥：后注册者覆盖前者。
func (l *Loop) WithSteeringQueue(q MessageQueue) *Loop {
	if q != nil {
		l.steeringQueue = q
		l.hooks.Steering = nil // 显式清掉，避免两条路径并存
	}
	return l
}

// WithFollowUpQueue（PI Phase 3）注入收尾续接队列。默认模式 QueueOneAtATime。
// 旧 Hooks.FollowUp 仍生效，但与本队列互斥。
func (l *Loop) WithFollowUpQueue(q FollowUpQueue) *Loop {
	if q != nil {
		l.followUpQueue = q
		l.hooks.FollowUp = nil
	}
	return l
}

// WithGuard 组装护栏中间件链并生成最终执行器。
func (l *Loop) WithGuard(ms ...Middleware) *Loop {
	l.exec = Chain(ms...)(Executor(l.cfg.Exec))
	return l
}

// Expose 限制本轮暴露给模型的工具；不调用则暴露全部已注册工具。
func (l *Loop) Expose(names []string) *Loop {
	if len(names) == 0 {
		l.exposed = nil
		return l
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	l.exposed = m
	return l
}

// Run 从头执行主循环；ctx 取消归一为 cancelled。
func (l *Loop) Run(ctx context.Context, history []*llm.Message) (*Outcome, error) {
	msgs := make([]*llm.Message, 0, len(history)+1)
	if l.cfg.System != "" {
		msgs = append(msgs, llm.SystemMessage(l.cfg.System))
	}
	msgs = append(msgs, history...)
	return l.runLoop(ctx, msgs, 1, &Outcome{Reason: ReasonEndTurn}, true)
}

// Resume 从检查点续跑：只认最后一轮，已完成工具调用经幂等中间件复用，不重放副作用。
// 不重发 run 起始事件——「本次是续跑」由调用方标注。
func (l *Loop) Resume(ctx context.Context) (*Outcome, error) {
	if l.cp == nil {
		return nil, pkg.New(5004, "检查点未启用", "")
	}
	cp, err := l.cp.LoadLast(l.meta.RunID)
	if err != nil {
		return nil, pkg.Wrap(5004, "加载检查点失败", err)
	}
	if cp == nil || len(cp.Messages) == 0 {
		return nil, pkg.New(5004, "无可用检查点", l.meta.RunID)
	}
	// 已完成调用跨进程复用：检查点里存了哪些副作用已经发生过。
	l.steps = NewMapSteps(cp.Steps)
	// 用量跨段累计：续跑不是新的计费周期。
	return l.runLoop(ctx, cp.Messages, cp.Turn+1, &Outcome{
		Reason: ReasonEndTurn, Usage: cp.Usage, Content: cp.Content, Thinking: cp.Thinking,
	}, false)
}

// runLoop 主循环：请求 → 执行 → 回填，三件事一个 for。Run 与 Resume 共用。
func (l *Loop) runLoop(ctx context.Context, msgs []*llm.Message, startTurn int, out *Outcome, emitStart bool) (*Outcome, error) {
	if l.provider == nil {
		return nil, pkg.New(5001, "provider 未就绪", "")
	}
	// 未装配护栏链必须失败而非回落裸执行器：后者会绕过暴露清单、注入检测与审批，
	// 一次漏装 WithGuard 等于给模型开了全权限，而症状只在事后审计里可见。
	if l.exec == nil {
		return nil, pkg.New(5000, "run 未装配护栏链", l.meta.RunID)
	}
	if l.steps == nil {
		l.steps = NewMapSteps(nil)
	}
	// run 身份注入 ctx：工具与审计经 core.RunIDFromCtx / SessionIDFromCtx 取用。
	ctx = WithRunContext(ctx, l.meta.RunID, l.meta.SessionID)
	if emitStart {
		l.emit(EventRunStart, 0, RunStartPayload{Model: l.cfg.Model, Provider: l.provider.Name()})
	}

	// PI Phase 3：2 层循环
	//   outer: 收尾缝（follow-up → 续接下一波）
	//   inner: 单轮 ReAct（请求 → 工具 → 检点）
	turn := startTurn
	var lastTurn *turnResult

	// === Outer loop: 续接 follow-up 队列 ===
	for {
		// === Inner loop: per-turn ===
		exitToOuter := false
		for !exitToOuter {
			if err := ctx.Err(); err != nil {
				return l.finish(out, ReasonCancelled, ""), nil
			}
			if turn > l.cfg.MaxTurns {
				return l.finish(out, ReasonMaxTurns, ""), nil
			}
			// 成本熔断：与轮次上限分工——一个管「花多少钱」，一个管「跑多大」。
			if l.cfg.MaxRunTokens > 0 && out.Usage.TotalTokens >= l.cfg.MaxRunTokens {
				return l.finish(out, ReasonBudget, ""), nil
			}

			// [PI Phase 3] PrepareNextTurn：上一轮结束后触发，本轮开始前应用。
			if lastTurn != nil && l.prepareNextTurn != nil {
				update, err := l.prepareNextTurn(ctx, LastTurnContext{
					Turn:       lastTurn.turn,
					StopReason: lastTurn.StopReason,
					Content:    lastTurn.Content,
					Thinking:   lastTurn.Thinking,
					Usage:      lastTurn.Usage,
				})
				if err != nil {
					out.Err = err
					return l.finish(out, ReasonError, ""), err
				}
				if update.Model != "" {
					l.cfg.Model = update.Model
				}
				if update.Thinking != nil {
					l.cfg.Thinking = update.Thinking
				}
				if len(update.ExtraMessages) > 0 {
					msgs = append(msgs, update.ExtraMessages...)
				}
				if update.CompressInfo != nil && update.CompressInfo.Removed > 0 {
					l.emit(EventCompressed, turn, CompressedPayload{
						Removed: update.CompressInfo.Removed,
						Summary: update.CompressInfo.Summary,
						Truncated: update.CompressInfo.Truncated,
						CutoffAt: time.Now().UnixMilli(),
					})
				}
			}

			// [PI Phase 3] TransformContext（首选）vs BeforeTurn（旧字段）
			if l.hooks.TransformContext != nil {
				transformed, err := l.hooks.TransformContext(ctx, turn, msgs)
				if err != nil {
					out.Err = err
					return l.finish(out, ReasonError, ""), err
				}
				if transformed != nil {
					msgs = transformed
				}
			} else if l.hooks.BeforeTurn != nil {
				msgs = l.hooks.BeforeTurn(ctx, turn, msgs)
			}

			if l.compress != nil && l.cfg.MaxInput > 0 {
				c0 := time.Now()
				var info CompressInfo
				msgs, info = l.compress.Compress(msgs, l.cfg.MaxInput)
				out.Timings.CompressMs += time.Since(c0).Milliseconds()
				// 压缩是上下文被改写的少数时刻，不发事件用户只会觉得「前面的聊天不见了」。
				if info.Removed > 0 {
					l.emit(EventCompressed, turn, CompressedPayload{
						Removed: info.Removed, Summary: info.Summary, Truncated: info.Truncated,
						CutoffAt: time.Now().UnixMilli(),
					})
				}
			}

			// [PI Phase 3] ConvertToLlm：协议边界归一，仅作用于本轮发送。
			sendMsgs := msgs
			if l.hooks.ConvertToLlm != nil {
				converted, err := l.hooks.ConvertToLlm(ctx, msgs)
				if err != nil {
					out.Err = err
					return l.finish(out, ReasonError, ""), err
				}
				if converted != nil {
					sendMsgs = converted
				}
			}

			l.emit(EventTurnStart, turn, nil)
			res, err := l.requestTurn(ctx, turn, sendMsgs)
			if err != nil {
				out.Err = err
				return l.finish(out, ReasonError, ""), err
			}
			out.Turns = turn
			out.Timings.LLMMs += res.LatencyMs
			out.Content += res.Content
			out.Thinking += res.Thinking
			out.Usage = addUsage(out.Usage, res.Usage)
			out.PerTurn = append(out.PerTurn, TurnUsage{Turn: turn, Usage: res.Usage, LatencyMs: res.LatencyMs})
			out.StopReason = res.StopReason

			assistant := &llm.Message{
				Role:      llm.RoleAssistant,
				Content:   res.Content,
				Thinking:  res.Thinking,
				ToolCalls: res.ToolCalls(),
			}
			msgs = append(msgs, assistant)
			out.Messages = append(out.Messages, assistant)
			l.emit(EventTurnEnd, turn, TurnEndPayload{
				Usage:      usagePayload(res.Usage),
				ToolCalls:  len(res.ToolCallsRaw),
				StopReason: res.StopReason,
				LatencyMs:  res.LatencyMs,
			})

			res.turn = turn
			lastTurn = res

			if len(res.ToolCallsRaw) == 0 {
				exitToOuter = true
				continue
			}

			t0 := time.Now()
			toolMsgs, terminal := l.runTools(ctx, turn, res.ToolCallsRaw)
			out.Timings.ToolsMs += time.Since(t0).Milliseconds()
			msgs = append(msgs, toolMsgs...)
			out.Messages = append(out.Messages, toolMsgs...)
			// [PI Phase 3] drainSteering：steering 队列（兼容旧 Hooks.Steering）
			steer := l.drainSteering(ctx)
			if len(steer) > 0 {
				msgs = append(msgs, steer...)
				l.emit(EventQueueDrained, turn, QueueDrainedPayload{
					Queue: "steering", Count: len(steer),
					Mode: string(l.steeringMode()),
				})
			}

			l.checkpoint(turn, msgs, out)
			// 终局工具：本轮结果全部自述已是终局 → 没有待模型判断的内容，直接收尾。
			// 有插话时不能收尾：用户刚说的话必须被处理，否则会静默丢消息。
			if terminal && len(steer) == 0 {
				exitToOuter = true
				continue
			}
			if l.hooks.ShouldStop != nil && l.hooks.ShouldStop(ctx, turn) {
				return l.finish(out, ReasonStopped, res.StopReason), nil
			}
			turn++
		}

		// === Outer: 收尾缝 ===
		fu := l.drainFollowUp(ctx)
		if len(fu) == 0 {
			return l.finish(out, ReasonEndTurn, lastTurn.StopReason), nil
		}
		msgs = append(msgs, fu...)
		l.emit(EventQueueDrained, lastTurn.turn, QueueDrainedPayload{
			Queue: "follow_up", Count: len(fu),
			Mode: string(l.followUpMode()),
		})
		turn++
	}
}

// finish 统一出口：先发终态事件再返回。
func (l *Loop) finish(out *Outcome, reason, stopReason string) *Outcome {
	if stopReason != "" {
		out.StopReason = stopReason
	}
	out.Reason = reason
	l.emit(EventRunDone, out.Turns, RunDonePayload{
		Reason: reason, StopReason: out.StopReason, Usage: usagePayload(out.Usage), Turns: out.Turns,
	})
	return out
}

// drainSteering 轮间缝：跑过工具后取插话。优先队列；旧 Hooks.Steering 自动包装为兼容队列。
func (l *Loop) drainSteering(ctx context.Context) []*llm.Message {
	if l.steeringQueue != nil {
		return l.steeringQueue.Drain()
	}
	if l.hooks.Steering != nil {
		// 旧用法：函数式接口直返消息，按 one-at-a-time 模式处理。
		return l.hooks.Steering(ctx)
	}
	return nil
}

// drainFollowUp 收尾缝：无工具调用、run 即将收尾时取续接消息。优先队列。
func (l *Loop) drainFollowUp(ctx context.Context) []*llm.Message {
	if l.followUpQueue != nil {
		return l.followUpQueue.Drain()
	}
	if l.hooks.FollowUp != nil {
		return l.hooks.FollowUp(ctx)
	}
	return nil
}

// steeringMode 取当前 steering 队列模式（默认 one-at-a-time）。
func (l *Loop) steeringMode() QueueMode {
	if l.steeringQueue != nil {
		return l.steeringQueue.Mode()
	}
	return QueueOneAtATime
}

// followUpMode 取当前 follow-up 队列模式（默认 one-at-a-time）。
func (l *Loop) followUpMode() QueueMode {
	if l.followUpQueue != nil {
		return l.followUpQueue.Mode()
	}
	return QueueOneAtATime
}

// checkpoint 写入续跑位点；失败仅记事件，不阻断 run。
func (l *Loop) checkpoint(turn int, msgs []*llm.Message, out *Outcome) {
	if l.cp == nil {
		return
	}
	cp := &Checkpoint{
		RunID:          l.meta.RunID,
		SessionID:      l.meta.SessionID,
		Turn:           turn,
		AssistantMsgID: l.assistantMsgID,
		Messages:       msgs,
		Content:        out.Content,
		Thinking:       out.Thinking,
		Usage:          out.Usage,
		Steps:          l.steps.Snapshot(),
		CreatedAt:      time.Now().UnixMilli(),
	}
	if err := l.cp.Append(cp); err != nil {
		l.emit(EventError, turn, ErrorPayload{Code: 5003, Message: "检查点写入失败", Kind: "persist"})
		return
	}
	l.emit(EventCheckpoint, turn, CheckpointPayload{Turn: turn})
}

// runTools 执行本轮工具调用；批内任一工具声明 Sequential 则整批串行，
// 全部 Parallel（未声明视同 Parallel）且并行度 > 1 时信号量并发。
// 结果严格按调用顺序回填，保证 assistant(tool_calls) 与 tool 结果配对完整。
// 第二个返回值为「本轮是否全部终局」（见 tool.MetaTerminate）。
func (l *Loop) runTools(ctx context.Context, turn int, calls []llm.NormalizedToolCall) ([]*llm.Message, bool) {
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.Name)
	}
	parallel := l.cfg.Parallel > 1 && len(calls) > 1 && l.registry.AllParallel(names)

	results := make([]*llm.Message, len(calls))
	terminal := make([]bool, len(calls))
	if !parallel {
		for i, c := range calls {
			results[i], terminal[i] = l.runOne(ctx, turn, c)
		}
		return results, allTerminal(terminal)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, l.cfg.Parallel)
	for i, c := range calls {
		wg.Add(1)
		go func(i int, c llm.NormalizedToolCall) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i], terminal[i] = l.runOne(ctx, turn, c)
		}(i, c)
	}
	wg.Wait()
	return results, allTerminal(terminal)
}

// allTerminal 本轮是否每个结果都自述终局；空批次为 false（没有结果就没有终局依据）。
func allTerminal(flags []bool) bool {
	if len(flags) == 0 {
		return false
	}
	for _, f := range flags {
		if !f {
			return false
		}
	}
	return true
}

// runOne 执行一次调用：经护栏链 → 发事件 → 转成 tool 消息。
// 第二个返回值是该结果是否请求跳过下一轮模型调用。
func (l *Loop) runOne(ctx context.Context, turn int, tc llm.NormalizedToolCall) (*llm.Message, bool) {
	t, _ := l.registry.Get(tc.Name)
	call := Call{ID: tc.ID, Name: tc.Name, Args: tc.Arguments, Tool: t, RunID: l.meta.RunID, Turn: turn}

	l.emit(EventToolCall, turn, ToolCallPayload{
		ID: tc.ID, Name: tc.Name, Arguments: string(tc.Arguments), Activity: tool.ActivityOf(t, tc.Arguments),
	})
	l.emit(EventToolStart, turn, ToolCallPayload{
		ID: tc.ID, Name: tc.Name, Activity: tool.ActivityOf(t, tc.Arguments),
	})

	start := time.Now()
	res := l.exec(ctx, call)
	if l.hooks.AfterToolCall != nil {
		l.hooks.AfterToolCall(ctx, call, res)
	}

	errText := ""
	if res.Err != nil {
		errText = res.Err.Error()
	}
	l.emit(EventToolResult, turn, ToolResultPayload{
		ToolCallID: tc.ID, Name: tc.Name, Content: res.Content, Err: errText,
		DurationMs: time.Since(start).Milliseconds(), Meta: res.Meta, Data: res.Data,
		Refused: res.Refused, RefusedReason: res.Meta["refused_reason"],
		UIHint: tool.MetaOf(t).UIHint,
	})

	content := res.Content
	if res.Err != nil {
		// 错误回执做摘要：长栈截断，避免单条失败把上下文撑爆。
		// AdaptiveLoopGuard 附加的 [guard] 段必须保留，否则摘要会盖掉改道提示。
		hint := adaptiveHintFromContent(res.Content)
		content = summarizeToolError(call.Name, res.Err)
		if hint != "" {
			content = content + "\n\n" + hint
		}
	}
	return llm.ToolMessage(tc.ID, tc.Name, content), tool.IsTerminal(res)
}

// adaptiveHintFromContent 从 AdaptiveLoopGuard 已修改的 Content 里抽离 [guard] 段。
// 摘要在错误上重建内容时不能让改道提示消失。
func adaptiveHintFromContent(content string) string {
	const marker = "[guard]"
	i := strings.Index(content, marker)
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(content[i:])
}

// summarizeToolError 工具错误的标准化摘要：保留类型与关键信息，截断长栈。
// 优先走 AdaptiveLoopGuard 在 Meta 里注入的 same_failure_count，让模型看见
// 「已经失败 N 次了」是改道信号；正文只给一行关键错误 + 多行栈的前若干行。
func summarizeToolError(name string, err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	const stackLines = 4
	const maxRunes = 600
	lines := strings.Split(text, "\n")
	if len(lines) > stackLines {
		text = strings.Join(lines[:stackLines], "\n") + "\n... (truncated, " + strconv.Itoa(len(lines)-stackLines) + " more lines)"
	}
	runes := []rune(text)
	if len(runes) > maxRunes {
		text = string(runes[:maxRunes]) + "... (truncated)"
	}
	return "工具执行失败: " + name + " -> " + text
}

// streamWithRetry 包装 provider.Stream：瞬时错误按 policy 退避后重试，
// 每轮重试前调 onRetry 回调发 agent.retry 事件给编排层桥接 chat:retry。
// 未配置重试策略时回退到原行为（一次尝试、瞬时错误即失败）。
func (l *Loop) streamWithRetry(ctx context.Context, turn int, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// 含首调的总尝试次数；默认值来自 Config.withDefaults。
	maxAttempts := l.retryPolicy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = l.cfg.MaxRetries + 1
	}
	for attempt := 0; ; attempt++ {
		ch, err := l.provider.Stream(ctx, req)
		if err == nil {
			return ch, nil
		}
		if l.onRetry == nil || !llm.IsTransient(err) || attempt+1 >= maxAttempts {
			return nil, err
		}
		delay := l.retryPolicy.Backoff(attempt, llm.RetryAfter(err))
		l.onRetry(attempt+1, delay)
		if waitErr := llm.Wait(ctx, delay); waitErr != nil {
			return nil, waitErr
		}
	}
}

// requestTurn 单轮模型请求；流式消费 chunk 并实时发增量事件。
func (l *Loop) requestTurn(ctx context.Context, turn int, msgs []*llm.Message) (*turnResult, error) {
	req := &llm.ChatRequest{
		Model:       l.cfg.Model,
		Messages:    msgs,
		Tools:       l.toolDefs(),
		Temperature: l.cfg.Temperature,
		MaxTokens:   l.cfg.MaxTokens,
		Thinking:    l.cfg.Thinking,
		SessionID:   l.meta.SessionID,
	}
	started := time.Now()
	ch, err := l.streamWithRetry(ctx, turn, req)
	if err != nil {
		l.emit(EventError, turn, ErrorPayload{Code: 3005, Message: err.Error(), Kind: "connection"})
		return nil, pkg.Wrap(3005, "建流失败", err)
	}

	res := &turnResult{}
	for chunk := range ch {
		if chunk.Err != nil {
			l.emit(EventError, turn, ErrorPayload{Code: 3006, Message: chunk.Err.Error(), Kind: "upstream"})
			res.HadError = true
			if res.streamErr == nil {
				res.streamErr = chunk.Err
			}
			continue
		}
		if s := chunk.Delta.Content; s != "" {
			res.Content += s
			l.emit(EventTurnDelta, turn, DeltaPayload{Kind: "content", Text: s})
		}
		if s := chunk.Delta.Thinking; s != "" {
			res.Thinking += s
			l.emit(EventTurnThinking, turn, DeltaPayload{Kind: "thinking", Text: s})
		}
		if chunk.ToolCall != nil {
			res.ToolCallsRaw = append(res.ToolCallsRaw, *chunk.ToolCall)
		}
		if chunk.FinalUsage != nil {
			res.Usage = *chunk.FinalUsage
		}
		if chunk.FinishReason != nil {
			res.StopReason = *chunk.FinishReason
		}
	}
	// 延迟必须覆盖消费全程：在建流返回时就取值会漏掉整段流式生成时间，
	// 仪表盘上的「等模型」永远偏小，据此做的耗时归因全部失真。
	res.LatencyMs = time.Since(started).Milliseconds()
	// 只报错、一个 token 都没产出的流不能算成功轮：否则空 assistant 会被回填进历史，
	// 上游下一轮直接 400（空 assistant + 工具结果不配对），而 run 却表现为正常收尾。
	if res.HadError && res.empty() {
		return nil, pkg.Wrap(3006, "上游返回错误且本轮无输出", res.streamErr)
	}
	return res, nil
}

// toolDefs 生成暴露给模型的工具定义。
func (l *Loop) toolDefs() []llm.ToolDefinition {
	all := l.registry.List()
	defs := make([]llm.ToolDefinition, 0, len(all))
	for _, t := range all {
		if l.exposed != nil && !l.exposed[t.Name()] {
			continue
		}
		s := t.Schema()
		var params map[string]any
		if len(s.Parameters) > 0 {
			_ = json.Unmarshal(s.Parameters, &params)
		}
		defs = append(defs, llm.ToolDefinition{Name: s.Name, Description: s.Description, Parameters: params})
	}
	return defs
}

// emit 注入 run 身份后推送事件。
func (l *Loop) emit(kind EventKind, turn int, payload any) {
	l.sink.Emit(Event{
		Kind: kind, RunID: l.meta.RunID, ParentRunID: l.meta.ParentRunID,
		SessionID: l.meta.SessionID, Turn: turn, Agent: l.meta.Agent, Payload: payload,
	})
}

// turnResult 单轮模型输出。
type turnResult struct {
	turn         int    // 该 turn 的轮次号；PrepareNextTurn 入参用
	Content      string
	Thinking     string
	ToolCallsRaw []llm.NormalizedToolCall
	Usage        llm.TokenUsage
	StopReason   string
	LatencyMs    int64
	HadError     bool
	streamErr    error // 首个 chunk 级错误，用于「空轮 + 报错」判定
}

// empty 本轮是否毫无产出（无正文、无思考、无工具调用）。
func (r *turnResult) empty() bool {
	return r.Content == "" && r.Thinking == "" && len(r.ToolCallsRaw) == 0
}

// ToolCalls 转成 assistant 消息的 ToolCall 结构。
func (r *turnResult) ToolCalls() []llm.ToolCall {
	if len(r.ToolCallsRaw) == 0 {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(r.ToolCallsRaw))
	for _, c := range r.ToolCallsRaw {
		out = append(out, llm.ToolCall{
			ID: c.ID, Type: "function",
			Function: llm.FunctionCall{Name: c.Name, Arguments: string(c.Arguments)},
		})
	}
	return out
}

// usagePayload 转事件载荷。
func usagePayload(u llm.TokenUsage) UsagePayload {
	return UsagePayload{
		Input: u.InputTokens, Output: u.OutputTokens,
		CacheRead: u.CacheReadTokens, CacheWrite: u.CacheWriteTokens, Total: u.TotalTokens,
	}
}
