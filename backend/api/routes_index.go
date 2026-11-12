package api

import (
	"github.com/gin-gonic/gin"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// indexStatus 索引状态。
func (s *Server) indexStatus(c *gin.Context) {
	pkg.OK(c, s.svc.Searcher.Status())
}

// indexSearch 工作区检索。
func (s *Server) indexSearch(c *gin.Context) {
	var req domain.IndexSearchREQ
	if !bind(c, &req) {
		return
	}
	hits, err := s.svc.Searcher.Search(c.Request.Context(), req.SearchQuery, req.SearchLimit)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, domain.IndexSearchRESP{
		SearchHits:  hits,
		IndexStatus: s.svc.Searcher.Status(),
	})
}

// indexRebuild 重建工作区索引。
func (s *Server) indexRebuild(c *gin.Context) {
	status, err := s.svc.Searcher.Rebuild(c.Request.Context())
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, status)
}
