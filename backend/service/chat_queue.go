// 对话的两条排队：插话折进本轮上下文，追加话在本轮收尾后另起一轮。
// 与「怎么发一句话」分开成文件：这里的每一次操作都要同时改引擎侧队列与界面镜像，
// 而发送路径只负责起跑。
package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/sse"
)

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

// FollowUp 追加一条话：本轮收尾后它作为新的用户消息自动再起一轮，
// 与插话（Steering，折进本轮上下文）是两条不同的队。
func (c *Chat) FollowUp(message string, attachments []domain.ChatAttachmentVO) error {
	composed := compose(message, attachments)
	if strings.TrimSpace(composed.text) == "" {
		return pkg.Err(pkg.CodeBadRequest, "追加内容不能为空")
	}
	c.mu.Lock()
	runner := c.runner
	c.mu.Unlock()
	if runner == nil {
		return pkg.Err(pkg.CodeNotFound, "当前没有正在进行的运行")
	}
	runner.EnqueueFollowUp(composed.message())
	c.refreshQueue()
	return nil
}

// Stop 中止当前运行，并交回被丢掉的两条排队话术。
// 中止会连排队一起作废：用户刚打的字不能在没有任何提示的情况下消失，
// 所以把原文还回去，界面可以放回输入框。
func (c *Chat) Stop() domain.QueuedMessagesVO {
	c.mu.Lock()
	cancel := c.cancel
	runner := c.runner
	c.mu.Unlock()
	discarded := domain.QueuedMessagesVO{}
	if runner != nil {
		// 先取队再取消：取消之后运行协程会收回 runner，队就取不到了。
		discarded = domain.QueuedMessagesVO{
			Steering: runner.Queued(),
			FollowUp: runner.FollowUpTexts(),
		}
		runner.ClearQueued()
	}
	if cancel != nil {
		cancel()
	}
	c.mu.Lock()
	c.queued = domain.QueuedMessagesVO{}
	c.mu.Unlock()
	if len(discarded.Steering) > 0 || len(discarded.FollowUp) > 0 {
		c.Hub.Broadcast("chat:queue", sse.EngineTag(map[string]any{
			"steering":  []string{},
			"follow_up": []string{},
		}))
	}
	return discarded
}

// ClearQueue 清空两条排队并返回被清掉的文本。
// 引擎侧的队一并清：只清镜像的话，被「清掉」的插话下一轮仍会注入会话。
func (c *Chat) ClearQueue() domain.QueuedMessagesVO {
	c.mu.Lock()
	runner := c.runner
	c.mu.Unlock()
	if runner != nil {
		runner.ClearQueued()
	}
	c.mu.Lock()
	out := c.queued
	c.queued = domain.QueuedMessagesVO{}
	c.mu.Unlock()
	c.Hub.Broadcast("chat:queue", sse.EngineTag(map[string]any{
		"steering":  []string{},
		"follow_up": []string{},
	}))
	return out
}

// refreshQueue 把排队镜像刷成引擎侧的真实排队。
func (c *Chat) refreshQueue() {
	c.mu.Lock()
	runner := c.runner
	c.mu.Unlock()
	if runner == nil {
		return
	}
	c.mu.Lock()
	c.queued = domain.QueuedMessagesVO{
		Steering: runner.Queued(),
		FollowUp: runner.FollowUpTexts(),
	}
	out := c.queued
	c.mu.Unlock()
	c.Hub.Broadcast("chat:queue", sse.EngineTag(map[string]any{
		"steering":  out.Steering,
		"follow_up": out.FollowUp,
	}))
}
