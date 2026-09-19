package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"WorkBaby/internal/capability"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/runtime"
	"WorkBaby/internal/skill"
)

// SkillService Skill 编排：skills 表唯一真相源 + Registry 运行时装载。
type SkillService struct {
	repo *repo.SkillRepo
	reg  *skill.Registry

	// 目录自动发现：全局 {home}/skills + 工作区 <ws>/.workbaby/skills 两级叠加
	globalRoot string
	activeWS   string // 已装载的工作区技能根；切换时整根摘除，防跨工作区串味
	skillMu    sync.Mutex
}

// NewSkillService 注入仓储与注册中心。
func NewSkillService(r *repo.SkillRepo, reg *skill.Registry) *SkillService {
	return &SkillService{repo: r, reg: reg}
}

// Summaries 已启用 Skill 的名称与用途（内存态，不走 DB）：
// 未命中技能时注入「有哪些技能可用」，避免模型对着空白上下文硬猜。
func (s *SkillService) Summaries() []capability.SkillSummary {
	if s.reg == nil {
		return nil
	}
	all := s.reg.List()
	out := make([]capability.SkillSummary, 0, len(all))
	for _, sk := range all {
		if sk == nil || sk.Skill == nil || !sk.Skill.Enabled {
			continue
		}
		out = append(out, capability.SkillSummary{Name: sk.Skill.Name, Description: sk.Skill.Description})
	}
	return out
}

// SyncBuiltin 启动期：内置 Skill upsert 进表 + 重建 Registry。
//
// 用户关闭状态必须保留：内置 SKILL.md 每次启动重新解析（Enabled 恒为 true），
// 直接 upsert 全字段覆盖会把用户 SetSkillEnabled(false) 的结果重置回 true。
func (s *SkillService) SyncBuiltin(ctx context.Context) error {
	rows, err := skill.LoadBuiltin()
	if err != nil {
		return err
	}
	for i := range rows {
		if exist, gerr := s.repo.GetByName(ctx, rows[i].Name); gerr == nil && exist != nil {
			rows[i].Enabled = exist.Enabled
		}
		if err := s.repo.Upsert(ctx, &rows[i]); err != nil {
			return err
		}
	}
	return s.Reload(ctx)
}

// Reload 从表重建 Registry（增删改/启停后调用）。
func (s *SkillService) Reload(ctx context.Context) error {
	list, err := s.repo.List(ctx)
	if err != nil {
		return err
	}
	return s.reg.Reload(list)
}

// WithGlobalDir 设置全局技能目录（{home}/skills）；链式装配。
func (s *SkillService) WithGlobalDir(dir string) *SkillService {
	s.globalRoot = dir
	return s
}

// GlobalDir 全局技能目录（UI 提示 / 诊断用）。
func (s *SkillService) GlobalDir() string { return s.globalRoot }

// SyncGlobal 启动期同步全局技能目录：扫描入库 + 摘除已消失条目 + 重建 Registry；
// 目录不存在返回空（首次启动的正常状态）。
func (s *SkillService) SyncGlobal(ctx context.Context) error {
	s.skillMu.Lock()
	defer s.skillMu.Unlock()
	if s.globalRoot == "" {
		return nil
	}
	s.upsertDir(ctx, s.globalRoot)
	s.pruneMissing(ctx, []string{s.globalRoot})
	return s.Reload(ctx)
}

// SyncWorkspace 按会话工作区叠加技能（工作区同名覆盖全局）。工作区未变化时直接返回；
// 切换工作区时先整根摘除上一工作区技能再重扫，避免跨工作区串味并恢复被覆盖的全局版本。
func (s *SkillService) SyncWorkspace(ctx context.Context, wsPath string) error {
	root := workspaceSkillRoot(wsPath)
	s.skillMu.Lock()
	defer s.skillMu.Unlock()
	if root == s.activeWS {
		return nil
	}
	if s.activeWS != "" {
		s.removeUnder(ctx, s.activeWS)
	}
	if s.globalRoot != "" {
		s.upsertDir(ctx, s.globalRoot)
	}
	if root != "" {
		s.upsertDir(ctx, root)
	}
	// 磁盘已消失的自动发现条目（含全局目录里删掉的技能）一并摘除
	s.pruneMissing(ctx, []string{s.globalRoot, root})
	s.activeWS = root
	return s.Reload(ctx)
}

// upsertDir 把目录下的技能写入表；已存在则保留用户启停状态（覆盖导入不重置开关）。
func (s *SkillService) upsertDir(ctx context.Context, root string) {
	for _, sk := range skill.DiscoverDir(root, domain.SkillSourceKindCustom) {
		if exist, err := s.repo.GetByName(ctx, sk.Name); err == nil && exist != nil {
			sk.Enabled = exist.Enabled
		}
		if err := s.repo.Upsert(ctx, &sk); err != nil {
			pkg.L.Warn("upsert discovered skill failed", "name", sk.Name, "root", root, "err", err.Error())
		}
	}
}

// removeUnder 摘除来源位于 root 之下的全部技能（工作区切换时整根失效）。
// 内置技能（SourceRef=assets/…）与 UI 自建技能（SourceRef 为空）永不匹配。
func (s *SkillService) removeUnder(ctx context.Context, root string) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return
	}
	for i := range rows {
		if !skill.UnderRoot(rows[i].SourceRef, root) {
			continue
		}
		if err := s.repo.Delete(ctx, rows[i].Name); err != nil {
			pkg.L.Warn("remove workspace skill failed", "name", rows[i].Name, "err", err.Error())
		}
	}
}

// pruneMissing 摘除「来源在 roots 下但磁盘 SKILL.md 已消失」的技能。
func (s *SkillService) pruneMissing(ctx context.Context, roots []string) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return
	}
	for _, name := range skill.StaleSkillNames(rows, roots) {
		if err := s.repo.Delete(ctx, name); err != nil {
			pkg.L.Warn("prune missing skill failed", "name", name, "err", err.Error())
		}
	}
}

// workspaceSkillRoot 工作区技能目录：<ws>/.workbaby/skills；未绑定工作区返回空串。
func workspaceSkillRoot(wsPath string) string {
	if strings.TrimSpace(wsPath) == "" {
		return ""
	}
	return filepath.Join(wsPath, runtime.SandboxDirName, "skills")
}

// List 返回全部 Skill（含 disabled）。
func (s *SkillService) List(ctx context.Context) ([]domain.SkillRESP, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SkillRESP, 0, len(rows))
	for i := range rows {
		out = append(out, toSkillRESP(&rows[i]))
	}
	return out, nil
}

// SetEnabled 启停切换并重建 Registry。
func (s *SkillService) SetEnabled(ctx context.Context, name string, enabled bool) error {
	row, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return err
	}
	row.Enabled = enabled
	if err := s.repo.Update(ctx, row); err != nil {
		return err
	}
	return s.Reload(ctx)
}

// marshalScripts 序列化脚本列表（空列表 → 空串 = 清除）。
func marshalScripts(scripts []domain.SkillScript) (string, error) {
	if len(scripts) == 0 {
		return "", nil
	}
	bs, err := json.Marshal(scripts)
	if err != nil {
		return "", pkg.Wrap(8002, "marshal scripts failed", err)
	}
	return string(bs), nil
}

// ImportCustom 导入自定义 Skill（UI 编辑 / SKILL.md 粘贴；scripts 随存）。
func (s *SkillService) ImportCustom(ctx context.Context, req *domain.SkillREQ) (*domain.SkillRESP, error) {
	if req.Name == "" || req.Body == "" {
		return nil, pkg.New(8002, "skill name and body are required", "")
	}
	tools, err := json.Marshal(req.AllowedTools)
	if err != nil {
		return nil, pkg.Wrap(8002, "marshal allowed_tools failed", err)
	}
	scripts, err := marshalScripts(req.Scripts)
	if err != nil {
		return nil, err
	}
	row := &domain.SkillDO{
		ID:           pkg.NewID(domain.IDSkill),
		Name:         req.Name,
		Description:  req.Description,
		WhenToUse:    req.WhenToUse,
		Body:         req.Body,
		AllowedTools: string(tools),
		ScriptsJSON:  scripts,
		SourceKind:   domain.SkillSourceKindCustom,
		Enabled:      true,
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}
	if err := s.Reload(ctx); err != nil {
		return nil, err
	}
	out := toSkillRESP(row)
	return &out, nil
}

// UpdateCustom 更新自定义 Skill（内置 Skill 拒绝；不存在则按 name 新建）。
func (s *SkillService) UpdateCustom(ctx context.Context, name string, req *domain.SkillREQ) (*domain.SkillRESP, error) {
	if req.Body == "" {
		return nil, pkg.New(8002, "skill body is required", "")
	}
	row, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if row != nil && row.SourceKind == domain.SkillSourceKindBuiltin {
		return nil, pkg.New(8003, "builtin skill is read-only", name)
	}
	tools, err := json.Marshal(req.AllowedTools)
	if err != nil {
		return nil, pkg.Wrap(8002, "marshal allowed_tools failed", err)
	}
	scripts, err := marshalScripts(req.Scripts)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = &domain.SkillDO{ID: pkg.NewID(domain.IDSkill), Name: name, SourceKind: domain.SkillSourceKindCustom, Enabled: true}
	}
	row.Description = req.Description
	row.WhenToUse = req.WhenToUse
	row.Body = req.Body
	row.AllowedTools = string(tools)
	row.ScriptsJSON = scripts
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}
	if err := s.Reload(ctx); err != nil {
		return nil, err
	}
	out := toSkillRESP(row)
	return &out, nil
}

// Delete 删除 Skill（内置 Skill 拒绝）。
func (s *SkillService) Delete(ctx context.Context, name string) error {
	row, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return err
	}
	if row == nil {
		return nil // 幂等
	}
	if row.SourceKind == domain.SkillSourceKindBuiltin {
		return pkg.New(8003, "builtin skill is read-only", name)
	}
	if err := s.repo.Delete(ctx, name); err != nil {
		return err
	}
	return s.Reload(ctx)
}

// Match 关键词路由（chat 层调用）。
func (s *SkillService) Match(content string) string {
	if s.reg == nil {
		return ""
	}
	return s.reg.Match(content)
}

// Get 按名取 Skill 运行时态（chat 层注入 body / 工具白名单）。
func (s *SkillService) Get(name string) (*skill.LoadedSkill, bool) {
	if s.reg == nil {
		return nil, false
	}
	return s.reg.Get(name)
}

// GetScript 解析 (skill, script) → 脚本代码与语言（run_skill_script 工具的 resolver）。
func (s *SkillService) GetScript(ctx context.Context, skillName, scriptName string) (string, string, error) {
	row, err := s.repo.GetByName(ctx, skillName)
	if err != nil {
		return "", "", err
	}
	if row == nil {
		return "", "", pkg.New(8001, "skill not found", skillName)
	}
	if !row.Enabled {
		return "", "", pkg.New(8004, "skill is disabled", skillName)
	}
	if row.ScriptsJSON == "" {
		return "", "", pkg.New(8004, "skill has no scripts", skillName)
	}
	var scripts []domain.SkillScript
	if err := json.Unmarshal([]byte(row.ScriptsJSON), &scripts); err != nil {
		return "", "", pkg.Wrap(8004, "parse scripts failed", err)
	}
	for i := range scripts {
		if scripts[i].Name == scriptName {
			return scripts[i].Code, scripts[i].Language, nil
		}
	}
	return "", "", pkg.New(8004, "script not found", scriptName)
}

func toSkillRESP(sk *domain.SkillDO) domain.SkillRESP {
	var scripts []domain.SkillScript
	if sk.ScriptsJSON != "" {
		_ = json.Unmarshal([]byte(sk.ScriptsJSON), &scripts)
	}
	return domain.SkillRESP{
		ID:           sk.ID,
		Name:         sk.Name,
		Description:  sk.Description,
		WhenToUse:    sk.WhenToUse,
		Body:         sk.Body,
		AllowedTools: parseTools(sk.AllowedTools),
		Scripts:      scripts,
		SourceKind:   sk.SourceKind,
		SourceRef:    sk.SourceRef,
		Version:      sk.Version,
		Enabled:      sk.Enabled,
		CreatedAt:    sk.CreatedAt,
		UpdatedAt:    sk.UpdatedAt,
	}
}

func parseTools(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}
