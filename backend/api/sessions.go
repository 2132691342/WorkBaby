// 会话资源 handler：列表 / 建 / 详情 / 改名 / 换工作区 / 删 / 换模型 / 权限档 / 思考档 / 分叉。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"github.com/gin-gonic/gin"
	"strconv"
)

// ListSessions 会话列表；offset 用于「加载更多」翻页。
func (h *Handler) ListSessions(c *gin.Context) {
	offset, _ := strconv.Atoi(c.Query("offset"))
	if offset < 0 {
		offset = 0
	}
	list, err := h.Svc.Sessions.List(offset)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// CreateSession 新建会话。
func (h *Handler) CreateSession(c *gin.Context) {
	var req domain.CreateSessionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Sessions.Create(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GetSession 会话详情。
func (h *Handler) GetSession(c *gin.Context) {
	detail, err := h.Svc.Sessions.Detail(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, detail)
}

// RenameSession 重命名会话。
func (h *Handler) RenameSession(c *gin.Context) {
	var req domain.RenameSessionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.Rename(c.Param("id"), req.Title); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetSessionWorkspace 切换会话工作目录：改完下一条消息立即生效。
func (h *Handler) SetSessionWorkspace(c *gin.Context) {
	var req domain.SetWorkspaceREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.SetWorkspace(c.Param("id"), req.Workspace); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// DeleteSession 删除会话。
func (h *Handler) DeleteSession(c *gin.Context) {
	id := c.Param("id")
	// 先摘运行态：否则正在跑的 run 会继续往已删会话写孤儿条目。
	h.Svc.Chat.Forget(id)
	if err := h.Svc.Sessions.Delete(id); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetSessionModel 切换会话模型。
func (h *Handler) SetSessionModel(c *gin.Context) {
	var req domain.SetModelREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.SetModel(c.Param("id"), req.ProviderID, req.Model); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetSessionPermission 切换会话权限档。
func (h *Handler) SetSessionPermission(c *gin.Context) {
	var req domain.SetPermissionREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.SetPermission(c.Param("id"), req.Permission); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// SetSessionThinking 切换会话思考档。
func (h *Handler) SetSessionThinking(c *gin.Context) {
	var req domain.SetThinkingREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	if err := h.Svc.Sessions.SetThinkingLevel(c.Param("id"), req.Level); err != nil {
		fail(c, err)
		return
	}
	ok(c, true)
}

// ForkSession 从某条历史分叉成一条新会话。
// PI 的 leaf 是内部状态，协议上没有「改 leaf」这回事，所以语义是分叉而不是回退。
func (h *Handler) ForkSession(c *gin.Context) {
	var req domain.ForkREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Sessions.Fork(c.Param("id"), req.EntryID)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}
