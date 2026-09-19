package server

// 工作域路由：目录信任 / 工作区文件 / 文件变更 / 产物 / 后台任务。

import (
	"WorkBaby/internal/api"
	"github.com/gin-gonic/gin"

	"WorkBaby/internal/domain"
)

func registerWorkspaceRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// 工作目录信任（工作目录信任三态）
	v1.GET("/trust", func(c *gin.Context) {
		v, err := h.ListTrust()
		unwrap(c, v, err)
	})
	v1.GET("/trust/resolve", func(c *gin.Context) {
		v, err := h.ResolveTrust(c.Query("path"))
		unwrap(c, v, err)
	})
	v1.POST("/trust", func(c *gin.Context) {
		var req domain.WorkspaceTrustREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.DecideTrust(req)
		unwrap(c, v, err)
	})
	v1.POST("/trust/revoke", func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		Fail(c, h.RevokeTrust(req.Path))
	})
	v1.GET("/trust/roots", func(c *gin.Context) {
		OK(c, h.TrustRoots())
	})

	// ---- workspace files ----
	v1.GET("/chat/workspace/:id/files", func(c *gin.Context) {
		v, err := h.ListWorkspaceFiles(c.Param("id"))
		unwrap(c, v, err)
	})
	// 懒加载列工作区单层目录（真实磁盘内容；path 相对工作区根）
	v1.GET("/chat/workspace/:id/ls", func(c *gin.Context) {
		v, err := h.ListWorkspaceDir(c.Param("id"), c.Query("path"))
		unwrap(c, v, err)
	})
	v1.GET("/chat/workspace/:id/file", func(c *gin.Context) {
		v, err := h.ReadWorkspaceFile(c.Param("id"), c.Query("path"))
		unwrap(c, v, err)
	})
	// 工作区文件管理：重命名 / 副本 / 删除（沙箱校验；面板右键菜单消费）
	v1.POST("/chat/workspace/:id/rename", func(c *gin.Context) {
		var req struct {
			Path    string `json:"path"`
			NewName string `json:"new_name"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.RenameWorkspaceEntry(c.Param("id"), req.Path, req.NewName)
		unwrap(c, v, err)
	})
	v1.POST("/chat/workspace/:id/copy", func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.CopyWorkspaceEntry(c.Param("id"), req.Path)
		unwrap(c, v, err)
	})
	v1.POST("/chat/workspace/:id/remove", func(c *gin.Context) {
		var req struct {
			Path string `json:"path"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.DeleteWorkspaceEntry(c.Param("id"), req.Path)
		unwrap(c, v, err)
	})

	// ---- file changes ----
	v1.GET("/chat/sessions/:id/changes", func(c *gin.Context) {
		limit := atoi(c.DefaultQuery("limit", "100"), 100)
		v, err := h.ListSessionChanges(c.Param("id"), limit)
		unwrap(c, v, err)
	})
	v1.GET("/chat/changes/:cid", func(c *gin.Context) {
		v, err := h.GetSessionChange(c.Param("cid"))
		unwrap(c, v, err)
	})
	v1.POST("/chat/changes/:cid/rollback", func(c *gin.Context) {
		Fail(c, h.RollbackSessionChange(c.Param("cid")))
	})

	// ---- artifacts ----
	v1.GET("/chat/sessions/:id/artifacts", func(c *gin.Context) {
		limit := atoi(c.DefaultQuery("limit", "100"), 100)
		v, err := h.ListSessionArtifacts(c.Param("id"), limit)
		unwrap(c, v, err)
	})
	v1.POST("/chat/artifacts/:aid/delete", func(c *gin.Context) {
		Fail(c, h.DeleteSessionArtifact(c.Param("aid")))
	})

}
