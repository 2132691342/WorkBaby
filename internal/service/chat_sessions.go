package service

import (
	"context"
	"strings"
	"time"

	"os"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/tool"
)

// 本文件：会话与消息 CRUD（列表 / 创建 / 重命名 / 工作区绑定 / 删除 / 截断 / 分叉 / 分页）。

func (s *ChatService) ListSessions(ctx context.Context, page, pageSize int) (*domain.SessionListRESP, error) {
	total, err := s.sessions.CountByUser(ctx, domain.LocalUserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.sessions.ListByUserPage(ctx, domain.LocalUserID, page, pageSize)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ChatSessionRESP, 0, len(rows))
	for i := range rows {
		out = append(out, toSessionRESP(&rows[i]))
	}
	return &domain.SessionListRESP{Items: out, Total: total}, nil
}

func toSessionRESP(s *domain.ChatSessionDO) domain.ChatSessionRESP {
	return domain.ChatSessionRESP{
		ID:             s.ID,
		Name:           s.Name,
		UserID:         s.UserID,
		ProviderID:     s.ProviderID,
		Model:          s.Model,
		WorkspaceID:    s.WorkspaceID,
		WorkspacePath:  s.WorkspacePath,
		Active:         s.Status == domain.SessionStatusActive,
		MessageCount:   s.MessageCount,
		LastMessageAt:  s.LastMessageAt,
		MetadataJSON:   s.MetadataJSON,
		Status:         s.Status,
		Pinned:         s.Pinned,
		Kind:           s.Kind,
		PermissionMode: s.PermissionMode,
		ParentID:       s.ParentID,
		BranchPoint:    s.BranchPoint,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

func toMessageRESP(m *domain.MessageDO) domain.MessageRESP {
	return domain.MessageRESP{
		ID:           m.ID,
		SessionID:    m.SessionID,
		RunID:        m.RunID,
		Role:         m.Role,
		Content:      m.Content,
		Thinking:     m.Thinking,
		ToolCallID:   m.ToolCallID,
		ToolCalls:    m.ToolCalls,
		Status:       m.Status,
		ContextScope: m.ContextScope,
		StopReason:   m.StopReason,
		Model:        m.Model,
		InputTokens:  m.InputTokens,
		OutputTokens: m.OutputTokens,
		CacheRead:    m.CacheRead,
		TotalTokens:  m.TotalTokens,
		LatencyMs:    m.LatencyMs,
		Attachments:  m.Attachments(),
		Cost:         m.Cost,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// CreateSession 新建会话：provider/model 缺省时按 model 精确匹配回查 provider，
// 都不中则取第一个 enabled；workspace_path 非空时创建即完成绑定与信任登记。
func (s *ChatService) CreateSession(ctx context.Context, req *domain.ChatSessionREQ) (*domain.ChatSessionRESP, error) {
	wp, err := s.validateWorkspace(ctx, req.WorkspacePath)
	if err != nil {
		return nil, err
	}
	row := &domain.ChatSessionDO{
		ID:            pkg.NewID(domain.IDSession),
		Name:          req.Name,
		UserID:        domain.LocalUserID,
		ProviderID:    req.ProviderID,
		Model:         req.Model,
		WorkspaceID:   req.WorkspaceID,
		WorkspacePath: wp,
		Status:        domain.SessionStatusActive,
	}
	if row.Name == "" {
		row.Name = DefaultSessionName
	}
	// 兜底：缺 provider_id / model 时自动选一个 enabled 的 Provider（聊天可用）
	if row.ProviderID == "" || row.Model == "" {
		if pid, model, ok := s.resolveDefaultProviderModel(ctx, row.Model); ok {
			row.ProviderID = pid
			row.Model = model
		}
	}
	if err := s.sessions.Create(ctx, row); err != nil {
		return nil, err
	}
	r := toSessionRESP(row)
	return &r, nil
}

// resolveDefaultProviderModel 选出一个 enabled Provider 与模型：按 model 精确匹配，
// 同名多命中取档位更高者，都不中则取第一个 enabled。只选唯一答案，不做重试与故障切换。
// found=false 时调用方应报「未配置 Provider」。
func (s *ChatService) resolveDefaultProviderModel(ctx context.Context, modelHint string) (string, string, bool) {
	if s.provRepo == nil {
		return "", "", false
	}
	all, err := s.provRepo.List(ctx)
	if err != nil || len(all) == 0 {
		return "", "", false
	}
	// 1) model 精确匹配（trim 后比较；同名模型取 primary 档）
	if modelHint != "" {
		hint := strings.TrimSpace(modelHint)
		var hit *domain.AiProviderDO
		for i := range all {
			p := all[i]
			if !p.Enabled {
				continue
			}
			if strings.TrimSpace(p.Model) == hint {
				if hit == nil || p.TierRank() < hit.TierRank() {
					hit = &p
				}
			}
		}
		if hit != nil {
			return hit.ID, hit.Model, true
		}
	}
	// 2) 退路：第一个 enabled（按 created_at ASC）
	for i := range all {
		if all[i].Enabled {
			return all[i].ID, all[i].Model, true
		}
	}
	return "", "", false
}

func (s *ChatService) GetSession(ctx context.Context, id string) (*domain.ChatSessionRESP, error) {
	row, err := s.sessions.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	r := toSessionRESP(row)
	return &r, nil
}

func (s *ChatService) RenameSession(ctx context.Context, id, name string) (*domain.ChatSessionRESP, error) {
	row, err := s.sessions.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	row.Name = name
	if err := s.sessions.Update(ctx, row); err != nil {
		return nil, err
	}
	r := toSessionRESP(row)
	return &r, nil
}

// SetSessionPermission 切换会话工具权限模式（restricted/default/auto_edit/yolo；空 = 跟随全局设置）。
// 归一化在 tool 层做，未知值回落 default，避免脏数据意外升权。
func (s *ChatService) SetSessionPermission(ctx context.Context, id, mode string) (*domain.ChatSessionRESP, error) {
	row, err := s.sessions.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(mode); v == "" {
		row.PermissionMode = ""
	} else {
		row.PermissionMode = string(tool.ParseSessionMode(v))
	}
	if err := s.sessions.Update(ctx, row); err != nil {
		return nil, err
	}
	r := toSessionRESP(row)
	return &r, nil
}

// SetSessionPinned 置顶/取消置顶（仅写库 + 排序，不影响进行中的 run）。
func (s *ChatService) SetSessionPinned(ctx context.Context, id string, pinned bool) (*domain.ChatSessionRESP, error) {
	row, err := s.sessions.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	row.Pinned = pinned
	if err := s.sessions.Update(ctx, row); err != nil {
		return nil, err
	}
	r := toSessionRESP(row)
	return &r, nil
}

// SetSessionArchived 归档/取消归档（status 在 active/archived 间切换；归档自动取消置顶）。
func (s *ChatService) SetSessionArchived(ctx context.Context, id string, archived bool) (*domain.ChatSessionRESP, error) {
	row, err := s.sessions.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if archived {
		row.Status = domain.SessionStatusArchived
		row.Pinned = false
	} else {
		row.Status = domain.SessionStatusActive
	}
	if err := s.sessions.Update(ctx, row); err != nil {
		return nil, err
	}
	r := toSessionRESP(row)
	return &r, nil
}

// UpdateWorkspace 绑定/解绑会话的外部工作目录。
//
// 绑定即授权：目录经对话框显式选择，登记为 allow 信任（后续工具不再为它弹审批）。
// 解绑（空路径）只清字段、不撤销已有信任登记——用户可能还要继续在原目录工作。
func (s *ChatService) UpdateWorkspace(ctx context.Context, id, workspacePath string) (*domain.ChatSessionRESP, error) {
	row, err := s.sessions.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	p, err := s.validateWorkspace(ctx, workspacePath)
	if err != nil {
		return nil, err
	}
	row.WorkspacePath = p
	if err := s.sessions.Update(ctx, row); err != nil {
		return nil, err
	}
	r := toSessionRESP(row)
	return &r, nil
}

// validateWorkspace 校验并规范化外部工作目录（空 = 未绑定，返回空串）。
// 非空时：NormalizeDir → 必须是真实存在的目录 → 登记 allow 信任（绑定即授权）。
// CreateSession 与 UpdateWorkspace 共用，保证两条绑定路径行为一致。
func (s *ChatService) validateWorkspace(ctx context.Context, workspacePath string) (string, error) {
	p := strings.TrimSpace(workspacePath)
	if p == "" {
		return "", nil
	}
	dir, derr := NormalizeDir(p)
	if derr != nil {
		return "", derr
	}
	info, serr := os.Stat(dir)
	if serr != nil || !info.IsDir() {
		return "", pkg.New(1021, "workspace directory does not exist", dir)
	}
	if s.trust != nil {
		if _, terr := s.trust.Decide(ctx, domain.WorkspaceTrustREQ{Path: dir, State: string(domain.TrustStateAllow)}); terr != nil {
			pkg.L.Warn("auto trust workspace failed", "path", dir, "err", terr.Error())
		}
	}
	// 绑定即落沙箱：.workbaby 及其子目录此刻必须存在，模型与工具才有明确的过程数据落点。
	// 晚一步建立就会出现「首次 file_write / exec 直接在用户项目根建 scripts、output」的污染。
	if err := runtime.SandboxOf(dir).Ensure(); err != nil {
		pkg.L.Warn("init workspace sandbox failed", "path", dir, "err", err.Error())
	}
	return dir, nil
}

// WorkspaceRoot 某会话工具链的工作区根：绑定了外部目录用外部目录，否则用默认根。
// handler 装配期把它包成 tool.RootResolver 注入文件类工具；默认根为空时返回空串
// （由 tool.ResolveRoot 的调用方兜底），避免把「未配置」误判成「当前目录」。
func (s *ChatService) WorkspaceRoot(ctx context.Context, sessionID, defRoot string) string {
	if sessionID == "" {
		return defRoot
	}
	row, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil || row == nil {
		if err != nil {
			pkg.L.Warn("workspace root resolve failed, fallback to default", "session", sessionID, "err", err.Error())
		}
		return defRoot
	}
	if p := strings.TrimSpace(row.WorkspacePath); p != "" {
		return p
	}
	return defRoot
}

// UpdateSessionModel 切换会话使用的 Provider/模型（切换后输入框的思考强度与温度展示跟随新模型）。
// 规则：ProviderID 非空时以它为准（校验 enabled）；否则按 Model 名字回查 provider；
// 两者都未命中返回 3003 便于前端提示「模型不可用」。
func (s *ChatService) UpdateSessionModel(ctx context.Context, id string, req *domain.ChatSessionModelREQ) (*domain.ChatSessionRESP, error) {
	row, err := s.sessions.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.TrimSpace(req.ProviderID) != "":
		p, perr := s.provRepo.GetByID(ctx, req.ProviderID)
		if perr != nil || p == nil || !p.Enabled {
			return nil, domain.ErrProviderNotReady
		}
		row.ProviderID = p.ID
		if strings.TrimSpace(req.Model) != "" {
			row.Model = req.Model
		} else {
			row.Model = p.Model
		}
	default:
		if pid, model, ok := s.resolveDefaultProviderModel(ctx, req.Model); ok {
			row.ProviderID = pid
			row.Model = model
		} else {
			return nil, domain.ErrProviderNotReady
		}
	}
	if err := s.sessions.Update(ctx, row); err != nil {
		return nil, err
	}
	r := toSessionRESP(row)
	return &r, nil
}

func (s *ChatService) DeleteSession(ctx context.Context, id string) error {
	return s.sessions.SoftDelete(ctx, id, time.Now().UnixMilli())
}

// DeleteSessions 批量软删。
func (s *ChatService) DeleteSessions(ctx context.Context, ids []string) (ok, failed []string, err error) {
	for _, id := range ids {
		if e := s.DeleteSession(ctx, id); e != nil {
			failed = append(failed, id)
			continue
		}
		ok = append(ok, id)
	}
	return ok, failed, nil
}

func (s *ChatService) ClearMessages(ctx context.Context, sessionID string) error {
	if s.blocks != nil {
		_ = s.blocks.DeleteBySession(ctx, sessionID) // 块级联清理；失败不阻断消息清空
	}
	return s.messages.DeleteAll(ctx, sessionID)
}

// DeleteMessage 删除单条消息；校验归属防止跨会话误删。
func (s *ChatService) DeleteMessage(ctx context.Context, sessionID, messageID string) error {
	m, err := s.messages.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if m.SessionID != sessionID {
		return domain.ErrMessageNotInSession
	}
	if s.blocks != nil {
		_ = s.blocks.DeleteByMessage(ctx, messageID)
	}
	return s.messages.DeleteByID(ctx, messageID)
}

// TruncateMessages 从指定消息起（含）截断该会话后续消息，用于「从此处重新生成」。
func (s *ChatService) TruncateMessages(ctx context.Context, sessionID, messageID string) error {
	m, err := s.messages.GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	if m.SessionID != sessionID {
		return domain.ErrMessageNotInSession
	}
	victims, lerr := s.messages.ListBySession(ctx, sessionID, m.Seq-1, 0)
	if lerr != nil {
		return lerr
	}
	if err := s.messages.DeleteFromSeq(ctx, sessionID, m.Seq); err != nil {
		return err
	}
	if s.blocks != nil {
		for i := range victims {
			_ = s.blocks.DeleteByMessage(ctx, victims[i].ID)
		}
	}
	return nil
}

// ForkSession 从指定消息处分叉：新建会话并复制 ≤ 该 seq 的全部消息。
// 复制体重置 run 关联（RunID 留空），避免新会话继承旧 run 的审批/取消语义。
//
// 分叉血缘写进 ParentID / BranchPoint：分支记住「从哪个会话的哪一条消息长出来」，
// 前端据此把会话组织成树而不是平铺列表。分叉可递归（分支再分叉），不额外记 root。
func (s *ChatService) ForkSession(ctx context.Context, sessionID, messageID string, name string) (*domain.ChatSessionRESP, error) {
	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	at, err := s.messages.GetByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if at.SessionID != sessionID {
		return nil, domain.ErrMessageNotInSession
	}
	src, err := s.messages.ListUpTo(ctx, sessionID, at.Seq)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = ses.Name + "（分叉）"
	}
	newSes := &domain.ChatSessionDO{
		ID:            pkg.NewID(domain.IDSession),
		Name:          name,
		UserID:        ses.UserID,
		ProviderID:    ses.ProviderID,
		Model:         ses.Model,
		WorkspaceID:   ses.WorkspaceID,
		Status:        domain.SessionStatusActive,
		MessageCount:  len(src),
		LastMessageAt: time.Now().UnixMilli(),
		ParentID:      ses.ID,
		BranchPoint:   at.Seq,
	}
	if err := s.sessions.Create(ctx, newSes); err != nil {
		return nil, err
	}
	for i := range src {
		cp := src[i]
		cp.ID = pkg.NewID(domain.IDMessage)
		cp.SessionID = newSes.ID
		cp.RunID = ""
		if err := s.messages.Insert(ctx, &cp); err != nil {
			return nil, err
		}
		// 过程块随消息复制：分叉会话保留完整工具过程
		if s.blocks != nil {
			srcBlocks, berr := s.blocks.ListByMessage(ctx, src[i].ID)
			if berr != nil {
				return nil, berr
			}
			for _, b := range srcBlocks {
				b.ID = pkg.NewID(domain.IDMessageBlock)
				b.MessageID = cp.ID
				b.SessionID = newSes.ID
				if berr := s.blocks.Create(ctx, &b); berr != nil {
					return nil, berr
				}
			}
		}
	}
	r := toSessionRESP(newSes)
	return &r, nil
}

// ListMessages 增量分页；assistant 消息附带持久化过程块（单次批量查询防 N+1）。
func (s *ChatService) ListMessages(ctx context.Context, sessionID string, afterSeq int64, limit int) (*domain.MessageListRESP, error) {
	rows, err := s.messages.ListBySession(ctx, sessionID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	blockMap, err := s.blocksByMessage(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := &domain.MessageListRESP{
		Items:   make([]domain.MessageRESP, 0, len(rows)),
		Total:   len(rows),
		NextSeq: -1,
	}
	var maxSeq int64
	for i := range rows {
		item := toMessageRESP(&rows[i])
		if bs, ok := blockMap[item.ID]; ok {
			item.Blocks = bs
		}
		out.Items = append(out.Items, item)
		if rows[i].Seq > maxSeq {
			maxSeq = rows[i].Seq
		}
	}
	if len(rows) > 0 {
		out.NextSeq = maxSeq + 1
	}
	return out, nil
}

// blocksByMessage 会话级批量取块并按 message_id 分组；未启用块持久化时返回 nil。
func (s *ChatService) blocksByMessage(ctx context.Context, sessionID string) (map[string][]domain.MessageBlockRESP, error) {
	if s.blocks == nil {
		return nil, nil
	}
	rows, err := s.blocks.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]domain.MessageBlockRESP, len(rows))
	for i := range rows {
		out[rows[i].MessageID] = append(out[rows[i].MessageID], domain.MessageBlockRESP{
			ID:        rows[i].ID,
			MessageID: rows[i].MessageID,
			Seq:       rows[i].Seq,
			Kind:      rows[i].Kind,
			Payload:   rows[i].Payload,
			CreatedAt: rows[i].CreatedAt,
		})
	}
	return out, nil
}

// SendStream 立即返回 runID/消息 ID；流式事件经 harness → event.Bus → api 层 → chat:* 推前端。
// params 为请求级采样参数（温度/思考开关；零值 = 不覆盖）。
