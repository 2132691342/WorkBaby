package service

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/sse"
)

// Chat 是对话编排：进程内的原生引擎，一次运行一个 Runner。
// 「引擎」不再是子进程——它就是本进程里的 agent 包，所以没有拉起、握手、崩溃重启。
type Chat struct {
	// Settings 设置（工作区、上次会话）
	Settings *Settings
	// Sessions 会话回放（清单、详情、隐藏台账）
	Sessions *Sessions
	// Models 服务商与模型
	Models *Models
	// Hub 事件广播
	Hub *sse.Hub
	// Stream 事件翻译器
	Stream *Stream
	// Tools 工具注册表
	Tools *agent.Registry
	// Store 会话文件存储
	Store *agent.Store
	// Asks 在等的提问：跨运行边界也要留着，界面刷新靠它补回卡片
	Asks *agent.PendingAsks
	// Templates 提示词模板（斜杠命令清单要它）
	Templates *Templates

	// mu 保护会话绑定与运行状态
	mu sync.Mutex
	// session 当前打开的会话；nil 表示还没开会话
	session *agent.Session
	// runner 当前运行的执行器；nil 表示空闲
	runner *agent.Runner
	// cancel 中止当前运行
	cancel context.CancelFunc
	// running 是否有运行在跑
	running atomic.Bool
	// level 思考档
	level string
	// queued 排队文本镜像（界面显示用）
	queued domain.QueuedMessagesVO
	// waiters 在等的提问：提问 ID → 回答通道
	waiters map[string]chan agent.AskResponse
}

// NewChat 造一个对话门面。
func NewChat(settings *Settings, sessions *Sessions, models *Models, hub *sse.Hub, store *agent.Store) *Chat {
	chat := &Chat{
		Settings: settings,
		Sessions: sessions,
		Models:   models,
		Hub:      hub,
		Stream:   NewStream(hub),
		Store:    store,
		Asks:     agent.NewPendingAsks(),
		level:    "medium",
		queued:   domain.QueuedMessagesVO{},
		waiters:  map[string]chan agent.AskResponse{},
	}
	chat.Tools = agent.NewRegistry(settings.Workspace)
	chat.Tools.Asker = agent.AskerFunc(chat.ask)
	return chat
}

// BindSession 把对话绑到某份会话文件；空串表示开一份新的。
func (c *Chat) BindSession(file string) (*agent.Session, error) {
	workspace := c.Settings.Workspace()
	if workspace == "" {
		return nil, pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	c.mu.Lock()
	current := c.session
	c.mu.Unlock()
	if file != "" {
		if current != nil && strings.EqualFold(current.File(), file) {
			return current, nil
		}
		if c.Busy() {
			return nil, pkg.Err(pkg.CodeEngineBusy, "有任务正在跑，等它结束再切会话")
		}
		opened, err := c.Store.Open(file)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.session = opened
		c.mu.Unlock()
		c.Settings.RememberSession(opened.File())
		c.applyPlanState(opened.Id())
		return opened, nil
	}
	if current != nil {
		return current, nil
	}
	created, err := c.Store.Create(workspace)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.session = created
	c.mu.Unlock()
	c.Settings.RememberSession(created.File())
	// 新会话是干净的：计划模式不该从上一份会话漏过来。
	c.Tools.Policy.SetReadOnly(created.Id(), false)
	return created, nil
}

// NewSession 开一份新会话。
func (c *Chat) NewSession() (string, error) {
	if c.Busy() {
		return "", pkg.Err(pkg.CodeEngineBusy, "有任务正在跑，等它结束再新建会话")
	}
	workspace := c.Settings.Workspace()
	if workspace == "" {
		return "", pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	created, err := c.Store.Create(workspace)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.session = created
	c.mu.Unlock()
	c.Settings.RememberSession(created.File())
	c.Hub.Broadcast("session:info", sse.EngineTag(nil))
	return created.File(), nil
}

// Busy 是否有运行在跑。
func (c *Chat) Busy() bool { return c.running.Load() }

// CurrentFile 当前会话文件。
func (c *Chat) CurrentFile() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		return ""
	}
	return c.session.File()
}

// Send 发一条消息并起一轮运行。
func (c *Chat) Send(sessionFile, message string, attachments []domain.ChatAttachmentVO) (domain.ChatSendRESP, error) {
	composed := compose(message, attachments)
	if strings.TrimSpace(composed.text) == "" {
		return domain.ChatSendRESP{}, pkg.Err(pkg.CodeBadRequest, "消息不能为空")
	}
	if c.Busy() {
		// 正在跑就排进队列：这是「插话」的正确语义，而不是报错让用户重打一遍。
		if err := c.Steer(message, attachments); err != nil {
			return domain.ChatSendRESP{}, err
		}
		return domain.ChatSendRESP{Disposition: domain.DispositionQueued}, nil
	}
	session, err := c.BindSession(sessionFile)
	if err != nil {
		return domain.ChatSendRESP{}, err
	}
	model, err := c.resolveModel()
	if err != nil {
		return domain.ChatSendRESP{}, err
	}
	runId := c.Stream.Begin(composed.text)
	runner := &agent.Runner{
		Streamer: agent.NewOpenAIStreamer(),
		Registry: c.Tools,
		Session:  session,
		Model:    model,
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.runner = runner
	c.cancel = cancel
	c.queued = domain.QueuedMessagesVO{}
	c.mu.Unlock()
	c.running.Store(true)

	go func() {
		defer func() {
			c.running.Store(false)
			c.mu.Lock()
			c.runner = nil
			c.cancel = nil
			c.queued = domain.QueuedMessagesVO{}
			c.mu.Unlock()
			cancel()
		}()
		runner.Run(ctx, composed.message(), c.handler())
	}()
	return domain.ChatSendRESP{RunId: runId, Disposition: domain.DispositionStarted}, nil
}

// Steer 在当前运行里插一条话，等本次工具调用做完再投递。
func (c *Chat) Steer(message string, attachments []domain.ChatAttachmentVO) error {
	composed := compose(message, attachments)
	if strings.TrimSpace(composed.text) == "" {
		return pkg.Err(pkg.CodeBadRequest, "插话内容不能为空")
	}
	c.mu.Lock()
	runner := c.runner
	c.mu.Unlock()
	if runner == nil {
		return pkg.Err(pkg.CodeNotFound, "当前没有正在进行的运行")
	}
	runner.Enqueue(composed.message())
	c.refreshQueue()
	return nil
}

// FollowUp 追加一条话，同样等这一轮收尾后投递。
func (c *Chat) FollowUp(message string, attachments []domain.ChatAttachmentVO) error {
	return c.Steer(message, attachments)
}

// Stop 中止当前运行。
func (c *Chat) Stop() error {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	return nil
}

// ClearQueue 清空排队并返回被清掉的文本。
func (c *Chat) ClearQueue() domain.QueuedMessagesVO {
	c.mu.Lock()
	runner := c.runner
	c.mu.Unlock()
	if runner != nil {
		for _, text := range runner.Queued() {
			_ = text
		}
	}
	c.mu.Lock()
	out := c.queued
	c.queued = domain.QueuedMessagesVO{}
	c.mu.Unlock()
	return out
}

// RefreshQueue 把排队镜像刷成引擎侧的真实排队。
func (c *Chat) refreshQueue() {
	c.mu.Lock()
	runner := c.runner
	c.mu.Unlock()
	if runner == nil {
		return
	}
	texts := runner.Queued()
	c.mu.Lock()
	c.queued = domain.QueuedMessagesVO{Steering: texts}
	out := c.queued
	c.mu.Unlock()
	c.Hub.Broadcast("chat:queue", sse.EngineTag(map[string]any{
		"steering":  out.Steering,
		"follow_up": out.FollowUp,
	}))
}

// ask 发起一次提问并等回答。
// 提问是阻塞式的：模型在等用户回话，这一轮不会往下走。
// 界面可能在这期间切走再回来，所以待答清单单独记着，刷新时补回卡片。
func (c *Chat) ask(ctx context.Context, req agent.AskRequest) agent.AskResponse {
	answer := make(chan agent.AskResponse, 1)
	c.mu.Lock()
	c.waiters[req.AskId] = answer
	c.mu.Unlock()
	c.Asks.Add(req)
	defer func() {
		c.mu.Lock()
		delete(c.waiters, req.AskId)
		c.mu.Unlock()
		c.Asks.Remove(req.AskId)
	}()
	c.Hub.Broadcast("chat:ask", sse.EngineTag(map[string]any{
		"ask_id":          req.AskId,
		"ask_method":      string(req.AskMethod),
		"ask_title":       req.AskTitle,
		"ask_message":     req.AskMessage,
		"ask_placeholder": req.AskPlaceholder,
		"ask_prefill":     req.AskPrefill,
		"ask_options":     req.AskOptions,
	}))
	select {
	case response := <-answer:
		return response
	case <-ctx.Done():
		// 中止时提问作废：等下去也不会有人答。
		return agent.AskResponse{Cancelled: true}
	}
}

// AskPending 在等的提问；界面刷新时靠它把卡片补回来。
func (c *Chat) AskPending() []domain.AskRequestVO {
	items := c.Asks.Items()
	out := make([]domain.AskRequestVO, 0, len(items))
	for _, req := range items {
		out = append(out, domain.AskRequestVO{
			AskId:          req.AskId,
			AskMethod:      string(req.AskMethod),
			AskTitle:       req.AskTitle,
			AskMessage:     req.AskMessage,
			AskPlaceholder: req.AskPlaceholder,
			AskPrefill:     req.AskPrefill,
			AskOptions:     req.AskOptions,
			AskTimeoutMs:   req.AskTimeoutMs,
		})
	}
	return out
}

// handler 组装运行事件到界面的翻译。
func (c *Chat) handler() agent.Handler {
	return agent.Handler{
		OnText:     c.Stream.TextDelta,
		OnThinking: c.Stream.ThinkingDelta,
		OnToolStart: func(callId, name, args string) {
			c.Stream.ToolStart(callId, name, args)
		},
		OnToolEnd: func(callId, name string, result agent.Result, durationMs int64) {
			c.Stream.ToolEnd(callId, name, result, durationMs)
		},
		OnAssistant: func(msg agent.Message) {
			c.Stream.AssistantMessage(msg)
		},
		OnUsage: func(usage agent.Usage) {
			c.Stream.Usage(usage)
		},
		OnError: func(message string) {
			c.Stream.Fail(message)
		},
		OnDone: func(aborted bool) {
			c.Stream.Finish(aborted)
		},
	}
}

// resolveModel 取当前生效的模型与凭据。
func (c *Chat) resolveModel() (agent.ModelRef, error) {
	provider := strings.TrimSpace(c.Settings.Get(KeyActiveProvider))
	modelId := strings.TrimSpace(c.Settings.Get(KeyActiveModel))
	if provider == "" || modelId == "" {
		return agent.ModelRef{}, pkg.Err(pkg.CodeNoProvider, "还没有选择模型，先去模型页选一个")
	}
	entry, key, err := c.Models.Lookup(provider, modelId)
	if err != nil {
		return agent.ModelRef{}, err
	}
	if key == "" {
		return agent.ModelRef{}, pkg.Err(pkg.CodeProviderBad, "服务商 "+provider+" 还没有填密钥")
	}
	providerEntry := c.Models.ProviderOf(provider)
	return agent.ModelRef{
		ProviderName:      provider,
		ModelId:           modelId,
		BaseUrl:           providerEntry.BaseUrl,
		ApiKey:            key,
		ContextWindow:     entry.ContextWindow,
		MaxTokens:         entry.MaxTokens,
		SupportsReasoning: entry.SupportsReasoning,
	}, nil
}

// CloneSession 克隆当前会话：把已有消息复制进一份新会话。
// 只复制消息，不复制元信息条目——新会话是往后走的新分支，
// 旧分支的模型切换与用量与它无关。
func (c *Chat) CloneSession() (string, error) {
	if c.Busy() {
		return "", pkg.Err(pkg.CodeEngineBusy, "有任务正在跑，等它结束再复制会话")
	}
	source, err := c.BindSession("")
	if err != nil {
		return "", err
	}
	created, err := c.Store.Create(c.Settings.Workspace())
	if err != nil {
		return "", err
	}
	for _, entry := range source.Entries() {
		if entry.EntryType != domain.SessionEntryMessage || entry.EntryMessage == nil {
			continue
		}
		if _, err := created.AppendMessage(agent.MessageFromDO(entry.EntryMessage)); err != nil {
			return "", err
		}
	}
	c.mu.Lock()
	c.session = created
	c.mu.Unlock()
	c.Settings.RememberSession(created.File())
	c.Hub.Broadcast("session:info", sse.EngineTag(nil))
	return created.File(), nil
}

// Fork 从一条用户消息分叉出一份新会话：原会话一个字节不动，只复制到分叉点（含那条用户消息）为止。
// 分叉点之后的内容属于另一条思路，带过去等于把旧路走完，新会话就没得重新想。
func (c *Chat) Fork(sessionFile, turnId string) (domain.ChatForkRESP, error) {
	turnId = strings.TrimSpace(turnId)
	if turnId == "" {
		return domain.ChatForkRESP{}, pkg.Err(pkg.CodeBadRequest, "没有指定分叉起点")
	}
	if c.Busy() {
		return domain.ChatForkRESP{}, pkg.Err(pkg.CodeEngineBusy, "有任务正在跑，等它结束再分叉")
	}
	source, err := c.BindSession(sessionFile)
	if err != nil {
		return domain.ChatForkRESP{}, err
	}
	// 先找分叉点在活动分支上的位置：不在活动分支上的条目分叉没有意义
	// （那是已被放弃的分支，用户看不见它）。
	entries := source.Entries()
	cut := -1
	for index, entry := range entries {
		if entry.EntryId == turnId {
			cut = index
			break
		}
	}
	if cut < 0 {
		return domain.ChatForkRESP{}, pkg.Err(pkg.CodeNotFound, "这条不在当前会话里", turnId)
	}
	created, err := c.Store.Create(c.Settings.Workspace())
	if err != nil {
		return domain.ChatForkRESP{}, err
	}
	// 复制到分叉点为止：分叉点之后的内容属于另一条思路，带过去就等于把旧路走完了。
	for _, entry := range entries[:cut+1] {
		if entry.EntryType != domain.SessionEntryMessage || entry.EntryMessage == nil {
			continue
		}
		if _, err := created.AppendMessage(agent.MessageFromDO(entry.EntryMessage)); err != nil {
			return domain.ChatForkRESP{}, err
		}
	}
	c.mu.Lock()
	c.session = created
	c.mu.Unlock()
	c.Settings.RememberSession(created.File())
	c.Hub.Broadcast("session:info", sse.EngineTag(nil))
	return domain.ChatForkRESP{SessionFile: created.File()}, nil
}

// Rename 给当前会话改名。
func (c *Chat) Rename(sessionFile, title string) error {
	session, err := c.BindSession(sessionFile)
	if err != nil {
		return err
	}
	if _, err := session.Rename(title); err != nil {
		return err
	}
	c.Hub.Broadcast("session:title", sse.EngineTag(map[string]any{"title": session.Title()}))
	return nil
}

// SetModel 切模型。
func (c *Chat) SetModel(_ string, providerName, modelId string) error {
	if providerName == "" || modelId == "" {
		return pkg.Err(pkg.CodeBadRequest, "服务商与模型都不能为空")
	}
	if _, _, err := c.Models.Lookup(providerName, modelId); err != nil {
		return err
	}
	if err := c.Settings.SaveSettings(map[string]string{
		KeyActiveProvider: providerName,
		KeyActiveModel:    modelId,
	}); err != nil {
		return err
	}
	c.Hub.Broadcast("chat:model", sse.EngineTag(map[string]any{
		"provider_name": providerName,
		"model_id":      modelId,
	}))
	return nil
}

// SetThinking 记下思考档。思考档影响的是系统提示里的一句话，
// 会话进行中改它不会重写已发生的对话。
func (c *Chat) SetThinking(_ string, level string) error {
	if level == "" {
		return pkg.Err(pkg.CodeBadRequest, "思考档不能为空")
	}
	c.mu.Lock()
	c.level = level
	c.mu.Unlock()
	return nil
}

// Shutdown 退出收尾：中止还在跑的运行。
func (c *Chat) Shutdown() {
	_ = c.Stop()
}

// Warm 预热：原生引擎没有冷启动，这里只保证会话可用。
func (c *Chat) Warm() {
	if c.Settings.Workspace() == "" {
		return
	}
	if _, err := c.BindSession(""); err != nil {
		pkg.Warn("会话初始化失败：%v", err)
	}
}

// OnWorkspaceChanged 工作区切换：丢掉旧会话绑定。
func (c *Chat) OnWorkspaceChanged() {
	if c.Busy() {
		_ = c.Stop()
	}
	c.mu.Lock()
	c.session = nil
	c.mu.Unlock()
}
