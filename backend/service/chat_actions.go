package service

import (
	"strings"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// composed 是一条要发给引擎的话术与附件。
type composed struct {
	// text 折进消息正文的文本（文件附件以路径引用形式追加）
	text string
	// images 走内容块的图片
	images []agent.Block
}

// compose 把附件折进消息：图片走内容块，文件走路径引用。
// 为什么文件不内联：引擎有读文件工具，让它自己按需读比塞进上下文省得多。
func compose(message string, attachments []domain.ChatAttachmentVO) composed {
	out := composed{text: message}
	for _, att := range attachments {
		switch att.AttachmentKind {
		case "image":
			if att.ImageBase64 == "" {
				continue
			}
			mime := att.ImageMime
			if mime == "" {
				mime = "image/png"
			}
			out.images = append(out.images, agent.Block{
				Kind: agent.KindImage, ImageBase64: att.ImageBase64, ImageMime: mime,
			})
		case "file":
			if att.FilePath == "" {
				continue
			}
			out.text += "\n\n[附件] " + att.FilePath
		case "session":
			// 会话引用：整段正文直接折进消息（会话文件在工作区外，引擎读不到）。
			if att.AttachmentBody == "" {
				continue
			}
			out.text += "\n\n" + att.AttachmentBody
		}
	}
	return out
}

// message 组装一条用户消息：有图时文字块在前，图片块随后。
func (c composed) message() agent.Message {
	blocks := make([]agent.Block, 0, len(c.images)+1)
	if text := strings.TrimSpace(c.text); text != "" {
		blocks = append(blocks, agent.Block{Kind: agent.KindText, Text: c.text})
	}
	blocks = append(blocks, c.images...)
	return agent.Message{Role: agent.RoleUser, Blocks: blocks, CreatedAt: pkg.NowMs()}
}

// Compact 压缩上下文：让模型把已有对话总结成一条，替换掉冗长的历史。
// 压缩在会话文件里是追加一条 compaction 条目，原文不删——回放仍能看清发生过什么。
func (c *Chat) Compact(sessionFile, instructions string) error {
	if c.Busy() {
		return pkg.Err(pkg.CodeEngineBusy, "有任务正在跑，等它结束再压缩")
	}
	session, err := c.BindSession(sessionFile)
	if err != nil {
		return err
	}
	if _, err := session.CompactHint(strings.TrimSpace(instructions)); err != nil {
		return err
	}
	c.Hub.Broadcast("chat:compaction", map[string]any{"state": "done", "engine_kind": string(domain.EngineNative)})
	return nil
}

// Answer 回答提问卡：把回答送到正在等的那次提问上。
// 提问是阻塞式的——模型在等这一句，所以回答到了它就继续往下走。
// 提问已经不在等（比如那一轮已经结束）时给可读原因，不静默吞掉。
func (c *Chat) Answer(req domain.ChatAnswerREQ) error {
	response := agent.AskResponse{
		Value:     strings.TrimSpace(req.Value),
		Confirmed: req.Confirmed != nil && *req.Confirmed,
		Cancelled: req.Cancelled,
	}
	if response.Value == "" && len(req.Answers) > 0 {
		first := req.Answers[0]
		response.Value = strings.Join(first.SelectedOptions, ", ")
		if response.Value == "" {
			response.Value = first.CustomInput
		}
	}
	c.mu.Lock()
	waiter, waiting := c.waiters[req.AskId]
	c.mu.Unlock()
	if !waiting {
		// 这条提问可能已经超时作废；把它当一条普通回复退回会话，
		// 用户说的话不该凭空消失。
		if response.Value == "" && !response.Cancelled {
			return pkg.Err(pkg.CodeBadRequest, "回答内容不能为空")
		}
		return c.appendAnswer(response)
	}
	select {
	case waiter <- response:
		return nil
	default:
		// 通道已满说明已经有回答在排队，丢弃这次的重复提交。
		return nil
	}
}

// appendAnswer 把一段回答作为用户消息追加进会话。
func (c *Chat) appendAnswer(response agent.AskResponse) error {
	text := response.Value
	if response.Cancelled {
		text = "（用户取消了那次提问）"
	}
	if strings.TrimSpace(text) == "" {
		return pkg.Err(pkg.CodeBadRequest, "回答内容不能为空")
	}
	session, err := c.BindSession("")
	if err != nil {
		return err
	}
	_, err = session.AppendMessage(agent.Message{
		Role:      agent.RoleUser,
		Blocks:    []agent.Block{{Kind: agent.KindText, Text: text}},
		CreatedAt: pkg.NowMs(),
	})
	return err
}

// RemoveQueued 单条移除排队消息。
// 原生引擎的排队是本进程内的切片，所以这是真正的「摘掉一条」，
// 不再需要「整队取出再重投」那套会丢话术的绕法。
func (c *Chat) RemoveQueued(kind string, index int) (domain.QueuedMessagesVO, error) {
	if kind != "steering" && kind != "followup" {
		return domain.QueuedMessagesVO{}, pkg.Err(pkg.CodeBadRequest, "队列类型只能是 steering 或 followup")
	}
	c.mu.Lock()
	runner := c.runner
	c.mu.Unlock()
	if runner == nil {
		return domain.QueuedMessagesVO{}, pkg.Err(pkg.CodeNotFound, "当前没有正在进行的运行")
	}
	if err := runner.RemoveQueued(index); err != nil {
		return domain.QueuedMessagesVO{}, err
	}
	c.refreshQueue()
	c.mu.Lock()
	out := c.queued
	c.mu.Unlock()
	return out, nil
}

// Export 导出会话为 HTML。
func (c *Chat) Export(sessionFile string) (domain.ChatExportRESP, error) {
	file := sessionFile
	if file == "" {
		file = c.CurrentFile()
	}
	if file == "" {
		return domain.ChatExportRESP{}, pkg.Err(pkg.CodeNotFound, "当前还没有会话可导出")
	}
	return c.Sessions.Export(file)
}

// SetAutoCompaction 自动压缩开关：记进设置，由下一次运行读取。
func (c *Chat) SetAutoCompaction(_ string, enabled bool) error {
	return c.Settings.SaveSettings(map[string]string{KeyAutoCompaction: boolString(enabled)})
}
