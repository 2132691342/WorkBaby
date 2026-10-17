// 版本历史与分支的只读查询：提交列表、单次提交内容、文件历史、分支清单。
// 全部实时走 git log / git branch，不另建一张提交表——换分支后历史整块变正是缓存不该存在的原因。
package git

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// maxCommits 单次列举的提交上限：翻页靠 limit，一次全拉会把深历史的解析时间推到界面上。
const maxCommits = 200

// History 列出 dir 当前分支最近的提交，按时间倒序。
func History(dir string, limit int) ([]domain.GitCommitVO, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxCommits {
		limit = maxCommits
	}
	// 用罕见的分隔符切字段：提交信息里出现制表符的概率比出现这个串低得多，
	// 且短 hash 固定 40 位以内，按它切比按 tab 切稳。
	const sep = "\x1f"
	res, err := run(dir, statusTimeout, "log",
		"-n", strconv.Itoa(limit),
		"--date=unix",
		"--pretty=format:%H"+sep+"%h"+sep+"%s"+sep+"%an"+sep+"%at"+sep,
		"--numstat")
	if err != nil {
		return nil, err
	}
	return parseLog(res.out, sep)
}

// CommitDetail 取一次提交的元信息、文件清单与逐行差异。
func CommitDetail(dir, hash string) (*domain.GitCommitDetailVO, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	if !validRef(hash) {
		return nil, pkg.New("1005", "提交号不合法", hash)
	}
	const sep = "\x1f"
	res, err := run(dir, statusTimeout, "show", "--no-color", "--no-ext-diff",
		"--date=unix",
		"--pretty=format:%H"+sep+"%h"+sep+"%s"+sep+"%b"+sep+"%an"+sep+"%at"+sep+"%P",
		hash)
	if err != nil {
		return nil, err
	}
	vo, err := parseShow(dir, res.out, sep)
	if err != nil {
		return nil, err
	}
	// 差异复用既有解析：一条链路两处配色规则，改格式只改一处。
	lines, dropped := parseDiff(res.out)
	vo.Diff = &domain.GitDiffVO{Path: "", Lines: lines, Dropped: dropped, Truncated: dropped > 0}
	return vo, nil
}

// FileHistory 列出一个文件的提交历史，按时间倒序。
func FileHistory(dir, rel string, limit int) ([]domain.GitCommitVO, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	// 路径必须落进工作目录：改写与读取同一道路障，这里也不例外。
	joined, err := pkg.SafeJoin(dir, strings.TrimSpace(rel))
	if err != nil {
		return nil, err
	}
	path, err := filepath.Rel(dir, joined)
	if err != nil {
		return nil, pkg.New("1004", "路径越出工作目录", rel)
	}
	path = filepath.ToSlash(path)
	if limit <= 0 || limit > maxCommits {
		limit = maxCommits
	}
	const sep = "\x1f"
	res, err := run(dir, statusTimeout, "log",
		"-n", strconv.Itoa(limit),
		"--date=unix",
		"--pretty=format:%H"+sep+"%h"+sep+"%s"+sep+"%an"+sep+"%at"+sep,
		"--numstat", "--", path)
	if err != nil {
		return nil, err
	}
	return parseLog(res.out, sep)
}

// Branches 列出全部分支，标注当前所在与各自相对上游的领先 / 落后数。
func Branches(dir string) ([]domain.GitBranchVO, error) {
	if err := requireRepo(dir); err != nil {
		return nil, err
	}
	res, err := run(dir, statusTimeout, "branch", "-vv", "--no-color")
	if err != nil {
		return nil, err
	}
	return parseBranches(res.out), nil
}

// requireRepo 统一的前置校验：装了 git 且这个目录在仓库里。
func requireRepo(dir string) error {
	if !Available() {
		return pkg.New("1015", "这台电脑上还没有 Git，装好后这里就能看到历史", "")
	}
	inside, err := insideWorkTree(dir)
	if err != nil {
		return err
	}
	if !inside {
		return pkg.New("1015", "这个工作目录还不是 Git 仓库，初始化后才能看历史", "")
	}
	return nil
}

// parseLog 解析 log + numstat 的输出：每个提交一段，段首是 pretty 行，随后是 numstat 行。
func parseLog(out, sep string) ([]domain.GitCommitVO, error) {
	commits := make([]domain.GitCommitVO, 0, 16)
	var cur *domain.GitCommitVO
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if !strings.Contains(line, sep) {
			// numstat 行：add<TAB>del<TAB>path；二进制文件前两列是 `-`。
			if cur == nil {
				continue
			}
			cols := strings.Split(line, "\t")
			if len(cols) < 3 {
				continue
			}
			cur.Files++
			if a, err := strconv.Atoi(strings.TrimSpace(cols[0])); err == nil {
				cur.Additions += a
			}
			if d, err := strconv.Atoi(strings.TrimSpace(cols[1])); err == nil {
				cur.Deletions += d
			}
			continue
		}
		f := strings.Split(line, sep)
		if len(f) < 6 {
			continue
		}
		when, _ := strconv.ParseInt(strings.TrimSpace(f[4]), 10, 64)
		if when > 0 {
			when *= 1000 // git 给秒，界面统一毫秒
		}
		commits = append(commits, domain.GitCommitVO{
			Hash: strings.TrimSpace(f[0]), Short: strings.TrimSpace(f[1]),
			Subject: strings.TrimSpace(f[2]), Author: strings.TrimSpace(f[3]), When: when,
		})
		cur = &commits[len(commits)-1]
	}
	return commits, nil
}

// parseShow 解析单次提交的元信息段（差异部分由 parseDiff 另走）。dir 用于补一次 numstat 查询。
func parseShow(dir, out, sep string) (*domain.GitCommitDetailVO, error) {
	head, _, _ := strings.Cut(out, "\ndiff --git")
	if !strings.Contains(head, sep) {
		return nil, pkg.New("1015", "读不到这次提交的内容", "")
	}
	f := strings.Split(head, sep)
	if len(f) < 7 {
		return nil, pkg.New("1015", "这次提交的信息读不完整", "")
	}
	when, _ := strconv.ParseInt(strings.TrimSpace(f[5]), 10, 64)
	if when > 0 {
		when *= 1000
	}
	vo := &domain.GitCommitDetailVO{
		Hash: strings.TrimSpace(f[0]), Short: strings.TrimSpace(f[1]),
		Subject: strings.TrimSpace(f[2]), Body: strings.TrimSpace(f[3]),
		Author: strings.TrimSpace(f[4]), When: when,
		Files: []domain.GitFileVO{},
	}
	for _, p := range strings.Fields(strings.TrimSpace(f[6])) {
		if p != "" {
			vo.Parents = append(vo.Parents, p)
		}
	}
	// 文件清单从 numstat 拿：show 默认不给，补一次带 --numstat 的查询比解析 diff 头更省。
	if ns, err := run(dir, statusTimeout, "show", "--no-color", "--numstat",
		"--pretty=format:", vo.Hash); err == nil {
		for _, line := range strings.Split(ns.out, "\n") {
			cols := strings.Split(line, "\t")
			if len(cols) < 3 {
				continue
			}
			path := unquotePath(filepath.ToSlash(strings.TrimSpace(cols[len(cols)-1])))
			if path == "" {
				continue
			}
			fv := domain.GitFileVO{Path: path, Status: domain.GitChangeModified}
			if a, err := strconv.Atoi(strings.TrimSpace(cols[0])); err == nil {
				fv.Additions = a
			}
			if d, err := strconv.Atoi(strings.TrimSpace(cols[1])); err == nil {
				fv.Deletions = d
			}
			vo.Files = append(vo.Files, fv)
		}
	}
	return vo, nil
}

// parseBranches 解析 branch -vv：`* main abc1234 [origin/main] 信息`。
func parseBranches(out string) []domain.GitBranchVO {
	list := make([]domain.GitBranchVO, 0, 8)
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cur := false
		// 无论是否当前分支都要先去前导空白：非当前分支行以两个空格开头，
		// 不去的话按空格切名字会得到空串，整条分支被静默丢掉。
		rest := strings.TrimLeft(line, " 	")
		if strings.HasPrefix(rest, "*") {
			cur = true
			rest = strings.TrimSpace(rest[1:])
		} else if strings.HasPrefix(rest, "+") {
			// `+` 表示该分支被别的 worktree 检出，同样不是当前分支。
			rest = strings.TrimSpace(rest[1:])
		}
		name, tail, _ := strings.Cut(rest, " ")
		if name == "" || strings.HasPrefix(name, "(") {
			continue
		}
		vo := domain.GitBranchVO{Name: name, Current: cur}
		// 方括号里是 `[上游] 领先/落后`；取不到就都是 0（没有上游的分支）。
		if i := strings.Index(tail, "["); i >= 0 {
			if j := strings.Index(tail[i:], "]"); j > 0 {
				vo.Ahead, vo.Behind = aheadBehindOf(tail[i+1 : i+j])
			}
		}
		list = append(list, vo)
	}
	return list
}

// aheadBehindOf 从 `origin/main: ahead 2, behind 1` 里取两个数；形态不认就返回 0,0。
func aheadBehindOf(s string) (int, int) {
	ahead, behind := 0, 0
	if i := strings.Index(s, "ahead "); i >= 0 {
		ahead = atoiPrefix(s[i+6:])
	}
	if i := strings.Index(s, "behind "); i >= 0 {
		behind = atoiPrefix(s[i+7:])
	}
	return ahead, behind
}

// validRef 限定提交号形态：十六进制串，长度 4..64。
// 不收 `--upload-pack=...` 这类看起来像选项的东西——它会被当成 ref 传给 git。
func validRef(s string) bool {
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// writeTimeout 写操作超时：commit 可能触发钩子，push 可能真的慢，都比读给得足。
const writeTimeout = 30 * time.Second

// pushTimeout 推送超时：远端慢时一分钟都算客气，超时报错让用户自己重试。
const pushTimeout = 60 * time.Second
