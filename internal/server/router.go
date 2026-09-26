package server

import (
	"strconv"
)

// registerRoutes 注册全部业务路由（/api/v1），按功能域拆分在 routes_*.go。
// 风格约定：仅 GET/POST，路径变量在末尾，body 与 query 字段 snake_case。
func (s *Server) registerRoutes() {
	h := s.handler
	v1 := s.engine.Group("/api/v1")

	registerMetaRoutes(s, v1, h)
	registerChatRoutes(v1, h)
	registerWorkspaceRoutes(v1, h)
	registerProviderRoutes(v1, h)
	registerSkillRoutes(v1, h)
	registerAgentRoutes(v1, h)
	registerKnowledgeRoutes(v1, h)
	registerFileRoutes(v1, h)
	registerHookRoutes(v1, h)
	registerTaskRoutes(v1, h)
}

// ---- 小工具 ----

func atoi(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func atoi64(s string, def int64) int64 {
	if s == "" {
		return def
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return n
}
