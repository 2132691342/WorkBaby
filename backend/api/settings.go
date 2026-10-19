// 设置与统计 handler：全量 KV、背景图、单项设置、PI 配置快照、工作目录候选与用量统计。
package api

import (
	"strconv"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// AllSettings 全部设置。
func (h *Handler) AllSettings(c *gin.Context) {
	all, err := h.Svc.Settings.All()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, all)
}

// SetSetting 写入设置。
func (h *Handler) SetSetting(c *gin.Context) {
	var req domain.SetSettingREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Settings.Set(req.Key, req.Value); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// GetPISettings PI 侧配置快照与全部文件位置。
func (h *Handler) GetPISettings(c *gin.Context) {
	vo, err := h.Svc.Settings.PISettings()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// SetPIThinking 写 PI 的默认思考档。
func (h *Handler) SetPIThinking(c *gin.Context) {
	var req domain.SetThinkingREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Settings.SetThinkingLevel(req.Level); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetPICompaction 写 PI 的压缩参数。
func (h *Handler) SetPICompaction(c *gin.Context) {
	var req domain.SetCompactionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Settings.SetCompaction(req); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ListWorkspaces 最近用过的工作目录。
func (h *Handler) ListWorkspaces(c *gin.Context) {
	ok(c, domain.WorkspacesRESP{
		Workspaces: h.Svc.Settings.WorkspaceCandidates(),
		Current:    h.Svc.Env.Workspace(),
	})
}

// GetBackground 取当前自定义背景。
func (h *Handler) GetBackground(c *gin.Context) {
	bg, err := h.Svc.Settings.Background()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, bg)
}

// SetBackground 设置自定义背景；空路径表示清除。
func (h *Handler) SetBackground(c *gin.Context) {
	var req domain.BackgroundREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	bg, err := h.Svc.Settings.SetBackground(req.Path, req.Opacity, req.Blur)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, bg)
}

// BackgroundAsset 吐出背景图本体。前端不能直接读数据目录里的文件，
// 只能拿到一个 URL——这也是背景图必须复制到数据目录的原因之一。
func (h *Handler) BackgroundAsset(c *gin.Context) {
	bg, err := h.Svc.Settings.Background()
	if err != nil {
		fail(c, err)
		return
	}
	if bg.Image == "" || !pkg.FileExists(bg.Image) {
		fail(c, pkg.New("1005", "还没有设置背景图", ""))
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.File(bg.Image)
}

// Stats 仪表盘统计：token 用量按天 / 按小时 / 按模型 / 按会话。
// span 取 24h / today / 7d / 14d / 30d；旧的 days 数字参数兜底换算。
func (h *Handler) Stats(c *gin.Context) {
	span := c.Query("span")
	if span == "" {
		if days := queryInt(c, "days"); days > 0 {
			span = strconv.Itoa(days) + "d"
		}
	}
	resp, err := h.Svc.Stats.Compute(span)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}
