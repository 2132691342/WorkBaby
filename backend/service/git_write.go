package service

import (
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 写操作统一在命令之后回一份最新状态：界面不需要再发一次 status 请求，
// 两次请求之间的竞态也就不会出现「按钮点完了界面还显示旧的改动」。

// Stage 暂存指定文件。
func (g *Git) Stage(paths []string) (domain.GitStatusRESP, error) {
	if len(paths) == 0 {
		return g.Status(), nil
	}
	if _, err := g.runOrFail(append([]string{"add", "--"}, paths...)...); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// Unstage 取消暂存。
func (g *Git) Unstage(paths []string) (domain.GitStatusRESP, error) {
	if len(paths) == 0 {
		return g.Status(), nil
	}
	if _, err := g.runOrFail(append([]string{"reset", "HEAD", "--"}, paths...)...); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// Discard 丢弃工作区改动：未跟踪文件删除，已跟踪文件还原，两条路径分开走。
// 混在一起会出现「点了丢弃，新文件还在」这种说不通的结果。
func (g *Git) Discard(paths []string) (domain.GitStatusRESP, error) {
	if len(paths) == 0 {
		return g.Status(), nil
	}
	before := g.Status()
	tracked := make([]string, 0, len(paths))
	untracked := make([]string, 0, len(paths))
	for _, path := range paths {
		if isUntracked(before, path) {
			untracked = append(untracked, path)
			continue
		}
		tracked = append(tracked, path)
	}
	if len(tracked) > 0 {
		if _, err := g.runOrFail(append([]string{"checkout", "--"}, tracked...)...); err != nil {
			return domain.GitStatusRESP{}, err
		}
	}
	if len(untracked) > 0 {
		if _, err := g.runOrFail(append([]string{"clean", "-f", "--"}, untracked...)...); err != nil {
			return domain.GitStatusRESP{}, err
		}
	}
	return g.Status(), nil
}

// isUntracked 判断一个路径在状态快照里是不是未跟踪。
func isUntracked(status domain.GitStatusRESP, path string) bool {
	for _, file := range status.ChangedFiles {
		if file.FilePath == path && file.ChangeCode == "?" {
			return true
		}
	}
	return false
}

// Resolve 把冲突文件标记为已解决。
func (g *Git) Resolve(paths []string) (domain.GitStatusRESP, error) {
	if len(paths) == 0 {
		return g.Status(), nil
	}
	if _, err := g.runOrFail(append([]string{"add", "--"}, paths...)...); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// Commit 提交暂存区。
func (g *Git) Commit(message string, amend bool) (domain.GitCommitRESP, error) {
	text := strings.TrimSpace(message)
	if text == "" {
		return domain.GitCommitRESP{}, pkg.Err(pkg.CodeBadRequest, "提交信息不能为空")
	}
	args := []string{"commit", "-m", text}
	if amend {
		args = append(args, "--amend")
	}
	if _, err := g.runOrFail(args...); err != nil {
		return domain.GitCommitRESP{}, err
	}
	hash, _ := g.runOrFail("rev-parse", "HEAD")
	return domain.GitCommitRESP{Status: g.Status(), CommitHash: strings.TrimSpace(hash)}, nil
}

// CreateBranch 新建并切到分支。
func (g *Git) CreateBranch(name string) (domain.GitStatusRESP, error) {
	branch := strings.TrimSpace(name)
	if err := validateBranchName(branch); err != nil {
		return domain.GitStatusRESP{}, err
	}
	if _, err := g.runOrFail("checkout", "-b", branch); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// SwitchBranch 切分支。
func (g *Git) SwitchBranch(name string) (domain.GitStatusRESP, error) {
	branch := strings.TrimSpace(name)
	if branch == "" {
		return domain.GitStatusRESP{}, pkg.Err(pkg.CodeBadRequest, "分支名不能为空")
	}
	if _, err := g.runOrFail("checkout", branch); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// DeleteBranch 删除分支。
func (g *Git) DeleteBranch(name string) (domain.GitStatusRESP, error) {
	branch := strings.TrimSpace(name)
	if branch == "" {
		return domain.GitStatusRESP{}, pkg.Err(pkg.CodeBadRequest, "分支名不能为空")
	}
	if _, err := g.runOrFail("branch", "-d", branch); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// Merge 合并分支。
func (g *Git) Merge(name string, rebase bool) (domain.GitStatusRESP, error) {
	branch := strings.TrimSpace(name)
	if branch == "" {
		return domain.GitStatusRESP{}, pkg.Err(pkg.CodeBadRequest, "分支名不能为空")
	}
	var err error
	if rebase {
		_, err = g.runOrFail("rebase", branch)
	} else {
		_, err = g.runOrFail("merge", "--no-edit", branch)
	}
	if err != nil {
		// 冲突不算命令失败：状态里会显示冲突文件，界面继续走解决流程。
		status := g.Status()
		if status.HasConflict || status.RepoState != "normal" {
			return status, nil
		}
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// State 冲突收尾：继续或中止。
func (g *Git) State(action string) (domain.GitStatusRESP, error) {
	status := g.Status()
	var args []string
	switch {
	case status.RepoState == "rebasing":
		if action == "continue" {
			args = []string{"rebase", "--continue"}
		} else {
			args = []string{"rebase", "--abort"}
		}
	case status.RepoState == "cherry-picking":
		if action == "continue" {
			args = []string{"cherry-pick", "--continue"}
		} else {
			args = []string{"cherry-pick", "--abort"}
		}
	default:
		if action == "abort" {
			args = []string{"merge", "--abort"}
		} else {
			return domain.GitStatusRESP{}, pkg.Err(pkg.CodeGitFailed, "当前没有需要收尾的状态")
		}
	}
	if _, err := g.runOrFail(args...); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// Init 初始化仓库。
func (g *Git) Init(branch string) (domain.GitStatusRESP, error) {
	args := []string{"init"}
	if name := strings.TrimSpace(branch); name != "" {
		args = append(args, "-b", name)
	}
	if _, err := g.runOrFail(args...); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// AddRemote 添加远端。
func (g *Git) AddRemote(name, url string) (domain.GitStatusRESP, error) {
	remote := strings.TrimSpace(name)
	target := strings.TrimSpace(url)
	if remote == "" || target == "" {
		return domain.GitStatusRESP{}, pkg.Err(pkg.CodeBadRequest, "远端名与地址都不能为空")
	}
	if _, err := g.runOrFail("remote", "add", remote, target); err != nil {
		return domain.GitStatusRESP{}, err
	}
	return g.Status(), nil
}

// validateBranchName 只做最基本的形状检查：真正的合法性由 git 自己判。
func validateBranchName(name string) error {
	if name == "" {
		return pkg.Err(pkg.CodeBadRequest, "分支名不能为空")
	}
	if strings.ContainsAny(name, " ~^:?*[\\") || strings.HasPrefix(name, "-") {
		return pkg.Err(pkg.CodeBadRequest, "分支名含非法字符", name)
	}
	return nil
}
