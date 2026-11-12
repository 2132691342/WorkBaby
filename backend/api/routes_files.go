package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 工作区文件的手工操作路由。与 git 分开成两个文件：
// 那一组关心的是「哪些改动要提交」，这一组关心的是「这个目录里有什么」。

// filesList 列目录。
func (s *Server) filesList(c *gin.Context) {
	out, err := s.svc.Files.List(strings.TrimSpace(c.Query("path")))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// filesSearch 按名搜索工作区。
func (s *Server) filesSearch(c *gin.Context) {
	out, err := s.svc.Files.Search(c.Query("query"), queryInt(c, "limit", 50))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// fileRead 读文本文件。
func (s *Server) fileRead(c *gin.Context) {
	out, err := s.svc.Files.Read(strings.TrimSpace(c.Query("path")))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// fileSave 保存文本文件（新建与覆盖同一入口）。
func (s *Server) fileSave(c *gin.Context) {
	var req domain.FileSaveREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Files.Save(req); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// fileMkdir 新建目录。
func (s *Server) fileMkdir(c *gin.Context) {
	var req domain.FilePathREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Files.Mkdir(req.FilePath); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// fileDelete 删除文件或目录（递归）。
func (s *Server) fileDelete(c *gin.Context) {
	var req domain.FilePathREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Files.Delete(req.FilePath); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}

// fileRename 改名或移动。
func (s *Server) fileRename(c *gin.Context) {
	var req domain.FileRenameREQ
	if !bind(c, &req) {
		return
	}
	if err := s.svc.Files.Rename(req); err != nil {
		pkg.Fail(c, err)
		return
	}
	ok(c)
}
