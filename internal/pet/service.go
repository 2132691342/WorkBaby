// Package pet 提供桌宠能力：配置单行 CRUD + sprite 管理（magic 校验）+ 状态机。
// 不依赖 Wails runtime；窗口切换由 api 层实现（CLAUDE §2.2 wails 依赖隔离）。
package pet

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// MaxSpriteBytes sprite 上传大小上限（5MB）。
const MaxSpriteBytes = 5 << 20

// BuiltinSpriteID 内置吉祥物 sprite 的固定 ID（播种幂等键）。
const BuiltinSpriteID = "SPRITE_builtin_workbaby"

//go:embed assets/workbaby.png
var builtinSpritePNG []byte

// Service 桌宠编排：配置 upsert + sprite 资产。
type Service struct {
	cfgRepo    *repo.PetConfigRepo
	spriteRepo *repo.PetSpriteRepo
	spriteDir  string // {home}/sprites
}

// NewService 构造；spriteDir 为 sprite 文件根目录。
func NewService(cfgRepo *repo.PetConfigRepo, spriteRepo *repo.PetSpriteRepo, spriteDir string) *Service {
	return &Service{cfgRepo: cfgRepo, spriteRepo: spriteRepo, spriteDir: spriteDir}
}

// GetConfig 读取配置；无记录返回默认值（不落库）。
func (s *Service) GetConfig(ctx context.Context) (domain.PetConfigRESP, error) {
	row, err := s.cfgRepo.Get(ctx, domain.LocalUserID)
	if err != nil {
		if errors.Is(err, domain.ErrPetConfigNotFound) {
			return domain.PetConfigRESP{UserID: domain.LocalUserID, Mode: "swing", Scale: 1, BubbleEnabled: true, BubbleDurationMs: 3000}, nil
		}
		return domain.PetConfigRESP{}, err
	}
	return toConfigRESP(row), nil
}

// UpdateConfig 更新配置（upsert 单行幂等）。
func (s *Service) UpdateConfig(ctx context.Context, req domain.PetConfigREQ) (domain.PetConfigRESP, error) {
	row, err := s.cfgRepo.Get(ctx, domain.LocalUserID)
	if err != nil && !errors.Is(err, domain.ErrPetConfigNotFound) {
		return domain.PetConfigRESP{}, err
	}
	if row == nil {
		row = &domain.PetConfigDO{UserID: domain.LocalUserID, Mode: "swing", Scale: 1, BubbleEnabled: true, BubbleDurationMs: 3000}
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	if req.Mode != "" {
		row.Mode = req.Mode
	}
	if req.SpriteID != "" {
		row.SpriteID = req.SpriteID
	}
	if req.PositionX != nil {
		row.PositionX = *req.PositionX
	}
	if req.PositionY != nil {
		row.PositionY = *req.PositionY
	}
	if req.Scale != nil {
		row.Scale = *req.Scale
	}
	if req.BubbleEnabled != nil {
		row.BubbleEnabled = *req.BubbleEnabled
	}
	if req.BubbleDurationMs != nil {
		row.BubbleDurationMs = *req.BubbleDurationMs
	}
	if req.ChatBackground != nil {
		row.ChatBackground = *req.ChatBackground
	}
	if req.BackgroundOpacity != nil {
		row.BackgroundOpacity = *req.BackgroundOpacity
	}
	if req.BackgroundBlurPx != nil {
		row.BackgroundBlurPx = *req.BackgroundBlurPx
	}
	if req.ClickThrough != nil {
		row.ClickThrough = *req.ClickThrough
	}
	if err := s.cfgRepo.Upsert(ctx, row); err != nil {
		return domain.PetConfigRESP{}, err
	}
	return toConfigRESP(row), nil
}

// ListSprites 全部 sprite。
func (s *Service) ListSprites(ctx context.Context) ([]domain.PetSpriteRESP, error) {
	rows, err := s.spriteRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.PetSpriteRESP, 0, len(rows))
	for i := range rows {
		out = append(out, toSpriteRESP(&rows[i]))
	}
	return out, nil
}

// CreateSprite 注册内置/占位 sprite（不落文件）。
func (s *Service) CreateSprite(ctx context.Context, req domain.PetSpriteREQ) (domain.PetSpriteRESP, error) {
	if req.Name == "" {
		return domain.PetSpriteRESP{}, pkg.New(9400, "sprite name required", "")
	}
	row := &domain.PetSpriteDO{
		Name:       req.Name,
		FilePath:   req.FilePath,
		Format:     req.Format,
		FrameCount: 1,
		IsBuiltin:  req.IsBuiltin,
	}
	if req.FrameCount != nil {
		row.FrameCount = *req.FrameCount
	}
	if err := s.spriteRepo.Create(ctx, row); err != nil {
		return domain.PetSpriteRESP{}, err
	}
	return toSpriteRESP(row), nil
}

// DeleteSprite 删除 sprite；内置不可删，非内置级联删文件。
func (s *Service) DeleteSprite(ctx context.Context, id string) error {
	row, err := s.spriteRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if row.IsBuiltin {
		return pkg.New(9400, "builtin sprite cannot be deleted", id)
	}
	if row.FilePath != "" {
		_ = os.Remove(row.FilePath)
	}
	return s.spriteRepo.Delete(ctx, id)
}

// UploadSprite 从本地路径读取文件 → magic 校验 → 落盘 {home}/sprites/{id}.{ext} → 落库。
func (s *Service) UploadSprite(ctx context.Context, name, srcPath string) (domain.PetSpriteRESP, error) {
	if srcPath == "" {
		return domain.PetSpriteRESP{}, pkg.New(9400, "sprite source path required", "")
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return domain.PetSpriteRESP{}, pkg.Wrap(9406, "read sprite file failed", err)
	}
	return s.SaveSpriteBytes(ctx, name, data)
}

// SaveSpriteBytes 落盘前端编辑（裁剪 / 旋转 / 抠图）后的图片字节并注册为 sprite。
func (s *Service) SaveSpriteBytes(ctx context.Context, name string, data []byte) (domain.PetSpriteRESP, error) {
	if name == "" {
		name = "sprite"
	}
	if len(data) > MaxSpriteBytes {
		return domain.PetSpriteRESP{}, pkg.New(9402, "sprite file exceeds 5MB", "")
	}
	ext := DetectExt(data)
	if ext == "" {
		return domain.PetSpriteRESP{}, pkg.New(9403, "unsupported image format (png/webp/gif/jpg)", "")
	}
	if err := os.MkdirAll(s.spriteDir, 0o755); err != nil {
		return domain.PetSpriteRESP{}, pkg.Wrap(9406, "mkdir sprites dir failed", err)
	}
	id := pkg.NewID(domain.IDSprite)
	p := filepath.Join(s.spriteDir, id+ext)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return domain.PetSpriteRESP{}, pkg.Wrap(9406, "write sprite file failed", err)
	}
	row := &domain.PetSpriteDO{
		ID:         id,
		Name:       name,
		FilePath:   p,
		Format:     ext[1:],
		FrameCount: 1,
		IsBuiltin:  false,
	}
	if err := s.spriteRepo.Create(ctx, row); err != nil {
		return domain.PetSpriteRESP{}, err
	}
	return toSpriteRESP(row), nil
}

// FindSprite 按 ID 查 sprite（AssetServer /files/sprites/{id} 用）。
func (s *Service) FindSprite(ctx context.Context, id string) (*domain.PetSpriteDO, error) {
	return s.spriteRepo.GetByID(ctx, id)
}

// SeedBuiltin 首启播种内置吉祥物 sprite（幂等）：落盘 + 注册 + 配置空时默认选中。
// 让聊天头像 / 桌宠开箱即有形象，无需用户手动上传。失败仅告警不阻断启动。
func (s *Service) SeedBuiltin(ctx context.Context) {
	if _, err := s.spriteRepo.GetByID(ctx, BuiltinSpriteID); err != nil {
		if err := os.MkdirAll(s.spriteDir, 0o755); err != nil {
			pkg.L.Warn("seed builtin sprite mkdir failed", "err", err.Error())
			return
		}
		p := filepath.Join(s.spriteDir, BuiltinSpriteID+".png")
		if err := os.WriteFile(p, builtinSpritePNG, 0o644); err != nil {
			pkg.L.Warn("seed builtin sprite write failed", "err", err.Error())
			return
		}
		row := &domain.PetSpriteDO{
			ID: BuiltinSpriteID, Name: "WorkBaby", FilePath: p,
			Format: "png", FrameCount: 1, IsBuiltin: true,
		}
		if err := s.spriteRepo.Create(ctx, row); err != nil {
			pkg.L.Warn("seed builtin sprite register failed", "err", err.Error())
			return
		}
	}
	s.ensureDefaultSprite(ctx)
}

// ensureDefaultSprite 配置未选形象时默认选中内置 sprite（已有选择不覆盖）。
func (s *Service) ensureDefaultSprite(ctx context.Context) {
	row, err := s.cfgRepo.Get(ctx, domain.LocalUserID)
	if err != nil && !errors.Is(err, domain.ErrPetConfigNotFound) {
		return
	}
	if row == nil {
		row = &domain.PetConfigDO{UserID: domain.LocalUserID, Mode: "swing", Scale: 1, BubbleEnabled: true, BubbleDurationMs: 3000}
	}
	if row.SpriteID != "" {
		return
	}
	row.SpriteID = BuiltinSpriteID
	if err := s.cfgRepo.Upsert(ctx, row); err != nil {
		pkg.L.Warn("seed default sprite config failed", "err", err.Error())
	}
}

func toConfigRESP(d *domain.PetConfigDO) domain.PetConfigRESP {
	return domain.PetConfigRESP{
		UserID: d.UserID, Enabled: d.Enabled, Mode: d.Mode, SpriteID: d.SpriteID,
		PositionX: d.PositionX, PositionY: d.PositionY, Scale: d.Scale,
		BubbleEnabled: d.BubbleEnabled, BubbleDurationMs: d.BubbleDurationMs,
		ChatBackground: d.ChatBackground, BackgroundOpacity: d.BackgroundOpacity,
		BackgroundBlurPx: d.BackgroundBlurPx, ClickThrough: d.ClickThrough, UpdatedAt: d.UpdatedAt,
	}
}

// toSpriteRESP 出参；filePath 经 AssetServer /files/sprites/{id} 暴露。
func toSpriteRESP(d *domain.PetSpriteDO) domain.PetSpriteRESP {
	return domain.PetSpriteRESP{
		ID: d.ID, Name: d.Name, FilePath: "/files/sprites/" + d.ID,
		Format: d.Format, FrameCount: d.FrameCount, IsBuiltin: d.IsBuiltin, CreatedAt: d.CreatedAt,
	}
}
