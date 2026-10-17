// 工作区版本视图的编排：先解析会话工作目录再问 git。
package service

// 工作区版本视图：把 git 能力域的只读结果按「助手实际干活的目录」装配给界面。

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/git"
	"WorkBaby/backend/pkg"
)

// GitService 提供工作区的版本状态与差异：只读、只在工作目录内。
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

// Branches 分支清单。
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

// Commit 提交；信息由用户填。
func (s *GitService) Commit(sessionID, message string) (*domain.GitCommitRESP, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Commit(root, message)
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

// Push 推送当前分支。
func (s *GitService) Push(sessionID string) (*domain.GitStatusVO, error) {
	root, err := s.workspace(sessionID)
	if err != nil {
		return nil, err
	}
	return git.Push(root)
}

// workspace 取助手实际干活的目录：会话自己设过就用会话的，否则用全局工作目录。
// 只用全局会看错地方——会话级工作目录才是这次改动真正发生的地方。
func (s *GitService) workspace(sessionID string) (string, error) {
	if id := strings.TrimSpace(sessionID); id != "" {
		sess, err := s.env.Repo.GetSession(id)
		if err != nil {
			// 会话不存在退回全局即可；其余（数据库故障）必须让用户看见，不能静默换目录
			if pkg.CodeOf(err) != "1101" {
				return "", err
			}
		} else if sess != nil {
			if ws := strings.TrimSpace(sess.Workspace); ws != "" {
				if !pkg.DirExists(ws) {
					return "", pkg.New("1003", "这个会话的工作目录不存在", ws)
				}
				return ws, nil
			}
		}
		// 会话查不到就退回全局：会话可能刚被删掉，此时不该让这一页变成错误页。
	}
	root := strings.TrimSpace(s.env.Cfg.Workspace)
	if root == "" {
		return "", pkg.New("1001", "还没有设置工作目录，先在对话页顶部选一个文件夹", "")
	}
	if !pkg.DirExists(root) {
		return "", pkg.New("1003", "工作目录不存在", root)
	}
	return root, nil
}
