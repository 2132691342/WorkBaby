package service

import (
	"context"
	"regexp"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// UserCommandService 自定义斜杠命令：保存的提示词模板 CRUD。
//
// 命令以 name 为键（/面板中的命令名），prompt 是灌入输入框的模板正文；
// 模板里的 `$ARGUMENTS` / `$1..$9` 由前端在插入时用命令后的参数展开。
// 与 {home}/commands/*.md 定义文件并列：同名时文件优先（见 command_file.go）。
type UserCommandService struct {
	repo *repo.UserCommandRepo
}

// NewUserCommandService 构造服务。
func NewUserCommandService(repo *repo.UserCommandRepo) *UserCommandService {
	return &UserCommandService{repo: repo}
}

// commandNamePattern kebab-case 命令名（与 / 面板输入习惯一致）。
var commandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// List 全量自定义命令。
func (s *UserCommandService) List(ctx context.Context) ([]domain.UserCommandRESP, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.UserCommandRESP, 0, len(rows))
	for i := range rows {
		out = append(out, toUserCommandRESP(&rows[i]))
	}
	return out, nil
}

// Upsert 创建/更新（按 name）；写库即生效（/ 面板每次拉取最新）。
func (s *UserCommandService) Upsert(ctx context.Context, req *domain.UserCommandREQ) (*domain.UserCommandRESP, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !commandNamePattern.MatchString(name) {
		return nil, pkg.New(8302, "命令名需为 kebab-case（小写字母开头，字母/数字/连字符，≤64 字符）", req.Name)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, pkg.New(8302, "命令提示词不能为空", name)
	}
	row := &domain.UserCommandDO{
		ID:          pkg.NewID(domain.IDUserCommand),
		Name:        name,
		Prompt:      req.Prompt,
		Description: strings.TrimSpace(req.Description),
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}
	out := toUserCommandRESP(row)
	return &out, nil
}

// Delete 删除自定义命令。
func (s *UserCommandService) Delete(ctx context.Context, name string) error {
	return s.repo.Delete(ctx, name)
}

func toUserCommandRESP(row *domain.UserCommandDO) domain.UserCommandRESP {
	return domain.UserCommandRESP{
		ID:          row.ID,
		Name:        row.Name,
		Prompt:      row.Prompt,
		Description: row.Description,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
