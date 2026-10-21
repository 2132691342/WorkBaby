// 工作区版本视图的编排：先解析会话工作目录再问 git，
// 把 git 能力域的读写结果按「助手实际干活的目录」装配给界面。
package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/git"
	"WorkBaby/backend/pkg"
)

// GitService 提供工作区的版本状态与受控写入：读全开放，写限白名单动作。
type GitService struct{ env *Env }

// NewGitService 构造版本视图服务。
func NewGitService(env *Env) *GitService { return &GitService{env: env} }

// Status 版本状态；没装 Git 或不是仓库时返回结果里的标记，不报错。
func (s *GitService) Status(sessionID string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Status(root)
}

// Diff 取 rel 文件的差异；rel 为空表示整个工作区。
func (s *GitService) Diff(sessionID, rel string) (*domain.GitDiffVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Diff(root, rel)
}

// History 提交列表，按时间倒序。
func (s *GitService) History(sessionID string, limit int) ([]domain.GitCommitVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.History(root, limit)
}

// CommitDetail 单次提交的完整内容。
func (s *GitService) CommitDetail(sessionID, hash string) (*domain.GitCommitDetailVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.CommitDetail(root, hash)
}

// FileHistory 一个文件的提交历史。
func (s *GitService) FileHistory(sessionID, rel string, limit int) ([]domain.GitCommitVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.FileHistory(root, rel, limit)
}

// Branches 分支清单（本地 + 远端）。
func (s *GitService) Branches(sessionID string) ([]domain.GitBranchVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Branches(root)
}

// Stage 入栈；写后返回重读的状态。
func (s *GitService) Stage(sessionID string, paths []string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Stage(root, paths)
}

// Unstage 出栈；只动索引。
func (s *GitService) Unstage(sessionID string, paths []string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Unstage(root, paths)
}

// Discard 丢弃工作区改动；必须显式指定文件。
func (s *GitService) Discard(sessionID string, paths []string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Discard(root, paths)
}

// ResolveConflict 把冲突文件标记为已解决。
func (s *GitService) ResolveConflict(sessionID string, paths []string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.ResolveConflict(root, paths)
}

// Commit 提交；信息由用户填，amend 改写上一次提交。
func (s *GitService) Commit(sessionID, message string, amend bool) (*domain.GitCommitRESP, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Commit(root, message, amend)
}

// CreateBranch 新建分支并切过去。
func (s *GitService) CreateBranch(sessionID, name string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.CreateBranch(root, name)
}

// SwitchBranch 切到已有分支。
func (s *GitService) SwitchBranch(sessionID, name string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.SwitchBranch(root, name)
}

// DeleteBranch 删除已合并的本地分支。
func (s *GitService) DeleteBranch(sessionID, name string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.DeleteBranch(root, name)
}

// Merge 合并分支；rebase 为真时用变基。冲突不算错误：状态里会带 state 与冲突文件。
func (s *GitService) Merge(sessionID, name string, rebase bool) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Merge(root, name, rebase)
}

// UpdateState 继续或中止未完成的合并 / 变基。
func (s *GitService) UpdateState(sessionID, action string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.UpdateState(root, action)
}

// Pull 从远端拉取并合入。
func (s *GitService) Pull(sessionID string, req domain.GitPullREQ) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Pull(root, req.Remote, req.Branch, req.Rebase)
}

// Push 推送当前分支；强制推送的级别由 req.Force 决定。
func (s *GitService) Push(sessionID string, req domain.GitPushREQ) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Push(root, req)
}

// Fetch 取回远端最新状态，不动工作区也不动本地分支。
func (s *GitService) Fetch(sessionID, remote string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Fetch(root, remote)
}

// Init 在当前工作目录初始化仓库。
func (s *GitService) Init(sessionID, branch string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Init(root, branch)
}

// AddRemote 添加远端地址。
func (s *GitService) AddRemote(sessionID, name, url string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.AddRemote(root, name, url)
}

// workspace 取助手实际干活的目录：会话自己设过就用会话的，否则用全局工作目录。
// 只用全局会看错地方——会话级工作目录才是这次改动真正发生的地方。
func (s *GitService) workspace(sessionID string) (string, error) {
	if id := strings.TrimSpace(sessionID); id != "" {
		if s.env.Sessions != nil {
			if sess, err := s.env.Sessions.Get(id); err == nil && sess != nil {
				if ws := strings.TrimSpace(sess.Workspace); ws != "" {
					if !pkg.DirExists(ws) {
						return "", pkg.New("1003", "这个会话的工作目录不存在", ws)
					}
					return ws, nil
				}
			}
		}
		// 会话查不到就退回全局：会话可能刚被删掉，此时不该让这一页变成错误页。
	}
	root := strings.TrimSpace(s.env.Workspace())
	if root == "" {
		return "", pkg.New("1001", "还没有设置工作目录，先在对话页顶部选一个文件夹", "")
	}
	if !pkg.DirExists(root) {
		return "", pkg.New("1003", "工作目录不存在", root)
	}
	return root, nil
}
