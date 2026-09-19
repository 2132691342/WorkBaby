// Package api Wiki 仓库导读 HTTP 接口：全仓扫描概览 + 单页（目录/文件）。
package api

import (
	"WorkBaby/internal/domain"
	"WorkBaby/internal/service"
)

// wikiSvc 构造指向当前会话工作区的导读服务（无状态，按需新建）。
func (h *Handler) wikiSvc(sessionID string) *service.WikiService {
	return service.NewWikiService(func() string { return h.workspaceSvc.Dir(sessionID) })
}

// WikiOverview 仓库导读首屏（语言统计 / 入口 / 目录树）。
func (h *Handler) WikiOverview(sessionID string) (*domain.WikiOverviewRESP, error) {
	return h.wikiSvc(sessionID).Overview()
}

// WikiPage 单页：目录页带子项清单，文件页带正文。
func (h *Handler) WikiPage(sessionID, path string) (*domain.WikiPageRESP, error) {
	return h.wikiSvc(sessionID).Page(path)
}
