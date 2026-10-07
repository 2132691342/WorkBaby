package service

import (
	"context"
	"strings"
	"sync"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
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

	if _, busy := c.cancel[sessionID]; busy {
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
		ID:       pkg.NewID(domain.PrefixEntry),
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
	c.queueOf(sessionID).Enqueue(llm.Message{Role: llm.RoleUser, Content: content})
	return nil
}

// appendSteered 把插话落一条 user 条目并广播，界面据此把它补进时间线。
func (c *ChatService) appendSteered(sessionID, content string) {
	sess, err := c.env.Repo.GetSession(sessionID)
	if err != nil {
		return
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
		return
	}
	c.env.Emitter.Emit(sessionID, domain.EventChatUser, domain.UserData{
		EntryID: entry.ID, Content: content,
	})
}

// run 是一次 run 的完整生命周期。
func (c *ChatService) run(ctx context.Context, runID, sessionID string) {
	defer func() {
		c.mu.Lock()
		delete(c.cancel, sessionID)
		c.mu.Unlock()
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

	deps := c.env.ToolDeps
	deps.Reads = c.readsOf(sessionID)
	permission := sess.Permission
	cap := c.capabilityOf(sess.ProviderID, model)
	state := &runState{sessionID: sessionID, providerID: sess.ProviderID, model: model, windowKnown: cap.Known}
	temp, topP, maxOutput := c.samplingOf(sess.ProviderID, model)

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
		MaxTurns:    32,
		Parallel:    4,
		Budget:      c.budget(sess.ProviderID, model, system),
		Emit:      func(e agent.Event) { c.onEvent(state, e) },
		Steering:  c.queueOf(sessionID),
		Gate: func(ctx context.Context, call *llm.ToolCall) (bool, string) {
			t, ok := c.env.Registry.Get(call.Name)
			if !ok {
				return true, "没有这个工具"
			}
			return c.approvals.Gate(ctx, sessionID, permission, t, ToolCallView{
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
	turnEntryID string
	lastEntryID string
	pending     string
	thinking    string
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
		st.pending = ""
		st.thinking = ""
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
			st.thinking += e.Delta
		} else {
			st.pending += e.Delta
			st.hasContent = true
		}
		c.env.Emitter.Emit(st.sessionID, domain.EventChatDelta, domain.DeltaData{
			EntryID: st.turnEntryID, Kind: e.DeltaKind, Delta: e.Delta,
		})
	case agent.EventToolStart:
		if e.ToolCall == nil {
			return
		}
		st.toolCalls = append(st.toolCalls, llm.ToolCall{
			ID: e.ToolCall.ID, Name: e.ToolCall.Name, Label: e.ToolTitle, Args: e.ToolCall.Args,
		})
		// 声明必须先于结果落库：上游要求 assistant(tool_calls) 紧邻它自己的
		// tool 结果，落晚了上游会以 tool result's tool id not found 直接 400。
		c.appendAssistantTurn(st)
		c.env.Emitter.Emit(st.sessionID, domain.EventChatToolStart, domain.ToolStartData{
			ToolCallID: e.ToolCall.ID, Tool: e.ToolCall.Name,
			Label: e.ToolTitle, Args: e.ToolCall.Args,
		})
	case agent.EventToolEnd:
		if e.ToolCall == nil {
			return
		}
		c.appendTool(st, e)
	case agent.EventSteering:
		// 插话在注入时刻落库：早了会插进 assistant(tool_calls) 与工具结果之间
		// 撕裂协议配对，晚了链序就与真实对话不一致。
		c.appendSteered(st.sessionID, e.UserContent)
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
			Ratio: ratioOf(e.TokensUsed, e.TokensWindow), Reserve: e.TokensReserve,
		})
	case agent.EventAgentEnd:
		st.stopReason = e.StopReason
	}
}

// ratioOf 换算成整数百分比；窗口认不出来时返回 0，让前端显示「未知」而不是瞎报。
func ratioOf(used, window int) int {
	if window <= 0 || used <= 0 {
		return 0
	}
	r := used * 100 / window
	if r > 100 {
		return 100
	}
	return r
}

// appendAssistantTurn 落本轮的 assistant 条目；空消息不落，避免下一轮上游 400。
func (c *ChatService) appendAssistantTurn(st *runState) {
	if !st.hasContent && st.thinking == "" && len(st.toolCalls) == 0 {
		st.turnEntryID = ""
		return
	}
	// 声明可能已在工具开始时落过一轮，这里再落时必须换一个 id：
	// 沿用已入库的旧 id 会撞主键约束，那条 assistant 消息就永远丢了。
	if st.turnEntryID == "" {
		st.turnEntryID = pkg.NewID(domain.PrefixEntry)
	}
	sess, err := c.env.Repo.GetSession(st.sessionID)
	if err != nil {
		return
	}
	entry := &domain.EntryDO{
		ID:        st.turnEntryID,
		SessionID: st.sessionID,
		ParentID:  sess.LeafEntryID,
		Type:      domain.EntryTypeMessage,
		Role:      domain.RoleAssistant,
		PayloadJSON: payloadOf(llm.Message{
			Role: llm.RoleAssistant, Content: st.pending, Thinking: st.thinking,
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
	st.turnEntryID = ""
	// 已落库的内容清空：下一段增量要作为独立的一条，不能与这条重复。
	st.pending = ""
	st.thinking = ""
	st.hasContent = false
	st.toolCalls = nil
}

// appendTool 落一条工具结果条目并按序接到树上。
func (c *ChatService) appendTool(st *runState, e agent.Event) {
	sess, err := c.env.Repo.GetSession(st.sessionID)
	if err != nil {
		return
	}
	entry := &domain.EntryDO{
		ID:        pkg.NewID(domain.PrefixEntry),
		SessionID: st.sessionID,
		ParentID:  sess.LeafEntryID,
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
	pkg.Infof("chat: 工具完成 session=%s tool=%s ok=%v cost=%dms",
		st.sessionID, e.ToolCall.Name, e.ToolOK, e.DurationMs)
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
	if !st.hasContent && st.thinking == "" {
		st.turnEntryID = ""
		return
	}
	c.appendAssistantTurn(st)
}

// fail 统一错误出口：已产出的内容不回滚，用户能看到半成品。
// 错误轮没有 turn_end，这里负责把已流出的正文兜底落库——不然界面上看得到的
// 内容刷新后就消失；上游已计量的 token 也要进仪表盘，否则漏算真实消耗。
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
	_ = c.env.Repo.AddUsage(&domain.TokenUsageDO{
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
	})
	if fresh, e2 := c.env.Repo.GetSession(sessionID); e2 == nil {
		_ = c.env.Repo.UpdateSessionColumns(sessionID, map[string]any{
			"total_tokens": fresh.TotalTokens + u.Total,
		})
	}
}

// resolveSkill 把 /skill:名字 展开成带正文的用户消息。
func (c *ChatService) resolveSkill(content string) (string, string) {
	if !strings.HasPrefix(content, "/") || c.env.Skills == nil {
		return content, content
	}
	name := strings.TrimPrefix(content, "/")
	name = strings.TrimPrefix(name, "skill:")
	name = strings.TrimSpace(name)
	if name == "" {
		return content, content
	}
	body, err := c.env.Skills.Content(name)
	if err != nil {
		return content, content
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

// autoTitle 用首条用户消息给会话起名。侧栏一排「新对话」没法分辨，
// 这点小事直接决定用户还能不能找回上上周那次对话。
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
