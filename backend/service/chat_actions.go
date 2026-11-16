package service

import (
	"context"
	"strings"
	"time"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/parser"
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
// 压缩在会话文件里是追加一条带摘要的 compaction 条目，原文不删——回放仍能看清发生过什么。
// 总结失败时整个动作失败：写一条空摘要进去等于骗用户「压过了」。
func (c *Chat) Compact(sessionFile, instructions string) error {
	if c.Busy() {
		return pkg.Err(pkg.CodeEngineBusy, "有任务正在跑，等它结束再压缩")
	}
	session, err := c.BindSession(sessionFile)
	if err != nil {
		return err
	}
	return c.compactNow(session, strings.TrimSpace(instructions))
}

// compactNow 是压缩的本体：调上游做一次同步总结，成功后落压缩条目。
// 不查 Busy：自动压缩在运行起跑前调用，那时没有运行；手动路径的拒绝留在 Compact 里。
// 总结失败返回错误但由调用方决定要不要打扰用户——自动路径静默落日志。
func (c *Chat) compactNow(session *agent.Session, instructions string) error {
	model, err := c.resolveModel()
	if err != nil {
		return err
	}
	messages := session.Entries()
	prompt := compactionPrompt(messages, instructions)
	if strings.TrimSpace(prompt) == "" {
		return pkg.Err(pkg.CodeBadRequest, "还没有可压缩的对话")
	}

	c.Hub.Broadcast("chat:compaction", map[string]any{"state": "start", "engine_kind": string(domain.EngineNative)})
	summary, err := c.summarize(model, prompt)
	if err != nil {
		pkg.Warn("压缩总结失败 code=%s：%s", pkg.As(err).Code, err.Error())
		c.Hub.Broadcast("chat:compaction", map[string]any{"state": "done", "engine_kind": string(domain.EngineNative)})
		return err
	}
	if _, err := session.AppendCompaction(strings.TrimSpace(summary), session.ContextUsage()); err != nil {
		pkg.Warn("压缩条目写入失败 code=%s：%s", pkg.As(err).Code, err.Error())
		c.Hub.Broadcast("chat:compaction", map[string]any{"state": "done", "engine_kind": string(domain.EngineNative)})
		return err
	}
	pkg.Info("上下文已压缩（会话 %s，摘要 %d 字）", session.Id(), len([]rune(summary)))
	c.Hub.Broadcast("chat:compaction", map[string]any{"state": "done", "engine_kind": string(domain.EngineNative)})
	return nil
}

// summarize 发一次同步总结调用：不经过 runner、不带工具、不发思考档——
// 压缩是一次后台整理，模型只管读历史吐摘要。复用共享连接池。
func (c *Chat) summarize(model agent.ModelRef, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	stream, err := c.streamer.Stream(ctx, agent.Request{
		Model:    model,
		System:   compactionSystem,
		Messages: []agent.Message{userMessageOf(prompt)},
	})
	if err != nil {
		return "", pkg.WrapErr(pkg.CodeProvision, "压缩总结调不通上游", err)
	}
	var text strings.Builder
	var lastErr string
	for ev := range stream {
		switch ev.Kind {
		case agent.EventText:
			text.WriteString(ev.Text)
		case agent.EventStop:
			if ev.StopReason == "error" {
				lastErr = ev.Text
			}
		}
	}
	if lastErr != "" {
		return "", pkg.Err(pkg.CodeProvision, "压缩总结被上游拒绝", lastErr)
	}
	out := strings.TrimSpace(text.String())
	if out == "" {
		return "", pkg.Err(pkg.CodeProvision, "压缩总结返回了空内容")
	}
	return out, nil
}

// compactionSystem 是总结调用的系统提示：只要摘要，不要对话。
const compactionSystem = "你是对话整理助手。把给出的对话历史压缩成一段摘要，保留用户的意图、已做的决定、未完成的事项与关键上下文。直接输出摘要正文，不要寒暄，不要评论。"

// compactionPrompt 把历史折成总结调用的正文；压缩指示有就带上。
// 总结输入自身也要限长：带着同样超窗的历史去请求摘要，恰在需要压缩时被上游拒掉。
func compactionPrompt(messages []domain.SessionEntryDO, instructions string) string {
	var out strings.Builder
	for _, entry := range messages {
		msg := entry.EntryMessage
		if msg == nil {
			continue
		}
		role := msg.MessageRole
		if isToolRole(role) {
			role = "tool"
		}
		text := strings.TrimSpace(parser.ContentText(msg.MessageContent))
		if text == "" {
			continue
		}
		out.WriteString(role + ": " + pkg.Truncate(text, 2000) + "\n\n")
	}
	body := strings.TrimSpace(out.String())
	// 保尾裁头：最近的对话对「接着干」最值钱，超长时丢最早的部分。
	const maxPromptChars = 120_000
	if runes := []rune(body); len(runes) > maxPromptChars {
		body = "（更早的对话已省略）\n" + string(runes[len(runes)-maxPromptChars:])
	}
	if instructions != "" {
		body += "\n\n用户对摘要的额外要求：" + instructions
	}
	return body
}

// isToolRole 判断一个落盘角色是不是工具结果。
func isToolRole(role string) bool {
	return role == string(agent.RoleTool) || role == "toolResult"
}

// userMessageOf 组装一条纯文本用户消息。
func userMessageOf(text string) agent.Message {
	return agent.Message{
		Role:      agent.RoleUser,
		Blocks:    []agent.Block{{Kind: agent.KindText, Text: text}},
		CreatedAt: pkg.NowMs(),
	}
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
