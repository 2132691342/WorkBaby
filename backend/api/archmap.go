// 项目架构图 HTTP handler：薄层。
package api

import (
	"WorkBaby/backend/domain"

	"github.com/gin-gonic/gin"
)

// AnalyzeArch 分析项目结构，返回分层依赖图。
func (h *Handler) AnalyzeArch(c *gin.Context) {
	var req domain.AnalyzeArchREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	g, err := h.Svc.ArchMap.Analyze(req.Path)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, g)
}

// ArchEntrypoints 入口文件清单。
func (h *Handler) ArchEntrypoints(c *gin.Context) {
	list, err := h.Svc.ArchMap.Entrypoints(c.Query("path"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}
