package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/internal/core"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/tool/file"
)

// detailMaxBytes 详情接口返回的 before/after 正文上限；超出截断（面板只做人工审阅）。
const detailMaxBytes = 256 << 10

// FileChangeService 文件变更登记与回滚：写前落快照 → 算 unified diff → 落库 →
// 推 chat:file-change 事件。前端面板据此渲染清单并可一键回滚；只覆盖 file_write
// （exec 的副作用无法可靠归因）。
type FileChangeService struct {
	repo      *repo.FileChangeRepo
	bus       *event.Bus
	emitter   *Emitter                      // 事件出口（与 ChatService 共享同一实例）
	snapshots string                        // 快照根目录（默认 {home}/snapshots）
	root      string                        // 工作区根（用于算相对路径）
	snapRoot  func(sessionID string) string // 可选：会话级快照目录（绑定外部工作区时改放 {dir}/.workbaby/snapshots）
}

// NewFileChangeService 构造；snapshots 为快照目录，root 为工作区根。
func NewFileChangeService(r *repo.FileChangeRepo, bus *event.Bus, snapshots, root string) *FileChangeService {
	// 默认出口可用：事件不因装配遗漏而静默丢失；装配方可用 WithEmitter 升级为带重放的共享实例。
	return &FileChangeService{repo: r, bus: bus, emitter: NewEmitter(bus, nil), snapshots: snapshots, root: root}
}

// WithSnapshotRoot 注入会话级快照目录解析器；注入后优先于固定根 + sessionID 推导。
func (s *FileChangeService) WithSnapshotRoot(f func(sessionID string) string) *FileChangeService {
	s.snapRoot = f
	return s
}

// WithEventLog 启用 run 事件日志：变更事件与 chat 事件共享序号空间，断线重放不缺帧。
// WithEmitter 注入事件出口（文件变更事件经它分配 seq 并广播）。
func (s *FileChangeService) WithEmitter(e *Emitter) *FileChangeService {
	s.emitter = e
	return s
}

// ChangeInput 一次文件写入的变更输入。
type ChangeInput struct {
	SessionID string
	RunID     string
	ToolName  string
	Path      string // 磁盘绝对路径
	Existed   bool   // 写入前文件是否存在（false = 新建）
	Before    []byte // 变更前内容（Existed=false 时忽略）
	After     []byte // 变更后内容
}

// Record 登记一次文件变更：落快照 → 算 diff → 落库 → 推事件。
func (s *FileChangeService) Record(ctx context.Context, in ChangeInput) (*domain.FileChangeDO, error) {
	rel := relativeTo(s.root, in.Path)
	action := domain.ChangeModify
	if !in.Existed {
		action = domain.ChangeCreate
	}
	row := &domain.FileChangeDO{
		ID:          pkg.NewID("CHG"),
		SessionID:   in.SessionID,
		RunID:       in.RunID,
		ToolName:    in.ToolName,
		Path:        in.Path,
		RelPath:     rel,
		Action:      action,
		BytesBefore: int64(len(in.Before)),
		BytesAfter:  int64(len(in.After)),
	}
	if in.Existed {
		snapPath, err := s.writeSnapshot(in.SessionID, row.ID, in.Before)
		if err != nil {
			pkg.L.Warn("file change snapshot failed", "path", in.Path, "err", err.Error())
		} else {
			row.SnapshotPath = snapPath
		}
	}
	d := pkg.UnifiedDiff(rel, string(in.Before), string(in.After))
	row.Diff = d.Text
	row.AddedLines = d.Added
	row.RemovedLines = d.Removed
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, err
	}
	// 越界告警：rel 落在 workspace 根（含其下子目录）但不在 .workbaby/ 子树内 →
	// 视为污染用户原有目录，emit chat:warn 让前端红色横幅提示用户立刻清理。
	// 不阻塞变更落库（已发生的副作用必须留痕供回滚），但用户必须看见。
	if s.root != "" && isOutOfSandbox(s.root, in.Path) {
		s.emit(ctx, "chat:warn", domain.ChatWarnEvent{
				Kind:      "file_out_of_sandbox",
				RelPath:   rel,
				Path:      in.Path,
				Workspace: s.root,
				Sandbox:   runtime.SandboxOf(s.root).Root,
				Message:   "文件写到了工作区 .workbaby/ 之外（污染原有目录）；右键文件变更卡片可一键回滚",
			})
	}
	s.emit(ctx, "chat:file-change", map[string]any{"change": toFileChangeRESP(row)})
	return row, nil
}

// ListByRun 单次 run 的变更记录（完成度证据：核对声明路径与本 run 产物）。
func (s *FileChangeService) ListByRun(ctx context.Context, runID string, limit int) ([]domain.FileChangeDO, error) {
	return s.repo.ListByRun(ctx, runID)
}

// List 会话变更清单（最新在前）。
func (s *FileChangeService) List(ctx context.Context, sessionID string, limit int) (domain.FileChangeListRESP, error) {
	rows, err := s.repo.ListBySession(ctx, sessionID, limit)
	if err != nil {
		return domain.FileChangeListRESP{}, err
	}
	out := domain.FileChangeListRESP{Items: make([]domain.FileChangeRESP, 0, len(rows)), Total: len(rows)}
	for i := range rows {
		out.Items = append(out.Items, toFileChangeRESP(&rows[i]))
	}
	return out, nil
}

// Detail 单条变更详情（diff + 变更前内容）。
func (s *FileChangeService) Detail(ctx context.Context, id string) (domain.FileChangeDetailRESP, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.FileChangeDetailRESP{}, err
	}
	out := domain.FileChangeDetailRESP{
		FileChangeRESP: toFileChangeRESP(row),
		Diff:           row.Diff,
	}
	if row.SnapshotPath != "" {
		if bs, err := os.ReadFile(row.SnapshotPath); err == nil {
			out.BeforeText = truncateBytes(bs, detailMaxBytes)
		}
	}
	if bs, err := os.ReadFile(row.Path); err == nil {
		out.AfterText = truncateBytes(bs, detailMaxBytes)
	}
	out.Truncated = int64(len(out.BeforeText)) < row.BytesBefore || int64(len(out.AfterText)) < row.BytesAfter
	return out, nil
}

// Rollback 回滚到变更前内容：新建的文件删除，修改的文件写回快照。
func (s *FileChangeService) Rollback(ctx context.Context, id string) error {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row.Action == domain.ChangeCreate {
		if err := os.Remove(row.Path); err != nil && !os.IsNotExist(err) {
			return pkg.Wrap(domain.ErrFileChangeRollback.Code, domain.ErrFileChangeRollback.Message, err)
		}
	} else {
		if row.SnapshotPath == "" {
			return domain.ErrFileChangeRollback
		}
		bs, err := os.ReadFile(row.SnapshotPath)
		if err != nil {
			return pkg.Wrap(domain.ErrFileChangeRollback.Code, domain.ErrFileChangeRollback.Message, err)
		}
		if err := os.WriteFile(row.Path, bs, 0o644); err != nil {
			return pkg.Wrap(domain.ErrFileChangeRollback.Code, domain.ErrFileChangeRollback.Message, err)
		}
	}
	if err := s.repo.MarkRolledBack(ctx, id); err != nil {
		return err
	}
	row.RolledBack = true
	s.emit(ctx, "chat:file-change", map[string]any{"change": toFileChangeRESP(row), "rolled_back": true})
	return nil
}

// fileChangeRecorder 把 file_write 的写操作接到变更追踪与工件登记。session/run 身份从 ctx 解析：
// 无会话上下文（工具单测等）时不记录，避免无归属的孤儿变更；两者都是旁路能力，失败只告警。
type fileChangeRecorder struct {
	svc      *FileChangeService
	artifact *ArtifactService
	toolName string
}

// NewFileChangeRecorder 构造 file.Recorder 适配器；artifact 可为 nil（不登记工件）。
func NewFileChangeRecorder(svc *FileChangeService, artifact *ArtifactService, toolName string) file.Recorder {
	return &fileChangeRecorder{svc: svc, artifact: artifact, toolName: toolName}
}

// RecordWrite 记录一次写入：先落变更（快照 + diff），再幂等登记工件。
func (r *fileChangeRecorder) RecordWrite(ctx context.Context, path string, existed bool, before, after []byte) {
	if r.svc == nil {
		return
	}
	sessionID := core.SessionIDFromCtx(ctx)
	if sessionID == "" {
		return
	}
	runID := core.RunIDFromCtx(ctx)
	if _, err := r.svc.Record(ctx, ChangeInput{
		SessionID: sessionID,
		RunID:     runID,
		ToolName:  r.toolName,
		Path:      path,
		Existed:   existed,
		Before:    before,
		After:     after,
	}); err != nil {
		pkg.L.Warn("record file change failed", "path", path, "err", err.Error())
	}
	if r.artifact == nil {
		return
	}
	if _, err := r.artifact.Register(ctx, ArtifactInput{
		SessionID: sessionID,
		RunID:     runID,
		Path:      path,
		Size:      int64(len(after)),
	}); err != nil {
		pkg.L.Warn("register artifact failed", "path", path, "err", err.Error())
	}
}

// writeSnapshot 写变更前内容到 {snapshots}/{session}/{id}.bak。
// 注入了会话级解析器时优先用解析目录（绑定外部工作区 → {dir}/.workbaby/snapshots/{session}）。
func (s *FileChangeService) writeSnapshot(sessionID, id string, before []byte) (string, error) {
	dir := filepath.Join(s.snapshots, sessionID)
	if s.snapRoot != nil {
		if d := s.snapRoot(sessionID); d != "" {
			dir = d
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", pkg.Wrap(4007, "mkdir snapshot dir failed", err)
	}
	p := filepath.Join(dir, id+".bak")
	if err := os.WriteFile(p, before, 0o644); err != nil {
		return "", pkg.Wrap(4007, "write snapshot failed", err)
	}
	return p, nil
}

// emit 发布变更事件（归属经 ctx 提取，统一走 Emitter）。
// 载荷支持 map[string]any（极简增量）或领域事件结构体（如 domain.ChatWarnEvent）；字段 snake_case。
func (s *FileChangeService) emit(ctx context.Context, name string, payload any) {
	s.emitter.EmitCtx(ctx, name, payload)
}

// toFileChangeRESP DO → 出参（不泄漏磁盘路径）。
func toFileChangeRESP(row *domain.FileChangeDO) domain.FileChangeRESP {
	return domain.FileChangeRESP{
		ID:           row.ID,
		SessionID:    row.SessionID,
		RunID:        row.RunID,
		ToolName:     row.ToolName,
		RelPath:      row.RelPath,
		Action:       row.Action,
		BytesBefore:  row.BytesBefore,
		BytesAfter:   row.BytesAfter,
		AddedLines:   row.AddedLines,
		RemovedLines: row.RemovedLines,
		RolledBack:   row.RolledBack,
		CreatedAt:    row.CreatedAt,
	}
}

// relativeTo 相对 root 的路径；不在 root 内或计算失败时退回原路径。
func relativeTo(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return filepath.ToSlash(rel)
}

// isOutOfSandbox 判定 p 是否落在 workspace 根之内但不在 .workbaby/ 子树内（即写进了用户原有目录）。
// 未绑定 workspace 或解析失败按「非越界」处理。
func isOutOfSandbox(workspace, p string) bool {
	if workspace == "" || p == "" {
		return false
	}
	absWS, err := filepath.Abs(workspace)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	relWS, err := filepath.Rel(absWS, absPath)
	if err != nil || relWS == ".." || strings.HasPrefix(relWS, ".."+string(filepath.Separator)) {
		// 路径在 workspace 外：也算越界（用户能直接看见该红色提示）
		return true
	}
	sandboxRoot := runtime.SandboxOf(absWS).Root
	if absPath == sandboxRoot {
		return false
	}
	relSandbox, err := filepath.Rel(sandboxRoot, absPath)
	if err != nil {
		return false
	}
	return relSandbox == ".." || strings.HasPrefix(relSandbox, ".."+string(filepath.Separator))
}

// truncateBytes 按字节上限截断（详情面板防大文件）。
func truncateBytes(bs []byte, max int) string {
	if len(bs) <= max {
		return string(bs)
	}
	return string(bs[:max]) + "\n... (truncated)"
}
