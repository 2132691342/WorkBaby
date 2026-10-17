// 受控写入：stage / unstage / commit / 新建分支 / 切分支 / push 六种白名单动作（specs/09）。
// 写入不可逆，因此每个动作都显式列举参数，命中禁止清单一律拒绝——不解析、不猜测。
package git

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Stage 把 paths 放进待提交区；paths 为空表示全部改动。
func Stage(dir string, paths []string) (*domain.GitStatusVO, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	args, err := pathsArg(dir, paths)
	if err != nil {
		return nil, err
	}
	if _, err := run(dir, writeTimeout, append([]string{"add", "--"}, args...)...); err != nil {
		return nil, err
	}
	return Status(dir)
}

// Unstage 把 paths 从待提交区取出；只动索引，不动工作区文件。
func Unstage(dir string, paths []string) (*domain.GitStatusVO, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	args, err := pathsArg(dir, paths)
	if err != nil {
		return nil, err
	}
	// 首次提交前还没有 HEAD，reset HEAD 会失败；那时只用 `--cached` 形式清索引。
	prefix := []string{"reset", "HEAD"}
	if !hasHead(dir) {
		prefix = []string{"reset"}
	}
	if _, err := run(dir, writeTimeout, append(prefix, append([]string{"--"}, args...)...)...); err != nil {
		return nil, err
	}
	return Status(dir)
}

// Commit 用用户填的信息提交；信息非空且不超上限，提交后返回重读的状态与新 hash。
func Commit(dir, message string) (*domain.GitCommitRESP, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return nil, pkg.New("1016", "提交信息还没填，写一句这次改了什么", "")
	}
	if len([]rune(msg)) > domain.GitCommitMsgLimit {
		return nil, pkg.New("1016", "提交信息太长了，缩短到一句摘要", "")
	}
	// 不带 --amend / --no-verify：改写已发布历史与跳过钩子都不该由界面替用户决定。
	res, err := run(dir, writeTimeout, "commit", "-m", msg)
	if err != nil {
		// 无改动可提交是 git 的 1，原文说清「没什么可提交」；折成 1017 让界面给出下一步。
		if strings.Contains(res.errOut, "nothing to commit") ||
			strings.Contains(res.out, "nothing to commit") {
			return nil, pkg.New("1017", "没有可提交的改动，先勾选文件入栈", "")
		}
		return nil, err
	}
	st, err := Status(dir)
	if err != nil {
		return nil, err
	}
	return &domain.GitCommitRESP{Status: st, Hash: headHash(dir)}, nil
}

// CreateBranch 新建分支并切过去；名字走白名单字符集。
func CreateBranch(dir, name string) (*domain.GitStatusVO, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	if !validBranchName(name) {
		return nil, pkg.New("1018", "分支名不合法", "只允许字母、数字、点、下划线、斜杠与连字符，且不能以连字符开头")
	}
	if _, err := run(dir, writeTimeout, "checkout", "-b", name); err != nil {
		return nil, err
	}
	return Status(dir)
}

// SwitchBranch 切到已有分支；不带 --force，避免用索引覆盖工作区改动。
func SwitchBranch(dir, name string) (*domain.GitStatusVO, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	if !validBranchName(name) {
		return nil, pkg.New("1018", "分支名不合法", name)
	}
	if _, err := run(dir, writeTimeout, "checkout", name); err != nil {
		return nil, err
	}
	return Status(dir)
}

// Push 推送当前分支。只允许不带参数的普通推送：--force 会覆盖远端历史，别人的提交会静默消失。
func Push(dir string) (*domain.GitStatusVO, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	if _, err := run(dir, pushTimeout, "push"); err != nil {
		return nil, err
	}
	return Status(dir)
}

// headHash 取当前 HEAD 的完整 hash，供提交后跳到历史页那一条。
func headHash(dir string) string {
	res, err := run(dir, statusTimeout, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(res.out)
}

// pathsArg 把界面给的路径收成 git 的 pathspec；越界直接报错，不降级成「全部」。
// 空表示全部改动，git 用 `.` 表达当前目录。
func pathsArg(dir string, paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// SafeJoin 是唯一的路障：它同时拒绝对路径与 `..`，写入时必须走它。
		joined, err := pkg.SafeJoin(dir, p)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(dir, joined)
		if err != nil {
			return nil, pkg.New("1004", "路径越出工作目录", p)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	if len(out) == 0 {
		return []string{"."}, nil
	}
	return out, nil
}

// validBranchName 分支名白名单：拒绝以 `-` 开头（会被当成选项）、拒绝 `..` 与空格。
// 斜杠允许：`feature/x` 是常见分层；但连续斜杠与结尾斜杠会让 git 造出嵌套 ref。
func validBranchName(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 128 || strings.HasPrefix(s, "-") {
		return false
	}
	if strings.Contains(s, "..") || strings.ContainsAny(s, " \t\n\\~^:?*[") {
		return false
	}
	if strings.HasSuffix(s, "/") || strings.Contains(s, "//") || strings.HasSuffix(s, ".lock") {
		return false
	}
	return true
}
