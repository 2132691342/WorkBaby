package api

import (
	"github.com/gin-gonic/gin"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// webSearch 联网搜索。
func (s *Server) webSearch(c *gin.Context) {
	var req domain.WebSearchREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Web.SearchRESP(c.Request.Context(), req.SearchQuery, req.SearchLimit)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// webFetch 抓取一个网页。
func (s *Server) webFetch(c *gin.Context) {
	var req domain.WebFetchREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Web.FetchRESP(c.Request.Context(), req.FetchUrl)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// webStatus 联网设置与状态。
func (s *Server) webStatus(c *gin.Context) {
	pkg.OK(c, s.svc.Web.SettingsRESP())
}

// webSettings 保存联网设置。
func (s *Server) webSettings(c *gin.Context) {
	var req domain.WebSettingsREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Web.SaveSettings(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}
