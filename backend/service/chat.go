// 对话编排：把用户的发送翻译成 PI 命令，把 PI 的事件接上落库与事件出口。
// 它不实现 Agent 循环——循环在 PI 进程里，这里只管会话级并发、命令选择与收尾。
package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	"WorkBaby/backend/pkg"
)

// promptAckTimeout 是「prompt 已被接受」的等待上限；它不代表模型答完。
const promptAckTimeout = 60 * time.Second

// ChatService 编排一次对话：命令选择、会话锁、事件翻译、收尾记账。
type ChatService struct {
	env       *Env
	sessions  *SessionService
	providers *ProviderService

	mu sync.Mutex
	// locks 是会话级互斥：同一会话不并发发命令。
	locks map[string]*sync.Mutex
	// running 标记会话是否在跑（从 agent_start 到 agent_settled）。
	running map[string]bool
	// runs 记每个会话当前的 runID，供事件与收尾对上。
	runs map[string]string
	// results 存每个会话最近一次的助手正文，供子代理取结论。
	results map[string]string
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
		results:     map[string]string{},
		translators: map[string]*translator{},
	}
}

// Send 发一条消息；返回时命令已被 PI 接受（不代表答完）。
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

	content, injected := c.resolveSkill(content)
	body, images := buildAttachment(sess.Workspace, attachments)
	if len(images) > 0 {
		cap := domain.ModelCapabilityOf(sess.Model)
		if !cap.Vision {
			return nil, pkg.New("3113", "当前模型不支持识图，换一个支持视觉的模型后再发图片", sess.Model)
		}
	}
	text := body + injected

	client, err := c.ensureClient(sessionID, sess)
	if err != nil {
		return nil, err
	}

	// 会话首次使用才建 PI 会话：用户点十次「新对话」只有一次真的发消息，
	// 每次都建会留下十个空文件，还拉长每次点击的等待。
	if sess.PiSessionID == "" {
		if err := c.bindNewSession(sessionID, sess, client); err != nil {
			return nil, err
		}
	}

	runID := pkg.NewTraceID()
	st := &runState{
		sessionID: sessionID, runID: runID,
		provider: sess.ProviderID, model: sess.Model,
		window:    domain.ModelCapabilityOf(sess.Model).ContextWindow,
		maxTokens: domain.ModelCapabilityOf(sess.Model).MaxOutput,
		started:   nowMillis(),
	}
	tr := c.translatorOfSession(lock, sess.Workspace)
	c.mu.Lock()
	c.runs[sessionID] = runID
	c.mu.Unlock()
	tr.setSession(sessionID, st)

	// 运行中必须带 streamingBehavior 发 prompt，否则 PI 直接拒绝。
	behavior := ""
	if c.IsRunning(sessionID) {
		behavior = "steer"
	}
	c.mu.Lock()
	c.markBusyLocked(sessionID)
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), promptAckTimeout)
	defer cancel()
	if _, err := client.Prompt(ctx, text, behavior, toPiImages(images)); err != nil {
		c.releaseSession(sessionID)
		tr.clearSession(sessionID)
		return nil, err
	}

	pkg.Infof("chat: 已发送 session=%s run=%s model=%s behavior=%s", sessionID, runID, sess.Model, behavior)
	return &domain.SendMessageRESP{RunID: runID, SessionID: sessionID}, nil
}

// Steer 插话：当前轮工具跑完后生效。
func (c *ChatService) Steer(sessionID, content string) error {
	return c.queueMessage(sessionID, content, true)
}

// FollowUp 排队：等助手彻底停下才投递。
func (c *ChatService) FollowUp(sessionID, content string) error {
	return c.queueMessage(sessionID, content, false)
}

func (c *ChatService) queueMessage(sessionID, content string, steer bool) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return domain.ErrEmptyContent
	}
	sess, err := c.env.Repo.GetSession(sessionID)
	if err != nil {
		return err
	}
	client, err := c.ensureClient(sessionID, sess)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), promptAckTimeout)
	defer cancel()

	// 空闲时没有 run 在消费队列：直接发一条 prompt，否则这句话会永远排在空队列里。
	if !c.IsRunning(sessionID) {
		_, err := client.Prompt(ctx, content, "", nil)
		return err
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
	sess, err := c.env.Repo.GetSession(sessionID)
	if err != nil {
		return err
	}
	client := c.env.PiSupervisor().Get(sess.Workspace)
	if client == nil || !client.Alive() {
		c.releaseSession(sessionID)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
	defer cancel()

	// 不取回就 abort，用户排队的那句话会凭空消失。
	if q, err := client.ClearQueue(ctx); err == nil && q != nil {
		for _, text := range append(q.Steering, q.FollowUp...) {
			if strings.TrimSpace(text) != "" {
				c.emit(sessionID, domain.EventChatUser, domain.UserData{EntryID: "", Content: text})
			}
		}
	}
	c.env.Approvals.CancelSession(sessionID)
	// 立刻广播：内核的收尾要走完落账与 done，用户不该靠猜才知道点没点着。
	c.emit(sessionID, domain.EventChatStopped, domain.StoppedData{Reason: "user"})

	go func() {
		if err := client.Abort(ctx); err != nil {
			pkg.Warnf("chat: 中止失败 session=%s: %v", sessionID, err)
		}
	}()
	pkg.Infof("chat: 收到停止请求 session=%s", sessionID)
	return nil
}

// IsRunning 报告会话是否有 run 在跑。
func (c *ChatService) IsRunning(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running[sessionID]
}

// Commands 取当前会话能跑的斜杠命令（扩展命令 / 提示模板 / 技能）。
// 真相在 PI 进程里：目录扫描只能看到磁盘上的技能，看不到扩展注册的命令。
func (c *ChatService) Commands(sessionID string) ([]domain.CommandVO, error) {
	sess, err := c.env.Repo.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	client, err := c.ensureClient(sessionID, sess)
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
// 重试期间界面看上去就是卡死，光有一条提示不够——要能把它停掉。
func (c *ChatService) AbortRetry(sessionID string) error {
	sess, err := c.env.Repo.GetSession(sessionID)
	if err != nil {
		return err
	}
	client := c.env.PiSupervisor().Get(sess.Workspace)
	if client == nil || !client.Alive() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := client.AbortRetry(ctx); err != nil {
		return err
	}
	return nil
}

// ExportHTML 把一条会话导成一份自带样式的 HTML 文件。
// 导出在 PI 里做（它才知道条目链的完整形态），本层只负责切到那条会话并传路径。
// 切会话会动进程的当前指针，所以同工作区有对话在跑时直接拒绝，不抢正在写的那条。
func (c *ChatService) ExportHTML(sessionID, outputPath string) (string, error) {
	sess, err := c.env.Repo.GetSession(sessionID)
	if err != nil {
		return "", err
	}
	if sess.PiSessionFile == "" {
		return "", pkg.New("5004", "这条对话还没有落盘，先聊一轮再导出", "")
	}
	for _, other := range c.sessionsIn(sess.Workspace) {
		if c.IsRunning(other) {
			return "", pkg.New("5005", "同一个文件夹里还有对话在跑，等它停下再导出", "")
		}
	}
	client, err := c.ensureClient(sessionID, sess)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
	defer cancel()
	if err := client.SwitchSession(ctx, sess.PiSessionFile); err != nil {
		return "", err
	}
	return client.ExportHTML(ctx, outputPath)
}

// saveResult 把本轮的助手正文暂存，供子代理取结论。
// 只留最近一次：父会话的正文不需要在工作进程里累积。
func (c *ChatService) saveResult(sessionID, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.results == nil {
		c.results = map[string]string{}
	}
	c.results[sessionID] = text
}

// TakeResult 取走并清掉某会话最近一次的正文（子代理专用）。
func (c *ChatService) TakeResult(sessionID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	text := c.results[sessionID]
	delete(c.results, sessionID)
	return text
}

// Forget 清掉一个会话的全部运行态（删会话时调用）。
func (c *ChatService) Forget(sessionID string) {
	c.mu.Lock()
	delete(c.running, sessionID)
	delete(c.runs, sessionID)
	c.mu.Unlock()
	if c.env.Approvals != nil {
		c.env.Approvals.CancelSession(sessionID)
		c.env.Approvals.ForgetSession(sessionID)
	}
	c.env.Emitter.Forget(sessionID)
}

// ensureClient 取（必要时拉起）该会话工作区的 PI 进程。
func (c *ChatService) ensureClient(sessionID string, sess *domain.SessionDO) (*pi.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := c.env.PiSupervisor().Ensure(ctx, sess.Workspace)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// bindNewSession 建一条 PI 会话并回写 SQLite。
func (c *ChatService) bindNewSession(sessionID string, sess *domain.SessionDO, client *pi.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.NewSession(ctx, ""); err != nil {
		return err
	}
	st, err := client.GetState(ctx)
	if err != nil {
		return err
	}
	if st.SessionID == "" {
		return pkg.New("9714", "建好了会话但拿不到它的编号", "")
	}
	// 模型与思考档在会话第一次真正开始时下发：PI 的默认可能和界面选的不一致。
	if sess.ProviderID != "" && sess.Model != "" {
		if err := client.SetModel(ctx, sess.ProviderID, sess.Model); err != nil {
			pkg.Warnf("chat: 设置模型失败（沿用 PI 默认）: %v", err)
		}
	}
	if sess.ThinkingLevel != "" {
		if err := client.SetThinkingLevel(ctx, sess.ThinkingLevel); err != nil {
			pkg.Warnf("chat: 设置思考档失败（沿用 PI 默认）: %v", err)
		}
	}
	if err := c.env.Repo.UpdateSessionColumns(sessionID, map[string]any{
		"pi_session_id":   st.SessionID,
		"pi_session_file": st.SessionFile,
	}); err != nil {
		return err
	}
	sess.PiSessionID = st.SessionID
	sess.PiSessionFile = st.SessionFile
	return nil
}

// translatorOfSession 取（必要时创建）工作区的翻译器。
func (c *ChatService) translatorOfSession(lock *sync.Mutex, workspace string) *translator {
	c.mu.Lock()
	defer c.mu.Unlock()
	tr := c.translators[workspace]
	if tr == nil {
		tr = &translator{svc: c, workspace: workspace, states: map[string]*runState{}}
		c.translators[workspace] = tr
	}
	return tr
}

// translatorOf 取会话所在工作区的翻译器。
func (c *ChatService) translatorOf(st *runState) *translator {
	sess, err := c.env.Repo.GetSession(st.sessionID)
	if err != nil {
		return &translator{svc: c, workspace: "", states: map[string]*runState{}}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	tr := c.translators[sess.Workspace]
	if tr == nil {
		tr = &translator{svc: c, workspace: sess.Workspace, states: map[string]*runState{}}
		c.translators[sess.Workspace] = tr
	}
	return tr
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

// sessionsIn 列出某工作区里正在跑的会话。
func (c *ChatService) sessionsIn(workspace string) []string {
	tr := c.translatorOfWorkspace(workspace)
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]string, 0, len(tr.states))
	for sid := range tr.states {
		out = append(out, sid)
	}
	return out
}

// markBusyLocked 标记会话在跑；调用方必须已持 c.mu。
func (c *ChatService) markBusyLocked(sessionID string) { c.running[sessionID] = true }

// releaseSession 摘掉会话的运行态。
func (c *ChatService) releaseSession(sessionID string) {
	c.mu.Lock()
	delete(c.running, sessionID)
	delete(c.runs, sessionID)
	c.mu.Unlock()
}

// lockOf 取会话级互斥锁。
func (c *ChatService) lockOf(sessionID string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	lk := c.locks[sessionID]
	if lk == nil {
		lk = &sync.Mutex{}
		c.locks[sessionID] = lk
	}
	return lk
}

// emit 是本服务的事件出口。
func (c *ChatService) emit(sessionID, event string, data any) {
	c.env.Emitter.Emit(sessionID, event, data)
	if tap := c.env.ChatTap; tap != nil {
		tap(sessionID, event, data)
	}
}

// recordUsage 落一行用量并累计进会话。
func (c *ChatService) recordUsage(sessionID, providerID, model string, ctxTokens int, u domain.UsageVO) {
	// 先确认会话还在：删会话与 run 收尾竞争的窗口里先落账，会留下没人认领的孤儿用量行。
	if _, err := c.env.Repo.GetSession(sessionID); err != nil {
		return
	}
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

// toPiImages 把附件图片转成 PI 的图片载荷。
func toPiImages(images []domain.MessageImage) []pi.Image {
	if len(images) == 0 {
		return nil
	}
	out := make([]pi.Image, 0, len(images))
	for _, img := range images {
		out = append(out, pi.NewImage(img.MIME, img.Base64))
	}
	return out
}
