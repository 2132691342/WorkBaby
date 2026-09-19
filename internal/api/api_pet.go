package api

import (
	"encoding/base64"
	"os"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pet"
	"WorkBaby/internal/pkg"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// 桌宠窗口尺寸（需容纳 pet-card ~216px + 悬浮余量）。
const (
	petWidth  = 260
	petHeight = 300
)

// 主窗口最小尺寸（与 main.go options.MinWidth/MinHeight 保持一致，退出桌宠态时还原）。
const (
	mainMinWidth  = 960
	mainMinHeight = 640
)

// GetPetConfig 桌宠配置。
func (h *Handler) GetPetConfig() (domain.PetConfigRESP, error) {
	return h.petSvc.GetConfig(h.ctx)
}

// UpdatePetConfig 保存桌宠配置（POST /pet/config/update）。
func (h *Handler) UpdatePetConfig(req domain.PetConfigREQ) (domain.PetConfigRESP, error) {
	return h.petSvc.UpdateConfig(h.ctx, req)
}

// ListPetSprites 桌宠 sprite 列表。
func (h *Handler) ListPetSprites() ([]domain.PetSpriteRESP, error) {
	return h.petSvc.ListSprites(h.ctx)
}

// CreatePetSprite 注册内置/占位 sprite。
func (h *Handler) CreatePetSprite(req domain.PetSpriteREQ) (domain.PetSpriteRESP, error) {
	return h.petSvc.CreateSprite(h.ctx, req)
}

// DeletePetSprite 删除 sprite（内置拒绝）。
func (h *Handler) DeletePetSprite(id string) (map[string]any, error) {
	if err := h.petSvc.DeleteSprite(h.ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "deleted": true}, nil
}

// UploadPetSprite 上传本地图片文件为 sprite（name + 本地路径，前端经文件对话框选路径）。
func (h *Handler) UploadPetSprite(name string, srcPath string) (domain.PetSpriteRESP, error) {
	return h.petSvc.UploadSprite(h.ctx, name, srcPath)
}

// ReadLocalImage 把本地图片读成 data URL，供前端在画布里裁剪 / 旋转 / 抠图。
// WebView2 不允许前端直接读 file:// 路径，这一步只能由后端代读。
func (h *Handler) ReadLocalImage(path string) (string, error) {
	if path == "" {
		return "", pkg.New(9400, "image path required", "")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", pkg.Wrap(9406, "read image file failed", err)
	}
	if len(data) > pet.MaxSpriteBytes {
		return "", pkg.New(9402, "image file exceeds 5MB", "")
	}
	mime := "image/png"
	switch pet.DetectExt(data) {
	case "":
		return "", pkg.New(9403, "unsupported image format (png/webp/gif/jpg)", "")
	case ".jpg":
		mime = "image/jpeg"
	case ".gif":
		mime = "image/gif"
	case ".webp":
		mime = "image/webp"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// SavePetSpriteImage 保存前端编辑后的图片（data URL）为 sprite。
func (h *Handler) SavePetSpriteImage(name string, dataURL string) (domain.PetSpriteRESP, error) {
	idx := strings.Index(dataURL, ",")
	if !strings.HasPrefix(dataURL, "data:") || idx < 0 {
		return domain.PetSpriteRESP{}, pkg.New(9403, "invalid image data url", "")
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[idx+1:])
	if err != nil {
		return domain.PetSpriteRESP{}, pkg.Wrap(9403, "decode image data failed", err)
	}
	return h.petSvc.SaveSpriteBytes(h.ctx, name, raw)
}

// GetPetState 当前桌宠状态。
func (h *Handler) GetPetState() (string, error) {
	if h.petCtrl == nil {
		return "idle", nil
	}
	return string(h.petCtrl.Current()), nil
}

// PetMode 返回当前是否桌宠形态（server/router 用）。
func (h *Handler) PetMode() bool { return h.petMode }

// PetToggleMode 主窗口 ⇄ 桌宠形态切换（位置记忆）。
func (h *Handler) PetToggleMode() (map[string]any, error) {
	if h.ctx == nil {
		return nil, pkg.New(9405, "window not ready", "")
	}
	if h.petMode {
		// 退出桌宠态：还原尺寸/位置/最小尺寸/置顶 + 清空命中区域，恢复普通窗口行为
		h.petX, h.petY = wruntime.WindowGetPosition(h.ctx)
		_ = pet.ClearHitRegion()
		wruntime.WindowSetSize(h.ctx, h.mainW, h.mainH)
		wruntime.WindowSetPosition(h.ctx, h.mainX, h.mainY)
		wruntime.WindowSetMinSize(h.ctx, mainMinWidth, mainMinHeight)
		wruntime.WindowSetAlwaysOnTop(h.ctx, false)
		h.petMode = false
		wruntime.EventsEmit(h.ctx, "pet:hide", nil)
	} else {
		h.mainX, h.mainY = wruntime.WindowGetPosition(h.ctx)
		h.mainW, h.mainH = wruntime.WindowGetSize(h.ctx)
		wruntime.WindowSetSize(h.ctx, petWidth, petHeight)
		wruntime.WindowSetPosition(h.ctx, h.petX, h.petY)
		wruntime.WindowSetMinSize(h.ctx, petWidth, petHeight)
		wruntime.WindowSetAlwaysOnTop(h.ctx, true)
		h.petMode = true
		wruntime.EventsEmit(h.ctx, "pet:show", nil)
	}
	return map[string]any{"mode": petModeName(h.petMode)}, nil
}

// SetPetClickThrough 设置桌宠命中区域：区域外像素不接收点击，交给下层窗口。
// 前端按可见元素包围盒上报物理像素矩形；enabled=false 或空列表恢复整窗可交互。
func (h *Handler) SetPetClickThrough(req domain.PetClickThroughREQ) (map[string]any, error) {
	if !req.Enabled || len(req.Rects) == 0 {
		_ = pet.ClearHitRegion()
		return map[string]any{"enabled": false}, nil
	}
	if err := pet.ApplyHitRegion(req.Rects); err != nil {
		return nil, pkg.Wrap(9407, "apply pet hit region failed", err)
	}
	return map[string]any{"enabled": true, "rects": len(req.Rects)}, nil
}

// PetMove 桌宠拖拽：相对位移。
func (h *Handler) PetMove(dx, dy int) (map[string]any, error) {
	if h.ctx == nil {
		return nil, pkg.New(9405, "window not ready", "")
	}
	x, y := wruntime.WindowGetPosition(h.ctx)
	nx, ny := x+dx, y+dy
	wruntime.WindowSetPosition(h.ctx, nx, ny)
	if h.petMode {
		h.petX, h.petY = nx, ny
	}
	return map[string]any{"x": nx, "y": ny}, nil
}

func petModeName(petMode bool) string {
	if petMode {
		return "pet"
	}
	return "main"
}
