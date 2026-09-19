package server

// 知识库与记忆路由。

import (
	"net/http"

	"WorkBaby/internal/api"
	"github.com/gin-gonic/gin"

	"WorkBaby/internal/domain"
)

func registerKnowledgeRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// ---- knowledge (kdocs) ----
	v1.GET("/kdocs", func(c *gin.Context) {
		v, err := h.ListKnowledgeDocs()
		unwrap(c, v, err)
	})
	v1.POST("/kdocs", func(c *gin.Context) {
		var req domain.KnowledgeDocREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.AddKnowledgeDoc(req)
		unwrap(c, v, err)
	})
	// 本地文件导入受管知识库：前端原生对话框选路径 → 后端复制到 {home}/knowledge 并后台索引
	v1.POST("/kdocs/import-file", func(c *gin.Context) {
		var req struct {
			Name       string  `json:"name"`
			SourcePath string  `json:"source_path"`
			FolderID   *string `json:"folder_id"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		folderID := ""
		if req.FolderID != nil {
			folderID = *req.FolderID
		}
		v, err := h.ImportKnowledgeFile(req.Name, req.SourcePath, folderID)
		unwrap(c, v, err)
	})
	v1.GET("/kdocs/search", func(c *gin.Context) {
		topK := atoi(c.DefaultQuery("limit", "20"), 20)
		v, err := h.SearchKnowledge(c.Query("q"), topK)
		unwrap(c, v, err)
	})
	v1.GET("/kdocs/groups", func(c *gin.Context) {
		v, err := h.ListKnowledgeGroups()
		unwrap(c, v, err)
	})
	v1.GET("/kdocs/group/:group", func(c *gin.Context) {
		v, err := h.ListKnowledgeByGroup(c.Param("group"))
		unwrap(c, v, err)
	})
	v1.POST("/kdocs/:id/update", func(c *gin.Context) {
		var req domain.KnowledgeDocREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpdateKnowledgeDoc(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.GET("/kdocs/:id", func(c *gin.Context) {
		v, err := h.GetKnowledgeDoc(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.POST("/kdocs/:id/delete", func(c *gin.Context) { Fail(c, h.DeleteKnowledgeDoc(c.Param("id"))) })
	v1.POST("/kdocs/:id/reindex", func(c *gin.Context) { Fail(c, h.ReindexKnowledgeDoc(c.Param("id"))) })

	// 受管文件下载：前端预览 PDF/Word/Excel 用；text/url 类型由前端直接渲染 source 文本。
	v1.GET("/kdocs/:id/file", func(c *gin.Context) {
		data, mime, err := h.GetKnowledgeFile(c.Param("id"))
		if err != nil {
			Fail(c, err)
			return
		}
		c.Data(http.StatusOK, mime, data)
	})

	// ---- memory（一份 MEMORY.md：概览 / 列表 / 检索 / 读写） ----
	v1.GET("/memory", func(c *gin.Context) {
		v, err := h.GetMemoryOverview()
		unwrap(c, v, err)
	})
	v1.GET("/memory/list", func(c *gin.Context) {
		v, err := h.ListMemory(c.Query("section"))
		unwrap(c, v, err)
	})
	v1.GET("/memory/search", func(c *gin.Context) {
		topK := atoi(c.DefaultQuery("k", "20"), 20)
		v, err := h.SearchMemory(c.Query("q"), topK)
		unwrap(c, v, err)
	})
	v1.GET("/memory/text", func(c *gin.Context) {
		v, err := h.GetMemoryText()
		unwrap(c, v, err)
	})
	v1.POST("/memory/append", func(c *gin.Context) {
		var req domain.MemoryWriteREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.AppendMemory(req))
	})
	v1.POST("/memory/delete", func(c *gin.Context) {
		var req domain.MemoryDeleteREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.DeleteMemory(req))
	})
	v1.POST("/memory/replace", func(c *gin.Context) {
		var req domain.MemoryReplaceREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.ReplaceMemory(req))
	})

}
