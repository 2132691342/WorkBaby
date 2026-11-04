package service

import (
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/chat"
	"WorkBaby/backend/service/engine"
	"WorkBaby/backend/service/parser"
	"WorkBaby/backend/sse"
)

// Chat 是对话编排：一个工作区一个引擎子进程；send / stop / steer / 切会话 / 状态组装。
type Chat struct {
	// Settings 设置（工作区、上次会话）
	Settings *Settings
	// Sessions 会话（历史回放、隐藏台账）
	Sessions *Sessions
	// Runtime 内置运行时
	Runtime *engine.Runtime
	// Hub 事件广播
	Hub *sse.Hub
	// Stream 事件翻译器
	Stream *chat.Stream

	// mu 保护下面的绑定状态：整块换绑定必须原子，否则会有半新半旧的快照。
	mu      sync.Mutex
	rpc     *chat.PiRpc
	rpcWs   string
	file    string
	level   string
	levels  []string
	cmds    []domain.SlashCommandVO
	autocmp bool
	queued  domain.QueuedMessagesVO
	bindErr string
}

// NewChat 造一个对话门面。
func NewChat(settings *Settings, sessions *Sessions, rt *engine.Runtime, hub *sse.Hub) *Chat {
	return &Chat{
		Settings: settings,
		Sessions: sessions,
		Runtime:  rt,
		Hub:      hub,
		Stream:   chat.NewStream(hub),
		level:    "medium",
		levels:   []string{"off", "minimal", "low", "medium", "high"},
		autocmp:  true,
	}
}

// EnsureRpc 确保子进程在跑且绑定当前工作区。
func (c *Chat) EnsureRpc() (*chat.PiRpc, error) {
	workspace := c.Settings.Workspace()
	if workspace == "" {
		return nil, pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	c.mu.Lock()
	current := c.rpc
	boundWs := c.rpcWs
	c.mu.Unlock()
	if current != nil && current.Alive() && boundWs == workspace {
		return current, nil
	}
	if current != nil {
		current.Stop()
	}

	nodeExe, err := c.Runtime.NodeExe()
	if err != nil {
		return nil, err
	}
	if !c.Runtime.PiInstalled() {
		return nil, pkg.Err(pkg.CodeRpcDown, "pi 尚未安装，请先在应用页安装")
	}
	client := chat.NewPiRpc(nodeExe, c.Runtime.PiEntry(), workspace, c.Runtime.Layout.SandboxEnv(nil), chat.Handlers{
		OnEvent:     c.onEngineEvent,
		OnUiRequest: c.onUiRequest,
		OnExit: func(code int) {
			c.mu.Lock()
			c.rpc = nil
			c.mu.Unlock()
			c.Hub.Broadcast("engine:exit", sse.EngineTag(map[string]any{"exit_code": code}))
			pkg.Info("pi 子进程退出：code=%d", code)
		},
	})
	if err := client.Start(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.rpc = client
	c.rpcWs = workspace
	c.file = ""
	c.cmds = nil
	c.mu.Unlock()

	state, err := client.Request(chat.Record{"type": "get_state"}, 0)
	if err != nil {
		c.mu.Lock()
		c.bindErr = pkg.As(err).Message
		c.mu.Unlock()
		pkg.Error("pi 绑定失败：%v", err)
		return client, nil
	}
	c.applyState(state)
	c.mu.Lock()
	c.bindErr = ""
	c.mu.Unlock()
	c.Hub.Broadcast("engine:ready", sse.EngineTag(nil))
	go c.refreshCommands(client)
	go c.refreshLevels(client)
	return client, nil
}

// applyState 把 get_state 的应答写进绑定状态。
func (c *Chat) applyState(state any) {
	fields, ok := state.(map[string]any)
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if file := stringOf(fields["sessionFile"]); file != "" {
		c.file = file
	}
	if level := stringOf(fields["thinkingLevel"]); level != "" {
		c.level = level
	}
	if flag, ok := fields["autoCompactionEnabled"].(bool); ok {
		c.autocmp = flag
	}
}

// refreshCommands 拉斜杠命令清单。
func (c *Chat) refreshCommands(client *chat.PiRpc) {
	data, err := client.Request(chat.Record{"type": "get_commands"}, 0)
	if err != nil {
		return
	}
	fields, ok := data.(map[string]any)
	if !ok {
		return
	}
	items, _ := fields["commands"].([]any)
	cmds := make([]domain.SlashCommandVO, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := stringOf(entry["name"])
		if name == "" {
			continue
		}
		cmds = append(cmds, domain.SlashCommandVO{
			CommandName:   name,
			CommandSource: defaultIfEmpty(stringOf(entry["source"]), "prompt"),
			CommandDesc:   stringOf(entry["description"]),
		})
	}
	c.mu.Lock()
	c.cmds = cmds
	c.mu.Unlock()
	c.Hub.Broadcast("engine:commands", sse.EngineTag(map[string]any{"commands": cmds}))
}

// refreshLevels 拉可用思考档。
func (c *Chat) refreshLevels(client *chat.PiRpc) {
	data, err := client.Request(chat.Record{"type": "get_available_thinking_levels"}, 0)
	if err != nil {
		return
	}
	fields, ok := data.(map[string]any)
	if !ok {
		return
	}
	levels := asStringSlice(fields["levels"])
	if len(levels) == 0 {
		return
	}
	c.mu.Lock()
	c.levels = levels
	c.mu.Unlock()
}

// onEngineEvent 引擎会话事件 → 翻译器。
func (c *Chat) onEngineEvent(rec chat.Record) {
	if stringOf(rec["type"]) == "queue_update" {
		c.mu.Lock()
		c.queued = domain.QueuedMessagesVO{
			Steering: asStringSlice(rec["steering"]),
			FollowUp: asStringSlice(rec["followUp"]),
		}
		c.mu.Unlock()
	}
	c.Stream.OnEvent(rec)
}

// onUiRequest 引擎 UI 请求：对话框转提问卡，其余转通知。
func (c *Chat) onUiRequest(rec chat.Record) {
	method := stringOf(rec["method"])
	id := stringOf(rec["id"])
	if id != "" && isDialogMethod(method) {
		ask := domain.AskRequestVO{
			AskId:          id,
			AskMethod:      method,
			AskTitle:       stringOf(rec["title"]),
			AskMessage:     stringOf(rec["message"]),
			AskPlaceholder: stringOf(rec["placeholder"]),
			AskPrefill:     stringOf(rec["prefill"]),
			AskOptions:     asStringSlice(rec["options"]),
			AskTimeoutMs:   chatInt(rec["timeout"]),
		}
		c.Hub.Broadcast("chat:ask", sse.EngineTag(map[string]any{
			"ask_id":          ask.AskId,
			"ask_method":      ask.AskMethod,
			"ask_title":       ask.AskTitle,
			"ask_message":     ask.AskMessage,
			"ask_placeholder": ask.AskPlaceholder,
			"ask_prefill":     ask.AskPrefill,
			"ask_options":     ask.AskOptions,
			"ask_timeout_ms":  ask.AskTimeoutMs,
		}))
		return
	}
	c.Stream.OnUiRequest(rec)
}

// isDialogMethod 判断是不是需要用户回答的对话框。
func isDialogMethod(method string) bool {
	switch method {
	case "select", "confirm", "input", "editor":
		return true
	}
	return false
}

// chatInt 安全取整数。
func chatInt(v any) int {
	if n, ok := v.(float64); ok {
		return int(n)
	}
	return 0
}

// State 聊天状态总装：引擎实况 + 会话文件回放。
func (c *Chat) State(sessionFile string) domain.ChatStateRESP {
	client, err := c.EnsureRpc()
	if err != nil {
		return c.emptyState(pkg.As(err).Message)
	}
	if sessionFile != "" && sessionFile != c.currentFile() {
		if _, err := client.Request(chat.Record{"type": "switch_session", "sessionPath": sessionFile}, 0); err != nil {
			return c.emptyState("切换会话失败：" + pkg.As(err).Message)
		}
		c.mu.Lock()
		c.file = sessionFile
		c.mu.Unlock()
		c.Settings.RememberSession(sessionFile)
	}
	state, err := client.Request(chat.Record{"type": "get_state"}, 0)
	if err != nil {
		return c.emptyState(pkg.As(err).Message)
	}
	c.applyState(state)

	stats, _ := client.Request(chat.Record{"type": "get_session_stats"}, 0)
	c.mu.Lock()
	file := c.file
	workspace := c.rpcWs
	level := c.level
	levels := append([]string(nil), c.levels...)
	cmds := append([]domain.SlashCommandVO(nil), c.cmds...)
	autocmp := c.autocmp
	queued := c.queued
	bindErr := c.bindErr
	c.mu.Unlock()
	c.Settings.RememberSession(file)

	resp := domain.ChatStateRESP{
		EngineKind:     domain.EnginePi,
		SessionFile:    file,
		WorkDir:        workspace,
		EngineRunning:  client.Alive(),
		Streaming:      c.Stream.Active(),
		Model:          parseModelInfo(state),
		ThinkingLevel:  level,
		ThinkingLevels: levels,
		AutoCompaction: autocmp,
		Queued:         queued,
		History:        c.Sessions.History(file),
		Commands:       cmds,
		Caps:           domain.PiCaps,
		BindErr:        bindErr,
	}
	resp.Stats, resp.ContextUsage = parseStats(stats)
	return resp
}

// parseModelInfo 从 get_state 应答里取当前模型。
func parseModelInfo(state any) *domain.ModelInfoVO {
	fields, ok := state.(map[string]any)
	if !ok {
		return nil
	}
	model, ok := fields["model"].(map[string]any)
	if !ok {
		return nil
	}
	info := &domain.ModelInfoVO{
		ModelId:           stringOf(model["id"]),
		ModelName:         stringOf(model["name"]),
		ProviderName:      stringOf(model["provider"]),
		ApiProtocol:       stringOf(model["api"]),
		ContextWindow:     chatInt(model["contextWindow"]),
		MaxTokens:         chatInt(model["maxTokens"]),
		InputKinds:        asStringSlice(model["input"]),
		SupportsReasoning: false,
	}
	if flag, ok := model["reasoning"].(bool); ok {
		info.SupportsReasoning = flag
	}
	return info
}

// parseStats 从 get_session_stats 应答里取累计用量与上下文占用。
func parseStats(stats any) (*domain.SessionStatsVO, *domain.ContextUsageVO) {
	fields, ok := stats.(map[string]any)
	if !ok {
		return nil, nil
	}
	out := &domain.SessionStatsVO{
		SessionId:         stringOf(fields["sessionId"]),
		UserMessages:      chatInt(fields["userMessages"]),
		AssistantMessages: chatInt(fields["assistantMessages"]),
		ToolCalls:         chatInt(fields["toolCalls"]),
		TotalMessages:     chatInt(fields["totalMessages"]),
	}
	if tokens, ok := fields["tokens"].(map[string]any); ok {
		out.InputTokens = chatInt(tokens["input"])
		out.OutputTokens = chatInt(tokens["output"])
		out.CacheReadTokens = chatInt(tokens["cacheRead"])
		out.CacheWriteTokens = chatInt(tokens["cacheWrite"])
		out.TotalTokens = chatInt(tokens["total"])
	}
	if cost, ok := fields["cost"].(float64); ok {
		out.CostUsd = cost
	}
	usage := parseContextUsage(fields["contextUsage"])
	out.ContextUsage = usage
	return out, usage
}

// parseContextUsage 解析上下文占用；压缩后还没新读数时两个值都是 null。
func parseContextUsage(raw any) *domain.ContextUsageVO {
	fields, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := &domain.ContextUsageVO{ContextWindow: chatInt(fields["contextWindow"])}
	if tokens, ok := fields["tokens"].(float64); ok {
		value := int(tokens)
		out.UsageTokens = &value
	}
	if percent, ok := fields["percent"].(float64); ok {
		out.UsagePercent = &percent
	}
	return out
}

// emptyState 绑定失败时给一个能画的空状态：界面据此显示原因，而不是白屏。
func (c *Chat) emptyState(reason string) domain.ChatStateRESP {
	c.mu.Lock()
	levels := append([]string(nil), c.levels...)
	cmds := append([]domain.SlashCommandVO(nil), c.cmds...)
	c.mu.Unlock()
	return domain.ChatStateRESP{
		EngineKind:     domain.EnginePi,
		ThinkingLevel:  "medium",
		ThinkingLevels: levels,
		AutoCompaction: true,
		Commands:       cmds,
		Caps:           domain.PiCaps,
		BindErr:        reason,
	}
}

// currentFile 当前绑定的会话文件。
func (c *Chat) currentFile() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.file
}

// rememberCurrent 记住当前会话（供界面刷新后恢复）。
func (c *Chat) rememberCurrent() {
	if file := c.currentFile(); file != "" {
		c.Settings.RememberSession(file)
	}
}

// Runtime 是引擎运行实例信息（概览页用）。
type Runtime struct {
	// Pid 子进程 PID
	Pid int
	// Running 是否存活
	Running bool
	// Busy 是否正在跑一轮
	Busy bool
	// SessionFile 当前会话
	SessionFile string
	// WorkDir 绑定工作区
	WorkDir string
	// StartedAt 启动时刻（毫秒）
	StartedAt int64
	// CallCount 已发出的命令数
	CallCount int
	// LastError 最近一次失败原因
	LastError string
}

// Instance 取当前引擎实例的运行信息。
func (c *Chat) Instance() Runtime {
	c.mu.Lock()
	client := c.rpc
	file := c.file
	workspace := c.rpcWs
	c.mu.Unlock()
	out := Runtime{SessionFile: file, WorkDir: workspace, Busy: c.Stream.Active()}
	if client != nil {
		out.Pid = client.Pid()
		out.Running = client.Alive()
		out.StartedAt = client.StartedAt
		out.CallCount = client.CallCount
		out.LastError = client.LastError
	}
	return out
}

// Restart 重启引擎：杀子进程，下次用到再拉。
func (c *Chat) Restart() {
	c.mu.Lock()
	client := c.rpc
	c.rpc = nil
	c.rpcWs = ""
	c.mu.Unlock()
	if client != nil {
		client.Stop()
	}
	c.Stream.Idle()
	c.Hub.Broadcast("engine:status", sse.EngineTag(nil))
}

// OnWorkspaceChanged 工作区切换时调用：绑定失效即重启。
func (c *Chat) OnWorkspaceChanged() {
	c.mu.Lock()
	bound := c.rpcWs
	c.mu.Unlock()
	if bound != "" && bound != c.Settings.Workspace() {
		c.Restart()
	}
}

// Shutdown 退出前收尾：给引擎一个短超时，别让退出卡住。
func (c *Chat) Shutdown() {
	c.mu.Lock()
	client := c.rpc
	c.rpc = nil
	c.mu.Unlock()
	if client != nil {
		client.Stop()
	}
}

// guardSessionPath 会话路径必须落在会话根内。
func (c *Chat) guardSessionPath(file string) (string, error) {
	if file == "" {
		return "", nil
	}
	return parser.GuardSession(c.Runtime.Layout.SessionsDir(), file)
}
