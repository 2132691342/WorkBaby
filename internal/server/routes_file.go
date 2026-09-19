package server

// 文件夹与受管文件路由。

import (
	"WorkBaby/internal/api"
	"github.com/gin-gonic/gin"

	"WorkBaby/internal/domain"
)

func registerFileRoutes(v1 *gin.RouterGroup, h *api.Handler) {
	// ---- folders / files ----
	v1.GET("/folders/tree", func(c *gin.Context) {
		v, err := h.GetFolderTree(c.Query("workspaceID"))
		unwrap(c, v, err)
	})
	v1.GET("/folders", func(c *gin.Context) {
		v, err := h.ListFolders(c.Query("parentID"))
		unwrap(c, v, err)
	})
	v1.POST("/folders", func(c *gin.Context) {
		var req domain.FolderREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.CreateFolder(req)
		unwrap(c, v, err)
	})
	v1.POST("/folders/:id/update", func(c *gin.Context) {
		var req domain.FolderREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UpdateFolder(c.Param("id"), req)
		unwrap(c, v, err)
	})
	v1.POST("/folders/:id/delete", func(c *gin.Context) {
		v, err := h.DeleteFolder(c.Param("id"))
		unwrap(c, v, err)
	})

	v1.GET("/files/search", func(c *gin.Context) {
		v, err := h.SearchFiles(c.Query("q"))
		unwrap(c, v, err)
	})
	v1.GET("/files", func(c *gin.Context) {
		v, err := h.ListFiles()
		unwrap(c, v, err)
	})
	v1.POST("/files/upload", func(c *gin.Context) {
		var req struct {
			Name       string `json:"name"`
			SourcePath string `json:"source_path"`
			SessionID  string `json:"session_id"`
			FolderID   string `json:"folder_id"`
		}
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UploadFile(req.Name, req.SourcePath, req.SessionID, req.FolderID)
		unwrap(c, v, err)
	})
	v1.POST("/files/upload-data", func(c *gin.Context) {
		var req domain.UploadDataREQ
		if err := BindJSON(c, &req); err != nil {
			Fail(c, err)
			return
		}
		v, err := h.UploadFileData(req)
		unwrap(c, v, err)
	})
	v1.POST("/files/:id/delete", func(c *gin.Context) {
		v, err := h.DeleteFile(c.Param("id"))
		unwrap(c, v, err)
	})
	v1.GET("/files/:id/preview-url", func(c *gin.Context) {
		v, err := h.GetFilePreviewURL(c.Param("id"))
		unwrap(c, v, err)
	})
}
