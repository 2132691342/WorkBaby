package service

import (
	"context"
	"runtime/debug"
	"strings"
	"sync"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/llm"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/tool"
)

// ChatService 编排一次对话：装配内核、映射事件、落库条目。
type ChatService struct {
	env       *Env
	sessions  *SessionService
	approvals *ApprovalService
	providers *ProviderService

	mu     sync.Mutex
	locks  map[string]*sync.Mutex
	cancel map[string]context.CancelFunc
	queues map[string]*agent.Queue
	reads  map[string]*readTracker
}

// NewChatService 构造对话服务。
func NewChatService(env *Env, sessions *SessionService, approvals *ApprovalService, providers *ProviderService) *ChatService {
	return &ChatService{
		env: env, sessions: sessions, approvals: approvals, providers: providers,
		locks: map[string]*sync.Mutex{}, cancel: map[string]context.CancelFunc{},
		queues: map[string]*agent.Queue{}, reads: map[string]*readTracker{},
	}
}

// Send 发一条消息：落 user 条目后起后台 goroutine 跑内核，立即返回 runID。
func (c *ChatService) Send(sessionID, content string, attachments []domain.AttachmentREQ) (*domain.SendMessageRESP, error) {
	content = strings.TrimSpace(content)
	if content == "" && len(attachments) == 0 {
		return nil, domain.ErrEmptyContent
	}
	sess, err := c.env.Repo.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	lock := c.lockOf(sessionID)
	lock.Lock()
	defer lock.Unlock()

	if c.busy(sessionID) {
		return nil, domain.ErrSessionBusy
	}

	content, injected := c.resolveSkill(content)
	body, images := buildAttachment(sess.Workspace, attachments)
	// 带图片就必须是识图模型：不拦截的话图片会被静默丢掉，用户以为发过去了
	if len(images) > 0 {
		cap := c.capabilityOf(sess.ProviderID, sess.Model)
		if !cap.Vision {
			return nil, pkg.New(3113, "当前模型不支持识图，换一个支持视觉的模型后再发图片", sess.Model)
		}
	}
	userEntry := &domain.EntryDO{
		ID:        pkg.NewID(domain.PrefixEntry),
		SessionID: sessionID,
		ParentID:  sess.LeafEntryID,
		Type:      domain.EntryTypeMessage,
		Role:      domain.RoleUser,
		PayloadJSON: payloadOf(llm.Message{
			Role: llm.RoleUser, Content: body + injected, Images: images,
		}, "", 0),
	}
	if err := c.sessions.Append(userEntry); err != nil {
		return nil, err
	}
	c.autoTitle(sess, content)

	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancel[sessionID] = cancel
	c.mu.Unlock()

	runID := pkg.NewTraceID()
	cap := c.capabilityOf(sess.ProviderID, sess.Model)
	pkg.Infof("chat: run 开始 session=%s run=%s model=%s provider=%s tools=%d window=%d(known=%v)",
		sessionID, runID, sess.Model, sess.ProviderID,
		len(c.env.Registry.Enabled(c.disabledTools())), cap.ContextWindow, cap.Known)
	go c.run(ctx, runID, sessionID)

	return &domain.SendMessageRESP{RunID: runID, EntryID: userEntry.ID, SessionID: sessionID}, nil
}

// Stop 停止当前 run：取消 ctx，已产出的条目照常保留。
func (c *ChatService) Stop(sessionID string) error {
	c.mu.Lock()
	cancel := c.cancel[sessionID]
	c.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	c.approvals.CancelSession(sessionID)
	pkg.Infof("chat: 收到停止请求 session=%s", sessionID)
	// 立刻广播停止：内核的收尾要走完落库与 done，用户不该靠猜才知道点没点着。
	c.env.Emitter.Emit(sessionID, domain.EventChatStopped, domain.StoppedData{Reason: "user"})
	return nil
}

// Steer 插话：下一轮上下文里立刻生效，打断当前工作。
func (c *ChatService) Steer(sessionID, content string) error {
	return c.enqueue(sessionID, content)
}

// FollowUp 排队：与插话共用一个队列，同样在轮间注入。
// 空闲时调用等价于「下次发送时带上这句」，不需要单独唤醒 run。
func (c *ChatService) FollowUp(sessionID, content string) error {
	return c.enqueue(sessionID, content)
}

func (c *ChatService) enqueue(sessionID, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return domain.ErrEmptyContent
	}
	// 会话锁保证「判忙」与 Send 不交错：要么赶在 run 结束前排进队列，
	// 要么确认空闲后直接落库，两条路都恰好持久化一次。
	lock := c.lockOf(sessionID)
	lock.Lock()
	defer lock.Unlock()
	c.mu.Lock()
	busy := c.cancel[sessionID] != nil
	c.mu.Unlock()
	if !busy {
		// 空闲时没有 run 来消费队列：直接落库进历史，下次发送模型自然看得到。
		// 不入队，否则下次 run 首轮 drain 会把同一句再注入一次。
		c.appendSteered(sessionID, content)
		return nil
	}
	if !c.queueOf(sessionID).Enqueue(llm.Message{Role: llm.RoleUser, Content: content}) {
		return pkg.New(5101, "插话排得太多了，等助手处理完这批再说", "")
	}
	return nil
}

// flushQueueLocked 把队列里没被消费的消息补落库；调用方必须已持有会话锁。
// run 收尾与「判忙后入队」恰好交错时，那句消息既没进上下文也没落库。
func (c *ChatService) flushQueueLocked(sessionID string) {
	q := c.queueOf(sessionID)
	if q == nil || !q.HasItems() {
		return
	}
	for _, m := range q.Drain() {
		if text := strings.TrimSpace(m.Content); text != "" {
			c.appendSteered(sessionID, text)
		}
	}
}

// Forget 清掉一个会话的全部运行态：取消句柄、插话队列与读账本。
// 删会话时不做这件事，正在跑的 run 会继续往已删会话写孤儿条目，几个 map 也只增不减。
func (c *ChatService) Forget(sessionID string) {
	c.mu.Lock()
	cancel := c.cancel[sessionID]
	delete(c.cancel, sessionID)
	delete(c.queues, sessionID)
	delete(c.reads, sessionID)
	// locks 故意保留：删掉后新建的锁与在跑 run 持有的那把不同，同一会话出现两把锁，
	// enqueue 依赖的串行化前提就没了（每会话一个 mutex 的内存可忽略）。
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if c.approvals != nil {
		c.approvals.CancelSession(sessionID)
		c.approvals.ForgetSession(sessionID)
	}
	// seq 与 SSE 重放窗口同样按会话记账：只删库不清这里，每建一个会话就留一段无人认领的内存。
	c.env.Emitter.Forget(sessionID)
}

// appendSteered 把插话落一条 user 条目并广播，返回条目 id（失败返回空串），
// 供 run 内的调用方同步落库位点。
func (c *ChatService) appendSteered(sessionID, content string) string {
	sess, err := c.env.Repo.GetSession(sessionID)
	if err != nil {
		return ""
	}
	entry := &domain.EntryDO{
		ID:        pkg.NewID(domain.PrefixEntry),
		SessionID: sessionID,
		ParentID:  sess.LeafEntryID,
		Type:      domain.EntryTypeMessage,
		Role:      domain.RoleUser,
		PayloadJSON: payloadOf(llm.Message{
			Role: llm.RoleUser, Content: content,
		}, "", 0),
	}
	if err := c.sessions.Append(entry); err != nil {
		pkg.Warnf("chat: 落插话条目失败: %v", err)
		return ""
	}
	c.env.Emitter.Emit(sessionID, domain.EventChatUser, domain.UserData{
		EntryID: entry.ID, Content: content,
	})
	return entry.ID
}

// run 是一次 run 的完整生命周期。
func (c *ChatService) run(ctx context.Context, runID, sessionID string) {
	defer func() {
		// 兜住内核之外的意外：桌面端不该因为一次对话里的 bug 把整进程带走。
		if rec := recover(); rec != nil {
			pkg.Errorf("chat: run 内部崩溃 session=%s run=%s: %v\n%s", sessionID, runID, rec, debug.Stack())
			c.fail(sessionID, nil, nil, pkg.New(5102, "助手内部出错了，已经停下来；可以点重试再来一次", ""))
		}
		// 「摘取消句柄」与「补落队列」必须在同一把会话锁里一次做完：
		// 两步之间 enqueue 会看到 !busy 直接落库，与 flushQueue 并发写同一条会话链。
		lock := c.lockOf(sessionID)
		lock.Lock()
		c.mu.Lock()
		cancel := c.cancel[sessionID]
		delete(c.cancel, sessionID)
		c.mu.Unlock()
		// 收尾窗口里入队的插话没人消费了：补落库，否则这句凭空消失。
		c.flushQueueLocked(sessionID)
		lock.Unlock()
		// 只摘句柄不调用 = 每个正常结束的 run 泄漏一个 ctx 及其整棵子树
		//（HTTP 连接、重试 goroutine 全挂在它下面）。
		if cancel != nil {
			cancel()
		}
	}()

	c.env.Emitter.Emit(sessionID, domain.EventChatStart, domain.StartData{RunID: runID, SessionID: sessionID})

	sess, err := c.env.Repo.GetSession(sessionID)
	if err != nil {
		c.fail(sessionID, nil, nil, err)
		return
	}
	// 存量会话可能是在「只存服务、不存模型」时代建的，模型名是空的。
	// 这里就地补齐并回写，否则界面永远显示「默认模型」、能力也查不出来。
	sess = c.withModel(sess)
	streamer, model, err := c.providers.Streamer(sess.ProviderID, sess.Model)
	if err != nil {
		c.fail(sessionID, nil, nil, err)
		return
	}
	history, err := c.sessions.History(sessionID)
	if err != nil {
		c.fail(sessionID, nil, nil, err)
		return
	}
	tools := c.env.Registry.Enabled(c.disabledTools())
	system := BuildSystem(c.env, tools, sess.Workspace)
	// 上游空闲超时是运行期可改的设置，装配时同步一次。
	c.applyStreamIdle()

	deps := c.env.ToolDepsSnapshot()
	deps.Reads = c.readsOf(sessionID)
	permission := sess.Permission
	cap := c.capabilityOf(sess.ProviderID, model)
	state := &runState{sessionID: sessionID, providerID: sess.ProviderID, model: model,
		windowKnown: cap.Known, leafID: sess.LeafEntryID}
	temp, topP, maxOutput := c.samplingOf(cap, sess.ProviderID, model)
	state.maxTokens = maxOutput
	budget := c.budget(cap, system)
	// 把真正要下发的数字落进日志：截断类问题的第一步永远是「当时给的预算是多少」，
	// 没有这行就只能靠反推配置。
	pkg.Infof("chat: 装配 session=%s run=%s model=%s max_tokens=%d reserve=%d window=%d(known=%v) temp=%s parallel=%d",
		sessionID, runID, model, maxOutput, budget.Reserve, cap.ContextWindow, cap.Known,
		fmtPtr(temp), c.toolParallel())

	loop := agent.New(agent.Config{
		Streamer:    streamer,
		Tools:       tools,
		Deps:        deps,
		Workspace:   sess.Workspace,
		System:      system,
		Model:       model,
		MaxTokens:   maxOutput,
		Temperature: temp,
		TopP:        topP,
		MaxTurns:    c.maxTurns(),
		Parallel:    c.toolParallel(),
		Budget:      budget,
		Emit:        func(e agent.Event) { c.onEvent(state, e) },
		Steering:    c.queueOf(sessionID),
		Gate: func(ctx context.Context, call *llm.ToolCall) (bool, string) {
			t, ok := c.env.Registry.Get(call.Name)
			if !ok {
				return true, "没有这个工具"
			}
			return c.approvals.Gate(ctx, sessionID, permission, t, domain.ApprovalCallDTO{
				ID: call.ID, Args: call.Args,
				Risk: riskOfCommand(t, call.Args), Reason: reasonOf(t, call.Args),
			})
		},
	}, history)

	res, err := loop.Run(ctx)
	if err != nil {
		c.fail(sessionID, state, res, err)
		return
	}

	c.appendAssistant(state, res)
	c.recordUsage(sessionID, sess.ProviderID, model, state.turnContext, res.Usage)

	usage := domain.UsageVO{
		Input: res.Usage.Input, Output: res.Usage.Output, Cached: res.Usage.Cached,
		Total: res.Usage.Total, Context: state.turnContext, LatencyMs: res.Usage.LatencyMs,
	}
	c.env.Emitter.Emit(sessionID, domain.EventChatDone, domain.DoneData{
		EntryID: state.lastEntryID, StopReason: res.StopReason, Usage: &usage,
		MaxTokens: state.maxTokens,
	})
	pkg.Infof("chat: run 结束 session=%s run=%s stop=%s turns=%d tokens=%d",
		sessionID, runID, res.StopReason, res.Turns, res.Usage.Total)
}

// withModel 给模型名为空的会话补上默认模型，并把结果回写进会话。
// 只读不写的话，界面每次刷新都还是「默认模型」，用户会以为自己没设过模型。
func (c *ChatService) withModel(sess *domain.SessionDO) *domain.SessionDO {
	if sess.Model != "" {
		return sess
	}
	providerID := sess.ProviderID
	if providerID == "" {
		providerID, _ = c.env.Repo.GetSetting(domain.SettingDefaultProvider)
	}
	model, _ := c.env.Repo.GetSetting(domain.SettingDefaultModel)
	if providerID == "" || model == "" {
		return sess
	}
	_ = c.env.Repo.UpdateSessionColumns(sess.ID, map[string]any{
		"provider_id": providerID, "model": model,
	})
	sess.ProviderID = providerID
	sess.Model = model
	return sess
}

// runState 收集一次 run 过程中的落库位点。
type runState struct {
	sessionID   string
	providerID  string
	model       string
	windowKnown bool
	maxTokens   int
	turnEntryID string
	lastEntryID string
	// leafID 是跟着 Append 推进的落库父位点：记住它就不必每条消息回查会话。
	leafID string
	// pending / thinking 用 Builder：本轮正文按 token 累加，走 `+=` 是每 token
	// 复制一遍整段，长回答末尾就是 O(n²)。
	pending     strings.Builder
	thinking    strings.Builder
	hasContent  bool
	toolCalls   []llm.ToolCall
	stopReason  string
	turnUsage   *llm.Usage
	turnContext int
	startedAt   int64
}

// onEvent 把内核事件映射成前端事件并落库。
func (c *ChatService) onEvent(st *runState, e agent.Event) {
	switch e.Kind {
	case agent.EventTurnStart:
		st.turnEntryID = pkg.NewID(domain.PrefixEntry)
		st.pending.Reset()
		st.thinking.Reset()
		st.hasContent = false
		st.toolCalls = nil
		st.startedAt = nowMillis()
	case agent.EventDelta:
		// 声明可能已在工具开始时落库并清空了 turnEntryID，此处正文又来了：
		// 补一个新的 id，让本轮剩余正文在 turn_end 能单独落一条。
		if st.turnEntryID == "" {
			st.turnEntryID = pkg.NewID(domain.PrefixEntry)
		}
		if e.DeltaKind == agent.DeltaThinking {
			st.thinking.WriteString(e.Delta)
		} else {
			st.pending.WriteString(e.Delta)
			st.hasContent = true
		}
		c.env.Emitter.Emit(st.sessionID, domain.EventChatDelta, domain.DeltaData{
			EntryID: st.turnEntryID, Kind: e.DeltaKind, Delta: e.Delta,
		})
	case agent.EventToolStart:
		if e.ToolCall == nil {
			return
		}
		// 只收集、不落库：逐条落会把同轮声明拆成多条 assistant，历史回放时
		// 上游对「结果找不到紧邻的声明」直接 400。声明在第一个结果到达前合并落。
		st.toolCalls = append(st.toolCalls, llm.ToolCall{
			ID: e.ToolCall.ID, Name: e.ToolCall.Name, Label: e.ToolTitle, Args: e.ToolCall.Args,
		})
		c.env.Emitter.Emit(st.sessionID, domain.EventChatToolStart, domain.ToolStartData{
			ToolCallID: e.ToolCall.ID, Tool: e.ToolCall.Name,
			Label: e.ToolTitle, Args: e.ToolCall.Args,
		})
	case agent.EventToolEnd:
		if e.ToolCall == nil {
			return
		}
		// 结果落库前先把声明合并落一条：上游要求 assistant(tool_calls) 紧邻
		// 它自己的 tool 结果，声明晚于结果或声明被拆散都会被 400 拒绝。
		c.appendAssistantTurn(st)
		c.appendTool(st, e)
	case agent.EventSteering:
		// 插话在注入时刻落库（早写撕裂协议配对、晚写链序失真），落库位点必须跟着推进——
		// 不推的话下一轮 assistant 会挂到分叉上，插话从链里消失。
		if id := c.appendSteered(st.sessionID, e.UserContent); id != "" {
			st.leafID = id
		}
	case agent.EventTurnEnd:
		// 本轮用量随 turn_end 到，落在这一轮最后写入的那条 assistant 上。
		st.turnUsage = e.Usage
		st.turnContext = e.ContextTokens
		c.appendAssistantTurn(st)
	case agent.EventCompressed:
		c.env.Emitter.Emit(st.sessionID, domain.EventChatCompressed, domain.CompressedData{
			TokensBefore: e.TokensBefore, TokensAfter: e.TokensAfter,
		})
	case agent.EventContext:
		c.env.Emitter.Emit(st.sessionID, domain.EventChatContext, domain.ContextData{
			Used: e.TokensUsed, Window: e.TokensWindow, Known: st.windowKnown,
			Ratio: ratioOf(e.TokensUsed, e.TokensWindow, e.TokensReserve), Reserve: e.TokensReserve,
		})
	case agent.EventAgentEnd:
		st.stopReason = e.StopReason
	}
}

// firstLine 取首行并截断，供日志记录失败原因：工具输出可能很长，也可能带上文件内容，
// 日志只留能定位问题的那一句。
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200]) + "…"
	}
	return s
}

// ratioOf 换算成整数百分比，分母是可用预算（窗口减输出余量），与内核压缩的触发线同口径：
// 水位环到 100% 就是「下一轮要整理」，拿窗口当分母会慢半拍（整理已发生，环上才 90%）。
func ratioOf(used, window, reserve int) int {
	usable := usableWindow(window, reserve)
	if usable <= 0 || used <= 0 {
		return 0
	}
	r := used * 100 / usable
	if r > 100 {
		return 100
	}
	return r
}

// usableWindow 是「还能塞多少」：窗口减去输出余量；余量吃掉整个窗口时退到一半，
// 与 agent.Compact 的兜底同一条规则——两处不一致时读数与实际触发点会错开。
func usableWindow(window, reserve int) int {
	if window <= 0 {
		return 0
	}
	if window-reserve <= 0 {
		return window / 2
	}
	return window - reserve
}

// leafOf 返回当前落库父位点：正常路径直接用 runState 记的位点，
// 拿不到才回查一次会话；仍拿不到说明会话已不在，没有挂靠点。
func (c *ChatService) leafOf(st *runState) string {
	if st.leafID != "" {
		return st.leafID
	}
	sess, err := c.env.Repo.GetSession(st.sessionID)
	if err != nil {
		return ""
	}
	st.leafID = sess.LeafEntryID
	return st.leafID
}

// appendAssistantTurn 落本轮的 assistant 条目；空消息不落，避免下一轮上游 400。
func (c *ChatService) appendAssistantTurn(st *runState) {
	if !st.hasContent && st.thinking.Len() == 0 && len(st.toolCalls) == 0 {
		st.turnEntryID = ""
		return
	}
	// 声明可能已在工具开始时落过一轮，这里再落时必须换一个 id：
	// 沿用已入库的旧 id 会撞主键约束，那条 assistant 消息就永远丢了。
	if st.turnEntryID == "" {
		st.turnEntryID = pkg.NewID(domain.PrefixEntry)
	}
	parent := c.leafOf(st)
	if parent == "" {
		return
	}
	entry := &domain.EntryDO{
		ID:        st.turnEntryID,
		SessionID: st.sessionID,
		ParentID:  parent,
		Type:      domain.EntryTypeMessage,
		Role:      domain.RoleAssistant,
		PayloadJSON: payloadOf(llm.Message{
			Role: llm.RoleAssistant, Content: st.pending.String(), Thinking: st.thinking.String(),
			ToolCalls: st.toolCalls,
		}, st.stopReason, nowMillis()-st.startedAt),
	}
	if st.turnUsage != nil {
		entry.UsageJSON = usageJSONOf(st.turnUsage, st.turnContext)
	}
	if err := c.sessions.Append(entry); err != nil {
		pkg.Warnf("chat: 落 assistant 条目失败: %v", err)
		return
	}
	st.lastEntryID = entry.ID
	st.leafID = entry.ID
	st.turnEntryID = ""
	// 已落库的内容清空：下一段增量要作为独立的一条，不能与这条重复。
	st.pending.Reset()
	st.thinking.Reset()
	st.hasContent = false
	st.toolCalls = nil
}

// appendTool 落一条工具结果条目并按序接到树上。
func (c *ChatService) appendTool(st *runState, e agent.Event) {
	parent := c.leafOf(st)
	if parent == "" {
		return
	}
	entry := &domain.EntryDO{
		ID:        pkg.NewID(domain.PrefixEntry),
		SessionID: st.sessionID,
		ParentID:  parent,
		Type:      domain.EntryTypeMessage,
		Role:      domain.RoleTool,
		PayloadJSON: payloadOf(llm.Message{
			Role: llm.RoleTool, ToolCallID: e.ToolCall.ID,
			Content: e.ToolOutput, IsError: !e.ToolOK,
		}, "", e.DurationMs, e.ToolCall.Name),
	}
	if err := c.sessions.Append(entry); err != nil {
		pkg.Warnf("chat: 落工具条目失败: %v", err)
		return
	}
	st.lastEntryID = entry.ID
	st.leafID = entry.ID
	if e.ToolOK {
		pkg.Infof("chat: 工具完成 session=%s tool=%s cost=%dms",
			st.sessionID, e.ToolCall.Name, e.DurationMs)
	} else {
		// 失败必须带原因：只写 ok=false 的话，事后翻日志查不出「为什么失败」。
		pkg.Warnf("chat: 工具失败 session=%s tool=%s cost=%dms reason=%s",
			st.sessionID, e.ToolCall.Name, e.DurationMs, firstLine(e.ToolOutput))
	}
	c.env.Emitter.Emit(st.sessionID, domain.EventChatToolEnd, domain.ToolEndData{
		ToolCallID: e.ToolCall.ID, OK: e.ToolOK,
		Title: e.ToolTitle, Output: e.ToolOutput, DurationMs: e.DurationMs,
	})
}

// appendAssistant 收尾时补落最后一轮（正常结束路径下 turn_end 已落过，这里只兜底）。
func (c *ChatService) appendAssistant(st *runState, res *agent.Result) {
	if st.turnEntryID == "" {
		return
	}
	st.stopReason = res.StopReason
	if !st.hasContent && st.thinking.Len() == 0 {
		st.turnEntryID = ""
		return
	}
	c.appendAssistantTurn(st)
}

// fail 统一错误出口：已产出的内容不回滚，用户能看到半成品。
// 错误轮没有 turn_end，这里兜底落库并记账，否则界面上看得到的内容刷新后消失。
func (c *ChatService) fail(sessionID string, st *runState, res *agent.Result, err error) {
	if st != nil {
		st.stopReason = agent.StopError
		c.appendAssistantTurn(st)
		if res != nil && (res.Usage.Input > 0 || res.Usage.Output > 0) {
			c.recordUsage(sessionID, st.providerID, st.model, st.turnContext, res.Usage)
		}
	}
	// 带上 code 与会话，用户截图里就能直接定位到是哪一类失败。
	pkg.Errorf("chat: run 失败 session=%s code=%d err=%v", sessionID, pkg.CodeOf(err), err)
	c.env.Emitter.Emit(sessionID, domain.EventChatError, domain.ErrorData{
		Code: pkg.CodeOf(err), Message: err.Error(),
	})
}

// recordUsage 落一行用量并累计进会话总额，正常与错误两条路径共用。
func (c *ChatService) recordUsage(sessionID, providerID, model string, ctxTokens int, u llm.Usage) {
	// 先确认会话还在：删会话与 run 收尾竞态的窗口里先落账，会留下永远没人认领的
	// 孤儿用量行，仪表盘按天聚合会把它算进去。
	if _, err := c.env.Repo.GetSession(sessionID); err != nil {
		pkg.Warnf("chat: 取会话失败 session=%s: %v", sessionID, err)
		return
	}
	// 记账失败不能让这次对话失败，但必须留下痕迹：整个仪表盘的数据全靠这张表，
	// 静默失败的表现是「用了很久，仪表盘一直是 0」。
	if err := c.env.Repo.AddUsage(&domain.TokenUsageDO{
		ID:        pkg.NewID(domain.PrefixUsage),
		SessionID: sessionID,
		Provider:  providerID,
		Model:     model,
		Input:     u.Input,
		Output:    u.Output,
		Cached:    u.Cached,
		Total:     u.Total,
		Context:   ctxTokens,
		LatencyMs: u.LatencyMs,
	}); err != nil {
		pkg.Warnf("chat: 用量落账失败 session=%s: %v", sessionID, err)
	}
	if err := c.env.Repo.AddSessionTokens(sessionID, u.Total); err != nil {
		pkg.Warnf("chat: 累计会话 token 失败 session=%s: %v", sessionID, err)
	}
}

// resolveSkill 把「/skill:名字 需求」展开成带正文的用户消息。
// 名字只取第一个空白前的整段：后面跟的具体要求原样保留，作为正文之外的补充指令。
func (c *ChatService) resolveSkill(content string) (string, string) {
	if !strings.HasPrefix(content, "/") || c.env.Skills == nil {
		return content, content
	}
	head := content
	if idx := strings.IndexAny(content, " \t\n\r"); idx >= 0 {
		head = content[:idx]
	}
	name := strings.TrimPrefix(head, "/")
	name = strings.TrimPrefix(name, "skill:")
	name = strings.TrimSpace(name)
	if name == "" {
		return content, content
	}
	body, err := c.env.Skills.Content(name)
	if err != nil {
		return content, content
	}
	extra := strings.TrimSpace(strings.TrimPrefix(content, head))
	if extra != "" {
		return content, "请按下面这个技能的步骤来做：\n\n" + body + "\n\n用户的具体要求：\n" + extra
	}
	return content, "请按下面这个技能的步骤来做：\n\n" + body
}

// disabledTools 从设置里读停用工具名单。
func (c *ChatService) disabledTools() []string {
	raw, _ := c.env.Repo.GetSetting(domain.SettingDisabledTools)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

// autoTitle 用首条用户消息给会话起名，避免侧栏一排分辨不出的「新对话」。
func (c *ChatService) autoTitle(sess *domain.SessionDO, content string) {
	if sess.MessageCount > 0 || (sess.Title != "" && sess.Title != defaultSessionTitle) {
		return
	}
	title := strings.Join(strings.Fields(content), " ")
	if len([]rune(title)) > 24 {
		title = string([]rune(title)[:24]) + "…"
	}
	if err := c.sessions.Rename(sess.ID, title); err != nil {
		pkg.Warnf("chat: 自动命名失败: %v", err)
	}
}

// busy 判断会话是否有 run 在跑。map 的读必须和写争同一把锁：
// 并发读写 map 不是「读到脏数据」而是直接 fatal error，recover 也救不回来。
func (c *ChatService) busy(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.cancel[sessionID]
	return ok
}

// lockOf 取会话级互斥锁。
func (c *ChatService) lockOf(sessionID string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.locks[sessionID] == nil {
		c.locks[sessionID] = &sync.Mutex{}
	}
	return c.locks[sessionID]
}

// queueOf 取会话的插话队列。
func (c *ChatService) queueOf(sessionID string) *agent.Queue {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.queues[sessionID] == nil {
		c.queues[sessionID] = agent.NewQueue()
	}
	return c.queues[sessionID]
}

// readsOf 取会话的「已读文件」记录。
func (c *ChatService) readsOf(sessionID string) *readTracker {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.reads[sessionID] == nil {
		c.reads[sessionID] = newReadTracker()
	}
	return c.reads[sessionID]
}

// riskOfCommand 只对命令类工具按内容定风险，其余按工具类型判断。
func riskOfCommand(t tool.Tool, args map[string]any) string {
	if t.Name() == "powershell" {
		return tool.RiskOf(toString(args["command"]))
	}
	switch t.Name() {
	case "write", "edit", "python":
		return domain.RiskMedium
	default:
		return domain.RiskLow
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// readTracker 是「写前必读」的会话级记录。
type readTracker struct {
	mu   sync.Mutex
	seen map[string]bool
}

func newReadTracker() *readTracker { return &readTracker{seen: map[string]bool{}} }

func (r *readTracker) HasRead(path string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seen[path]
}

func (r *readTracker) MarkRead(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen[path] = true
}
