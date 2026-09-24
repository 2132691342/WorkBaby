package service

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// DefaultSessionName 新建会话的默认标题（首条消息后由自动命名覆盖）。
const DefaultSessionName = "新会话"

// 会话检索范围（SessionSearchREQ.Scope）。
const (
	SearchScopeAll     = "all"
	SearchScopeTitle   = "title"
	SearchScopeContent = "content"
)

// sessionMetaKeyCompactInstructions 保留指示的元数据键（/compact 落点）。
const sessionMetaKeyCompactInstructions = "compact_instructions"

// sessionMetaKeyArchiveSummary 归档摘要的元数据键（/compact 落点，buildSystem 注入）。
const sessionMetaKeyArchiveSummary = "compact_archive_summary"

// sessionMetaKeyCompressBoundary 压缩边界（SummaryBoundary）的元数据键：
// 记录最近一次压缩覆盖到哪一轮、涉及哪些工具调用锚点，供对账与问题定位。
const sessionMetaKeyCompressBoundary = "compress_boundary"

// sessionMetaKeyAgentName 会话级 Agent 覆盖：非空时 SendStream 优先使用，
// 由 /agent <name> 命令写入；空回退 harness 内置 defaultAgentName。
const sessionMetaKeyAgentName = "agent_name"

// SearchSessions 跨会话检索（/resume 快速检索）。标题命中优先于正文命中（都命中只出现一次）；
// 正文只扫 user/assistant（tool 结果与 system 提示词噪声太大）；结果按最大 seq 倒序。
func (s *ChatService) SearchSessions(ctx context.Context, req domain.SessionSearchREQ) (*domain.SessionListRESP, error) {
	q := strings.TrimSpace(req.Query)
	if q == "" {
		return &domain.SessionListRESP{Items: []domain.ChatSessionRESP{}, Total: 0}, nil
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	scope := req.Scope
	if scope == "" {
		scope = SearchScopeAll
	}
	lower := strings.ToLower(q)

	all, err := s.sessions.ListByUser(ctx, domain.LocalUserID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.ChatSessionDO, len(all))
	for i := range all {
		byID[all[i].ID] = all[i]
	}

	items := make([]domain.ChatSessionRESP, 0, limit)
	seen := make(map[string]bool, limit)

	if scope != SearchScopeContent {
		for i := range all {
			if len(items) >= limit {
				break
			}
			if strings.Contains(strings.ToLower(all[i].Name), lower) {
				items = append(items, toSessionRESP(&all[i]))
				seen[all[i].ID] = true
			}
		}
	}
	if scope != SearchScopeTitle && len(items) < limit {
		hits, err := s.messages.SearchSessions(ctx, q, limit)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			if len(items) >= limit {
				break
			}
			if seen[h.SessionID] {
				continue
			}
			row, ok := byID[h.SessionID]
			if !ok {
				continue // 会话已软删
			}
			items = append(items, toSessionRESP(&row))
			seen[h.SessionID] = true
		}
	}
	return &domain.SessionListRESP{Items: items, Total: len(items)}, nil
}

// autoTitleSession 首条用户消息后自动命名（/resume 的「会话可辨认」前提）。只在会话仍是默认
// 标题时改写（用户手动重命名过的不会被覆盖）；命名失败保持原名，不阻断发送。
func (s *ChatService) autoTitleSession(ctx context.Context, ses *domain.ChatSessionDO, content string) {
	if ses.Name != "" && ses.Name != DefaultSessionName {
		return
	}
	title := SessionTitleFrom(content)
	if title == "" {
		return
	}
	ses.Name = title
	if err := s.sessions.Update(ctx, ses); err != nil {
		pkg.L.Warn("auto rename session failed", "sessionID", ses.ID, "err", err.Error())
	}
}

// SessionTitleFrom 从首条用户消息推导会话标题（确定性，不调 LLM）：取首个非空行 → 剥掉
// Markdown 装饰 → 折叠空白 → 按 rune 截断到 40。斜杠命令保留原样（正是用户当轮要做的事）。
func SessionTitleFrom(content string) string {
	var first string
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) != "" {
			first = line
			break
		}
	}
	if first == "" {
		return ""
	}
	s := strings.TrimSpace(first)
	// 循环剥前缀：标题符、列表符、数字有序列表（"1. " / "2) "）、代码块围栏
	for {
		trimmed := strings.TrimLeft(s, "#>*-+\t ")
		if trimmed == "```" {
			trimmed = ""
		}
		// 数字有序列表："1. " / "1) "
		trimmed = trimNumberedPrefix(trimmed)
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == s {
			break
		}
		s = trimmed
	}
	if s == "" {
		return ""
	}
	return pkg.TruncateRunes(collapseSpaces(s), 40)
}

// trimNumberedPrefix 剥掉 "1. " / "2) " 这类数字有序列表前缀；没匹配则原样返回。
func trimNumberedPrefix(s string) string {
	for i, r := range s {
		if r >= '0' && r <= '9' {
			continue
		}
		if i > 0 && (r == '.' || r == ')') {
			return strings.TrimLeft(s[i+1:], " \t")
		}
		break
	}
	return s
}

// collapseSpaces 把连续空白折叠为单空格（含全角空格）。
func collapseSpaces(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r)
	}), " ")
}

// readSessionMeta 解析会话元数据；解析失败返回空 map（元数据是旁路信息，不阻断主流程）。
func readSessionMeta(ses *domain.ChatSessionDO) map[string]any {
	m := map[string]any{}
	if ses == nil || ses.MetadataJSON == "" {
		return m
	}
	_ = json.Unmarshal([]byte(ses.MetadataJSON), &m)
	return m
}

// writeSessionMeta 写回会话元数据（整键覆盖，未知键保持不变）。
func (s *ChatService) writeSessionMeta(ctx context.Context, ses *domain.ChatSessionDO, m map[string]any) error {
	bs, err := json.Marshal(m)
	if err != nil {
		return pkg.Wrap(2041, "encode session metadata failed", err)
	}
	ses.MetadataJSON = string(bs)
	return s.sessions.Update(ctx, ses)
}

// persistCompressBoundary 记录最近一次压缩的边界（SummaryBoundary）：覆盖了哪些消息、
// 涉及哪些工具调用锚点、何时发生。压缩是上下文被改写的少数时刻，不留证据事后无法解释
// 「模型为什么忘了刚才的事」，也无法验证是否把没送进模型的内容误标成已摘要。
func (s *ChatService) persistCompressBoundary(ctx context.Context, ses *domain.ChatSessionDO, b core.CompressedPayload) {
	if ses == nil {
		return
	}
	bs, err := json.Marshal(compressBoundaryMeta{
		RemovedMsgs: b.Removed,
		CutoffAt:    b.CutoffAt,
		Summary:     b.Summary,
		FilterKey:   compressFilterMicro,
	})
	if err != nil {
		return
	}
	meta := readSessionMeta(ses)
	meta[sessionMetaKeyCompressBoundary] = string(bs)
	if err := s.writeSessionMeta(ctx, ses, meta); err != nil {
		pkg.L.Warn("write compress boundary failed", "sessionID", ses.ID, "err", err.Error())
	}
}

// compressFilterMicro 压缩器标识：内核只保留确定性折叠一种压缩器。
const compressFilterMicro = "micro"

// compressBoundaryMeta 落进会话元数据的压缩边界（前端「查看折叠纪要」的数据源）。
type compressBoundaryMeta struct {
	RemovedMsgs int    `json:"removed_msgs"`
	CutoffAt    int64  `json:"cutoff_at"`
	Summary     string `json:"summary,omitempty"`
	FilterKey   string `json:"filter_key"`
}

// compactInstructions 读会话上的保留指示；未设置返回空串。
func compactInstructions(ses *domain.ChatSessionDO) string {
	v, _ := readSessionMeta(ses)[sessionMetaKeyCompactInstructions].(string)
	return strings.TrimSpace(v)
}

// archiveSummary 读会话上的归档摘要；未设置返回空串。
func archiveSummary(ses *domain.ChatSessionDO) string {
	v, _ := readSessionMeta(ses)[sessionMetaKeyArchiveSummary].(string)
	return strings.TrimSpace(v)
}
