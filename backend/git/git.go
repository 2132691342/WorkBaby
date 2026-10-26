// Package git 读写工作区的版本状态：让界面能回答「它到底改了什么」并能把改动收成一次提交。
// 读全开放（status / diff / 历史），写限白名单动作，且每种都由用户点下去。
package git

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 命令超时：仓库再大也只给这么久，界面不能为一个状态卡住。
const (
	statusTimeout = 10 * time.Second
	diffTimeout   = 15 * time.Second
)

// maxLines 差异行数上限：一个几万行的生成文件会把前端渲染拖死，行数先兜住。
const maxLines = 3000

// maxLineBytes 单行上限：minified 的一行可能上兆，Scanner 不设上限就是整行进内存。
const maxLineBytes = 256 * 1024

// maxUntrackedBytes 未跟踪文件的读取上限：Git 给不出它的差异，只能读正文，
// 超了就得说清楚而不是把整文件闷进内存。
const maxUntrackedBytes = 8 << 20

// gitExeMu / gitExe 缓存 git 可执行文件的解析结果：一次进程只解析一次，
// 状态页反复打开不再反复探盘。装配期注入的那份优先（与助手用的是同一份）。
var (
	gitExeMu sync.Mutex
	gitExe   string
	gitDone  bool
)

// SetExe 注入当前生效的 git：助手用它跑命令，界面也用它读状态，两处不能各挑一份。
func SetExe(exe string) {
	if exe == "" {
		return
	}
	gitExeMu.Lock()
	gitExe, gitDone = exe, true
	gitExeMu.Unlock()
}

// resolveGit 解析 git 可执行文件：注入的那份优先，其次 PATH 与常见安装位置。
func resolveGit() string {
	gitExeMu.Lock()
	defer gitExeMu.Unlock()
	if gitDone {
		return gitExe
	}
	if p, err := exec.LookPath("git"); err == nil {
		gitExe = p
		gitDone = true
		return gitExe
	}
	for _, cand := range gitCandidatePaths() {
		if pkg.FileExists(cand) {
			gitExe = cand
			break
		}
	}
	gitDone = true
	return gitExe
}

// gitCandidatePaths 常见安装位置（按环境变量拼，不写死盘符）。
func gitCandidatePaths() []string {
	out := make([]string, 0, 4)
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
		if root := os.Getenv(env); root != "" {
			out = append(out, filepath.Join(root, "Git", "cmd", "git.exe"))
		}
	}
	if la := os.Getenv("LocalAppData"); la != "" {
		out = append(out, filepath.Join(la, "Microsoft", "WinGet", "Links", "git.exe"))
	}
	return out
}

// Available 报告系统里有没有可执行的 git。
func Available() bool { return resolveGit() != "" }

// insideWorkTree 探测 dir 是否位于 Git 工作树内；探测失败按「不是仓库」处理（两种情况界面动作相同）。
func insideWorkTree(dir string) (bool, error) {
	res, err := run(dir, statusTimeout, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if isTimeout(err) {
			return false, err
		}
		return false, nil
	}
	return strings.TrimSpace(res.stdout) == "true", nil
}

// Version 返回 git 版本串；不可用返回空串。
func Version() string {
	if !Available() {
		return ""
	}
	res, err := run("", statusTimeout, "--version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(res.stdout)
}

// Status 读取 dir 的仓库状态；不是仓库或没装 git 时返回 Available/IsRepo 标记而不是错误。
func Status(dir string) (*domain.GitStatusVO, error) {
	vo := &domain.GitStatusVO{
		Workspace: dir,
		State:     domain.GitStateNone,
		Remotes:   []domain.GitRemoteVO{},
		Files:     []domain.GitFileVO{},
	}
	if !Available() {
		vo.Reason = "这台电脑上还没有 Git，装好后这里就能看到改动"
		return vo, nil
	}
	vo.Available = true
	vo.Git = Version()

	scope, err := openScope(dir)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		vo.Reason = "这个工作目录还不是 Git 仓库，初始化后这里就能看到改动"
		return vo, nil
	}
	vo.IsRepo = true
	vo.RepoRoot = scope.root
	vo.Branch = scope.branch()
	vo.Upstream = scope.upstream()
	vo.Remotes = scope.remotes()
	st := scope.state()
	vo.State = st.Kind
	vo.StateDetail = st.Detail
	vo.Ahead, vo.Behind = scope.aheadBehind()
	files, err := scope.changedFiles()
	if err != nil {
		return nil, err
	}
	vo.Files = files
	for _, f := range files {
		if f.Conflict {
			vo.Conflict = true
			break
		}
	}
	return vo, nil
}

// Diff 读取 dir 工作的 rel 文件差异；rel 为空表示整个工作区。路径必须落在 dir 之内。
func Diff(dir, rel string) (*domain.GitDiffVO, error) {
	if !Available() {
		return nil, pkg.New("1015", "这台电脑上还没有 Git，装好后就能看改动", "")
	}
	scope, err := openScope(dir)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		return nil, pkg.New("1015", "这个工作目录还不是 Git 仓库，初始化后才能看改动", "")
	}

	rel = strings.TrimSpace(rel)
	if rel == "" {
		if !pkg.DirExists(dir) {
			return nil, pkg.New("1003", "工作目录不存在", dir)
		}
		return scope.diffAll()
	}

	full, err := pkg.SafeJoin(dir, rel)
	if err != nil {
		return nil, err
	}
	inside, err := relTo(dir, full)
	if err != nil {
		return nil, pkg.New("1004", "路径越出工作目录", rel)
	}
	// git 的路径参数是仓库根相对，界面给的是工作区相对：两者之差就是工作区前缀。
	fromRoot := scope.relFromRoot(inside)
	if !pkg.FileExists(full) && !scope.isTracked(fromRoot) {
		return nil, pkg.New("1005", "文件不存在", rel)
	}
	return scope.diffOne(full, rel, fromRoot)
}

// scope 是一次 git 读写的定位结果：仓库根 + 工作区前缀。所有命令都在仓库根执行、
// 路径参数都是仓库根相对——界面上的 path 在 status / diff / add 之间是同一个东西。
type scope struct {
	// root 仓库根目录（git 的 top-level）
	root string
	// prefix 工作区相对仓库根的斜杠路径；工作区就是仓库根时为空
	prefix string
	// quotePath 仓库是否对非 ASCII 路径做八进制转义（core.quotepath）
	quotePath bool
}

// openScope 解析 dir 所在的仓库根与工作区前缀；不在仓库里返回 (nil, nil)。
func openScope(dir string) (*scope, error) {
	res, err := run(dir, statusTimeout, "rev-parse", "--show-toplevel", "--show-prefix")
	if err != nil {
		if isTimeout(err) {
			return nil, err
		}
		return nil, nil
	}
	// 不能用 splitLines：工作区就是仓库根时 --show-prefix 输出空行，
	// 去掉空行后只剩一半，于是根目录反而被判成「不是仓库」。
	parts := strings.Split(strings.ReplaceAll(res.stdout, "\r\n", "\n"), "\n")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return nil, nil
	}
	sc := &scope{
		root:      filepath.FromSlash(filepath.Clean(strings.TrimSpace(parts[0]))),
		quotePath: true,
	}
	if len(parts) > 1 {
		sc.prefix = strings.Trim(strings.TrimSpace(filepath.ToSlash(parts[1])), "/")
	}
	if q, err := run(sc.root, statusTimeout, "config", "--get", "core.quotepath"); err == nil {
		if strings.TrimSpace(q.stdout) == "false" {
			sc.quotePath = false
		}
	}
	return sc, nil
}

// relFromRoot 把工作区相对的界面上报路径换成仓库根相对的 git pathspec；
// 工作区就是仓库根时原样返回。空串表示整个工作区。
func (s *scope) relFromRoot(wsRel string) string {
	wsRel = strings.Trim(strings.TrimSpace(filepath.ToSlash(wsRel)), "/")
	if s.prefix == "" {
		return wsRel
	}
	if wsRel == "" || wsRel == "." {
		return s.prefix
	}
	return s.prefix + "/" + wsRel
}

// relToWorkspace 把仓库根相对的路径折回界面上报的工作区相对形式。
// 落在工作区之外的文件不上报——那些不是这个工作区的改动。
func (s *scope) relToWorkspace(rootRel string) (string, bool) {
	rootRel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rootRel)), "/")
	if s.prefix == "" {
		return rootRel, true
	}
	if rootRel == s.prefix {
		return "", true
	}
	rest, ok := strings.CutPrefix(rootRel, s.prefix+"/")
	if !ok {
		return "", false
	}
	return rest, true
}

// branch 当前分支名；还没有第一次提交时返回空串。
func (s *scope) branch() string {
	res, err := run(s.root, statusTimeout, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(res.stdout)
	if name == "HEAD" {
		return ""
	}
	return name
}

// upstream 当前分支的上游引用；没有上游返回空串。
func (s *scope) upstream() string {
	res, err := run(s.root, statusTimeout, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(res.stdout)
}

// remotes 列出全部远端及其推送地址；没有远端返回空数组。
func (s *scope) remotes() []domain.GitRemoteVO {
	res, err := run(s.root, statusTimeout, "remote", "-v")
	if err != nil {
		return []domain.GitRemoteVO{}
	}
	out := make([]domain.GitRemoteVO, 0, 2)
	seen := map[string]bool{}
	for _, line := range splitLines(res.stdout) {
		name, url, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		// `remote -v` 给每行都续一个 ` (fetch)` / ` (push)` 标记，地址里不该带它。
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if i := strings.LastIndex(url, " ("); i > 0 {
			url = url[:i]
		}
		out = append(out, domain.GitRemoteVO{Name: name, URL: strings.TrimSpace(url)})
	}
	return out
}

// remoteNames 只要远端名，供推送推断目标用。
func (s *scope) remoteNames() []string {
	list := s.remotes()
	names := make([]string, 0, len(list))
	for _, r := range list {
		names = append(names, r.Name)
	}
	return names
}

// repoState 是仓库当前所处的中间状态：普通 / 合并中 / 变基中 / 拣选或回退中。
type repoState struct {
	Kind   string
	Detail string
}

// state 判定仓库是否有未完成的合并或变基；判定靠 git 自己，不靠猜目录里的文件。
func (s *scope) state() repoState {
	gitDir := s.gitDir()
	if gitDir == "" {
		return repoState{Kind: domain.GitStateNone}
	}
	if pkg.DirExists(filepath.Join(gitDir, "rebase-merge")) ||
		pkg.DirExists(filepath.Join(gitDir, "rebase-apply")) {
		return repoState{Kind: domain.GitStateRebase, Detail: s.readText(filepath.Join(gitDir, "rebase-merge", "head-name"))}
	}
	if pkg.FileExists(filepath.Join(gitDir, "MERGE_HEAD")) {
		return repoState{Kind: domain.GitStateMerge, Detail: s.readText(filepath.Join(gitDir, "MERGE_MSG"))}
	}
	if pkg.FileExists(filepath.Join(gitDir, "CHERRY_PICK_HEAD")) {
		return repoState{Kind: domain.GitStateCherryPick}
	}
	if pkg.FileExists(filepath.Join(gitDir, "REVERT_HEAD")) {
		return repoState{Kind: domain.GitStateRevert}
	}
	return repoState{Kind: domain.GitStateNone}
}

// gitDir 解析 .git 的真实路径（工作区可能是子目录，也可能是 worktree）。
func (s *scope) gitDir() string {
	res, err := run(s.root, statusTimeout, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(res.stdout)
}

// readText 读一个小文本文件的首行，读不到返回空串。
func (s *scope) readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line)
}

// changedFiles 列出工作区下被改动的文件，并补上增删行数。
func (s *scope) changedFiles() ([]domain.GitFileVO, error) {
	args := append([]string{"status", "--porcelain", "-z"}, s.pathspec("")...)
	res, err := run(s.root, statusTimeout, args...)
	if err != nil {
		return nil, err
	}
	counts := s.numstat()
	out := make([]domain.GitFileVO, 0, 32)
	for _, entry := range parsePorcelainZ(res.stdout) {
		if entry.path == "" {
			continue
		}
		rel, ok := s.relToWorkspace(entry.path)
		if !ok {
			continue
		}
		c := counts[entry.path]
		out = append(out, domain.GitFileVO{
			Path:      rel,
			Status:    entry.status,
			Staged:    entry.staged,
			Conflict:  entry.conflict,
			Additions: c.add,
			Deletions: c.del,
		})
	}
	return out, nil
}

// unquote 去掉 git 给非 ASCII / 带空格路径加的引号与八进制转义。
// 只有 core.quotepath 打开时才会加引号（默认打开），关掉时路径原样。
func (s *scope) unquote(raw string) string {
	p := filepath.ToSlash(strings.TrimSpace(raw))
	if !s.quotePath || len(p) < 2 || p[0] != '"' || p[len(p)-1] != '"' {
		return p
	}
	if unquoted, err := strconv.Unquote(p); err == nil {
		return filepath.ToSlash(unquoted)
	}
	return p[1 : len(p)-1]
}

// headerToWorkspace 把差异文件头里的仓库根相对路径折回工作区相对；落在工作区外的原样保留。
func (s *scope) headerToWorkspace(rootRel string) string {
	if rel, ok := s.relToWorkspace(rootRel); ok {
		return rel
	}
	return rootRel
}

// pathspec 拼出「限定在当前工作区内」的 pathspec 段。
func (s *scope) pathspec(rootRel string) []string {
	target := s.relFromRoot(rootRel)
	if target == "" {
		target = "."
	}
	return []string{"--", target}
}

type counters struct{ add, del int }

// numstat 取各文件的增删行数；二进制文件那两列是 `-`，按 0 计。
// 按 tab 切而不是按空白：路径里带空格是常态，Fields 会把路径切碎。
func (s *scope) numstat() map[string]counters {
	counts := make(map[string]counters)
	args := []string{"diff", "--numstat"}
	if s.hasHead() {
		args = append(args, "HEAD")
	}
	args = append(args, s.pathspec("")...)
	res, err := run(s.root, statusTimeout, args...)
	if err != nil {
		return counts
	}
	for _, line := range splitLines(res.stdout) {
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}
		path := s.unquote(cols[len(cols)-1])
		add, _ := strconv.Atoi(strings.TrimSpace(cols[0]))
		del, _ := strconv.Atoi(strings.TrimSpace(cols[1]))
		counts[path] = counters{add: add, del: del}
	}
	return counts
}

// diffAll 整个工作区的差异。未跟踪文件不在索引里，git diff 给不出内容，逐个补读正文。
func (s *scope) diffAll() (*domain.GitDiffVO, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "-U3"}
	if s.hasHead() {
		args = append(args, "HEAD")
	}
	args = append(args, s.pathspec("")...)
	res, err := run(s.root, statusTimeout, args...)
	if err != nil {
		return nil, err
	}
	lines, dropped := parseDiff(res.stdout, s.headerToWorkspace)
	total := dropped

	// 未跟踪文件整篇按新增读出来，否则「新建的文件」在这一页永远看不到自己。
	for _, st := range s.untrackedInScope() {
		if len(lines) >= maxLines {
			total++
			continue
		}
		rel, ok := s.relToWorkspace(st)
		if !ok {
			continue
		}
		add, rest := s.untrackedSection(rel, st)
		lines = append(lines, add...)
		total += rest
	}
	return &domain.GitDiffVO{Path: "", Lines: lines, Dropped: total, Truncated: total > 0}, nil
}

// diffOne 单个文件的差异；未跟踪文件同样补读正文。
func (s *scope) diffOne(full, wsRel, rootRel string) (*domain.GitDiffVO, error) {
	if !s.isTracked(rootRel) {
		lines, dropped := s.untrackedSection(wsRel, rootRel)
		return &domain.GitDiffVO{Path: wsRel, Lines: lines, Dropped: dropped, Truncated: dropped > 0}, nil
	}
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "-U3"}
	if s.hasHead() {
		args = append(args, "HEAD")
	}
	args = append(args, "--", rootRel)
	res, err := run(s.root, statusTimeout, args...)
	if err != nil {
		return nil, err
	}
	lines, dropped := parseDiff(res.stdout, s.headerToWorkspace)
	return &domain.GitDiffVO{Path: wsRel, Lines: lines, Dropped: dropped, Truncated: dropped > 0}, nil
}

// untrackedInScope 列出工作区内未跟踪且未被忽略的文件（仓库根相对路径）。
func (s *scope) untrackedInScope() []string {
	args := append([]string{"ls-files", "--others", "--exclude-standard", "-z"}, s.pathspec("")...)
	res, err := run(s.root, statusTimeout, args...)
	if err != nil {
		return nil
	}
	out := make([]string, 0, 8)
	for _, p := range strings.Split(res.stdout, "\x00") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, filepath.ToSlash(p))
		}
	}
	return out
}

// untrackedSection 把一个未跟踪文件读成「整篇新增」的行块；返回行块与被丢掉的行数。
func (s *scope) untrackedSection(wsRel, rootRel string) ([]domain.GitDiffLineVO, int) {
	full := filepath.Join(s.root, filepath.FromSlash(rootRel))
	info, err := os.Stat(full)
	if err != nil {
		return nil, 0
	}
	// 目录与超大文件都不读正文：前者要展开成许多文件，后者会把内存吃掉。
	if info.IsDir() {
		return nil, 0
	}
	if info.Size() > maxUntrackedBytes {
		return nil, 1
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, 0
	}
	out := []domain.GitDiffLineVO{{Kind: domain.GitLineFile, Text: wsRel}}
	dropped := 0
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rows := strings.Split(text, "\n")
	for i, row := range rows {
		if i == len(rows)-1 && row == "" {
			break
		}
		if i >= maxLines {
			dropped++
			continue
		}
		out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineAdd, NewNo: i + 1, Text: row})
	}
	return out, dropped
}

// aheadBehind 与上游分支的领先 / 落后数；没有上游时两者都是 0。
func (s *scope) aheadBehind() (int, int) {
	if !s.hasHead() {
		return 0, 0
	}
	res, err := run(s.root, statusTimeout, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(res.stdout)
	if len(fields) != 2 {
		return 0, 0
	}
	ahead, _ := strconv.Atoi(fields[0])
	behind, _ := strconv.Atoi(fields[1])
	return ahead, behind
}

// hasHead 报告仓库有没有第一次提交：没有 HEAD 的仓库 `git diff HEAD` 会直接失败。
func (s *scope) hasHead() bool {
	res, err := run(s.root, statusTimeout, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return false
	}
	return strings.TrimSpace(res.stdout) != ""
}

// isTracked 判断仓库根相对路径是否已被 Git 跟踪。
func (s *scope) isTracked(rootRel string) bool {
	res, err := run(s.root, statusTimeout, "ls-files", "--error-unmatch", "--", rootRel)
	return err == nil && strings.TrimSpace(res.stdout) != ""
}

// porcelainEntry 是 porcelain -z 里的一条记录。
type porcelainEntry struct {
	path     string // 仓库根相对路径
	status   string // 取 domain.GitChange*
	staged   bool
	conflict bool
}

// parsePorcelainZ 解析 `status --porcelain -z`：NUL 分隔不做路径转义（中文空格路径免反解）；
// 改名记录占两条 NUL 段（新路径在前、原路径在后），原路径要跳过。
func parsePorcelainZ(raw string) []porcelainEntry {
	parts := strings.Split(raw, "\x00")
	out := make([]porcelainEntry, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		seg := parts[i]
		if len(seg) < 4 {
			continue
		}
		x, y, name := seg[0], seg[1], seg[3:]
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			i++ // 下一条是原路径，不是独立改动
		}
		staged, status, conflict := classify(x, y)
		if status == "" {
			continue
		}
		out = append(out, porcelainEntry{path: filepath.ToSlash(name), status: status, staged: staged, conflict: conflict})
	}
	return out
}

// unmergedCodes 是 porcelain 里七种未合并状态码，全部意味着「这个文件有冲突没解决」。
var unmergedCodes = map[string]bool{
	"DD": true, "AU": true, "UD": true, "UA": true, "DU": true, "AA": true, "UU": true,
}

// classify 把 porcelain 的两位状态码折成一个界面标签、是否已进待提交区、是否冲突。
func classify(x, y byte) (staged bool, status string, conflict bool) {
	if x == '?' && y == '?' {
		return false, domain.GitChangeUntracked, false
	}
	if x == '!' && y == '!' {
		return false, "", false
	}
	if unmergedCodes[string([]byte{x, y})] {
		return true, domain.GitChangeConflict, true
	}
	if x != ' ' && x != '?' {
		return true, string(rune(x)), false
	}
	return false, string(rune(y)), false
}

// parseDiff 把统一差异文本折成行结构；超出上限的行被丢掉并计入 dropped。
// fileHeader 把 `diff --git` 行里的仓库根相对路径折回工作区相对（工作区就是仓库根时原样）。
func parseDiff(text string, fileHeader func(string) string) ([]domain.GitDiffLineVO, int) {
	out := make([]domain.GitDiffLineVO, 0, 128)
	oldNo, newNo, dropped := 0, 0, 0
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			header := diffPath(line)
			if fileHeader != nil {
				header = fileHeader(header)
			}
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineFile, Text: header})
			continue
		case strings.HasPrefix(line, "@@"):
			oldNo, newNo = hunkPos(line)
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineHunk, Text: line})
			continue
		case strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") ||
			strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "new file ") ||
			strings.HasPrefix(line, "deleted file ") || strings.HasPrefix(line, "similarity ") ||
			strings.HasPrefix(line, "rename ") || strings.HasPrefix(line, "old mode ") ||
			strings.HasPrefix(line, "new mode ") || strings.HasPrefix(line, "Binary files "):
			continue
		}
		if len(out) >= maxLines {
			dropped++
			continue
		}
		// @@ 头给出的就是首行行号，先落号再推进，否则整块行号偏移一行。
		switch {
		case strings.HasPrefix(line, "+"):
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineAdd, NewNo: newNo, Text: line[1:]})
			newNo++
		case strings.HasPrefix(line, "-"):
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineDel, OldNo: oldNo, Text: line[1:]})
			oldNo++
		case strings.HasPrefix(line, "\\"):
			// `\ No newline at end of file` 是注解，不是内容行
			continue
		default:
			body := line
			if len(line) > 0 {
				body = line[1:]
			}
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineCtx, OldNo: oldNo, NewNo: newNo, Text: body})
			oldNo++
			newNo++
		}
	}
	// 扫描提前中止（超长行）必须算进丢弃数：静默丢掉的差异比截断更糟。
	if err := sc.Err(); err != nil {
		dropped++
	}
	return out, dropped
}

// hunkPos 解析 @@ -a,b +c,d @@ 拿到两侧起始行号。
func hunkPos(line string) (int, int) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return 0, 0
	}
	return atoiPrefix(strings.TrimPrefix(fields[1], "-")), atoiPrefix(strings.TrimPrefix(fields[2], "+"))
}

// atoiPrefix 取 "12,3" 里的起始行号。
func atoiPrefix(s string) int {
	if i := strings.Index(s, ","); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}

// diffPath 从 `diff --git a/x b/x` 里取改动后的路径。
func diffPath(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return strings.TrimSpace(line)
	}
	return strings.TrimPrefix(fields[len(fields)-1], "b/")
}

// splitLines 按 LF 切行并去掉末尾空行与 CR。
func splitLines(s string) []string {
	rows := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r = strings.TrimSuffix(r, "\r"); r != "" || len(out) > 0 {
			out = append(out, r)
		}
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// result 是一次 git 调用的输出；code 是退出码，committed 只对写命令有意义。
type result struct {
	stdout string
	stderr string
	code   int
}

// output 合并 stdout 与 stderr 供错误归因：git 把大部分失败原因写 stdout
// （`nothing to commit`、`no upstream branch` 都在 stdout），只看 stderr 会退化成 `exit status 1`。
func (r result) output() string {
	parts := make([]string, 0, 2)
	if s := strings.TrimSpace(r.stderr); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(r.stdout); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}

// failed 报告 git 到底有没有跑起来并返回了非零退出码。
// 退出码 0 而 err 非空（超时 / 起不了进程）不是「git 拒绝了这个操作」，不能当 1019 归因。
func (r result) failed() bool { return r.code != 0 }

// run 执行一条 git 命令。「是不是仓库」由调用方先探（openScope）；
// 探过之后仍出现的 128 按仓库损坏报，原文进 details。
func run(dir string, timeout time.Duration, args ...string) (result, error) {
	return runEnv(dir, timeout, nil, args...)
}

// runEnv 同 run，但可以追加环境变量（提交时补身份、推送时禁止交互式提问）。
func runEnv(dir string, timeout time.Duration, env []string, args ...string) (result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// 解析出的绝对路径优先；没解析到（没装）时退回 "git"，让标准错误形态不变。
	name := "git"
	if exe := resolveGit(); exe != "" {
		name = exe
	}
	cmd := exec.CommandContext(ctx, name, args...)
	hideConsole(cmd)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := result{stdout: stdout.String(), stderr: stderr.String()}
	if err == nil {
		return res, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		res.code = ee.ExitCode()
	}
	if ctx.Err() != nil {
		return res, pkg.Wrap("1015", "读取版本信息超时，仓库可能太大", err)
	}
	msg := res.output()
	if msg == "" {
		msg = err.Error()
	}
	return res, pkg.Wrap("1015", "读取版本信息失败："+firstLine(msg), err)
}

// isTimeout 判断错误是不是超时（超时已经是 1015，不该被当成「不是仓库」吞掉）。
func isTimeout(err error) bool {
	return strings.Contains(err.Error(), "超时")
}

// firstLine 取一段输出的首行：完整输出可能有几十行，toast 里放不下。
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}
