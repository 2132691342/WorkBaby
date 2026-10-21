// 版本历史与分支的只读查询：提交列表、单次提交内容、文件历史、分支清单。
// 全部实时走 git log / git branch，不另建一张提交表——换分支后历史整块变正是缓存不该存在的原因。
package git

import (
	"path/filepath"
	"strconv"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// maxCommits 单次列举的提交上限：翻页面 limit，一次全拉会把深历史的解析时间推到界面上。
const maxCommits = 200

// logSep 是 pretty 格式里的字段分隔符：提交信息里出现制表符的概率比出现这个串低得多。
const logSep = "\x1f"

// History 列出 dir 当前分支最近的提交，按时间倒序。
func History(dir string, limit int) ([]domain.GitCommitVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxCommits {
		limit = maxCommits
	}
	// 没有第一次提交时 git log 会失败；那不是错误，是这个仓库还没有历史。
	if !sc.hasHead() {
		return []domain.GitCommitVO{}, nil
	}
	args := []string{"log", "-n", strconv.Itoa(limit), "--date=unix",
		"--pretty=format:%H" + logSep + "%h" + logSep + "%s" + logSep + "%an" + logSep + "%at" + logSep,
		"--numstat"}
	args = append(args, sc.pathspec("")...)
	res, err := run(sc.root, statusTimeout, args...)
	if err != nil {
		return nil, err
	}
	return parseLog(res.stdout), nil
}

// CommitDetail 取一次提交的元信息、文件清单与逐行差异。
func CommitDetail(dir, hash string) (*domain.GitCommitDetailVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	if !validRef(hash) {
		return nil, pkg.New("1005", "提交号不合法", hash)
	}
	res, err := run(sc.root, statusTimeout, "show", "--no-color", "--no-ext-diff", "--date=unix",
		"--pretty=format:%H"+logSep+"%h"+logSep+"%s"+logSep+"%b"+logSep+"%an"+logSep+"%at"+logSep+"%P",
		hash)
	if err != nil {
		return nil, err
	}
	vo, err := parseShow(sc, res.stdout)
	if err != nil {
		return nil, err
	}
	// 差异复用既有解析：一条链路两处配色规则，改格式只改一处。
	// show 输出的路径是仓库根相对；历史抽屉展示的也是它，无需折回工作区。
	lines, dropped := parseDiff(res.stdout, nil)
	vo.Diff = &domain.GitDiffVO{Path: "", Lines: lines, Dropped: dropped, Truncated: dropped > 0}
	return vo, nil
}

// FileHistory 列出一个文件的提交历史，按时间倒序。
func FileHistory(dir, rel string, limit int) ([]domain.GitCommitVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	// 路径必须落进工作目录：改写与读取同一道路闸，这里也不例外。
	full, err := pkg.SafeJoin(dir, strings.TrimSpace(rel))
	if err != nil {
		return nil, err
	}
	wsRel, err := relTo(dir, full)
	if err != nil {
		return nil, pkg.New("1004", "路径越出工作目录", rel)
	}
	if limit <= 0 || limit > maxCommits {
		limit = maxCommits
	}
	if !sc.hasHead() {
		return []domain.GitCommitVO{}, nil
	}
	args := []string{"log", "-n", strconv.Itoa(limit), "--date=unix",
		"--pretty=format:%H" + logSep + "%h" + logSep + "%s" + logSep + "%an" + logSep + "%at" + logSep,
		"--numstat", "--", sc.relFromRoot(wsRel)}
	res, err := run(sc.root, statusTimeout, args...)
	if err != nil {
		return nil, err
	}
	return parseLog(res.stdout), nil
}

// Branches 列出本地与远端分支，标注当前所在与各自相对上游的领先 / 落后数。
func Branches(dir string) ([]domain.GitBranchVO, error) {
	sc, err := requireScope(dir)
	if err != nil {
		return nil, err
	}
	list := make([]domain.GitBranchVO, 0, 8)
	if res, err := run(sc.root, statusTimeout, "branch", "-vv", "--no-color"); err == nil {
		list = append(list, parseBranches(res.stdout)...)
	}
	// 远端分支单列：只列本地会让「拉别人的分支」这件事在界面上无处可去。
	if res, err := run(sc.root, statusTimeout, "branch", "-r", "--no-color"); err == nil {
		list = append(list, parseRemoteBranches(res.stdout)...)
	}
	return list, nil
}

// parseLog 解析 log + numstat 的输出：每个提交一段，段首是 pretty 行，随后是 numstat 行。
func parseLog(out string) []domain.GitCommitVO {
	commits := make([]domain.GitCommitVO, 0, 16)
	for _, line := range splitLines(out) {
		if !strings.Contains(line, logSep) {
			// numstat 行：add<TAB>del<TAB>path；二进制文件前两列是 `-`。
			if len(commits) == 0 {
				continue
			}
			cols := strings.Split(line, "\t")
			if len(cols) < 3 {
				continue
			}
			cur := &commits[len(commits)-1]
			cur.Files++
			if a, err := strconv.Atoi(strings.TrimSpace(cols[0])); err == nil {
				cur.Additions += a
			}
			if d, err := strconv.Atoi(strings.TrimSpace(cols[1])); err == nil {
				cur.Deletions += d
			}
			continue
		}
		f := strings.Split(line, logSep)
		if len(f) < 5 {
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
	}
	return commits
}

// parseShow 解析单次提交的元信息段（差异部分另走 parseDiff）。
func parseShow(sc *scope, out string) (*domain.GitCommitDetailVO, error) {
	head, _, _ := strings.Cut(out, "\ndiff --git")
	if !strings.Contains(head, logSep) {
		return nil, pkg.New("1015", "读不到这次提交的内容", "")
	}
	f := strings.Split(head, logSep)
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
		Parents: []string{}, Files: []domain.GitFileVO{},
	}
	for _, p := range strings.Fields(strings.TrimSpace(f[6])) {
		if p != "" {
			vo.Parents = append(vo.Parents, p)
		}
	}
	// 文件清单从 numstat 拿：show 默认不给，补一次带 --numstat 的查询比解析 diff 头更省。
	res, err := run(sc.root, statusTimeout, "show", "--no-color", "--numstat", "--pretty=format:", vo.Hash)
	if err != nil {
		return vo, nil
	}
	for _, line := range splitLines(res.stdout) {
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}
		path := sc.unquote(cols[len(cols)-1])
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
	return vo, nil
}

// parseBranches 解析 branch -vv：`* main abc1234 [origin/main] 信息`。
func parseBranches(out string) []domain.GitBranchVO {
	list := make([]domain.GitBranchVO, 0, 8)
	for _, line := range splitLines(out) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cur := false
		// 非当前分支行以两个空格开头，不去前缀按空格切名字会得到空串。
		rest := strings.TrimLeft(line, " \t")
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

// parseRemoteBranches 解析 branch -r：`origin/main`、`origin/HEAD -> origin/main`。
func parseRemoteBranches(out string) []domain.GitBranchVO {
	list := make([]domain.GitBranchVO, 0, 8)
	for _, line := range splitLines(out) {
		name := strings.TrimSpace(line)
		if name == "" || strings.Contains(name, "->") {
			continue // HEAD 指针别名不是可切换的分支
		}
		remote, _, ok := strings.Cut(name, "/")
		if !ok {
			continue
		}
		list = append(list, domain.GitBranchVO{Name: name, Remote: remote})
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

// relTo 把绝对路径折成相对 dir 的斜杠形式，与 status 报的路径同一套写法。
// 两侧先取真实路径：短名（RUNNER~1）与长名字面不等时 Rel 会算出 ../..，git 会判成越界。
func relTo(dir, full string) (string, error) {
	rel, err := filepath.Rel(pkg.RealPath(dir), pkg.RealPath(full))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}
