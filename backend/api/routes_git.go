package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// mountGit 挂上 git 的全部端点。
// 读走 GET，写走 POST；写接口一律回一份最新状态，界面不需要再发一次 status。
func mountGit(g *gin.RouterGroup, s *Server) {
	g.GET("/git/status", s.gitStatus)
	g.GET("/git/diff", s.gitDiff)
	g.GET("/git/history", s.gitHistory)
	g.GET("/git/commit", s.gitCommitDetail)
	g.GET("/git/file-history", s.gitFileHistory)
	g.GET("/git/file-diff", s.gitFileDiff)
	g.GET("/git/branches", s.gitBranches)
	g.GET("/git/stash/list", s.gitStashList)

	g.POST("/git/stash/push", s.gitStashPush)
	g.POST("/git/stash/pop", s.gitStashPop)
	g.POST("/git/stash/drop", s.gitStashDrop)
	g.POST("/git/stage", s.gitStage)
	g.POST("/git/unstage", s.gitUnstage)
	g.POST("/git/discard", s.gitDiscard)
	g.POST("/git/resolve", s.gitResolve)
	g.POST("/git/conflict/resolve", s.gitResolveConflict)
	g.POST("/git/commit", s.gitCommit)
	g.POST("/git/branch", s.gitCreateBranch)
	g.POST("/git/branch/delete", s.gitDeleteBranch)
	g.POST("/git/switch", s.gitSwitchBranch)
	g.POST("/git/merge", s.gitMerge)
	g.POST("/git/state", s.gitState)
	g.POST("/git/init", s.gitInit)
	g.POST("/git/remote", s.gitAddRemote)
	g.POST("/git/push", s.gitPush)
	g.POST("/git/pull", s.gitPull)
	g.POST("/git/fetch", s.gitFetch)
}

// gitStatus 仓库状态。
func (s *Server) gitStatus(c *gin.Context) {
	pkg.OK(c, s.svc.Git.Status())
}

// gitDiff 工作区差异。
func (s *Server) gitDiff(c *gin.Context) {
	out, err := s.svc.Git.Diff(strings.TrimSpace(c.Query("path")))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitHistory 提交历史。
func (s *Server) gitHistory(c *gin.Context) {
	out, err := s.svc.Git.History(queryInt(c, "limit", 50))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, map[string]any{"commits": out})
}

// gitCommitDetail 单个提交详情。
func (s *Server) gitCommitDetail(c *gin.Context) {
	out, err := s.svc.Git.CommitDetail(strings.TrimSpace(c.Query("hash")))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitFileHistory 单文件提交历史。
func (s *Server) gitFileHistory(c *gin.Context) {
	out, err := s.svc.Git.FileHistory(strings.TrimSpace(c.Query("path")), queryInt(c, "limit", 40))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, map[string]any{"commits": out})
}

// gitFileDiff 某个提交里某文件的差异。
func (s *Server) gitFileDiff(c *gin.Context) {
	out, err := s.svc.Git.FileDiff(c.Query("hash"), c.Query("path"))
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitBranches 分支清单。
func (s *Server) gitBranches(c *gin.Context) {
	out, err := s.svc.Git.Branches()
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitStage 暂存。
func (s *Server) gitStage(c *gin.Context) {
	s.withPaths(c, s.svc.Git.Stage)
}

// gitUnstage 取消暂存。
func (s *Server) gitUnstage(c *gin.Context) {
	s.withPaths(c, s.svc.Git.Unstage)
}

// gitDiscard 丢弃改动。
func (s *Server) gitDiscard(c *gin.Context) {
	s.withPaths(c, s.svc.Git.Discard)
}

// gitResolve 标记冲突已解决。
func (s *Server) gitResolve(c *gin.Context) {
	s.withPaths(c, s.svc.Git.Resolve)
}

// gitResolveConflict 按指定口径解决一个冲突文件：保留我方 / 保留对方 / 我改完了。
func (s *Server) gitResolveConflict(c *gin.Context) {
	var req domain.GitConflictREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.ResolveConflict(req.FilePath, req.ResolveSide)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// withPaths 把「一批路径 → 最新状态」的写接口收成一处。
func (s *Server) withPaths(c *gin.Context, action func([]string) (domain.GitStatusRESP, error)) {
	var req domain.GitPathsREQ
	if !bind(c, &req) {
		return
	}
	out, err := action(req.FilePaths)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitCommit 提交。
func (s *Server) gitCommit(c *gin.Context) {
	var req domain.GitCommitREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.Commit(req.CommitMessage, req.IsAmend)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitCreateBranch 新建分支。
func (s *Server) gitCreateBranch(c *gin.Context) {
	var req domain.GitBranchREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.CreateBranch(req.BranchName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitSwitchBranch 切分支。
func (s *Server) gitSwitchBranch(c *gin.Context) {
	var req domain.GitBranchREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.SwitchBranch(req.BranchName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitDeleteBranch 删分支。
func (s *Server) gitDeleteBranch(c *gin.Context) {
	var req domain.GitBranchREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.DeleteBranch(req.BranchName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitMerge 合并 / rebase。
func (s *Server) gitMerge(c *gin.Context) {
	var req domain.GitBranchREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.Merge(req.BranchName, req.IsRebase)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitState 冲突收尾。
func (s *Server) gitState(c *gin.Context) {
	var req domain.GitStateREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.State(req.StateAction)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitInit 初始化仓库。
func (s *Server) gitInit(c *gin.Context) {
	var req domain.GitInitREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.Init(req.BranchName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitAddRemote 添加远端。
func (s *Server) gitAddRemote(c *gin.Context) {
	var req domain.GitRemoteREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.AddRemote(req.RemoteName, req.RemoteUrl)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitStashList stash 清单。
func (s *Server) gitStashList(c *gin.Context) {
	pkg.OK(c, s.svc.Git.StashList())
}

// gitStashPush 收藏改动进 stash。
func (s *Server) gitStashPush(c *gin.Context) {
	var req domain.GitStashREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.StashPush(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitStashPop 恢复最新一条 stash。
func (s *Server) gitStashPop(c *gin.Context) {
	out, err := s.svc.Git.StashPop()
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitStashDrop 丢弃指定 stash。
func (s *Server) gitStashDrop(c *gin.Context) {
	var req domain.GitStashREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.StashDrop(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitPush 推送。
func (s *Server) gitPush(c *gin.Context) {
	var req domain.GitSyncREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.Push(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitPull 拉取。
func (s *Server) gitPull(c *gin.Context) {
	var req domain.GitSyncREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.Pull(req)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}

// gitFetch 更新远端引用。
func (s *Server) gitFetch(c *gin.Context) {
	var req domain.GitSyncREQ
	if !bind(c, &req) {
		return
	}
	out, err := s.svc.Git.Fetch(req.RemoteName)
	if err != nil {
		pkg.Fail(c, err)
		return
	}
	pkg.OK(c, out)
}
