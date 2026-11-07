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

// alignSession 把引擎的当前会话对齐到调用方指定的文件；空串或已一致则不动。
// 会话级动作（发送、压缩、切模型）都要先走这里：界面可能在引擎还绑着
// 上一条会话时就对当前看到的会话发起操作，不对齐就会写错文件。
func (c *Chat) alignSession(client *chat.PiRpc, sessionFile string) error {
	if sessionFile == "" || sessionFile == c.currentFile() {
		return nil
	}
	guarded, err := c.guardSessionPath(sessionFile)
	if err != nil {
		return err
	}
	if _, err := client.Request(chat.Record{"type": "switch_session", "sessionPath": guarded}, 0); err != nil {
		return err
	}
	c.mu.Lock()
	c.file = guarded
	c.mu.Unlock()
	return nil
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
	if err := c.alignSession(client, sessionFile); err != nil {
		return domain.ChatSendRESP{}, err
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

// RemoveQueued 单条移除排队消息。引擎只有整队清空一个动作，这里用
// 「取出全队 → 摘掉一条 → 其余按原序重新入队」实现；插队重投失败时
// 降级为追加投递，宁可晚说也不能把用户的话弄丢。
func (c *Chat) RemoveQueued(kind string, index int) (domain.QueuedMessagesVO, error) {
	client, err := c.EnsureRpc()
	if err != nil {
		return domain.QueuedMessagesVO{}, err
	}
	data, err := client.Request(chat.Record{"type": "clear_queue"}, 0)
	if err != nil {
		return domain.QueuedMessagesVO{}, err
	}
	fields, _ := data.(map[string]any)
	steering := asStringSlice(fields["steering"])
	followUp := asStringSlice(fields["followUp"])
	target := &steering
	switch kind {
	case "steering":
	case "followup":
		target = &followUp
	default:
		return domain.QueuedMessagesVO{}, pkg.Err(pkg.CodeBadRequest, "队列类型只能是 steering 或 followup")
	}
	if index < 0 || index >= len(*target) {
		return domain.QueuedMessagesVO{}, pkg.Err(pkg.CodeBadRequest, "要移除的位置不在队列里")
	}
	*target = append((*target)[:index], (*target)[index+1:]...)

	// requeue 投一条回引擎；插队投不进去就降级为追加。
	// 每一层都要显式返回 error：闭包沿用外层 err 会把它覆盖成 nil，
	// 调用方就再也看不出重投失败——用户以为删掉了一条，实际整队都没投回去。
	requeue := func(text, wire string) error {
		_, err := client.Request(compose(text, nil).wire(wire), 0)
		if err == nil {
			return nil
		}
		if wire != "steer" {
			return err
		}
		if _, fallbackErr := client.Request(compose(text, nil).wire("follow_up"), 0); fallbackErr != nil {
			return fallbackErr
		}
		return nil
	}
	for _, text := range steering {
		if err := requeue(text, "steer"); err != nil {
			return domain.QueuedMessagesVO{}, pkg.Err(pkg.CodeRpcCommand, "排队消息重投失败", text)
		}
	}
	for _, text := range followUp {
		if err := requeue(text, "follow_up"); err != nil {
			return domain.QueuedMessagesVO{}, pkg.Err(pkg.CodeRpcCommand, "排队消息重投失败", text)
		}
	}
	out := domain.QueuedMessagesVO{Steering: steering, FollowUp: followUp}
	c.mu.Lock()
	c.queued = out
	c.mu.Unlock()
	return out, nil
}

// Compact 手动压缩上下文。
func (c *Chat) Compact(sessionFile, instructions string) error {
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	if err := c.alignSession(client, sessionFile); err != nil {
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
func (c *Chat) SetModel(sessionFile, providerName, modelId string) error {
	if providerName == "" || modelId == "" {
		return pkg.Err(pkg.CodeBadRequest, "服务商与模型都不能为空")
	}
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	if err := c.alignSession(client, sessionFile); err != nil {
		return err
	}
	_, err = client.Request(chat.Record{"type": "set_model", "provider": providerName, "modelId": modelId}, 0)
	return err
}

// SetThinking 切思考档。
func (c *Chat) SetThinking(sessionFile, level string) error {
	if level == "" {
		return pkg.Err(pkg.CodeBadRequest, "思考档不能为空")
	}
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	if err := c.alignSession(client, sessionFile); err != nil {
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
func (c *Chat) SetAutoCompaction(sessionFile string, enabled bool) error {
	client, err := c.EnsureRpc()
	if err != nil {
		return err
	}
	if err := c.alignSession(client, sessionFile); err != nil {
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
	case req.Cancelled:
		wire["cancelled"] = true
	default:
		// 什么都没带：视为用户放弃这次提问。
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
