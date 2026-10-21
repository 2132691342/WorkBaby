// 工作区版本视图的 HTTP handler：status / diff / 历史 / 分支为只读，
// stage / unstage / commit / branch / switch / push / message 为受控写入（specs/08）。
// 编排全部在 service 层，这里只做参数与响应。
package api

import (
	"strconv"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// GitStatus 版本状态。带 session_id 时看该会话的工作目录，否则看全局工作目录。
func (h *Handler) GitStatus(c *gin.Context) {
	vo, err := h.Svc.Git.Status(c.Query("session_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitDiff 取 path 指定文件的差异；不带 path 表示整个工作区。
func (h *Handler) GitDiff(c *gin.Context) {
	vo, err := h.Svc.Git.Diff(c.Query("session_id"), c.Query("path"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitHistory 提交列表，按时间倒序。
func (h *Handler) GitHistory(c *gin.Context) {
	vo, err := h.Svc.Git.History(c.Query("session_id"), atoiDefault(c.Query("limit"), 50))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitCommitDetail 单次提交的元信息、文件清单与逐行差异。
func (h *Handler) GitCommitDetail(c *gin.Context) {
	vo, err := h.Svc.Git.CommitDetail(c.Query("session_id"), c.Query("hash"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitFileHistory 一个文件的提交历史。
func (h *Handler) GitFileHistory(c *gin.Context) {
	vo, err := h.Svc.Git.FileHistory(c.Query("session_id"), c.Query("path"), atoiDefault(c.Query("limit"), 50))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitBranches 分支清单，标注当前分支与各自领先 / 落后数。
func (h *Handler) GitBranches(c *gin.Context) {
	vo, err := h.Svc.Git.Branches(c.Query("session_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitStage 入栈。写后返回重读的状态，界面用一份数据刷新列表。
func (h *Handler) GitStage(c *gin.Context) {
	var req domain.GitPathsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Git.Stage(c.Query("session_id"), req.Paths)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitUnstage 出栈。只动索引，不动工作区文件。
func (h *Handler) GitUnstage(c *gin.Context) {
	var req domain.GitPathsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Git.Unstage(c.Query("session_id"), req.Paths)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitCommit 提交。信息来自用户填写的输入框，后端不预填也不代填。
func (h *Handler) GitCommit(c *gin.Context) {
	var req domain.GitCommitREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Git.Commit(c.Query("session_id"), req.Message)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitCreateBranch 新建分支并切过去。
func (h *Handler) GitCreateBranch(c *gin.Context) {
	var req domain.GitBranchREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Git.CreateBranch(c.Query("session_id"), req.Name)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitSwitchBranch 切到已有分支。
func (h *Handler) GitSwitchBranch(c *gin.Context) {
	var req domain.GitBranchREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Git.SwitchBranch(c.Query("session_id"), req.Name)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitPush 推送当前分支。只允许普通推送，禁止清单见 specs/08。
func (h *Handler) GitPush(c *gin.Context) {
	vo, err := h.Svc.Git.Push(c.Query("session_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// GitDraftMessage 让助手按当前 diff 起草提交信息。
// 它可能跑几十秒，因此同步等待但前端给加载态；失败返回 Err 让用户手写。
func (h *Handler) GitDraftMessage(c *gin.Context) {
	var req domain.GitPathsREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	ok(c, h.Svc.Git.DraftCommitMessage(c.Query("session_id"), req.Paths))
}

// atoiDefault 解析 limit 参数；缺失或非法时给默认值。
func atoiDefault(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
