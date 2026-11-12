package service

// 聊天状态总装：一次请求给出界面画聊天页要用的全部事实。

import (
	"strings"

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
		Commands:       nativeCommands(),
		Caps:           domain.NativeCaps,
	}
	planState := c.currentPlanState()
	resp.PlanState = &planState
	if provider == "" || modelId == "" {
		resp.BindErr = "还没有选择模型，先去模型页选一个"
	}
	return resp
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
		Commands:       nativeCommands(),
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
