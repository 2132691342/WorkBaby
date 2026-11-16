package service

// 聊天状态总装：一次请求给出界面画聊天页要用的全部事实。

import (
	"strings"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// State 聊天状态总装：会话实况 + 会话文件回放。
// 引擎在进程内，所以没有「等引擎冷启动」这件事：有会话就能画。
func (c *Chat) State(sessionFile string) domain.ChatStateRESP {
	workspace := c.Settings.Workspace()
	if workspace == "" {
		return c.emptyState("请先在设置里选择工作区")
	}
	session, err := c.BindSession(sessionFile)
	if err != nil {
		return c.emptyState(pkg.As(err).Message)
	}
	file := session.File()
	provider := c.Settings.Get(KeyActiveProvider)
	modelId := c.Settings.Get(KeyActiveModel)
	model := c.modelInfo(provider, modelId)

	c.mu.Lock()
	level := c.level
	queued := c.queued
	c.mu.Unlock()

	resp := domain.ChatStateRESP{
		EngineKind:     domain.EngineNative,
		SessionFile:    file,
		WorkDir:        workspace,
		EngineRunning:  true,
		Streaming:      c.Busy(),
		Model:          model,
		ThinkingLevel:  level,
		ThinkingLevels: []string{"off", "low", "medium", "high"},
		AutoCompaction: c.Settings.Get(KeyAutoCompaction) != "0",
		Queued:         queued,
		History:        c.Sessions.History(file),
		Todos:          c.Sessions.Todos(file),
		PlanMode:       c.Sessions.PlanMode(file),
		Commands:       c.commands(),
		Caps:           domain.NativeCaps,
	}
	planState := c.currentPlanState()
	resp.PlanState = &planState
	resp.ContextUsage, resp.Stats = c.usageSnapshot(session, model)
	resp.ContextWarn = c.contextWarnText(resp.ContextUsage, resp.AutoCompaction)
	resp.AskPending = c.AskPending()
	if provider == "" || modelId == "" {
		resp.BindErr = "还没有选择模型，先去模型页选一个"
	}
	return resp
}

// usageSnapshot 组装上下文占用与会话统计。
// 上下文窗口取当前模型的定义；窗口未知（0）时不算水位，只有读数。
func (c *Chat) usageSnapshot(session *agent.Session, model *domain.ModelInfoVO) (*domain.ContextUsageVO, *domain.SessionStatsVO) {
	entries := session.Entries()
	used := session.ContextUsage()

	userMessages, assistantMessages, toolCalls := 0, 0, 0
	var inputTokens, outputTokens, cacheRead, cacheWrite, totalTokens int
	for _, entry := range entries {
		msg := entry.EntryMessage
		if msg == nil {
			continue
		}
		switch {
		case msg.MessageRole == string(agent.RoleUser):
			userMessages++
		case msg.MessageRole == string(agent.RoleAssistant):
			assistantMessages++
		case msg.MessageRole == string(agent.RoleTool):
			toolCalls++
		}
		if msg.MessageUsage != nil {
			// 分项也要加起来：界面的读数按「输入 / 输出 / 缓存」分开显示，
			// 只给合计等于把三个数都报成 0，那是假数据而不是缺数据。
			inputTokens += msg.MessageUsage.UsageInput
			outputTokens += msg.MessageUsage.UsageOutput
			cacheRead += msg.MessageUsage.UsageCacheRead
			cacheWrite += msg.MessageUsage.UsageCacheWrite
			totalTokens += msg.MessageUsage.UsageTotalTokens
		}
	}

	window := 0
	if model != nil {
		window = model.ContextWindow
	}
	usage := &domain.ContextUsageVO{UsageTokens: &used, ContextWindow: window}
	if window > 0 {
		percent := float64(used) / float64(window)
		usage.UsagePercent = &percent
	}
	stats := &domain.SessionStatsVO{
		SessionId:         session.Id(),
		UserMessages:      userMessages,
		AssistantMessages: assistantMessages,
		ToolCalls:         toolCalls,
		TotalMessages:     userMessages + assistantMessages + toolCalls,
		InputTokens:       inputTokens,
		OutputTokens:      outputTokens,
		CacheReadTokens:   cacheRead,
		CacheWriteTokens:  cacheWrite,
		TotalTokens:       totalTokens,
		ContextUsage:      usage,
	}
	return usage, stats
}

// contextWarnText 判断要不要提示上下文水位：自动压缩开着才提示——
// 压缩会在下一轮开始前自动发生；关了开关的话，提示也换不来一次压缩。
func (c *Chat) contextWarnText(usage *domain.ContextUsageVO, autoCompaction bool) string {
	if !autoCompaction || usage == nil || usage.UsagePercent == nil {
		return ""
	}
	if *usage.UsagePercent <= 0.8 {
		return ""
	}
	return "上下文占用已超过八成，下一轮开始前会自动压缩。"
}

// modelInfo 组装当前模型信息；没选模型时给 nil。
func (c *Chat) modelInfo(provider, modelId string) *domain.ModelInfoVO {
	if provider == "" || modelId == "" {
		return nil
	}
	entry, _, err := c.Models.Lookup(provider, modelId)
	if err != nil {
		return nil
	}
	return &domain.ModelInfoVO{
		ModelId:           modelId,
		ModelName:         defaultIfEmpty(entry.ModelName, modelId),
		ProviderName:      provider,
		ApiProtocol:       entry.ApiProtocol,
		ContextWindow:     entry.ContextWindow,
		MaxTokens:         entry.MaxTokens,
		SupportsReasoning: entry.SupportsReasoning,
		InputKinds:        entry.InputKinds,
	}
}

// commands 是当前可用的全部斜杠命令：内置命令 + 用户的提示词模板。
// 模板为空时只回内置清单，不多跑一次空扫描。
func (c *Chat) commands() []domain.SlashCommandVO {
	out := nativeCommands()
	if c.Templates != nil {
		out = append(out, c.Templates.Commands()...)
	}
	return out
}

// nativeCommands 是内置的斜杠命令清单。
func nativeCommands() []domain.SlashCommandVO {
	return []domain.SlashCommandVO{
		{CommandName: "new", CommandSource: "builtin", CommandDesc: "开一份新会话"},
		{CommandName: "compact", CommandSource: "builtin", CommandDesc: "压缩上下文，腾出空间"},
		{CommandName: "export", CommandSource: "builtin", CommandDesc: "把这份会话导出成 HTML"},
		{CommandName: "plan", CommandSource: "builtin", CommandDesc: "进入/退出计划模式：只读不改，先把方案想清楚"},
	}
}

// emptyState 给一个能画的空状态：界面据此显示原因，而不是白屏。
func (c *Chat) emptyState(reason string) domain.ChatStateRESP {
	c.mu.Lock()
	level := c.level
	c.mu.Unlock()
	return domain.ChatStateRESP{
		EngineKind:     domain.EngineNative,
		ThinkingLevel:  level,
		ThinkingLevels: []string{"off", "low", "medium", "high"},
		AutoCompaction: true,
		Commands:       c.commands(),
		Caps:           domain.NativeCaps,
		BindErr:        reason,
	}
}

// HandleCommand 处理输入框里的斜杠命令；返回 true 表示已被消费，不应再发给模型。
func (c *Chat) HandleCommand(text string) (bool, error) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return false, nil
	}
	fields := strings.Fields(trimmed)
	switch fields[0] {
	case "/new":
		file, err := c.NewSession()
		if err != nil {
			return true, err
		}
		c.Hub.Broadcast("session:info", nil)
		pkg.Info("新建会话：%s", file)
		return true, nil
	case "/compact":
		instructions := ""
		if len(fields) > 1 {
			instructions = strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))
		}
		return true, c.Compact("", instructions)
	case "/plan":
		return true, c.togglePlan(trimmed)
	case "/export":
		// 与前端内置 export（Markdown）语义区分：后端这条导出自包含 HTML。
		_, err := c.Export("")
		return true, err
	}
	return false, nil
}

// togglePlan 处理 /plan 命令：/plan 进入计划模式，/plan off 退出。
// 不带参数时按当前状态取反——用户敲 /plan 多半是想「开」，
// 但连续敲两次就成���，取反才符合「开关」的直觉。
func (c *Chat) togglePlan(text string) error {
	fields := strings.Fields(text)
	enabled := !c.PlanMode()
	if len(fields) > 1 {
		switch strings.ToLower(fields[1]) {
		case "on", "true", "1":
			enabled = true
		case "off", "false", "0":
			enabled = false
		}
	}
	return c.SetPlanMode(enabled)
}
