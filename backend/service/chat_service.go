package service

import (
	"sync"
	"sync/atomic"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/chat"
	"WorkBaby/backend/service/engine"
	"WorkBaby/backend/service/parser"
	"WorkBaby/backend/sse"
)

// Chat 是对话编排：一个工作区一个引擎子进程；send / stop / steer / 切会话。
// 状态总装在 chat_state.go。
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
	mu sync.Mutex
	// startMu 串行化引擎的「检查→停旧→拉新→换绑」整段：并发 EnsureRpc 只允许拉起一个引擎。
	startMu sync.Mutex
	rpc     *chat.PiRpc
	rpcWs   string
	file    string
	level   string
	levels  []string
	cmds    []domain.SlashCommandVO
	autocmp bool
	queued  domain.QueuedMessagesVO
	// crashStreak 连续崩溃次数：自动重启的退避基数，成功绑定后归零。
	crashStreak int
	// warming 是否已有一个预热在跑：没有它，状态接口每次未命中都会再排一个 EnsureRpc。
	warming atomic.Bool
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
// 整段在 startMu 内：检查与换绑分离曾允许并发请求各拉一个引擎、互相覆盖绑定。
func (c *Chat) EnsureRpc() (*chat.PiRpc, error) {
	workspace := c.Settings.Workspace()
	if workspace == "" {
		return nil, pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	c.startMu.Lock()
	defer c.startMu.Unlock()

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
	var client *chat.PiRpc
	client = chat.NewPiRpc(nodeExe, c.Runtime.PiEntry(), workspace, c.Runtime.Layout.SandboxEnv(nil), chat.Handlers{
		OnEvent:     c.onEngineEvent,
		OnUiRequest: c.onUiRequest,
		OnExit: func(code int) {
			c.mu.Lock()
			// 只有「退出的仍是当前绑定实例」才清绑：被换下的旧进程退出得晚，不能抹掉新绑定。
			// 清绑成功即视为意外退出（主动 Restart / Shutdown 先把 c.rpc 置 nil，不走这里）。
			if c.rpc == client {
				c.rpc = nil
				c.mu.Unlock()
				c.scheduleRestart()
			} else {
				c.mu.Unlock()
			}
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
		// 进程起来了就说明这次拉起是成功的：崩溃计数按「起不来」算，
		// 不是按「握手没答上」。不归零的话，连续几次握手失败会永久停掉自动重启。
		c.mu.Lock()
		c.crashStreak = 0
		// 先摘绑定再关停：OnExit 只清「仍是当前实例」的绑定，摘干净它就不会
		// 把这次握手失败再记成一次意外崩溃去叠加自动重启。
		c.rpc = nil
		c.rpcWs = ""
		c.mu.Unlock()
		client.Stop()
		pkg.Error("pi 绑定失败：%v", err)
		return nil, pkg.Err(pkg.CodeRpcDown, "pi 引擎握手失败", pkg.As(err).Message)
	}
	c.applyState(state)
	c.mu.Lock()
	// 新进程不继承旧进程的排队镜像：队列真相在引擎里，镜像只配对当前实例。
	c.queued = domain.QueuedMessagesVO{}
	c.crashStreak = 0
	c.mu.Unlock()
	c.Hub.Broadcast("engine:ready", sse.EngineTag(nil))
	go c.refreshCommands(client)
	go c.refreshLevels(client)
	return client, nil
}

// Warm 后台把引擎拉起来。打开应用就该预热，而不是等用户发第一句话时
// 才陪着十几个插件的冷加载一起等；已就绪或已有预热在跑时是空操作。
func (c *Chat) Warm() {
	workspace := c.Settings.Workspace()
	if workspace == "" || !c.Runtime.Layout.NodeReady() || !c.Runtime.PiInstalled() {
		return
	}
	c.mu.Lock()
	alive := c.rpc != nil && c.rpc.Alive()
	c.mu.Unlock()
	if alive || !c.warming.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer c.warming.Store(false)
		if _, err := c.EnsureRpc(); err != nil {
			pkg.Warn("引擎预热失败：%v", err)
			// 预热失败只发一条通知：真正的原因留给用户下次动作时的报错，
			// 这里不拦界面——对话页的历史回放不依赖引擎。
			c.Hub.Broadcast("chat:notify", sse.EngineTag(map[string]any{
				"level":   "warn",
				"message": "引擎后台启动失败：" + pkg.As(err).Message,
			}))
			return
		}
		pkg.Info("引擎预热完成")
	}()
}

// scheduleRestart 引擎意外退出后的自动拉起：指数退避，连崩 5 次就停手交给用户。
// 稳定性优先于不休止的重试——坏掉的引擎反复拉起只会反复打断用户。
func (c *Chat) scheduleRestart() {
	c.mu.Lock()
	if c.crashStreak >= 5 {
		c.mu.Unlock()
		pkg.Warn("pi 连续崩溃 %d 次，停止自动重启，等下一次手动操作", c.crashStreak)
		return
	}
	delay := time.Duration(2<<c.crashStreak) * time.Second
	c.crashStreak++
	c.mu.Unlock()
	pkg.Warn("pi 意外退出，%s 后自动重启", delay)
	time.AfterFunc(delay, func() {
		// 到点时绑定已被手动操作补上就不再叠加：自动重启只为填补空档。
		c.mu.Lock()
		if c.rpc != nil {
			c.mu.Unlock()
			return
		}
		c.mu.Unlock()
		if _, err := c.EnsureRpc(); err != nil {
			pkg.Warn("pi 自动重启失败：%v", err)
			return
		}
		pkg.Info("pi 自动重启完成")
	})
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

// KillEngine 退出时兜底：把引擎进程树整棵杀掉。
// 退出流程只给 1.5 秒，等不了 Stop 的 5 秒优雅期；pi 与它的 bash 子孙留在后台
// 会占着 CPU、内存与包目录的锁，下次启动撞上「目录被占用」。
func (c *Chat) KillEngine() {
	c.mu.Lock()
	client := c.rpc
	c.rpc = nil
	c.mu.Unlock()
	if client == nil {
		return
	}
	if pid := client.Pid(); pid > 0 {
		pkg.KillTree(pid)
	}
}

// guardSessionPath 会话路径必须落在会话根内。
func (c *Chat) guardSessionPath(file string) (string, error) {
	if file == "" {
		return "", nil
	}
	return parser.GuardSession(c.Runtime.Layout.SessionsDir(), file)
}
