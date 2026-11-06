package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Push 推送。force 会毁掉别人的提交，所以要确认词：界面让用户把分支名原样敲一遍，
// 后端比对无误才加 --force-with-lease。--force 一律不用——它会把远端未拉到的提交静默抹掉。
func (g *Git) Push(req domain.GitSyncREQ) (domain.GitStatusRESP, error) {
	remote := defaultIfEmpty(strings.TrimSpace(req.RemoteName), "origin")
	branch := strings.TrimSpace(req.BranchName)
	args := []string{"push"}
	switch req.ForceMode {
	case "with-lease":
		if strings.TrimSpace(req.Confirm) != branch {
			return domain.GitStatusRESP{}, pkg.Err(pkg.CodeBadRequest, "强制推送需要确认分支名："+branch)
		}
		args = append(args, "--force-with-lease")
	case "", "none":
	default:
		return domain.GitStatusRESP{}, pkg.Err(pkg.CodeBadRequest, "未知的强制推送模式", req.ForceMode)
	}
	if req.SetUpstream && branch != "" {
		args = append(args, "-u")
	}
	args = append(args, remote)
	if branch != "" {
		args = append(args, branch)
	}
	if _, err := g.runWriteOrFail(args...); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// Pull 拉取。
func (g *Git) Pull(req domain.GitSyncREQ) (domain.GitStatusRESP, error) {
	remote := defaultIfEmpty(strings.TrimSpace(req.RemoteName), "origin")
	branch := strings.TrimSpace(req.BranchName)
	args := []string{"pull"}
	if req.IsRebase {
		args = append(args, "--rebase")
	}
	args = append(args, remote)
	if branch != "" {
		args = append(args, branch)
	}
	if _, err := g.runWriteOrFail(args...); err != nil {
		status := g.Status()
		if status.HasConflict {
			// 冲突是拉取的正常结果之一：交给界面走解决流程。
			return status, nil
		}
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// Fetch 只更新远端引用，不碰工作区。
func (g *Git) Fetch(remote string) (domain.GitStatusRESP, error) {
	args := []string{"fetch", "--prune"}
	if name := strings.TrimSpace(remote); name != "" {
		args = append(args, name)
	}
	if _, err := g.runWriteOrFail(args...); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}
