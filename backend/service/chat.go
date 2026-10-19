// 对话编排：把用户的发送翻译成 PI 命令，把 PI 的事件接上事件出口。
// 它不实现 Agent 循环——循环在 PI 进程里，这里只管会话级并发、命令选择与问答桥。
package service

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	"WorkBaby/backend/pkg"
)

// promptAckTimeout 是「prompt 已被接受」的等待上限；它不代表模型答完。
const promptAckTimeout = 60 * time.Second

// pendingAsk 是引擎抛回来、等用户回答的一次提问。
type pendingAsk struct {
	ID        string
	SessionID string
	Workspace string
	Method    string
	AskedAt   int64
}

// ChatService 编排一次对话：命令选择、会话锁、事件翻译、问答桥。
type ChatService struct {
	env       *Env
	sessions  *SessionService
	providers *ProviderService

	mu sync.Mutex
	// locks 是会话级互斥：同一会话不并发发命令。
	locks map[string]*sync.Mutex
	// running 标记会话是否在跑（agent_start 到 agent_settled）；runs 记当前 runID。
	running map[string]bool
	runs    map[string]string
	// runWs 记每个在跑的会话属于哪个工作区：RunningIn 要在锁内直接读出。
	runWs map[string]string
	// asks 是按 id 索引的在途提问。
	asks map[string]*pendingAsk
	// current 记每个工作区进程当前停在哪条会话文件上。
	current map[string]string
	// translators 按工作区索引翻译器。
	translators map[string]*translator
}

// NewChatService 构造对话服务。
func NewChatService(env *Env, sessions *SessionService, providers *ProviderService) *ChatService {
	return &ChatService{
		env: env, sessions: sessions, providers: providers,
		locks:       map[string]*sync.Mutex{},
		running:     map[string]bool{},
		runs:        map[string]string{},
		runWs:       map[string]string{},
		asks:        map[string]*pendingAsk{},
		current:     map[string]string{},
		translators: map[string]*translator{},
	}
}

// Send 发一条消息；返回时命令已被 PI 接受（不代表答完）。
// session_id 为空表示这条对话还没落地：由 PI 现场新建，返回时带上新编号。
func (c *ChatService) Send(req domain.SendMessageREQ) (*domain.SendMessageRESP, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" && len(req.Attachments) == 0 {
		return nil, domain.ErrEmptyContent
	}

	workspace := strings.TrimSpace(req.Workspace)
	sessionID := strings.TrimSpace(req.SessionID)
	file := ""
	if sessionID != "" {
		m, err := c.sessions.meta(sessionID)
		if err != nil {
			return nil, err
		}
		workspace, file = m.Cwd, m.File
	}
	if workspace == "" {
		workspace = c.env.Workspace()
	}
	if workspace == "" {
		return nil, pkg.New("1002", "先选一个工作目录", "会话跑在目录里，助手要在这里读写文件")
	}

	lock := c.lockOf(sessionIDKey(sessionID, workspace))
	lock.Lock()
	defer lock.Unlock()

	text, images := buildPrompt(content, req.Attachments)

	ensureCtx, ensureCancel := background(30 * time.Second)
	defer ensureCancel()
	client, err := c.env.PiSupervisor().Ensure(ensureCtx, workspace)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 会话还没落地（或进程停在别的会话上）时先对齐指针。
	// 一个工作区只有一个进程、一个当前会话：另一条对话还在跑时不能抢指针。
	if sessionID == "" || c.currentFile(workspace) != file {
		if c.RunningIn(workspace) {
			return nil, domain.ErrSessionRunning
		}
		if err := c.bindSession(ctx, client, workspace, file); err != nil {
			return nil, err
		}
	}
	st, err := client.GetState(ctx)
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		sessionID = st.SessionID
	}
	if sessionID == "" {
		return nil, pkg.New("9714", "建好了会话但拿不到它的编号", "")
	}
	c.setCurrent(workspace, st.SessionFile)

	// 运行中必须带 streamingBehavior 发 prompt，否则 PI 直接拒绝。
	behavior := ""
	if c.IsRunning(sessionID) {
		behavior = "steer"
	}
	runID, err := c.beginRun(client, workspace, sessionID, st, text, behavior, images)
	if err != nil {
		return nil, err
	}

	// 首条消息之后会话文件才出现：让下一次列表刷新读到它。
	pi.InvalidateSessionCache(st.SessionFile)
	pkg.Infof("chat: 已发送 session=%s run=%s model=%s", sessionID, runID, st.Model.ID)
	return &domain.SendMessageRESP{RunID: runID, SessionID: sessionID}, nil
}

// beginRun 给一轮运行记账并发 prompt：runState + busy 标记 + 等「已接受」。
// 空闲会话无论从 Send 还是从排队投递进来都必须走它——翻译器没有 runState 时
// 这一轮的界面事件会被整轮丢掉，用户看到自己发了话却永远等不到回应。
func (c *ChatService) beginRun(
	client *pi.Client, workspace, sessionID string, st *pi.State,
	text, behavior string, images []pi.Image,
) (string, error) {
	runID := pkg.NewTraceID()
	rs := &runState{
		sessionID: sessionID, runID: runID,
		provider: st.Model.Provider, model: st.Model.ID,
		window:    st.Model.ContextWindow,
		maxTokens: st.Model.MaxTokens,
		started:   nowMillis(),
	}
	tr := c.translatorOfWorkspace(workspace)
	tr.setSession(sessionID, rs)
	c.markBusy(sessionID, runID, workspace)

	ackCtx, ackCancel := context.WithTimeout(context.Background(), promptAckTimeout)
	defer ackCancel()
	if _, err := client.Prompt(ackCtx, text, behavior, images); err != nil {
		c.release(sessionID)
		tr.clearSession(sessionID)
		return "", err
	}
	return runID, nil
}

// Steer 插话：当前轮工具跑完后生效。
func (c *ChatService) Steer(req domain.SteerREQ) error {
	return c.queueMessage(req.SessionID, req.Content, true)
}

// FollowUp 排队：等助手彻底停下才投递。
func (c *ChatService) FollowUp(req domain.SteerREQ) error {
	return c.queueMessage(req.SessionID, req.Content, false)
}

func (c *ChatService) queueMessage(sessionID, content string, steer bool) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return domain.ErrEmptyContent
	}
	m, err := c.sessions.meta(sessionID)
	if err != nil {
		return err
	}
	ensureCtx, ensureCancel := background(30 * time.Second)
	defer ensureCancel()
	client, err := c.env.PiSupervisor().Ensure(ensureCtx, m.Cwd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), promptAckTimeout)
	defer cancel()

	// 空闲时没有 run 在消费队列：走一次完整的发送记账，并回显这条消息。
	// 与 Send 同一把会话锁，堵住「查空闲时还没人跑、发出去时已有新一轮」的窗口；
	// 锁住后复查，真有 run 进来了就落到下面的 steer / follow_up。
	if !c.IsRunning(sessionID) {
		lock := c.lockOf(sessionID)
		lock.Lock()
		defer lock.Unlock()
		if !c.IsRunning(sessionID) {
			st, err := client.GetState(ctx)
			if err != nil {
				return err
			}
			if _, err := c.beginRun(client, m.Cwd, sessionID, st, content, "", nil); err != nil {
				return err
			}
			c.emit(sessionID, domain.EventChatUser, domain.UserData{EntryID: "", Content: content})
			return nil
		}
	}
	if steer {
		_, err = client.Steer(ctx, content, nil)
	} else {
		_, err = client.FollowUp(ctx, content, nil)
	}
	if err != nil {
		return err
	}
	c.emit(sessionID, domain.EventChatUser, domain.UserData{EntryID: "", Content: content})
	return nil
}

// Stop 停止当前 run：先取回未消费的排队文本，再中止。
func (c *ChatService) Stop(sessionID string) error {
	m, err := c.sessions.meta(sessionID)
	if err != nil {
		return err
	}
	client := c.env.PiSupervisor().Get(m.Cwd)
	if client == nil || !client.Alive() {
		c.release(sessionID)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
	// 这里刻意不 defer cancel：abort 在后台等内核收尾，Stop 一返回就取消会让它
	// 立刻报 9703（白打一条「中止失败」日志，也丢掉了等内核空闲的那次等待）。

	// 不取回就 abort，用户排队的那句话会凭空消失。
	if q, err := client.ClearQueue(ctx); err == nil && q != nil {
		for _, text := range append(q.Steering, q.FollowUp...) {
			if strings.TrimSpace(text) != "" {
				c.emit(sessionID, domain.EventChatUser, domain.UserData{EntryID: "", Content: text})
			}
		}
	}
	c.closeAsksOf(sessionID, "cancelled")
	// 立刻广播：内核的收尾要走完落账与 done，用户不该靠猜才知道点没点着。
	c.emit(sessionID, domain.EventChatStopped, domain.StoppedData{Reason: "user"})

	go func() {
		defer cancel()
		if err := client.Abort(ctx); err != nil {
			pkg.Warnf("chat: 中止失败 session=%s: %v", sessionID, err)
		}
	}()
	return nil
}

// IsRunning 报告会话是否有 run 在跑。
func (c *ChatService) IsRunning(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running[sessionID]
}

// RunningIn 报告某个工作目录下有没有会话在跑。
// 工作区跟着运行态一起记录，所以这里只在锁内查内存：去翻会话文件会把 I/O
// 塞进全局锁，而 RunningIn 在发送、切会话、导出的主路径上都要调一次。
func (c *ChatService) RunningIn(workspace string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for sessionID := range c.running {
		if samePath(c.runWs[sessionID], workspace) {
			return true
		}
	}
	return false
}

// Commands 取当前会话能跑的斜杠命令（扩展命令 / 提示模板 / 技能）。
// 真相在 PI 进程里：目录扫描只能看到磁盘上的技能，看不到扩展注册的命令。
func (c *ChatService) Commands(sessionID string) ([]domain.CommandVO, error) {
	m, err := c.sessions.meta(sessionID)
	if err != nil {
		return nil, err
	}
	ensureCtx, ensureCancel := background(30 * time.Second)
	defer ensureCancel()
	client, err := c.env.PiSupervisor().Ensure(ensureCtx, m.Cwd)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	list, err := client.GetCommands(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.CommandVO, 0, len(list))
	for _, cmd := range list {
		out = append(out, domain.CommandVO{
			Name: cmd.Name, Description: cmd.Description, Source: cmd.Source,
		})
	}
	return out, nil
}

// AbortRetry 叫停 PI 正在进行的自动重试。
func (c *ChatService) AbortRetry(sessionID string) error {
	m, err := c.sessions.meta(sessionID)
	if err != nil {
		return err
	}
	client := c.env.PiSupervisor().Get(m.Cwd)
	if client == nil || !client.Alive() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return client.AbortRetry(ctx)
}

// ExportHTML 把一条会话导成一份自带样式的 HTML 文件。
// 导出在 PI 里做（它才知道条目链的完整形态），本层只负责切到那条会话并传路径。
func (c *ChatService) ExportHTML(sessionID, outputPath string) (string, error) {
	m, err := c.sessions.meta(sessionID)
	if err != nil {
		return "", err
	}
	if c.RunningIn(m.Cwd) {
		return "", domain.ErrSessionRunning
	}
	ensureCtx, ensureCancel := background(30 * time.Second)
	defer ensureCancel()
	client, err := c.env.PiSupervisor().Ensure(ensureCtx, m.Cwd)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
	defer cancel()
	if err := client.SwitchSession(ctx, m.File); err != nil {
		return "", err
	}
	c.setCurrent(m.Cwd, m.File)
	return client.ExportHTML(ctx, outputPath)
}

// Answer 回答引擎抛回来的问题：confirm / select / input 三种方法都要回显 id。
func (c *ChatService) Answer(req domain.AnswerREQ) error {
	c.mu.Lock()
	ask := c.asks[req.ID]
	delete(c.asks, req.ID)
	c.mu.Unlock()
	if ask == nil {
		return pkg.New("5010", "这个问题已经不需要回答了", req.ID)
	}
	client := c.env.PiSupervisor().Get(ask.Workspace)
	if client == nil || !client.Alive() {
		return pkg.New("9701", "助手进程已经不在了", "")
	}
	payload := map[string]any{"type": "extension_ui_response", "id": req.ID}
	switch ask.Method {
	case "confirm":
		payload["confirmed"] = req.Confirmed
	default:
		if req.Cancelled {
			payload["cancelled"] = true
		} else {
			payload["value"] = req.Value
		}
	}
	if err := client.WriteResponse(payload); err != nil {
		return err
	}
	c.emit(ask.SessionID, domain.EventChatAskClosed, domain.AskClosedData{ID: req.ID, Reason: "answered"})
	return nil
}

// Forget 清掉一个会话的全部运行态（删会话时调用）。
func (c *ChatService) Forget(sessionID string) {
	c.mu.Lock()
	delete(c.running, sessionID)
	delete(c.runs, sessionID)
	delete(c.runWs, sessionID)
	c.mu.Unlock()
	c.closeAsksOf(sessionID, "gone")
	c.env.Emitter.Forget(sessionID)
}

// bindSession 让进程停在目标会话上；file 为空表示新建一条。
func (c *ChatService) bindSession(ctx context.Context, client *pi.Client, workspace, file string) error {
	if file == "" {
		return client.NewSession(ctx, "")
	}
	return client.SwitchSession(ctx, file)
}

// currentFile 读工作区进程当前停在哪条会话文件上。
func (c *ChatService) currentFile(workspace string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current[wsKey(workspace)]
}

// setCurrent 记下工作区进程当前停的会话文件。
func (c *ChatService) setCurrent(workspace, file string) {
	c.mu.Lock()
	c.current[wsKey(workspace)] = file
	c.mu.Unlock()
}

// wsKey 是工作区在 map 里的键：盘符大小写与分隔符写法不同不能算两条。
func wsKey(workspace string) string {
	if workspace == "" {
		return ""
	}
	return strings.ToLower(filepath.Clean(workspace))
}

// emit 是本服务的事件出口。
func (c *ChatService) emit(sessionID, event string, data any) {
	c.env.Emitter.Emit(sessionID, event, data)
}

// markBusy 标记会话在跑，并记下它所在的工作区。
func (c *ChatService) markBusy(sessionID, runID, workspace string) {
	c.mu.Lock()
	c.running[sessionID] = true
	c.runs[sessionID] = runID
	c.runWs[sessionID] = workspace
	c.mu.Unlock()
}

// release 摘掉会话的运行态。
func (c *ChatService) release(sessionID string) {
	c.mu.Lock()
	delete(c.running, sessionID)
	delete(c.runs, sessionID)
	delete(c.runWs, sessionID)
	c.mu.Unlock()
}

// lockOf 取会话级互斥锁。
func (c *ChatService) lockOf(key string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	lk := c.locks[key]
	if lk == nil {
		lk = &sync.Mutex{}
		c.locks[key] = lk
	}
	return lk
}

// sessionIDKey 给「还没落地的会话」一把稳定的锁：同一个工作目录串行建会话。
func sessionIDKey(sessionID, workspace string) string {
	if sessionID != "" {
		return sessionID
	}
	return "new@" + workspace
}

// translatorOfWorkspace 按工作区取翻译器。
func (c *ChatService) translatorOfWorkspace(workspace string) *translator {
	c.mu.Lock()
	defer c.mu.Unlock()
	tr := c.translators[workspace]
	if tr == nil {
		tr = &translator{svc: c, workspace: workspace, states: map[string]*runState{}}
		c.translators[workspace] = tr
	}
	return tr
}

// openAsk 登记一次在途提问并广播给界面。
func (c *ChatService) openAsk(workspace, sessionID string, ev pi.Event) {
	ask := &pendingAsk{
		ID: ev.ID, SessionID: sessionID, Workspace: workspace,
		Method: ev.Method, AskedAt: nowMillis(),
	}
	c.mu.Lock()
	c.asks[ev.ID] = ask
	c.mu.Unlock()
	c.emit(sessionID, domain.EventChatAsk, domain.AskData{
		ID: ev.ID, Method: ev.Method,
		Title:   pkg.StripANSI(ev.Title),
		Message: pkg.StripANSI(ev.Body),
		Options: ev.Options,
		// 提示语里可能带扩展自己的格式说明（多选题就是靠它告诉用户怎么填），原样转给界面。
		Placeholder: pkg.StripANSI(ev.Placeholder),
		Prefill:     pkg.StripANSI(ev.Prefill),
		TimeoutMs:   ev.TimeoutMs,
	})
}

// closeAsksOf 关掉某个会话的全部在途提问。
func (c *ChatService) closeAsksOf(sessionID, reason string) {
	c.mu.Lock()
	ids := []string{}
	for id, ask := range c.asks {
		if ask.SessionID == sessionID {
			ids = append(ids, id)
			delete(c.asks, id)
		}
	}
	c.mu.Unlock()
	for _, id := range ids {
		c.emit(sessionID, domain.EventChatAskClosed, domain.AskClosedData{ID: id, Reason: reason})
	}
}

// buildPrompt 把正文、@ 引用与图片拼成一次发送。
func buildPrompt(content string, attachments []domain.AttachmentREQ) (string, []pi.Image) {
	var b strings.Builder
	b.WriteString(content)
	images := make([]pi.Image, 0, len(attachments))
	var refs []string
	for _, a := range attachments {
		if a.ImageBase64 != "" && a.Path == "" {
			mime := guessMIME(a.ImageBase64)
			images = append(images, pi.NewImage(mime, a.ImageBase64))
			continue
		}
		if a.Path != "" {
			refs = append(refs, "- "+a.Path)
		}
	}
	if len(refs) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("这次要看这些文件：\n")
		b.WriteString(strings.Join(refs, "\n"))
	}
	return b.String(), images
}

// guessMIME 从 base64 头部认出图片类型；认不出按 png 处理。
func guessMIME(b64 string) string {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) < 4 {
		return "image/png"
	}
	switch {
	case raw[0] == 0x89 && raw[1] == 'P':
		return "image/png"
	case raw[0] == 0xFF && raw[1] == 0xD8:
		return "image/jpeg"
	case raw[0] == 'G' && raw[1] == 'I':
		return "image/gif"
	case raw[0] == 'R' && raw[1] == 'I':
		return "image/webp"
	default:
		return "image/png"
	}
}

// background 返回一个带超时的 context；调用方必须 defer 取消，否则定时器与它的
// goroutine 会滞留到超时那一刻（快路径里那个时间可能是一次对话的好几倍）。
func background(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
