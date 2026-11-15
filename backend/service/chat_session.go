package service

// 会话生命周期动作：克隆、分叉、改名、切模型与思考档、预热与收尾。
// 与「怎么发一句话」分开——那些是运行，这些是会话本身的处置。

import (
	"strings"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/sse"
)

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

// SetThinking 记下思考档。档位写进设置台账：重启后还是这一档，
// 会话进行中改它不会重写已发生的对话。
func (c *Chat) SetThinking(_ string, level string) error {
	level = strings.TrimSpace(level)
	if !thinkingLevels[level] {
		return pkg.Err(pkg.CodeBadRequest, "未知的思考档："+level)
	}
	c.mu.Lock()
	c.level = level
	c.mu.Unlock()
	return c.Settings.SaveSettings(map[string]string{KeyThinkingLevel: level})
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
