// 引擎 handler：PI 与本机运行时的状态、联网版本对账、升级。
// 升级与安装一样是后台动作，进度走 chat:setup 事件流；接口只负责发起。
package api

import (
	"WorkBaby/backend/domain"

	"github.com/gin-gonic/gin"
)

// EngineStatus 返回引擎的本地快照（不联网）。
func (h *Handler) EngineStatus(c *gin.Context) {
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	ok(c, h.Svc.Engine.Status())
}

// EngineLatest 联网查最新版本：PI 与 npm 来源的插件包。
func (h *Handler) EngineLatest(c *gin.Context) {
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	ok(c, h.Svc.Engine.Latest())
}

// EngineUpdate 升级 PI 自身。
func (h *Handler) EngineUpdate(c *gin.Context) {
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	go func() {
		h.emitSetup("update", 0, "正在升级助手引擎…")
		if err := h.Svc.Engine.Update(func(line string) { h.emitSetup("update", 0, line) }); err != nil {
			h.emitSetup("update", 0, "升级失败："+err.Error())
			return
		}
		h.emitSetup("update", 100, "助手引擎已是最新版本")
	}()
	ok(c, true)
}

// EngineUpdatePackages 更新插件包：source 为空表示全部。
func (h *Handler) EngineUpdatePackages(c *gin.Context) {
	if h.Svc == nil || h.Svc.Engine == nil {
		fail(c, domain.ErrEngineMissing)
		return
	}
	var req domain.PackageREQ
	_ = bindOptionalBody(c, &req)
	go func() {
		h.emitSetup("packages", 0, "正在更新插件包…")
		err := h.Svc.Engine.UpdatePackages(req.Source, func(line string) {
			h.emitSetup("packages", 0, line)
		})
		if err != nil {
			h.emitSetup("packages", 0, "更新失败："+err.Error())
			return
		}
		h.emitSetup("packages", 100, "插件包已更新")
	}()
	ok(c, true)
}
