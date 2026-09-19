package domain

import "WorkBaby/internal/pkg"

// PetConfigDO 桌宠配置（pet_configs 表；全局单行，UserID 主键）。
type PetConfigDO struct {
	UserID           string  `gorm:"primaryKey;size:64" json:"user_id"`
	Enabled          bool    `gorm:"default:false" json:"enabled"`
	Mode             string  `gorm:"size:32" json:"mode"`
	SpriteID         string  `gorm:"size:64" json:"sprite_id"`
	PositionX        int     `gorm:"default:0" json:"position_x"`
	PositionY        int     `gorm:"default:0" json:"position_y"`
	Scale            float64 `gorm:"default:1" json:"scale"`
	BubbleEnabled    bool    `gorm:"default:true" json:"bubble_enabled"`
	BubbleDurationMs int     `gorm:"default:3000" json:"bubble_duration_ms"`
	// 聊天背景：同一个 sprite 复用为聊天区背景（可开关 / 调不透明度与模糊），
	// 与桌宠共享形象来源，避免用户为两处各传一张图。
	ChatBackground    bool `gorm:"default:false" json:"chat_background"`
	BackgroundOpacity int  `gorm:"default:18" json:"background_opacity"` // 不透明度百分比（0~100）
	BackgroundBlurPx  int  `gorm:"default:0" json:"background_blur_px"`
	// ClickThrough 桌宠形态下透明区域点击穿透（Windows 原生窗口区域裁剪）。
	ClickThrough bool  `gorm:"default:false" json:"click_through"`
	CreatedAt    int64 `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt    int64 `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 固定表名。
func (PetConfigDO) TableName() string { return "pet_configs" }

// PetConfigREQ 配置写入请求（对齐前端 PetConfigReq）。
type PetConfigREQ struct {
	Enabled           *bool    `json:"enabled"`
	Mode              string   `json:"mode"`
	SpriteID          string   `json:"sprite_id"`
	PositionX         *int     `json:"position_x"`
	PositionY         *int     `json:"position_y"`
	Scale             *float64 `json:"scale"`
	BubbleEnabled     *bool    `json:"bubble_enabled"`
	BubbleDurationMs  *int     `json:"bubble_duration_ms"`
	ChatBackground    *bool    `json:"chat_background"`
	BackgroundOpacity *int     `json:"background_opacity"`
	BackgroundBlurPx  *int     `json:"background_blur_px"`
	ClickThrough      *bool    `json:"click_through"`
}

// PetConfigRESP 出参（对齐前端 PetConfig）。
type PetConfigRESP struct {
	UserID            string  `json:"user_id"`
	Enabled           bool    `json:"enabled"`
	Mode              string  `json:"mode"`
	SpriteID          string  `json:"sprite_id"`
	PositionX         int     `json:"position_x"`
	PositionY         int     `json:"position_y"`
	Scale             float64 `json:"scale"`
	BubbleEnabled     bool    `json:"bubble_enabled"`
	BubbleDurationMs  int     `json:"bubble_duration_ms"`
	ChatBackground    bool    `json:"chat_background"`
	BackgroundOpacity int     `json:"background_opacity"`
	BackgroundBlurPx  int     `json:"background_blur_px"`
	ClickThrough      bool    `json:"click_through"`
	UpdatedAt         int64   `json:"updated_at"`
}

// PetHitRect 桌宠命中矩形（窗口客户区物理像素；区域并集 = 可交互范围）。
type PetHitRect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// PetClickThroughREQ 设置桌宠命中区域：enabled=false 或不带矩形时恢复整窗可交互。
type PetClickThroughREQ struct {
	Enabled bool         `json:"enabled"`
	Rects   []PetHitRect `json:"rects"`
}

// PetSpriteDO 桌宠 sprite（pet_sprites 表；文件落 {home}/sprites/{id}.{ext}）。
type PetSpriteDO struct {
	ID         string `gorm:"primaryKey;size:64" json:"id"`
	Name       string `gorm:"size:128" json:"name"`
	FilePath   string `gorm:"size:512" json:"-"` // 磁盘绝对路径
	Format     string `gorm:"size:16" json:"format"`
	FrameCount int    `gorm:"default:1" json:"frame_count"`
	IsBuiltin  bool   `gorm:"default:false" json:"is_builtin"`
	CreatedAt  int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	DeletedAt  int64  `gorm:"default:0;index" json:"deleted_at"` // 软删
}

// TableName 固定表名。
func (PetSpriteDO) TableName() string { return "pet_sprites" }

// PetSpriteREQ 创建/更新请求（对齐前端 PetSpriteReq）。
type PetSpriteREQ struct {
	Name       string `json:"name"`
	FilePath   string `json:"file_path"`
	Format     string `json:"format"`
	FrameCount *int   `json:"frame_count"`
	IsBuiltin  bool   `json:"is_builtin"`
}

// PetSpriteRESP 出参（对齐前端 PetSprite；filePath 经 AssetServer /files/sprites/{id} 暴露）。
type PetSpriteRESP struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	FilePath   string `json:"file_path"`
	Format     string `json:"format"`
	FrameCount int    `json:"frame_count"`
	IsBuiltin  bool   `json:"is_builtin"`
	CreatedAt  int64  `json:"created_at"`
}

// 错误变量；段位 9400-9406。
var (
	ErrPetSpriteNotFound = pkg.New(9404, "pet sprite not found", "")
	ErrPetConfigNotFound = pkg.New(9404, "pet config not found", "")
)
