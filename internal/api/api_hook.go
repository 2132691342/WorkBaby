// Package api 用户钩子 HTTP 接口：CRUD + 试跑。
package api

import (
	"WorkBaby/internal/domain"
)

// ListHooks 全量钩子。
func (h *Handler) ListHooks() ([]domain.UserHookRESP, error) {
	return h.hookSvc.List(h.ctx)
}

// UpsertHook 创建/更新钩子。
func (h *Handler) UpsertHook(req domain.UserHookREQ) (*domain.UserHookRESP, error) {
	return h.hookSvc.Upsert(h.ctx, &req)
}

// DeleteHook 删除钩子。
func (h *Handler) DeleteHook(id string) error {
	return h.hookSvc.Delete(h.ctx, id)
}

// TestHook 设置页试跑一次。
func (h *Handler) TestHook(id string) (*domain.HookTestRESP, error) {
	return h.hookSvc.Test(h.ctx, id)
}
