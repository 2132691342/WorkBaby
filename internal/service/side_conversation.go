package service

import (
	"context"
	"regexp"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// 本文件：辅助对话（side 会话）——功能完整的独立会话，但不进侧栏（kind='side' 过滤）、
// 一个主会话至多一个（Ensure 幂等）、每轮把主会话历史按 rune 预算拼在自己历史之前，
// 且不继承主会话的目标模式 / 待发送队列 / 后台任务。

// sideParentHistoryRunes 主会话历史拼接进辅助会话上下文的 rune 预算。
// 辅助会话是短平快的小对话，历史只承担「看得见主任务在干什么」，不承担完整重放。
const sideParentHistoryRunes = 12000

// EnsureSideConversation 返回主会话的辅助会话（不存在则创建；模型/工作区/权限模式随主会话）。
func (s *ChatService) EnsureSideConversation(ctx context.Context, mainSessionID string) (*domain.ChatSessionRESP, error) {
	main, err := s.sessions.GetByID(ctx, mainSessionID)
	if err != nil {
		return nil, err
	}
	if main.Kind == domain.SessionKindSide {
		// 对辅助会话本身调用 ensure → 语义修正为返回它自己（幂等，不嵌套）
		out := toSessionRESP(main)
		return &out, nil
	}
	existing, err := s.sessions.FindSideByParent(ctx, main.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		out := toSessionRESP(existing)
		return &out, nil
	}
	row := &domain.ChatSessionDO{
		ID:             pkg.NewID(domain.IDSession),
		Name:           "辅 · " + main.Name,
		UserID:         main.UserID,
		ProviderID:     main.ProviderID,
		Model:          main.Model,
		WorkspaceID:    main.WorkspaceID,
		WorkspacePath:  main.WorkspacePath,
		Status:         domain.SessionStatusActive,
		Kind:           domain.SessionKindSide,
		PermissionMode: main.PermissionMode,
		ParentID:       main.ID,
	}
	if err := s.sessions.Create(ctx, row); err != nil {
		return nil, err
	}
	out := toSessionRESP(row)
	return &out, nil
}

// GetSideConversation 返回主会话已有的辅助会话；没有返回 nil（面板显示空态）。
func (s *ChatService) GetSideConversation(ctx context.Context, mainSessionID string) (*domain.ChatSessionRESP, error) {
	existing, err := s.sessions.FindSideByParent(ctx, mainSessionID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	out := toSessionRESP(existing)
	return &out, nil
}

// sideParentMessages 辅助会话 run 的主会话历史前缀：直接从消息行提取 user/assistant
// 文本叙事（含已归档轮次——重度压缩后它们是结论的唯一完整来源；工具过程不进辅助上下文），
// 按 rune 预算从尾部截取，起点回溯对齐到 user 消息。
func (s *ChatService) sideParentMessages(ctx context.Context, ses *domain.ChatSessionDO, vision bool) ([]*llm.Message, error) {
	if ses.Kind != domain.SessionKindSide || ses.ParentID == "" {
		return nil, nil
	}
	main, err := s.sessions.GetByID(ctx, ses.ParentID)
	if err != nil {
		// 主会话已删除：辅助会话降级为普通独立会话，不阻断 run
		return nil, nil
	}
	hists, err := s.messages.ListBySession(ctx, main.ID, 0, 0)
	if err != nil {
		return nil, err
	}
	narrative := make([]*llm.Message, 0, len(hists))
	for i := range hists {
		m := &hists[i]
		// 流式占位与界面消息不进；archived（压缩归档轮次）照收——它们正是叙事主体
		if m.Status == domain.MessageStatusStreaming || m.ContextScope == domain.MessageScopeUI {
			continue
		}
		switch m.Role {
		case domain.MessageRoleUser:
			if strings.TrimSpace(m.Content) != "" {
				narrative = append(narrative, &llm.Message{Role: llm.RoleUser, Content: m.Content})
			}
		case domain.MessageRoleAssistant:
			// 纯文本结论即入叙事（带整轮 tool_calls 的收尾消息正文同样要——
			// run 的结论写在它身上，工具过程本身已被排除）；<think> 残段剥离
			if c := stripThinkTag(m.Content); strings.TrimSpace(c) != "" {
				narrative = append(narrative, &llm.Message{Role: llm.RoleAssistant, Content: c})
			}
		}
	}
	// 压缩归档摘要兜底（/compact 落点）：叙事被裁空的极端情况下仍有「做过什么」可依
	if len(narrative) == 0 {
		meta := readSessionMeta(main)
		if summary, ok := meta[sessionMetaKeyArchiveSummary].(string); ok && strings.TrimSpace(summary) != "" {
			narrative = append(narrative, &llm.Message{
				Role:    llm.RoleUser,
				Content: "主任务的压缩归档摘要（更早轮次已折叠为以下纪要）：\n" + summary,
			})
		}
	}
	if len(narrative) == 0 {
		return nil, nil
	}
	// 从尾部按预算累积；统一按内容 rune 计
	start := 0
	used := 0
	for i := len(narrative) - 1; i >= 0; i-- {
		used += len([]rune(narrative[i].Content))
		if used > sideParentHistoryRunes {
			break
		}
		start = i
	}
	// 起点回溯到最近的 user 消息：截断可能落在 assistant 回答中间，
	// 轮次以 user 提问开头才完整（回溯只多不少，找不到就保留整段）
	for i := start; i >= 0; i-- {
		if narrative[i].Role == llm.RoleUser {
			start = i
			break
		}
	}
	narrative = narrative[start:]
	if len(narrative) == 0 {
		return nil, nil
	}
	// 强约束标注：模型不得把并行背景当成「没有历史」
	header := llm.SystemMessage(
		"背景注入：你在辅助对话中，与一个并行推进的主任务共享背景。下面是该主任务的近期纪要" +
			"（用户提问与已得出的结论）。回答当前问题时必须结合这份背景；" +
			"不要声称「这是第一条消息」「没有执行过任何操作」，也不要要求用户重复交代背景。")
	return append([]*llm.Message{header}, narrative...), nil
}

// stripThinkTag 剥离正文中的 <think>…</think> 段（含未闭合尾部）。
// 思考走独立 thinking 通道，继承叙事不需要它。
func stripThinkTag(content string) string {
	if !strings.Contains(content, "<think>") {
		return content
	}
	out := regexpThinkBlock.ReplaceAllString(content, "")
	out = regexpThinkTail.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

var (
	regexpThinkBlock = regexp.MustCompile(`(?s)<think>.*?</think>`)
	regexpThinkTail  = regexp.MustCompile(`(?s)<think>.*$`)
)
