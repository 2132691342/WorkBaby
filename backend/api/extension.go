// 插件 handler：列出扩展与包、启停、导入、删除、装包卸包。
// 装包走 PI 自己的 CLI，进度经 chat:setup 事件流式回传。
package api

import (
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// packageMu 保证同一时刻只有一次装包 / 卸包：两次 pi install 并发会写坏同一个包目录。
var packageMu sync.Mutex

// ListExtensions 扩展目录与已登记的包。
func (h *Handler) ListExtensions(c *gin.Context) {
	vo, err := h.Svc.Extensions.List()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// ToggleExtension 启停一条扩展（改 settings.json 的 extensions 并重启 PI）。
func (h *Handler) ToggleExtension(c *gin.Context) {
	var req domain.ToggleExtensionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Extensions.Toggle(req.Path, req.Enabled); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ImportExtensions 从磁盘导入扩展（.ts / .js 文件或目录）。
func (h *Handler) ImportExtensions(c *gin.Context) {
	var req domain.ImportExtensionsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	resp, err := h.Svc.Extensions.Import(req.Paths)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// DeleteExtension 删除一条扩展（文件或目录 + 登记项）。
func (h *Handler) DeleteExtension(c *gin.Context) {
	var req domain.DeleteExtensionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Extensions.Delete(req.Path); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// InstallPackage 装一个包：接口立刻返回，输出走 chat:setup。
func (h *Handler) InstallPackage(c *gin.Context) {
	h.runPackage(c, true)
}

// RemovePackage 卸一个包：同上。
func (h *Handler) RemovePackage(c *gin.Context) {
	h.runPackage(c, false)
}

// runPackage 是装 / 卸的共同实现：同一把锁 + 同一路事件。
func (h *Handler) runPackage(c *gin.Context, install bool) {
	var req domain.PackageREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if !packageMu.TryLock() {
		fail(c, pkg.New("9719", "正在装 / 卸插件，等这一趟结束", ""))
		return
	}
	phase := "package-remove"
	start := "正在卸载插件…"
	if install {
		phase = "package-install"
		start = "正在安装插件…"
	}
	go func() {
		defer packageMu.Unlock()
		h.emitSetup(phase, 0, start)
		run := h.Svc.Extensions.Remove
		if install {
			run = h.Svc.Extensions.Install
		}
		err := run(req.Source, func(line string) { h.emitSetup(phase, 0, line) })
		if err != nil {
			h.emitSetup(phase, 0, "失败："+err.Error())
			pkg.Errorf("extensions: 插件命令失败 source=%s: %v", req.Source, err)
			return
		}
		h.emitSetup(phase, 100, "插件已就绪")
	}()
	ok(c, true)
}

// ListCommands 取当前会话能跑的斜杠命令（扩展命令 / 提示模板 / 技能）。
func (h *Handler) ListCommands(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		// 没有会话就没有 PI 进程：宁可返回空列表，也不要凭空编一份命令表。
		ok(c, []domain.CommandVO{})
		return
	}
	list, err := h.Svc.Chat.Commands(sessionID)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// CancelRetry 叫停正在进行的自动重试。
func (h *Handler) CancelRetry(c *gin.Context) {
	var req domain.StopRunREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	if req.SessionID == "" {
		req.SessionID = c.Query("session_id")
	}
	if err := h.Svc.Chat.AbortRetry(req.SessionID); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ExportSession 把会话导成 HTML：path 由前端的保存对话框给出。
func (h *Handler) ExportSession(c *gin.Context) {
	var req domain.ExportSessionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if req.Path == "" {
		fail(c, pkg.New("1108", "没有给导出路径", ""))
		return
	}
	path, err := h.Svc.Chat.ExportHTML(c.Param("id"), req.Path)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.ExportSessionRESP{Path: path})
}
