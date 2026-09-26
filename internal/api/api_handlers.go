package api

import (
	"context"
	"encoding/base64"
	"net/url"
	"path/filepath"
	"strings"

	"WorkBaby/internal/config"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// MetaService 健康检查 + 版本元信息（由 service 包下沉到 api 层，省一个壳文件）。
type MetaService struct{ cfg *config.Config }

// NewMetaService 构造元信息服务。
func NewMetaService(cfg *config.Config) *MetaService { return &MetaService{cfg: cfg} }

func (s *MetaService) GetVersion(_ context.Context) domain.VersionInfo {
	cfg := s.cfg
	return domain.VersionInfo{
		AppName:       cfg.App.Name,
		Version:       cfg.App.Version,
		Phase:         "phase-1-mvp",
		Env:           cfg.App.Env,
		DefaultTenant: "default",
		LocalUserID:   "local",
	}
}

func (s *MetaService) GetHealth(_ context.Context, providerCount int, dbOK bool) domain.HealthInfo {
	return domain.HealthInfo{
		Status:    "ok",
		Phase:     "phase-1-mvp",
		DBEnabled: dbOK,
		Providers: providerCount,
	}
}

func (h *Handler) ListAgentProfiles() ([]domain.AgentProfileRESP, error) {
	return h.agentSvc.List(h.ctx)
}

// UpsertAgentProfile 创建/更新自定义子智能体（按 name upsert）。
func (h *Handler) UpsertAgentProfile(req domain.AgentProfileREQ) (domain.AgentProfileRESP, error) {
	p, err := h.agentSvc.Upsert(h.ctx, &req)
	if err != nil {
		return domain.AgentProfileRESP{}, err
	}
	return *p, nil
}

// SetAgentProfileEnabled 启停自定义子智能体。
func (h *Handler) SetAgentProfileEnabled(name string, enabled bool) error {
	return h.agentSvc.SetEnabled(h.ctx, name, enabled)
}

// DeleteAgentProfile 删除自定义子智能体。
func (h *Handler) DeleteAgentProfile(name string) error {
	return h.agentSvc.Delete(h.ctx, name)
}
func (h *Handler) DecideApproval(id string, req domain.DecideApprovalREQ) error {
	return h.approvalSvc.Decide(id, req.Approved, req.Scope)
}

// AnswerInput 用户对补充输入请求（request_input 工具，risk=input_required）的回复回填。
func (h *Handler) AnswerInput(id string, req domain.AnswerInputREQ) error {
	return h.approvalSvc.Answer(id, req.Answer)
}

// SkipApproval 用户跳过：审批按拒绝处理，补充输入按未回复处理（模型自行假设继续）。
func (h *Handler) SkipApproval(id string) error {
	return h.approvalSvc.Skip(id)
}

// ListPendingApprovals 返回当前未决审批（前端刷新/重连后恢复审批用）。
func (h *Handler) ListPendingApprovals() []domain.ApprovalPendingRESP {
	return h.approvalSvc.Pending()
}

// ListApprovalGrants 免审授权列表（设置页查看/撤销「本会话允许」持久化授权）。
func (h *Handler) ListApprovalGrants() []domain.ApprovalGrantRESP {
	return h.approvalSvc.ListGrants(h.ctx)
}

// RevokeApprovalGrant 撤销单条免审授权（库内删除 + 进程内免审表同步摘除）。
func (h *Handler) RevokeApprovalGrant(id string) error {
	return h.approvalSvc.RevokeGrant(h.ctx, id)
}
func (h *Handler) ListSessionArtifacts(sessionID string, limit int) (domain.ArtifactListRESP, error) {
	return h.artifactSvc.List(h.ctx, sessionID, limit)
}

// DeleteSessionArtifact 删除工件登记（不删磁盘文件）。
func (h *Handler) DeleteSessionArtifact(id string) error {
	return h.artifactSvc.Delete(h.ctx, id)
}
func (h *Handler) GetDashboardStats() (*domain.DashboardStatsRESP, error) {
	return h.dashSvc.Stats(h.ctx)
}

// GetDashboardTrend 返回最近 N 天趋势（range 校验在 service/repo 兜底）。
func (h *Handler) GetDashboardTrend(days int) (*domain.DashboardTrendRESP, error) {
	return h.dashSvc.Trend(h.ctx, days)
}

// GetTokenTrend 返回 token 消耗折线图（scope 决定粒度：today→24 小时，其余→按天）。
func (h *Handler) GetTokenTrend(req domain.TokenTrendREQ) (*domain.TokenTrendRESP, error) {
	return h.dashSvc.TokenTrend(h.ctx, req)
}
func (h *Handler) OpenFileDialog(title, pattern string) (string, error) {
	opts := wruntime.OpenDialogOptions{Title: title}
	if pattern != "" {
		opts.Filters = []wruntime.FileFilter{{DisplayName: pattern, Pattern: pattern}}
	}
	return wruntime.OpenFileDialog(h.ctx, opts)
}

// OpenDirectoryDialog 选目录，返回绝对路径；用户取消返回空串。
// 供会话工作区绑定（WorkspacePickerDialog）调原生壳，避免让用户手敲绝对路径。
func (h *Handler) OpenDirectoryDialog(title, defaultPath string) (string, error) {
	return wruntime.OpenDirectoryDialog(h.ctx, wruntime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: defaultPath,
	})
}

// OpenExternal 用系统默认浏览器打开外部链接（http/https/file）。聊天正文与产物卡里的链接
// 一律经此打开：WebView 内直接导航会把整个 SPA 页面替换掉，应用随之不可操作。
func (h *Handler) OpenExternal(url string) error {
	u := strings.TrimSpace(url)
	lower := strings.ToLower(u)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") &&
		!strings.HasPrefix(lower, "file:///") {
		return pkg.New(1003, "不支持的链接协议", u)
	}
	if h.ctx == nil {
		return pkg.New(1004, "应用未就绪", "")
	}
	wruntime.BrowserOpenURL(h.ctx, u)
	return nil
}
func (h *Handler) ListDocs() ([]domain.DocItemRESP, error) {
	return h.docsSvc.List(h.ctx)
}

// GetDoc 返回单篇内置文档详情。
func (h *Handler) GetDoc(name string) (*domain.DocDetailRESP, error) {
	return h.docsSvc.Get(h.ctx, name)
}
func (h *Handler) SearchFiles(q string) ([]domain.FileRESP, error) {
	return h.fileSvc.Search(h.ctx, q)
}

// ListFiles 托管目录全部文件。
func (h *Handler) ListFiles() ([]domain.FileRESP, error) {
	return h.fileSvc.ListFiles(h.ctx)
}

// UploadFile 上传本地文件（srcPath 由前端文件对话框选路径；复制到托管目录）。
func (h *Handler) UploadFile(name, srcPath, sessionID, folderID string) (domain.FileRESP, error) {
	if srcPath == "" {
		return domain.FileRESP{}, pkg.New(1203, "source path required", "")
	}
	return h.fileSvc.Upload(h.ctx, name, srcPath, sessionID, folderID)
}

// UploadFileData 上传内存字节（粘贴 / 拖拽的图片没有本地路径，只能传内容）。
func (h *Handler) UploadFileData(req domain.UploadDataREQ) (domain.FileRESP, error) {
	if req.DataBase64 == "" {
		return domain.FileRESP{}, pkg.New(1203, "file data required", "")
	}
	data, err := base64.StdEncoding.DecodeString(req.DataBase64)
	if err != nil {
		return domain.FileRESP{}, pkg.Wrap(1202, "decode file data failed", err)
	}
	return h.fileSvc.UploadData(h.ctx, req.Name, data, req.SessionID, req.FolderID)
}

// DeleteFile 删除文件（磁盘 + 记录）。
func (h *Handler) DeleteFile(id string) (map[string]any, error) {
	if err := h.fileSvc.Delete(h.ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "deleted": true}, nil
}

// GetFilePreviewURL 生成预览 URL（背景图等场景；AssetServer /files/files/{id}）。
func (h *Handler) GetFilePreviewURL(id string) (map[string]any, error) {
	if _, err := h.fileSvc.Get(h.ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"url": "/files/files/" + id}, nil
}

// ListWorkspaceFiles 会话工作区文件面板列表。
func (h *Handler) ListWorkspaceFiles(sessionID string) ([]domain.WorkspaceFileItem, error) {
	return h.workspaceSvc.ListFiles(h.ctx, sessionID)
}

// ListWorkspaceDir 懒加载列工作区单层目录（真实磁盘内容；路径相对工作区根）。
func (h *Handler) ListWorkspaceDir(sessionID, path string) (domain.WorkspaceListRESP, error) {
	return h.workspaceSvc.ListDir(h.ctx, sessionID, path)
}

// ReadWorkspaceFile 读取工作区单文件内容（沙箱校验；返回 base64 便于绑定跨边界传输）。
func (h *Handler) ReadWorkspaceFile(sessionID, path string) (string, error) {
	bs, _, err := h.workspaceSvc.ReadFile(h.ctx, sessionID, path)
	if err != nil {
		return "", err
	}
	return url.PathEscape(string(bs)), nil
}

// RenameWorkspaceEntry 工作区内同级重命名（文件/目录）。
func (h *Handler) RenameWorkspaceEntry(sessionID, path, newName string) (map[string]any, error) {
	if err := h.workspaceSvc.RenameEntry(h.ctx, sessionID, path, newName); err != nil {
		return nil, err
	}
	return map[string]any{"renamed": true}, nil
}

// CopyWorkspaceEntry 复制文件为同级副本，返回新相对路径。
func (h *Handler) CopyWorkspaceEntry(sessionID, path string) (map[string]any, error) {
	newPath, err := h.workspaceSvc.CopyEntry(h.ctx, sessionID, path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": newPath}, nil
}

// DeleteWorkspaceEntry 删除文件/目录（目录递归；沙箱校验）。
func (h *Handler) DeleteWorkspaceEntry(sessionID, path string) (map[string]any, error) {
	if err := h.workspaceSvc.DeleteEntry(h.ctx, sessionID, path); err != nil {
		return nil, err
	}
	return map[string]any{"deleted": true}, nil
}
func (h *Handler) ListSessionChanges(sessionID string, limit int) (domain.FileChangeListRESP, error) {
	return h.changeSvc.List(h.ctx, sessionID, limit)
}

// GetSessionChange 单条变更详情（diff 正文 + 变更前内容）。
func (h *Handler) GetSessionChange(id string) (domain.FileChangeDetailRESP, error) {
	return h.changeSvc.Detail(h.ctx, id)
}

// RollbackSessionChange 回滚到变更前内容（新建的文件删除，修改的文件写回快照）。
func (h *Handler) RollbackSessionChange(id string) error {
	return h.changeSvc.Rollback(h.ctx, id)
}
func (h *Handler) ListFolders(parentID string) ([]domain.FolderRESP, error) {
	var pid *string
	if parentID != "" {
		pid = &parentID
	}
	return h.folderSvc.ListByParent(h.ctx, pid)
}

// GetFolderTree 文件夹树；workspaceId 非空时仅返回绑定该 workspace 的树。
func (h *Handler) GetFolderTree(workspaceID string) ([]domain.FolderTreeNode, error) {
	if workspaceID != "" {
		return h.folderSvc.TreeForWorkspace(h.ctx, workspaceID)
	}
	return h.folderSvc.Tree(h.ctx)
}

// CreateFolder 新建文件夹。
func (h *Handler) CreateFolder(req domain.FolderREQ) (domain.FolderRESP, error) {
	if req.Name == "" {
		return domain.FolderRESP{}, domain.ErrFolderNameEmpty
	}
	return h.folderSvc.Create(h.ctx, req)
}

// UpdateFolder 重命名/移动。
func (h *Handler) UpdateFolder(id string, req domain.FolderREQ) (domain.FolderRESP, error) {
	if req.Name == "" {
		return domain.FolderRESP{}, domain.ErrFolderNameEmpty
	}
	return h.folderSvc.Update(h.ctx, id, req)
}

// DeleteFolder 删除文件夹（非空拒绝）。
func (h *Handler) DeleteFolder(id string) (map[string]any, error) {
	if err := h.folderSvc.Delete(h.ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "deleted": true}, nil
}
func (h *Handler) ListHooks() ([]domain.UserHookRESP, error) {
	return h.hookSvc.List(h.ctx)
}

// UpsertHook 创建/更新钩子。
func (h *Handler) UpsertHook(req domain.UserHookREQ) (*domain.UserHookRESP, error) {
	return h.hookSvc.Upsert(h.ctx, &req)
}

// DeleteHook 删除钩子。
func (h *Handler) DeleteHook(id string) error {
	return h.hookSvc.Delete(h.ctx, id)
}

// TestHook 设置页试跑一次。
func (h *Handler) TestHook(id string) (*domain.HookTestRESP, error) {
	return h.hookSvc.Test(h.ctx, id)
}
func (h *Handler) AddKnowledgeDoc(req domain.KnowledgeDocREQ) (domain.KnowledgeDocRESP, error) {
	p, err := h.knowledgeSvc.AddDoc(h.ctx, &req)
	if err != nil {
		return domain.KnowledgeDocRESP{}, err
	}
	return *p, nil
}

// ImportKnowledgeFile 本地文件导入受管知识库（复制进 {home}/knowledge 后注册 + 后台索引）。
// 参数：name（文档名，可空自动取文件名）、sourcePath（原生对话框返回的绝对路径）、folderID（可空空串）。
func (h *Handler) ImportKnowledgeFile(name, sourcePath, folderID string) (domain.KnowledgeDocRESP, error) {
	var f *string
	if folderID != "" {
		f = &folderID
	}
	p, err := h.knowledgeSvc.ImportLocalFile(h.ctx, name, sourcePath, f)
	if err != nil {
		return domain.KnowledgeDocRESP{}, err
	}
	return *p, nil
}

// ListKnowledgeDocs 文档列表（含索引状态）。
func (h *Handler) ListKnowledgeDocs() ([]domain.KnowledgeDocRESP, error) {
	return h.knowledgeSvc.ListDocs(h.ctx)
}

// GetKnowledgeDoc 单个文档（轮询索引状态用）。
func (h *Handler) GetKnowledgeDoc(id string) (domain.KnowledgeDocRESP, error) {
	p, err := h.knowledgeSvc.GetDoc(h.ctx, id)
	if err != nil {
		return domain.KnowledgeDocRESP{}, err
	}
	return *p, nil
}

// DeleteKnowledgeDoc 删除文档。
func (h *Handler) DeleteKnowledgeDoc(id string) error {
	return h.knowledgeSvc.DeleteDoc(h.ctx, id)
}

// ReindexKnowledgeDoc 重新索引。
func (h *Handler) ReindexKnowledgeDoc(id string) error {
	return h.knowledgeSvc.ReindexDoc(h.ctx, id)
}

// SearchKnowledge 检索知识库。
func (h *Handler) SearchKnowledge(query string, topK int) ([]domain.KnowledgeHitRESP, error) {
	return h.knowledgeSvc.Search(h.ctx, query, topK)
}

// UpdateKnowledgeDoc 更新知识文档（改名/改来源+重索引）。
func (h *Handler) UpdateKnowledgeDoc(id string, req domain.KnowledgeDocREQ) (domain.KnowledgeDocRESP, error) {
	p, err := h.knowledgeSvc.UpdateDoc(h.ctx, id, &req)
	if err != nil {
		return domain.KnowledgeDocRESP{}, err
	}
	return *p, nil
}

// ListKnowledgeGroups 知识库分组列表。
func (h *Handler) ListKnowledgeGroups() ([]string, error) {
	return h.knowledgeSvc.ListGroups(h.ctx)
}

// ListKnowledgeByGroup 按来源类型过滤。
func (h *Handler) ListKnowledgeByGroup(group string) ([]domain.KnowledgeDocRESP, error) {
	return h.knowledgeSvc.ListByGroup(h.ctx, group)
}

// GetKnowledgeFile 提供受管文件给前端预览（PDF / Word / Excel）。
// 仅对 source_type=file 的受管导入文档生效；text/url 类型前端直接用 source 文本。
// 路由层负责 Fail + c.Data：避免 api 包反向依赖 server 包的 gin/Fail。
func (h *Handler) GetKnowledgeFile(id string) ([]byte, string, error) {
	data, mime, _, err := h.knowledgeSvc.ReadManagedFile(h.ctx, id)
	if err != nil {
		return nil, "", err
	}
	return data, mime, nil
}
func (h *Handler) ListMcpServers() ([]domain.McpServerRESP, error) {
	return h.mcpSvc.List(h.ctx)
}

// AddMcpServer 新增或更新 MCP server 配置，并立即起停子进程。
func (h *Handler) AddMcpServer(req domain.McpServerREQ) (domain.McpServerRESP, error) {
	p, err := h.mcpSvc.Add(h.ctx, &req)
	if err != nil {
		return domain.McpServerRESP{}, err
	}
	return *p, nil
}

// SetMcpServerEnabled 启停 MCP server。
func (h *Handler) SetMcpServerEnabled(name string, enabled bool) error {
	return h.mcpSvc.SetEnabled(h.ctx, name, enabled)
}

// RemoveMcpServer 删除配置并停掉子进程。
func (h *Handler) RemoveMcpServer(name string) error {
	return h.mcpSvc.Remove(h.ctx, name)
}

// GetMcpRaw 返回 mcp.json 兼容原始内容（JSON 编辑器用）。
func (h *Handler) GetMcpRaw() (domain.McpRawRESP, error) {
	content, err := h.mcpSvc.Raw(h.ctx)
	if err != nil {
		return domain.McpRawRESP{}, err
	}
	return domain.McpRawRESP{Content: content}, nil
}

// SaveMcpRaw 解析 mcp.json 原始内容并全量对齐 + 热重载。
func (h *Handler) SaveMcpRaw(req domain.McpRawREQ) (domain.McpReloadRESP, error) {
	active, err := h.mcpSvc.SaveRaw(h.ctx, req.Content)
	if err != nil {
		return domain.McpReloadRESP{}, err
	}
	return domain.McpReloadRESP{Active: active, Saved: true}, nil
}

// ReloadMcpServers 热重载全部 MCP server。
func (h *Handler) ReloadMcpServers() (domain.McpReloadRESP, error) {
	active, err := h.mcpSvc.Reload(h.ctx)
	if err != nil {
		return domain.McpReloadRESP{}, err
	}
	return domain.McpReloadRESP{Active: active}, nil
}

// RevealMcpFile 打开数据目录（Go 版 MCP 配置存于数据库，无 mcp.json 文件）。
func (h *Handler) RevealMcpFile() error {
	if h.paths == nil || h.paths.Home == "" {
		return pkg.New(8000, "home path not ready", "")
	}
	wruntime.BrowserOpenURL(h.ctx, "file:///"+filepath.ToSlash(h.paths.Home))
	return nil
}
func (h *Handler) GetMemoryOverview() (domain.MemoryOverviewRESP, error) {
	return h.memProxy.Overview(), nil
}

// ListMemory 记忆条目列表；section 为空列出全部。
func (h *Handler) ListMemory(section string) (domain.MemoryListRESP, error) {
	return h.memProxy.List(section), nil
}

// SearchMemory 检索记忆条目。
func (h *Handler) SearchMemory(query string, topK int) ([]domain.MemoryEntryRESP, error) {
	return h.memProxy.Search(query, topK), nil
}

// GetMemoryText 读 MEMORY.md 全文（设置页编辑器）。
func (h *Handler) GetMemoryText() (string, error) {
	return h.memProxy.Read(), nil
}

// AppendMemory 追加一条记忆。
func (h *Handler) AppendMemory(req domain.MemoryWriteREQ) error {
	return h.memProxy.Append(&req)
}

// DeleteMemory 删除一条记忆（分节 + 正文精确匹配）。
func (h *Handler) DeleteMemory(req domain.MemoryDeleteREQ) error {
	return h.memProxy.Delete(&req)
}

// ReplaceMemory 整篇覆盖 MEMORY.md。
func (h *Handler) ReplaceMemory(req domain.MemoryReplaceREQ) error {
	return h.memProxy.Replace(&req)
}
func (h *Handler) GetVersion() domain.VersionInfo {
	return h.metaSvc.GetVersion(h.ctx)
}

// GetContractVersion 返回前后端契约版本（前端启动时比对）。
func (h *Handler) GetContractVersion() int { return domain.ContractVersion }

// GetHealth 简化的健康检查（前端 readiness probe）。
// GetRuntimeStatus 返回内置运行时（python）状态，供设置页「关于」排障。
func (h *Handler) GetRuntimeStatus() domain.RuntimeStatusRESP {
	status := domain.RuntimeStatusRESP{Assets: []domain.RuntimeAssetStatus{}}
	if h.paths != nil {
		status.Home = h.paths.Home
	}
	if h.runtimeMgr == nil {
		status.Error = "runtime manager not ready"
		return status
	}
	return h.runtimeMgr.Status()
}

func (h *Handler) GetHealth() domain.HealthInfo {
	pc := 0
	if h.app.ProvRepo != nil {
		if rows, err := h.app.ProvRepo.List(h.ctx); err == nil {
			pc = len(rows)
		}
	}
	return h.metaSvc.GetHealth(h.ctx, pc, h.cfg != nil)
}
func (h *Handler) ListSessions(page, pageSize int) (*domain.SessionListRESP, error) {
	return h.chatSvc.ListSessions(h.ctx, page, pageSize)
}

// CreateSession 新建会话。
func (h *Handler) CreateSession(req domain.ChatSessionREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.CreateSession(h.ctx, &req)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// GetSession 单条会话。
func (h *Handler) GetSession(id string) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.GetSession(h.ctx, id)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// RenameSession 重命名。
func (h *Handler) RenameSession(id string, name string) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.RenameSession(h.ctx, id, name)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// EffectiveParams 当前会话生效参数快照（输入框与设置页共用）。
func (h *Handler) EffectiveParams(id string) (*domain.EffectiveParamsRESP, error) {
	return h.chatSvc.EffectiveParams(h.ctx, id)
}

// SetSessionPermission 切换会话工具权限模式。
func (h *Handler) SetSessionPermission(id string, req domain.ChatSessionPermissionREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.SetSessionPermission(h.ctx, id, req.Mode)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// SetSessionPinned 置顶/取消置顶。
func (h *Handler) SetSessionPinned(id string, req domain.SessionPinREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.SetSessionPinned(h.ctx, id, req.Pinned)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// SetSessionArchived 归档/取消归档。
func (h *Handler) SetSessionArchived(id string, req domain.SessionArchiveREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.SetSessionArchived(h.ctx, id, req.Archived)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// SetSessionModel 切换会话使用的 Provider/模型（参数展示与后续 run 跟随所选模型）。
func (h *Handler) SetSessionModel(id string, req domain.ChatSessionModelREQ) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.UpdateSessionModel(h.ctx, id, &req)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}

// DeleteSession 软删单条。
func (h *Handler) DeleteSession(id string) error {
	return h.chatSvc.DeleteSession(h.ctx, id)
}

// DeleteSessions 批量软删（OK / Failed 列表）。
func (h *Handler) DeleteSessions(ids []string) (domain.BatchDeleteResult, error) {
	ok, failed, err := h.chatSvc.DeleteSessions(h.ctx, ids)
	if err != nil {
		return domain.BatchDeleteResult{}, err
	}
	return domain.BatchDeleteResult{Ok: ok, Failed: failed}, nil
}

// UpdateSessionWorkspace 绑定/解绑会话外部工作目录（绝对路径；空 = 回默认工作区）。
func (h *Handler) UpdateSessionWorkspace(id string, workspacePath string) (domain.ChatSessionRESP, error) {
	s, err := h.chatSvc.UpdateWorkspace(h.ctx, id, workspacePath)
	if err != nil {
		return domain.ChatSessionRESP{}, err
	}
	return *s, nil
}
func (h *Handler) GetSetting(key string) (domain.SystemSettingRESP, error) {
	s, err := h.setSvc.Get(h.ctx, key)
	if err != nil {
		return domain.SystemSettingRESP{}, err
	}
	return *s, nil
}

// SetSetting 写入运行时配置项（覆盖式）。
func (h *Handler) SetSetting(key string, value string) (domain.SystemSettingRESP, error) {
	s, err := h.setSvc.Set(h.ctx, key, value)
	if err != nil {
		return domain.SystemSettingRESP{}, err
	}
	return *s, nil
}

// SettingValue 读设置原始字符串（内部使用：如托盘关闭行为；未配置返回空串不报错）。
func (h *Handler) SettingValue(ctx context.Context, key string) (string, error) {
	s, err := h.setSvc.Get(ctx, key)
	if err != nil {
		return "", err
	}
	if s == nil {
		return "", nil
	}
	return s.V, nil
}

// ListSettings 全部运行时配置（设置面板"高级"页用）。
func (h *Handler) ListSettings() ([]domain.SystemSettingRESP, error) {
	return h.setSvc.ListAll(h.ctx)
}

// GetWebSearchConfig 读取联网搜索配置。
func (h *Handler) GetWebSearchConfig() (domain.WebSearchConfigRESP, error) {
	return h.setSvc.GetWebSearchConfig(h.ctx)
}

// SaveWebSearchConfig 保存联网搜索配置。
func (h *Handler) SaveWebSearchConfig(req domain.WebSearchConfigRESP) (map[string]any, error) {
	if err := h.setSvc.SaveWebSearchConfig(h.ctx, req); err != nil {
		return nil, err
	}
	return map[string]any{"saved": true}, nil
}

// GetGeneralSettings 读取通用设置（主题 / 背景 / 字体等）。
func (h *Handler) GetGeneralSettings() (map[string]any, error) {
	return h.setSvc.GetGeneral(h.ctx)
}

// SaveGeneralSettings 保存通用设置（JSON 值）。
func (h *Handler) SaveGeneralSettings(req map[string]any) (map[string]any, error) {
	return h.setSvc.SaveGeneral(h.ctx, req)
}

// GetExecWhitelist 读取 exec 工具二进制白名单。
func (h *Handler) GetExecWhitelist() ([]string, error) {
	return h.setSvc.GetExecWhitelist(h.ctx)
}

// SaveExecWhitelist 保存 exec 工具二进制白名单。
func (h *Handler) SaveExecWhitelist(binaries []string) ([]string, error) {
	return h.setSvc.SaveExecWhitelist(h.ctx, binaries)
}
func (h *Handler) ListSkills() ([]domain.SkillRESP, error) {
	return h.skillSvc.List(h.ctx)
}

// SetSkillEnabled 启停 Skill。
func (h *Handler) SetSkillEnabled(name string, enabled bool) error {
	return h.skillSvc.SetEnabled(h.ctx, name, enabled)
}

// ImportSkill 导入自定义 Skill（UI 编辑；同名覆盖）。
func (h *Handler) ImportSkill(req domain.SkillREQ) (domain.SkillRESP, error) {
	p, err := h.skillSvc.ImportCustom(h.ctx, &req)
	if err != nil {
		return domain.SkillRESP{}, err
	}
	return *p, nil
}

// UpdateSkill 更新自定义 Skill 内容（内置 Skill 拒绝）。
func (h *Handler) UpdateSkill(name string, req domain.SkillREQ) (domain.SkillRESP, error) {
	p, err := h.skillSvc.UpdateCustom(h.ctx, name, &req)
	if err != nil {
		return domain.SkillRESP{}, err
	}
	return *p, nil
}

// DeleteSkill 删除 Skill（内置 Skill 拒绝）。
func (h *Handler) DeleteSkill(name string) error {
	return h.skillSvc.Delete(h.ctx, name)
}

// ImportSkillsZip 批量导入技能包 zip（zipPath 由原生文件对话框提供）；
// 返回 { imported: []string, skipped: []string, failed: [{path, error}] }。
func (h *Handler) ImportSkillsZip(zipPath string) (map[string]any, error) {
	return h.skillSvc.ImportZip(h.ctx, zipPath)
}
func (h *Handler) SubmitTask(req domain.ChatTaskREQ) (*domain.ChatTaskDO, error) {
	return h.taskSvc.Submit(h.ctx, req.SessionID, req.Agent, req.Prompt)
}

// ListTasks 任务列表（GET /tasks?limit=50，最新在前）。
func (h *Handler) ListTasks(limit int) (*domain.ChatTaskListRESP, error) {
	return h.taskSvc.List(h.ctx, limit)
}

// CancelTask 取消任务（POST /tasks/:id/cancel）；幂等：已终态无事发生。
func (h *Handler) CancelTask(id string) error {
	return h.taskSvc.Cancel(h.ctx, id)
}
func (h *Handler) ListTools() ([]domain.ToolMeta, error) {
	return h.toolSvc.ListTools(h.ctx)
}

// SetToolEnabled 持久化工具启停状态。
func (h *Handler) SetToolEnabled(name string, enabled bool) error {
	return h.toolSvc.SetToolEnabled(h.ctx, name, enabled)
}
func (h *Handler) ListTrust() ([]domain.WorkspaceTrustRESP, error) {
	return h.trustSvc.List(h.ctx)
}

// ResolveTrust 解析某目录的信任态（不落库；前端实时查询时使用）。
func (h *Handler) ResolveTrust(path string) (domain.TrustResolveRESP, error) {
	return h.trustSvc.Resolve(h.ctx, path)
}

// DecideTrust 写入信任决策（allow / ask / deny）。
func (h *Handler) DecideTrust(req domain.WorkspaceTrustREQ) (domain.WorkspaceTrustRESP, error) {
	return h.trustSvc.Decide(h.ctx, req)
}

// RevokeTrust 撤销某目录的信任登记（回到默认 ask）。
func (h *Handler) RevokeTrust(path string) error {
	return h.trustSvc.Revoke(h.ctx, path)
}

// TrustRoots 恒信任根（前端展示「这些目录默认信任」用）。
func (h *Handler) TrustRoots() []string {
	if h.trustSvc == nil {
		return nil
	}
	return h.trustSvc.Roots()
}

// GetAdminOverview 返回管理后台概览（版本/环境/计数聚合）。
func (h *Handler) GetAdminOverview() (*domain.AdminOverviewRESP, error) {
	version := h.metaSvc.GetVersion(h.ctx)
	home := ""
	if h.paths != nil {
		home = h.paths.Home
	}
	return h.dashSvc.Overview(h.ctx, version.Version, version.Phase, home, h.cfg != nil)
}

// CleanupMisreportedTokenUsage 清理上游误报的缓存 token：把 cache_read_tokens > input_tokens
// 的存量行 cache_read / cache_write 清零（新数据已在解析时 clamp）。不可逆、幂等。
func (h *Handler) CleanupMisreportedTokenUsage() (map[string]any, error) {
	if h.app.UsageRepo == nil {
		return nil, pkg.New(2012, "token usage repo not ready", "")
	}
	n, err := h.app.UsageRepo.CleanupMisreported(h.ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"cleaned": n}, nil
}
