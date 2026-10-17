// 知识库 handler：文档增删、重建索引与检索。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"github.com/gin-gonic/gin"
)

// ListDocs 知识库文档列表。
func (h *Handler) ListDocs(c *gin.Context) {
	list, err := h.Svc.Knowledge.List()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// AddDocs 添加文档。
func (h *Handler) AddDocs(c *gin.Context) {
	var req domain.AddDocsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	added, err := h.Svc.Knowledge.Add(req.Paths)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.AddDocsRESP{Added: added})
}

// DeleteDoc 删除文档。
func (h *Handler) DeleteDoc(c *gin.Context) {
	if err := h.Svc.Knowledge.Delete(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// StartReindex 启动后台重建：大库重建会把同步请求占上一分钟，必须走任务 + 进度 + 可取消。
func (h *Handler) StartReindex(c *gin.Context) {
	started, err := h.Svc.Knowledge.StartReindex()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.StartReindexRESP{Started: started})
}

// ReindexStatus 取重建进度。
func (h *Handler) ReindexStatus(c *gin.Context) {
	ok(c, h.Svc.Knowledge.ReindexStatus())
}

// CancelReindex 中断重建。
func (h *Handler) CancelReindex(c *gin.Context) {
	ok(c, h.Svc.Knowledge.CancelReindex())
}

// SearchKnowledge 检索知识库。
func (h *Handler) SearchKnowledge(c *gin.Context) {
	var req domain.SearchREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	hits, err := h.Svc.Knowledge.Search(c.Request.Context(), req.Query, req.Limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, domain.SearchRESP{Hits: hits})
}
