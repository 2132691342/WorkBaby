package service

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/mcp"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// mcpNamePattern server 名会成为工具名前缀（mcp_{server}_{tool}），必须限定字符集。
var mcpNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// maskedEnvValue 敏感环境变量在 RESP 中的占位；回传该值表示「保留原值不覆盖」。
const maskedEnvValue = "***"

// McpService MCP 编排：配置落库（env 加密）+ 子进程生命周期 + 运行时状态回写。
type McpService struct {
	repo       *repo.McpServerRepo
	mgr        *mcp.Manager
	cipher     *pkg.Cipher
	configPath string // mcp.json 落盘路径；空表示只同步 DB
}

// NewMcpService 注入仓储、子进程管理器与加密器。
func NewMcpService(r *repo.McpServerRepo, mgr *mcp.Manager, cipher *pkg.Cipher) *McpService {
	return &McpService{repo: r, mgr: mgr, cipher: cipher}
}

// WithConfigPath 指定 mcp.json 落盘路径；保存原始 JSON 时先备份再原子写，失败自动回滚。
func (s *McpService) WithConfigPath(path string) *McpService {
	s.configPath = path
	return s
}

// Sync 启动期对齐：拉取全部配置 → 起停子进程。
func (s *McpService) Sync(ctx context.Context) error {
	list, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	s.reload(ctx, list)
	return nil
}

// List 返回配置 + 运行时状态（env 值统一掩码）。
func (s *McpService) List(ctx context.Context) ([]domain.McpServerRESP, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.McpServerRESP, 0, len(rows))
	for i := range rows {
		out = append(out, *s.toRESP(&rows[i]))
	}
	return out, nil
}

// Add 新增或更新 server 配置（按 name upsert）并重载子进程。
func (s *McpService) Add(ctx context.Context, req *domain.McpServerREQ) (*domain.McpServerRESP, error) {
	if !mcpNamePattern.MatchString(req.Name) {
		return nil, pkg.New(8000, "mcp server name must match [A-Za-z0-9_-]{1,32}", req.Name)
	}
	transport := req.Transport
	if transport == "" {
		transport = domain.McpTransportStdio
	}
	if transport != domain.McpTransportStdio {
		return nil, pkg.New(domain.ErrMcpTransport.Code, domain.ErrMcpTransport.Message, string(transport))
	}
	if req.Command == "" {
		return nil, pkg.New(domain.ErrMcpStartFailed.Code, "mcp command is required", req.Name)
	}
	if _, err := exec.LookPath(req.Command); err != nil {
		return nil, pkg.Wrap(domain.ErrMcpStartFailed.Code, "mcp command not found", err)
	}

	row := &domain.McpServerDO{
		ID:        pkg.NewID(domain.IDMcpServer),
		Name:      req.Name,
		Transport: transport,
		Command:   req.Command,
		Enabled:   true,
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	if len(req.Args) > 0 {
		bs, err := json.Marshal(req.Args)
		if err != nil {
			return nil, pkg.Wrap(8000, "marshal mcp args failed", err)
		}
		row.Args = string(bs)
	}
	env, err := s.encryptEnv(ctx, req.Name, req.Env)
	if err != nil {
		return nil, err
	}
	row.Env = env

	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}
	return s.reloadAndGet(ctx, row.Name)
}

// SetEnabled 启停并重载。
func (s *McpService) SetEnabled(ctx context.Context, name string, enabled bool) error {
	row, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return err
	}
	row.Enabled = enabled
	if err := s.repo.Update(ctx, row); err != nil {
		return err
	}
	_, err = s.reloadAll(ctx)
	return err
}

// Remove 删除配置并停掉子进程、注销其工具。
func (s *McpService) Remove(ctx context.Context, name string) error {
	if err := s.repo.Delete(ctx, name); err != nil {
		return err
	}
	_, err := s.reloadAll(ctx)
	return err
}

// Status 返回运行时状态（工具数 / 是否就绪 / 错误）。
func (s *McpService) Status() []mcp.ServerStatus { return s.mgr.List() }

// Raw 返回 mcp.json 兼容原始内容（env 值掩码），供前端 JSON 编辑器编辑。
func (s *McpService) Raw(ctx context.Context) (string, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return "", err
	}
	servers := make(map[string]mcpRawEntry, len(list))
	for i := range list {
		row := &list[i]
		env, _ := s.decryptEnv(row.Env)
		masked := make(map[string]string, len(env))
		for k := range env {
			masked[k] = maskedEnvValue
		}
		servers[row.Name] = mcpRawEntry{
			Command: row.Command,
			Args:    parseStringList(row.Args),
			Env:     masked,
			Enabled: row.Enabled,
		}
	}
	bs, err := json.MarshalIndent(map[string]any{"mcpServers": servers}, "", "  ")
	if err != nil {
		return "", pkg.Wrap(8000, "marshal mcp raw failed", err)
	}
	return string(bs), nil
}

// SaveRaw 解析 mcp.json 原始内容并全量对齐（新增 / 覆盖 / 删除），随后热重载。
// 返回就绪 server 数；env 值为掩码占位时保留原密文。
// 配置了 configPath 时先备份并原子写 mcp.json；DB 对齐失败自动回滚文件内容。
func (s *McpService) SaveRaw(ctx context.Context, content string) (int, error) {
	var root struct {
		McpServers map[string]mcpRawEntry `json:"mcpServers"` // MCP 生态标准字段名，保持兼容
	}
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return 0, pkg.Wrap(8000, "parse mcp.json failed", err)
	}
	if root.McpServers == nil {
		return 0, pkg.New(8000, "mcp.json must contain mcpServers", "")
	}
	rollback := func() {}
	if s.configPath != "" {
		rb, err := writeConfigFile(s.configPath, content)
		if err != nil {
			return 0, err
		}
		rollback = rb
	}
	active, err := s.alignRaw(ctx, root.McpServers)
	if err != nil {
		rollback()
		return 0, err
	}
	return active, nil
}

// alignRaw 把 mcpServers 映射全量对齐到 DB（新增 / 覆盖 / 删除），随后热重载子进程。
func (s *McpService) alignRaw(ctx context.Context, servers map[string]mcpRawEntry) (int, error) {
	existing, err := s.repo.List(ctx)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]struct{}, len(servers))
	for name, e := range servers {
		if !mcpNamePattern.MatchString(name) {
			return 0, pkg.New(8000, "mcp server name must match [A-Za-z0-9_-]{1,32}", name)
		}
		row := &domain.McpServerDO{
			ID:        pkg.NewID(domain.IDMcpServer),
			Name:      name,
			Transport: domain.McpTransportStdio,
			Command:   strings.TrimSpace(e.Command),
			Enabled:   e.Enabled,
		}
		if row.Command == "" {
			return 0, pkg.New(8000, "mcp command is required", name)
		}
		if len(e.Args) > 0 {
			bs, err := json.Marshal(e.Args)
			if err != nil {
				return 0, pkg.Wrap(8000, "marshal mcp args failed", err)
			}
			row.Args = string(bs)
		}
		env, err := s.encryptEnv(ctx, name, e.Env)
		if err != nil {
			return 0, err
		}
		row.Env = env
		if err := s.repo.Upsert(ctx, row); err != nil {
			return 0, err
		}
		seen[name] = struct{}{}
	}
	for i := range existing {
		if _, ok := seen[existing[i].Name]; !ok {
			if err := s.repo.Delete(ctx, existing[i].Name); err != nil {
				return 0, err
			}
		}
	}
	return s.reloadAll(ctx)
}

// Reload 重新对齐子进程（前端「热重载」按钮）。返回就绪 server 数。
func (s *McpService) Reload(ctx context.Context) (int, error) {
	return s.reloadAll(ctx)
}

// mcpRawEntry mcp.json 单条 server 条目（标准格式）。
type mcpRawEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled bool              `json:"enabled"`
}

// Close 停掉全部子进程（应用退出）。
func (s *McpService) Close() { s.mgr.Close() }

// reloadAll 重新拉配置并对齐子进程；返回就绪 server 数。
func (s *McpService) reloadAll(ctx context.Context) (int, error) {
	list, err := s.repo.List(ctx)
	if err != nil {
		return 0, err
	}
	s.reload(ctx, list)
	ready := 0
	for _, st := range s.mgr.List() {
		if st.Ready {
			ready++
		}
	}
	return ready, nil
}

// reloadAndGet 重载后返回单个 server 的最新视图。
func (s *McpService) reloadAndGet(ctx context.Context, name string) (*domain.McpServerRESP, error) {
	if _, err := s.reloadAll(ctx); err != nil {
		return nil, err
	}
	row, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return s.toRESP(row), nil
}

// reload 对齐子进程并把运行时工具数回写落库（下次启动未起服务时也能显示）。
func (s *McpService) reload(ctx context.Context, list []domain.McpServerDO) {
	s.mgr.Reload(ctx, list)
	for _, st := range s.mgr.List() {
		row, err := s.repo.GetByName(ctx, st.Name)
		if err != nil || row.ToolCount == st.ToolCount {
			continue
		}
		row.ToolCount = st.ToolCount
		_ = s.repo.Update(ctx, row)
	}
}

func (s *McpService) toRESP(row *domain.McpServerDO) *domain.McpServerRESP {
	st := s.mgr.Status(row.Name)
	env, _ := s.decryptEnv(row.Env)
	masked := make(map[string]string, len(env))
	for k := range env {
		masked[k] = maskedEnvValue
	}
	toolCount := st.ToolCount
	if toolCount == 0 {
		toolCount = row.ToolCount
	}
	return &domain.McpServerRESP{
		ID:        row.ID,
		Name:      row.Name,
		Transport: row.Transport,
		Command:   row.Command,
		Args:      parseStringList(row.Args),
		Env:       masked,
		Enabled:   row.Enabled,
		ToolCount: toolCount,
		Ready:     st.Ready,
		Error:     st.Error,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// encryptEnv 环境变量值逐项加密；值为掩码占位时保留原密文，避免 UI 回写覆盖真实密钥。
func (s *McpService) encryptEnv(ctx context.Context, name string, env map[string]string) (string, error) {
	if len(env) == 0 {
		return "", nil
	}
	old := map[string]string{}
	if row, err := s.repo.GetByName(ctx, name); err == nil {
		old, _ = s.decryptEnv(row.Env)
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		if v == maskedEnvValue {
			if ov, ok := old[k]; ok {
				v = ov
			}
		}
		enc, err := s.cipher.Encrypt(v)
		if err != nil {
			return "", pkg.Wrap(2028, "encrypt mcp env failed", err)
		}
		out[k] = enc
	}
	bs, err := json.Marshal(out)
	if err != nil {
		return "", pkg.Wrap(8000, "marshal mcp env failed", err)
	}
	return string(bs), nil
}

// decryptEnv 解密环境变量；解密失败返回已成功的部分（设置页仍可展示键名）。
func (s *McpService) decryptEnv(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	var enc map[string]string
	if err := json.Unmarshal([]byte(raw), &enc); err != nil {
		return nil, pkg.Wrap(8000, "parse mcp env failed", err)
	}
	out := make(map[string]string, len(enc))
	for k, v := range enc {
		pt, err := s.cipher.Decrypt(v)
		if err != nil {
			return out, pkg.Wrap(2028, "decrypt mcp env failed", err)
		}
		out[k] = pt
	}
	return out, nil
}

// parseStringList 解析 Args 列（JSON 数组）；空值返回 nil。
func parseStringList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
