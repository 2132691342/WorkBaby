// 观测面 HTTP handler：运行记录、检查点、评估与回归。薄层，逻辑在 service。
package api

import (
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"

	"github.com/gin-gonic/gin"
)

// ListRuns 运行记录列表；session_id 为空表示跨会话。
func (h *Handler) ListRuns(c *gin.Context) {
	list, err := h.Svc.Runs.List(c.Query("session_id"), queryInt(c, "limit"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// RunDetail 一次运行的详情：记录 + 时间线 + 上下文快照 + 检查点。
func (h *Handler) RunDetail(c *gin.Context) {
	vo, err := h.Svc.Runs.Detail(c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// ListCheckpoints 一个会话的检查点。
func (h *Handler) ListCheckpoints(c *gin.Context) {
	list, err := h.Svc.Runs.Checkpoints(c.Query("session_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// RestoreCheckpoint 回到某个检查点（移动会话叶子，不删条目）。
func (h *Handler) RestoreCheckpoint(c *gin.Context) {
	var req domain.RestoreCheckpointREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	vo, err := h.Svc.Runs.Restore(req.SessionID, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// ForkCheckpoint 从某个检查点分叉出一条新会话。
func (h *Handler) ForkCheckpoint(c *gin.Context) {
	var req domain.RestoreCheckpointREQ
	if err := bindOptionalBody(c, &req); err != nil {
		fail(c, err)
		return
	}
	vo, err := h.Svc.Runs.Fork(req.SessionID, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// ListEvals 评估列表。
func (h *Handler) ListEvals(c *gin.Context) {
	list, err := h.Svc.Evals.List(c.Query("scope"), c.Query("scope_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// RegisterEval 登记主观分。
func (h *Handler) RegisterEval(c *gin.Context) {
	var req domain.RegisterEvalREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Evals.Register(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// ListBaselines 基线列表。
func (h *Handler) ListBaselines(c *gin.Context) {
	list, err := h.Svc.Evals.Baselines()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// SaveBaseline 存基线。
func (h *Handler) SaveBaseline(c *gin.Context) {
	var req domain.SaveBaselineREQ
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, pkg.Wrap("1107", "请求格式不正确", err))
		return
	}
	vo, err := h.Svc.Evals.SaveBaseline(req)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// DeleteBaseline 删基线。
func (h *Handler) DeleteBaseline(c *gin.Context) {
	if err := h.Svc.Evals.DeleteBaseline(c.Param("id")); err != nil {
		fail(c, err)
		return
	}
	ok(c, nil)
}

// CompareEval 回归对比。
func (h *Handler) CompareEval(c *gin.Context) {
	vo, err := h.Svc.Evals.Compare(c.Query("baseline_id"), c.Query("run_id"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, vo)
}

// ArchiveNow 手动触发一次按保留期归档。
func (h *Handler) ArchiveNow(c *gin.Context) {
	res, err := h.Svc.Archive.Now()
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, res)
}
