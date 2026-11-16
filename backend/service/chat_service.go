package service

import (
	"context"
	"fmt"
	"runtime/debug"
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
	// SkillSource 已启用技能索引的来源；每轮开跑前重取一次
	SkillSource func() []agent.Skill

	// streamer 上游适配器；一次装配共用一份，连接池才有意义
	streamer agent.Streamer

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

// fallbackThinkingLevel 是两个思考档来源都缺省时的兜底：中等档是最不易踩坑的选择。
const fallbackThinkingLevel = "medium"

// resolveThinkingLevel 定生效思考档：当前档优先，用户没选过就用新会话默认档。
// 兜底常量只在两处都没有时登场——不设它的话「设了默认档」这回事会静默失效。
func resolveThinkingLevel(settings *Settings) string {
	if level := strings.TrimSpace(settings.Get(KeyThinkingLevel)); thinkingLevels[level] {
		return level
	}
	return settings.DefaultThinkingLevel()
}

// thinkingLevels 是可用的思考档集合。
var thinkingLevels = map[string]bool{
	"off": true, "minimal": true, "low": true, "medium": true, "high": true,
}

// NewChat 造一个对话门面。
func NewChat(settings *Settings, sessions *Sessions, models *Models, hub *sse.Hub, store *agent.Store) *Chat {
	level := resolveThinkingLevel(settings)
	chat := &Chat{
		Settings: settings,
		Sessions: sessions,
		Models:   models,
		Hub:      hub,
		Stream:   NewStream(hub),
		Store:    store,
		Asks:     agent.NewPendingAsks(),
		level:    level,
		queued:   domain.QueuedMessagesVO{},
		waiters:  map[string]chan agent.AskResponse{},
		streamer: agent.NewOpenAIStreamer(),
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
	c.level = resolveThinkingLevel(c.Settings)
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
	// 新会话从「新会话默认档」起步：用户没显式选过当前档时，
	// 设置里那个默认档必须在每份新会话上真的生效。
	c.level = resolveThinkingLevel(c.Settings)
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
// 先认内置斜杠命令：/new、/compact、/plan 由本进程直接处理，
// 不发给模型——发过去模型只会看到一句它不理解的话。
func (c *Chat) Send(sessionFile, message string, attachments []domain.ChatAttachmentVO) (domain.ChatSendRESP, error) {
	composed := compose(message, attachments)
	if strings.TrimSpace(composed.text) == "" {
		return domain.ChatSendRESP{}, pkg.Err(pkg.CodeBadRequest, "消息不能为空")
	}
	// 附件随命令一起提交时按消息处理：命令只认纯文本输入。
	if len(attachments) == 0 {
		if handled, err := c.HandleCommand(composed.text); handled || err != nil {
			if err != nil {
				return domain.ChatSendRESP{}, err
			}
			// 命令已消费：不登记运行，界面据此停在静止态。
			return domain.ChatSendRESP{Disposition: domain.DispositionHandled}, nil
		}
	}
	if c.Busy() {
		// 正在跑就排进队列：这是「插话」的正确语义，而不是报错让用户重打一遍。
		if err := c.Steer(message, attachments); err != nil {
			pkg.Warn("聊天发送排队失败 code=%s：%s", pkg.As(err).Code, pkg.As(err).Message)
			return domain.ChatSendRESP{}, err
		}
		return domain.ChatSendRESP{Disposition: domain.DispositionQueued}, nil
	}
	session, err := c.BindSession(sessionFile)
	if err != nil {
		ae := pkg.As(err)
		pkg.Warn("聊天发送绑定会话失败 code=%s：file=%q %s", ae.Code, sessionFile, ae.Message)
		return domain.ChatSendRESP{}, err
	}
	model, err := c.resolveModel()
	if err != nil {
		ae := pkg.As(err)
		pkg.Warn("聊天发送取模型失败 code=%s：%s %s", ae.Code, ae.Message, ae.Detail)
		return domain.ChatSendRESP{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	runner := &agent.Runner{
		Streamer: c.streamer,
		Registry: c.Tools,
		Session:  session,
		Model:    model,
		Thinking: c.level,
		// 模型定义里的输出上限要传给上游：不传的话上游按自己的默认值给，
		// 用户按「这个模型的 max_tokens」预期的长度就永远对不上。
		MaxTokens: model.MaxTokens,
	}
	// 占运行位与登记 runner 必须是同一步：分两步做时两个并发请求会各自
	// 起一轮，同一份会话被两路追加。抢不到就按插话处理，不报错。
	if !c.tryBegin(runner, cancel) {
		cancel()
		if err := c.Steer(message, attachments); err != nil {
			return domain.ChatSendRESP{}, err
		}
		return domain.ChatSendRESP{Disposition: domain.DispositionQueued}, nil
	}
	// 每轮开跑前重取技能索引：开关就是搬目录，重扫一遍就能立即生效。
	// 比在每个技能写接口里各推一次便宜——那些接口有六个，漏一个就长期不一致。
	c.refreshSkills()
	runId := c.Stream.Begin(composed.text)

	go func() {
		defer func() {
			// 引擎的 panic 不能终止整个应用：HTTP 协程有 gin.Recovery 兜着，
			// 运行协程没有。这里把它翻成一条可见的失败，再照常收尾。
			if recovered := recover(); recovered != nil {
				pkg.Error("聊天运行崩溃：%v\n%s", recovered, debug.Stack())
				c.Stream.Fail("运行时内部错误，已中止这一轮：" + fmt.Sprint(recovered))
				c.Stream.Finish(false)
			}
			c.finish()
			cancel()
		}()
		// 水位过线先压缩再开跑：带着爆掉的上下文问模型，
		// 要么直接被上游拒，要么烧掉一轮不必要的输入 token。
		// 压缩失败保持安静落日志：别让一次整理失败挡住用户这句话。
		if c.autoCompactionDue(session, model.ContextWindow) {
			_ = c.compactNow(session, "")
		}
		runner.Run(ctx, composed.message(), c.handler())
		// 追加队非空就依次再起一轮：那是用户排在「这一轮之后」的话，
		// 静默吞掉等于丢消息。中止后不再派发——用户已经喊停了。
		for _, msg := range runner.FollowUpQueued() {
			if ctx.Err() != nil {
				return
			}
			c.Stream.Begin(msg.Text())
			runner.Run(ctx, msg, c.handler())
		}
	}()
	return domain.ChatSendRESP{RunId: runId, Disposition: domain.DispositionStarted}, nil
}

// refreshSkills 把当前已启用技能重扫一遍装进注册表。
// 索引在开跑前一次性换成快照：一轮运行中途不会因为用户点了开关而改变提示，
// 下一轮才变——同一轮里提示与工具列表就该是同一份现实。
func (c *Chat) refreshSkills() {
	if c.SkillSource == nil {
		return
	}
	c.Tools.SetSkills(c.SkillSource())
}

// tryBegin 尝试占用运行位并登记 runner；已有运行在跑返回 false。
// 判定与置位在同一把锁里：分成两步的话并发发话会起两轮。
func (c *Chat) tryBegin(runner *agent.Runner, cancel context.CancelFunc) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running.Load() {
		return false
	}
	c.runner = runner
	c.cancel = cancel
	c.queued = domain.QueuedMessagesVO{}
	c.running.Store(true)
	return true
}

// finish 收回运行位与登记状态。
func (c *Chat) finish() {
	c.mu.Lock()
	c.runner = nil
	c.cancel = nil
	c.queued = domain.QueuedMessagesVO{}
	c.running.Store(false)
	c.mu.Unlock()
}

// autoCompactionDue 判断这一轮开跑前要不要先压缩：
// 开关开着、窗口已知、占用过八成三者缺一不可。
func (c *Chat) autoCompactionDue(session *agent.Session, contextWindow int) bool {
	if c.Settings.Get(KeyAutoCompaction) == "0" {
		return false
	}
	if contextWindow <= 0 {
		return false
	}
	return float64(session.ContextUsage())/float64(contextWindow) > 0.8
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

// AskPending 当前在等的提问清单。刷新界面后卡片回不来：路由还没把它导出到 /chat/state。
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
			// 运行失败只走 SSE 的话，日志里什么都查不到——
			// 发布版没有控制台，这一行就是排查「为什么报错」的唯一入口。
			pkg.Warn("聊天运行失败：%s", message)
			c.Stream.Fail(message)
		},
		OnDone: func(aborted bool) {
			c.Stream.Finish(aborted)
		},
	}
}

// resolveModel 取当前生效的模型与凭据。
// 指向的服务商或模型已被删掉时，把活跃设置清掉：
// 那是坏指针，不清的话每一句话都会撞同一句报错。
func (c *Chat) resolveModel() (agent.ModelRef, error) {
	provider := strings.TrimSpace(c.Settings.Get(KeyActiveProvider))
	modelId := strings.TrimSpace(c.Settings.Get(KeyActiveModel))
	if provider == "" || modelId == "" {
		return agent.ModelRef{}, pkg.Err(pkg.CodeNoProvider, "还没有选择模型，先去模型页选一个")
	}
	entry, key, err := c.Models.Lookup(provider, modelId)
	if err != nil {
		// 配置指向已删掉的服务商 / 模型：清掉它，让下一句话
		// 拿到的是「去选一个」而不是同一句听不懂的错。
		c.Models.DropActiveIf(provider, modelId)
		ae := pkg.WrapErr(pkg.CodeNoProvider, "之前选的模型已经不存在了，去模型页重新选一个", err)
		pkg.Warn("解析活跃模型失败 code=%s：%s/%s %s", ae.Code, provider, modelId, ae.Detail)
		return agent.ModelRef{}, ae
	}
	if key == "" {
		pkg.Warn("服务商缺密钥 code=%s：%s", pkg.CodeProviderBad, provider)
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
