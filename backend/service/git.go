package service

import (
	"strconv"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/engine"
)

// maxDiffLines 是单份差异的行数上限：几千行的差异在界面里既看不清也扛不住。
const maxDiffLines = 4000

// Git 是 git 编排。所有命令都走内置 PortableGit，绝不回退系统 git。
type Git struct {
	// Settings 设置（工作区根）
	Settings *Settings
	// Runtime 内置运行时
	Runtime *engine.Runtime
}

// NewGit 造一个 git 门面。
func NewGit(settings *Settings, rt *engine.Runtime) *Git {
	return &Git{Settings: settings, Runtime: rt}
}

// run 跑一条 git 命令；cwd 缺省是工作区。
func (g *Git) run(args ...string) (*pkg.ProcResult, error) {
	exe, err := g.Runtime.GitExe()
	if err != nil {
		return nil, err
	}
	dir := g.Settings.Workspace()
	if dir == "" {
		return nil, pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	return pkg.RunProc(exe, args, pkg.ProcOptions{
		Dir:       dir,
		Env:       g.Runtime.Layout.SandboxEnv(nil),
		TimeoutMs: 120_000,
	})
}

// runOrFail 跑 git 并要求退出码为 0；失败时把 stderr 原话带出来。
func (g *Git) runOrFail(args ...string) (string, error) {
	res, err := g.run(args...)
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", pkg.Err(pkg.CodeGitFailed, "git 命令失败", strings.TrimSpace(res.Stderr))
	}
	return res.Stdout, nil
}

// Status 仓库状态快照。
func (g *Git) Status() domain.GitStatusRESP {
	out := domain.GitStatusRESP{
		Workspace: g.Settings.Workspace(),
		RepoState: "normal",
	}
	if !g.Runtime.Layout.GitReady() {
		out.UnavailableReason = "内置 PortableGit 尚未就绪"
		return out
	}
	out.GitAvailable = true
	out.GitVersion = g.Runtime.GitVersion()
	root, err := g.runOrFail("rev-parse", "--show-toplevel")
	if err != nil {
		// 不是仓库是正常状态，不是错误：界面会显示「初始化仓库」入口。
		return out
	}
	out.IsRepo = true
	out.RepoRoot = strings.TrimSpace(root)
	g.fillBranch(&out)
	g.fillRemotes(&out)
	g.fillChangedFiles(&out)
	g.fillRepoState(&out)
	return out
}

// fillBranch 填当前分支与上下游差距。
func (g *Git) fillBranch(out *domain.GitStatusRESP) {
	if head, err := g.runOrFail("rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		out.BranchName = strings.TrimSpace(head)
	}
	if upstream, err := g.runOrFail("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil {
		out.UpstreamRef = strings.TrimSpace(upstream)
	}
	if counts, err := g.runOrFail("rev-list", "--left-right", "--count", "@{u}...HEAD"); err == nil {
		fields := strings.Fields(counts)
		if len(fields) == 2 {
			// 左是上游独有（behind），右是本地独有（ahead）。
			out.Behind = atoiSafe(fields[0])
			out.Ahead = atoiSafe(fields[1])
		}
	}
}

// fillRemotes 填远端清单。
func (g *Git) fillRemotes(out *domain.GitStatusRESP) {
	text, err := g.runOrFail("remote", "-v")
	if err != nil {
		return
	}
	seen := map[string]struct{}{}
	for _, line := range pkg.SplitLines(text) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if _, ok := seen[fields[0]]; ok {
			continue
		}
		seen[fields[0]] = struct{}{}
		out.Remotes = append(out.Remotes, domain.GitRemoteVO{RemoteName: fields[0], RemoteUrl: fields[1]})
	}
}

// fillChangedFiles 解析 `git status --porcelain=v1`。
func (g *Git) fillChangedFiles(out *domain.GitStatusRESP) {
	text, err := g.runOrFail("status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return
	}
	files := make([]domain.GitFileVO, 0, 32)
	for _, line := range pkg.SplitLines(text) {
		if len(line) < 4 {
			continue
		}
		// 前两位是暂存区 / 工作区状态；rename 后面跟 "原路径 -> 新路径"。
		staged, worktree := line[0], line[1]
		path := strings.TrimSpace(line[3:])
		if idx := strings.LastIndex(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		file := domain.GitFileVO{
			FilePath:    path,
			ChangeCode:  changeCode(staged, worktree),
			IsStaged:    staged != ' ' && staged != '?',
			HasConflict: staged == 'U' || worktree == 'U',
		}
		if file.HasConflict {
			out.HasConflict = true
		}
		files = append(files, file)
	}
	out.ChangedFiles = files
}

// changeCode 把两位状态压成一个界面用的字母。
func changeCode(staged, worktree byte) string {
	for _, code := range []byte{staged, worktree} {
		switch code {
		case 'M':
			return "M"
		case 'A':
			return "A"
		case 'D':
			return "D"
		case 'R':
			return "R"
		case 'C':
			return "C"
		case 'T':
			return "T"
		case 'U':
			return "U"
		case '?':
			return "?"
		}
	}
	return "M"
}

// fillRepoState 判断是否处在合并 / rebase / cherry-pick 中间态。
func (g *Git) fillRepoState(out *domain.GitStatusRESP) {
	dir, err := g.runOrFail("rev-parse", "--git-dir")
	if err != nil {
		return
	}
	gitDir := strings.TrimSpace(dir)
	if !strings.HasPrefix(gitDir, "/") && !strings.Contains(gitDir, ":") {
		gitDir = strings.TrimSuffix(out.RepoRoot, `\`) + `\` + strings.ReplaceAll(gitDir, "/", `\`)
	}
	switch {
	case pkg.Exists(gitDir + `\rebase-merge`), pkg.Exists(gitDir + `\rebase-apply`):
		out.RepoState = "rebasing"
		out.RepoStateDetail = "rebase 未完成：继续或中止它"
	case pkg.Exists(gitDir + `\MERGE_HEAD`):
		out.RepoState = "merging"
		out.RepoStateDetail = "合并未完成：解决冲突后提交"
	case pkg.Exists(gitDir + `\CHERRY_PICK_HEAD`):
		out.RepoState = "cherry-picking"
		out.RepoStateDetail = "cherry-pick 未完成：继续或中止它"
	}
}

// Diff 未暂存 + 已暂存的差异。
func (g *Git) Diff(path string) (domain.GitDiffRESP, error) {
	args := []string{"diff", "--no-color", "HEAD"}
	if path != "" {
		args = append(args, "--", path)
	}
	text, err := g.runOrFail(args...)
	if err != nil {
		// 没有 HEAD（还没首次提交）时退回工作区差异，别让界面一片空白。
		text, err = g.runOrFail("diff", "--no-color")
		if err != nil {
			return domain.GitDiffRESP{}, err
		}
	}
	return cutDiff(path, text), nil
}

// cutDiff 把 diff 文本转成差异行并做长度上限。
func cutDiff(path, text string) domain.GitDiffRESP {
	lines := parseDiff(text)
	out := domain.GitDiffRESP{FilePath: path, DiffLines: lines}
	if len(lines) > maxDiffLines {
		out.Dropped = len(lines) - maxDiffLines
		out.DiffLines = lines[:maxDiffLines]
		out.Truncated = true
	}
	return out
}

// parseDiff 复用会话解析里的切分：两处必须给界面同一种差异形状。
func parseDiff(text string) []domain.DiffLineVO {
	return parseDiffShared(text)
}

// atoiSafe 把数字串转 int；坏值给 0。
func atoiSafe(text string) int {
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return n
}
