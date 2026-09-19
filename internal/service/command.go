package service

import (
	"context"
	"strings"
	"time"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// BuiltinCommands 内置斜杠命令表（命令面板）。
//
// 单一真相源：前端启动拉一次 GET /api/v1/chat/commands 渲染面板。
// ClientOnly 命令的执行逻辑在前端（切模型/开面板/导出），后端不重复实现；
// 非 ClientOnly 命令的面板项直接跳到现有端点（/chat/sessions/:id/compact 等）。
func BuiltinCommands() []domain.SlashCommand {
	cmds := []domain.SlashCommand{
		// 会话闭环：新建 / 重命名 / 清空 / 分叉 / 截断
		{Name: "new", Desc: "新建会话", Group: "session", ClientOnly: true},
		{Name: "rename", Args: "<新标题>", Desc: "重命名当前会话", Group: "session", ClientOnly: true},
		{Name: "clear", Desc: "清空当前会话消息", Group: "session", ClientOnly: true},
		{Name: "fork", Args: "[消息ID]", Desc: "从指定消息分叉新会话（不传则从末尾分叉）", Group: "session"},
		{Name: "truncate", Args: "<消息ID>", Desc: "截断会话到指定消息", Group: "session"},
		// 上下文闭环（空上下文管理）
		{Name: "compact", Args: "[保留指示]", Desc: "压缩历史上下文，可附带压缩后必须保留的要点", Group: "session"},
		{Name: "context", Desc: "查看当前上下文占用分段（system / 历史 / 工具）", Group: "session"},
		// 跨会话恢复
		{Name: "resume", Args: "<会话ID|关键词>", Desc: "跨会话快速恢复（关键词搜 title / content / all）", Group: "session"},
		// 目标模式：设定后每轮自动校验、未达标自动续跑（详见 goal.go）
		{Name: "goal", Args: "[<目标描述>|pause|resume|clear]", Desc: "查看/设定/暂停/恢复/清除当前会话目标；设定后每轮自动校验是否达成并续跑", Group: "session"},
		// 模型与 Agent
		{Name: "model", Args: "<模型名>", Desc: "切换当前会话模型", Group: "model", ClientOnly: true},
		{Name: "agent", Args: "<default|coding|research|writer>", Desc: "切换当前 Agent（工具集与记忆策略随之切换）", Group: "agent", ClientOnly: true},
		{Name: "trust", Args: "[plan|restricted|confirm|default|auto|auto-edit|full|yolo]", Desc: "循环或直接切换执行模式（计划 / 需确认 / 自动 / 完全访问）", Group: "agent", ClientOnly: true},
		// 工具开关
		{Name: "tasks", Desc: "打开后台任务中心", Group: "system", ClientOnly: true},
		{Name: "export", Desc: "导出当前会话为 Markdown", Group: "session", ClientOnly: true},
		{Name: "help", Desc: "查看全部命令说明", Group: "system", ClientOnly: true},
	}
	// 来源标记集中在此处理：逐条写字面量会让「加一条命令」变成两处修改。
	for i := range cmds {
		cmds[i].Source = domain.CommandSourceBuiltin
	}
	return cmds
}

// CompactSession 确定性压缩历史（/compact 的后端实现）：早期轮次标 archived 剔出
// LLM 上下文并生成逐轮摘要进会话元数据（buildSystem 常驻注入），最近 keep 条保留。
// 幂等可反复执行；归档边界不落在 assistant(tool_calls) 与其 tool 结果之间（防孤儿 tool）。
func (s *ChatService) CompactSession(ctx context.Context, sessionID string, req domain.CompactREQ) (domain.CompactResultRESP, error) {
	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return domain.CompactResultRESP{}, err
	}
	rows, err := s.messages.ListBySession(ctx, sessionID, 0, compactScanLimit)
	if err != nil {
		return domain.CompactResultRESP{}, err
	}
	keep := req.KeepRecent
	if keep <= 0 {
		keep = compactKeepRecent
	}
	out := domain.CompactResultRESP{SessionID: sessionID, KeptRecent: keep, TotalBefore: len(rows)}

	// 保留指示先钉住：即使无需压缩也要生效（用户可能只想钉一条要点）
	if ins := strings.TrimSpace(req.Instructions); ins != "" {
		out.Pinned = s.pinCompactInstructions(ctx, ses, ins)
	}
	if len(rows) <= keep {
		return out, nil
	}

	// 归档边界：rows[:cut] 归档，rows[cut:] 保留。边界前移到最近的非 tool 行，
	// 保证「assistant + 其连续 tool 结果」整体同侧（协议铁律）。
	cut := len(rows) - keep
	for cut > 0 && cut < len(rows) && rows[cut].Role == domain.MessageRoleTool {
		cut--
	}
	if cut <= 0 {
		return out, nil
	}

	// 逐行标 archived（幂等：已归档行跳过），同时统计释放量与生成归档摘要
	now := time.Now().UnixMilli()
	var digest []string
	freedChars := 0
	for i := 0; i < cut; i++ {
		row := rows[i]
		if row.Status == domain.MessageStatusArchived {
			continue
		}
		if err := s.messages.UpdateStatus(ctx, row.ID, map[string]any{
			"status":     domain.MessageStatusArchived,
			"updated_at": now,
		}); err != nil {
			out.Failed++
			pkg.L.Warn("archive message failed", "sessionID", sessionID, "messageID", row.ID, "err", err.Error())
			continue
		}
		out.Compacted++
		freedChars += len(row.Content) + len(row.Thinking)
		if line := archiveDigestLine(row); line != "" && len(digest) < compactDigestLines {
			digest = append(digest, line)
		}
	}
	out.FreedChars = freedChars
	out.FreedTokens = estimateTokensFromChars(freedChars)

	// 归档摘要写会话元数据（buildSystem 每轮注入）；本轮没有新归档行则不覆盖旧摘要
	if len(digest) > 0 {
		meta := readSessionMeta(ses)
		meta[sessionMetaKeyArchiveSummary] = strings.Join(digest, "\n")
		if err := s.writeSessionMeta(ctx, ses, meta); err != nil {
			pkg.L.Warn("write archive summary failed", "sessionID", sessionID, "err", err.Error())
		}
	}
	return out, nil
}

// SetSessionAgent 切换会话 Agent（仅写元数据，不影响进行中的 run）。
func (s *ChatService) SetSessionAgent(ctx context.Context, sessionID, agentName string) error {
	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	meta := readSessionMeta(ses)
	if agentName == "" {
		delete(meta, sessionMetaKeyAgentName)
	} else {
		// 内置 + 自定义子智能体均可切换（core.Agent 会回退 default，但写库前先校验避免歧义）
		known := false
		for _, d := range core.AllAgents() {
			if d.Name == agentName {
				known = true
				break
			}
		}
		if !known {
			return pkg.New(5008, "未知 Agent", agentName)
		}
		meta[sessionMetaKeyAgentName] = agentName
	}
	return s.writeSessionMeta(ctx, ses, meta)
}

// SessionAgent 返回会话级 Agent 名称（空表示回退 default）。
func SessionAgent(ses *domain.ChatSessionDO) string {
	if ses == nil {
		return ""
	}
	meta := readSessionMeta(ses)
	if v, ok := meta[sessionMetaKeyAgentName].(string); ok {
		return v
	}
	return ""
}

// pinCompactInstructions 把保留指示写进会话元数据（后续 run 装配期注入 system 段）。
func (s *ChatService) pinCompactInstructions(ctx context.Context, ses *domain.ChatSessionDO, ins string) bool {
	meta := readSessionMeta(ses)
	meta[sessionMetaKeyCompactInstructions] = ins
	if err := s.writeSessionMeta(ctx, ses, meta); err != nil {
		pkg.L.Warn("pin compact instructions failed", "sessionID", ses.ID, "err", err.Error())
		return false
	}
	return true
}

// estimateTokensFromChars 字符数 → 估算 token（rune/4 近似，与 core.EstimateTokens 同口径）。
func estimateTokensFromChars(chars int) int { return chars / 4 }

// 压缩参数：扫描上限、保留窗口与摘要规模。
const (
	compactScanLimit   = 1000
	compactKeepRecent  = 20
	compactDigestLines = 40 // 归档摘要最多行数（超出部分以「…」收尾）
	compactDigestWidth = 60 // 每行摘要取内容前缀的 rune 数
)

// archiveDigestLine 归档摘要的一行：role + 内容前缀（确定性、无 LLM）。
// tool 消息不出现在摘要里（其结论已由 assistant 行代表）；空行跳过。
func archiveDigestLine(row domain.MessageDO) string {
	var role string
	switch row.Role {
	case domain.MessageRoleUser:
		role = "用户"
	case domain.MessageRoleAssistant:
		role = "助手"
	default:
		return ""
	}
	text := strings.TrimSpace(row.Content)
	if text == "" {
		return ""
	}
	r := []rune(text)
	if len(r) > compactDigestWidth {
		r = append(r[:compactDigestWidth], []rune("…")...)
	}
	return role + "：" + string(r)
}
