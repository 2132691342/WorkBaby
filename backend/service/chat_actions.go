package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/chat"
)

// composed 是一条要发给引擎的话术与附件。
type composed struct {
	// text 折进消息正文的文本（文件附件以路径引用形式追加）
	text string
	// images 走 images 字段的图片（唯一的二进制通道）
	images []map[string]any
}

// compose 把附件折进消息：图片走 images 字段，文件走路径引用。
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
			out.images = append(out.images, map[string]any{
				"type": "image", "data": att.ImageBase64, "mimeType": mime,
			})
		case "file":
			if att.FilePath == "" {
				continue
			}
			out.text += "\n\n[附件] " + att.FilePath
		}
	}
	return out
}

// wireOf 组装一条 prompt / steer / follow_up 命令。
func (c composed) wire(kind string) chat.Record {
	rec := chat.Record{"type": kind, "message": c.text}
	if len(c.images) > 0 {
		rec["images"] = c.images
	}
	return rec
}

// Send 发消息：登记运行 → 发 prompt；被拒则回 error 帧。
func (c *Chat) Send(sessionFile, message string, attachments []domain.ChatAttachmentVO) (domain.ChatSendRESP, error) {
	if strings.TrimSpace(message) == "" && len(attachments) == 0 {
		return domain.ChatSendRESP{}, pkg.Err(pkg.CodeBadRequest, "消息不能为空")
	}
	client, err := c.EnsureRpc()
	if err != nil {
		return domain.ChatSendRESP{}, err
	}
	if sessionFile != "" && sessionFile != c.currentFile() {
		if _, err := client.Request(chat.Record{"type": "switch_session", "sessionPath": sessionFile}, 0); err != nil {
			return domain.ChatSendRESP{}, err
		}
		c.mu.Lock()
		c.file = sessionFile
		c.mu.Unlock()
	}
	runId := c.Stream.Begin(message)
	if _, err := client.Request(compose(message, attachments).wire("prompt"), 0); err != nil {
		c.Stream.Idle()
		message := pkg.As(err).Message
		c.Hub.Broadcast("chat:error", map[string]any{"run_id": runId, "message": message, "engine_kind": "pi"})
		return domain.ChatSendRESP{}, err
	}
	c.rememberCurrent()
	return domain.ChatSendRESP{RunId: runId}, nil
}

// Stop 中止当前运行。
func (c *Chat) Stop() error {
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	_, err = client.Request(chat.Record{"type": "abort"}, 0)
	return err
}

// Steer 插话：当前轮的工具调用做完后、下一次模型调用之前投递。
func (c *Chat) Steer(message string, attachments []domain.ChatAttachmentVO) error {
	if strings.TrimSpace(message) == "" {
		return pkg.Err(pkg.CodeBadRequest, "插话内容不能为空")
	}
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	_, err = client.Request(compose(message, attachments).wire("steer"), 0)
	return err
}

// FollowUp 追加：等引擎停下来再投递。
func (c *Chat) FollowUp(message string, attachments []domain.ChatAttachmentVO) error {
	if strings.TrimSpace(message) == "" {
		return pkg.Err(pkg.CodeBadRequest, "追加内容不能为空")
	}
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	_, err = client.Request(compose(message, attachments).wire("follow_up"), 0)
	return err
}

// ClearQueue 清空两条队，返回被清掉的话术（界面据此把它们还原到输入框）。
func (c *Chat) ClearQueue() (domain.QueuedMessagesVO, error) {
	client, err := c.EnsureRpc()
	if err != nil {
		return domain.QueuedMessagesVO{}, err
	}
	data, err := client.Request(chat.Record{"type": "clear_queue"}, 0)
	if err != nil {
		return domain.QueuedMessagesVO{}, err
	}
	fields, _ := data.(map[string]any)
	out := domain.QueuedMessagesVO{
		Steering: asStringSlice(fields["steering"]),
		FollowUp: asStringSlice(fields["followUp"]),
	}
	c.mu.Lock()
	c.queued = out
	c.mu.Unlock()
	return out, nil
}

// RemoveQueued 单条移除：引擎只支持整队清空，能力位已关闭这个入口。
func (c *Chat) RemoveQueued() error {
	return pkg.Err(pkg.CodeEngineBusy, "引擎不支持单条移除，请整队清空")
}

// Compact 手动压缩上下文。
func (c *Chat) Compact(instructions string) error {
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	wire := chat.Record{"type": "compact"}
	if text := strings.TrimSpace(instructions); text != "" {
		wire["customInstructions"] = text
	}
	_, err = client.Request(wire, 0)
	return err
}

// NewSession 新建会话。
func (c *Chat) NewSession() (string, error) {
	client, err := c.EnsureRpc()
	if err != nil {
		return "", err
	}
	if _, err := client.Request(chat.Record{"type": "new_session"}, 0); err != nil {
		return "", err
	}
	file, err := c.syncSessionFile(client)
	if err != nil {
		return "", err
	}
	c.Hub.Broadcast("session:info", map[string]any{"engine_kind": "pi"})
	return file, nil
}

// CloneSession 复制当前会话开一条新分支。
func (c *Chat) CloneSession() (string, error) {
	client, err := c.EnsureRpc()
	if err != nil {
		return "", err
	}
	if _, err := client.Request(chat.Record{"type": "clone"}, 0); err != nil {
		return "", err
	}
	return c.syncSessionFile(client)
}

// syncSessionFile 重新读一次会话文件并记住它。
func (c *Chat) syncSessionFile(client *chat.PiRpc) (string, error) {
	state, err := client.Request(chat.Record{"type": "get_state"}, 0)
	if err != nil {
		return "", err
	}
	c.applyState(state)
	file := c.currentFile()
	c.Settings.RememberSession(file)
	return file, nil
}

// SetModel 切模型。
func (c *Chat) SetModel(providerName, modelId string) error {
	if providerName == "" || modelId == "" {
		return pkg.Err(pkg.CodeBadRequest, "服务商与模型都不能为空")
	}
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	_, err = client.Request(chat.Record{"type": "set_model", "provider": providerName, "modelId": modelId}, 0)
	return err
}

// SetThinking 切思考档。
func (c *Chat) SetThinking(level string) error {
	if level == "" {
		return pkg.Err(pkg.CodeBadRequest, "思考档不能为空")
	}
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	if _, err := client.Request(chat.Record{"type": "set_thinking_level", "level": level}, 0); err != nil {
		return err
	}
	c.mu.Lock()
	c.level = level
	c.mu.Unlock()
	return nil
}

// SetAutoCompaction 自动压缩开关。
func (c *Chat) SetAutoCompaction(enabled bool) error {
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	if _, err := client.Request(chat.Record{"type": "set_auto_compaction", "enabled": enabled}, 0); err != nil {
		return err
	}
	c.mu.Lock()
	c.autocmp = enabled
	c.mu.Unlock()
	return nil
}

// Answer 回答引擎的提问卡。
func (c *Chat) Answer(req domain.ChatAnswerREQ) error {
	if req.AskId == "" {
		return pkg.Err(pkg.CodeBadRequest, "提问 ID 不能为空")
	}
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	wire := chat.Record{"type": "extension_ui_response", "id": req.AskId}
	switch {
	case len(req.Answers) > 0:
		first := req.Answers[0]
		value := strings.Join(first.SelectedOptions, ", ")
		if value == "" {
			value = first.CustomInput
		}
		if value != "" {
			wire["value"] = value
		} else {
			wire["cancelled"] = true
		}
	case req.Value != "":
		wire["value"] = req.Value
	case req.Confirmed != nil:
		wire["confirmed"] = *req.Confirmed
	default:
		wire["cancelled"] = true
	}
	return client.Send(wire)
}

// Export 导出当前会话为 HTML。
func (c *Chat) Export(sessionFile string) (domain.ChatExportRESP, error) {
	file := sessionFile
	if file == "" {
		file = c.currentFile()
	}
	if file == "" {
		return domain.ChatExportRESP{}, pkg.Err(pkg.CodeNotFound, "当前还没有会话可导出")
	}
	return c.Sessions.Export(file)
}
