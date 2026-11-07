package service

// 聊天状态总装：一次请求给出界面画聊天页要用的全部事实。
// 从 chat_service 拆出来只为守 500 行硬限，语义不变。

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/chat"
)

// State 聊天状态总装：引擎实况 + 会话文件回放。
// 引擎没就绪时不把打开应用的这条路卡在冷启动上：历史与待办先从会话文件回放，
// 引擎交给后台预热，就绪后 engine:ready 会带着界面再刷一次实况。
func (c *Chat) State(sessionFile string) domain.ChatStateRESP {
	workspace := c.Settings.Workspace()
	if !c.engineBound(workspace) {
		c.Warm()
		return c.replayState(sessionFile, workspace)
	}
	c.mu.Lock()
	client := c.rpc
	c.mu.Unlock()
	if sessionFile != "" && sessionFile != c.currentFile() {
		if err := c.alignSession(client, sessionFile); err != nil {
			return c.emptyState("切换会话失败：" + pkg.As(err).Message)
		}
	}
	state, err := client.Request(chat.Record{"type": "get_state"}, 0)
	if err != nil {
		return c.emptyState(pkg.As(err).Message)
	}
	c.applyState(state)

	stats, _ := client.Request(chat.Record{"type": "get_session_stats"}, 0)
	c.mu.Lock()
	file := c.file
	boundWs := c.rpcWs
	level := c.level
	levels := append([]string(nil), c.levels...)
	cmds := append([]domain.SlashCommandVO(nil), c.cmds...)
	autocmp := c.autocmp
	queued := c.queued
	c.mu.Unlock()

	resp := domain.ChatStateRESP{
		EngineKind:     domain.EnginePi,
		SessionFile:    file,
		WorkDir:        boundWs,
		EngineRunning:  client.Alive(),
		Streaming:      c.Stream.Active(),
		Model:          parseModelInfo(state),
		ThinkingLevel:  level,
		ThinkingLevels: levels,
		AutoCompaction: autocmp,
		Queued:         queued,
		History:        c.Sessions.History(file),
		Todos:          c.Sessions.Todos(file),
		Commands:       cmds,
		Caps:           domain.PiCaps,
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

// engineBound 引擎是否活着且绑定当前工作区。绑定是快路径的前提：
// 活着但绑在别的工作区，等于没绑。
func (c *Chat) engineBound(workspace string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rpc != nil && c.rpc.Alive() && c.rpcWs == workspace
}

// replayState 引擎还没就绪时的回放态：历史与待办从会话文件来，不欠引擎的。
// BindErr 说的是现状而不是故障——预热结果会经 engine:ready 或通知跟进。
func (c *Chat) replayState(sessionFile, workspace string) domain.ChatStateRESP {
	c.mu.Lock()
	levels := append([]string(nil), c.levels...)
	cmds := append([]domain.SlashCommandVO(nil), c.cmds...)
	c.mu.Unlock()
	reason := "引擎正在后台启动，就绪后自动连上"
	switch {
	case workspace == "":
		reason = "请先在设置里选择工作区"
	case !c.Runtime.Layout.NodeReady() || !c.Runtime.PiInstalled():
		reason = "pi 引擎尚未就绪，等应用页装配完成"
	}
	guarded, err := c.guardSessionPath(sessionFile)
	if err != nil {
		guarded = ""
	}
	resp := domain.ChatStateRESP{
		EngineKind:     domain.EnginePi,
		SessionFile:    guarded,
		WorkDir:        workspace,
		EngineRunning:  false,
		ThinkingLevel:  "medium",
		ThinkingLevels: levels,
		AutoCompaction: true,
		Commands:       cmds,
		Caps:           domain.PiCaps,
		BindErr:        reason,
	}
	// 回放自带头部守卫：越界路径在这里落空，拿到的是空历史而不是报错。
	if guarded != "" {
		resp.History = c.Sessions.History(guarded)
		resp.Todos = c.Sessions.Todos(guarded)
	}
	return resp
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
