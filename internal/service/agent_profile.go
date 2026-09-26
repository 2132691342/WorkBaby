package service

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// AgentProfileService 自定义子智能体：CRUD + 启动同步进 harness 注册表。注册表是运行期
// 唯一消费点（delegate_task 按名委派、会话 Agent 切换），本服务把 enabled 的行物化成 agent.Definition。
type AgentProfileService struct {
	repo     *repo.AgentProfileRepo
	dataHome string // {home}；非空时额外加载 {home}/agents/*.md 定义文件
}

// NewAgentProfileService 构造服务；注册表同步由调用方在启动期显式 Sync（失败不阻断启动）。
func NewAgentProfileService(repo *repo.AgentProfileRepo) *AgentProfileService {
	return &AgentProfileService{repo: repo}
}

// WithDataHome 注入数据根：启用 {home}/agents/*.md 定义文件（文件与表同构，一并物化）。
func (s *AgentProfileService) WithDataHome(home string) *AgentProfileService {
	s.dataHome = home
	return s
}

// agentNamePattern kebab-case 唯一名（与 Skill 命名约定一致）。
var agentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// Sync 把 enabled 的自定义子智能体物化进 harness 注册表（启动期与每次写操作后调用）。
// 来源两处：agent_profiles 表（界面维护）+ {home}/agents/*.md（文件维护）。
// 同名时以表为准——界面上的显式修改优先于磁盘上的历史文件，避免改了界面却不生效。
func (s *AgentProfileService) Sync(ctx context.Context) error {
	rows, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	defs := make([]agent.Definition, 0, len(rows))
	taken := make(map[string]struct{}, len(rows))
	for i := range rows {
		d := profileToDefinition(&rows[i])
		defs = append(defs, d)
		taken[d.Name] = struct{}{}
	}
	for _, d := range LoadAgentFiles(s.dataHome) {
		if _, dup := taken[d.Name]; dup {
			pkg.L.Warn("agent file shadowed by profile row", "agent", d.Name)
			continue
		}
		defs = append(defs, d)
	}
	agent.SetCustomAgents(defs)
	return nil
}

// List 全量（含 disabled），设置页展示。
func (s *AgentProfileService) List(ctx context.Context) ([]domain.AgentProfileRESP, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentProfileRESP, 0, len(rows))
	for i := range rows {
		out = append(out, profileToRESP(&rows[i]))
	}
	return out, nil
}

// Upsert 按 name 创建或整体更新自定义子智能体；写库成功即重同步注册表。
func (s *AgentProfileService) Upsert(ctx context.Context, req *domain.AgentProfileREQ) (*domain.AgentProfileRESP, error) {
	row, err := validateProfileReq(req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}
	if err := s.Sync(ctx); err != nil {
		return nil, err
	}
	out := profileToRESP(row)
	return &out, nil
}

// SetEnabled 启停；停用的行从注册表摘除（下次 run 起生效）。
func (s *AgentProfileService) SetEnabled(ctx context.Context, name string, enabled bool) error {
	if err := s.repo.SetEnabled(ctx, name, enabled); err != nil {
		return err
	}
	return s.Sync(ctx)
}

// Delete 软删并重同步注册表。
func (s *AgentProfileService) Delete(ctx context.Context, name string) error {
	if err := s.repo.Delete(ctx, name); err != nil {
		return err
	}
	return s.Sync(ctx)
}

// validateProfileReq 校验入参并物化 DO：name 唯一性与内置名互斥是硬约束。
func validateProfileReq(req *domain.AgentProfileREQ) (*domain.AgentProfileDO, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !agentNamePattern.MatchString(name) {
		return nil, pkg.New(8203, "agent 名需为 kebab-case（小写字母开头，字母/数字/连字符，≤64 字符）", req.Name)
	}
	// 只保护内置名：自定义名之间的唯一性由 repo 的唯一约束负责。
	for _, builtin := range agent.BuiltinAgentNames() {
		if builtin == name {
			return nil, pkg.New(8202, "该名称为内置 Agent 保留名", name)
		}
	}
	if req.MaxTurns < 0 || req.MaxTurns > 40 {
		return nil, pkg.New(8203, "max_turns 取值范围 0-40（0 用委派默认）", "")
	}
	model := strings.TrimSpace(req.Model)
	thinking := strings.ToLower(strings.TrimSpace(req.Thinking))
	if thinking != "" {
		if _, ok := thinkingLevels[thinking]; !ok {
			return nil, pkg.New(8203, "推理强度取值 off / low / medium / high", req.Thinking)
		}
		// 不指定模型 = 完全继承主 Agent，此时单独改思考档位会让用户当次选择失效，直接拒绝比静默忽略好。
		if model == "" {
			return nil, pkg.New(8203, "指定推理强度前需先指定模型（留空表示继承主 Agent）", "")
		}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return &domain.AgentProfileDO{
		ID:           pkg.NewID(domain.IDAgentProfile),
		Name:         name,
		Description:  strings.TrimSpace(req.Description),
		SystemPrompt: req.SystemPrompt,
		ToolsAllow:   marshalGlobList(req.ToolsAllow),
		ToolsDeny:    marshalGlobList(req.ToolsDeny),
		MemoryEnable: req.MemoryEnable,
		MaxTurns:     req.MaxTurns,
		Model:        model,
		Thinking:     thinking,
		Enabled:      enabled,
	}, nil
}

// profileToDefinition 物化成 agent.Definition；空 JSON 数组视为不限制。
func profileToDefinition(row *domain.AgentProfileDO) agent.Definition {
	def := agent.Definition{
		Name:        row.Name,
		Description: row.Description,
		Persona:     row.SystemPrompt,
		Tools: agent.ToolPolicy{
			Allow: unmarshalGlobList(row.ToolsAllow),
			Deny:  unmarshalGlobList(row.ToolsDeny),
		},
		Memory:   agent.MemoryPolicy{Enabled: row.MemoryEnable},
		Model:    strings.TrimSpace(row.Model),
		Thinking: strings.TrimSpace(row.Thinking),
	}
	if row.MaxTurns > 0 {
		def.Budget.MaxTurns = row.MaxTurns
	}
	return def
}

// profileToRESP DO → 出参。
func profileToRESP(row *domain.AgentProfileDO) domain.AgentProfileRESP {
	return domain.AgentProfileRESP{
		ID:           row.ID,
		Name:         row.Name,
		Description:  row.Description,
		SystemPrompt: row.SystemPrompt,
		ToolsAllow:   unmarshalGlobList(row.ToolsAllow),
		ToolsDeny:    unmarshalGlobList(row.ToolsDeny),
		MemoryEnable: row.MemoryEnable,
		MaxTurns:     row.MaxTurns,
		Model:        row.Model,
		Thinking:     row.Thinking,
		Enabled:      row.Enabled,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

// marshalGlobList 序列化工具 glob 列表；空列表存空串（= 不限制）。
func marshalGlobList(list []string) string {
	clean := make([]string, 0, len(list))
	for _, v := range list {
		if v = strings.TrimSpace(v); v != "" {
			clean = append(clean, v)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	b, err := json.Marshal(clean)
	if err != nil {
		return ""
	}
	return string(b)
}

// unmarshalGlobList 反序列化工具 glob 列表；空串/坏数据返回 nil。
func unmarshalGlobList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
