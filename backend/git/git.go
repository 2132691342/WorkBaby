// Package git 只读读取工作区的版本状态与差异：让界面能回答「它到底改了什么」。
// 只做读（status / diff），不代用户提交——写入版本历史是不可逆操作，不该由界面替他决定。
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
// 超了就得说清楚而不是把整文件吞进内存。
const maxUntrackedBytes = 8 << 20

// gitExeOnce / gitExe 缓存 git 可执行文件的解析结果：一次进程只解析一次，
// 状态页反复打开不再反复探盘。
var (
	gitExeOnce sync.Once
	gitExe     string
)

// resolveGit 解析 git 可执行文件：PATH 优先，其次常见安装位置（便携版与 winget 版不在 PATH 上）。
func resolveGit() string {
	gitExeOnce.Do(func() {
		if p, err := exec.LookPath("git"); err == nil {
			gitExe = p
			return
		}
		for _, cand := range gitCandidatePaths() {
			if pkg.FileExists(cand) {
				gitExe = cand
				return
			}
		}
	})
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
	// winget 的 Links 目录：`winget install Git.Git` 的落地位置
	if la := os.Getenv("LocalAppData"); la != "" {
		out = append(out, filepath.Join(la, "Microsoft", "WinGet", "Links", "git.exe"))
	}
	return out
}

// Available 报告系统里有没有可执行的 git。
func Available() bool { return resolveGit() != "" }

// insideWorkTree 探测 dir 是否位于 Git 工作树内；探测失败按「不是仓库」处理（两种情况界面动作相同）。
// 不看 git 的 128 原文：它同时覆盖「未初始化」与「仓库损坏」，且文案随系统语言变。
func insideWorkTree(dir string) (bool, error) {
	res, err := run(dir, statusTimeout, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		if strings.Contains(err.Error(), "超时") {
			return false, err
		}
		return false, nil
	}
	return strings.TrimSpace(res.out) == "true", nil
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
	return strings.TrimSpace(res.out)
}

// Status 读取 dir 的仓库状态；不是仓库或没装 git 时返回 Available/IsRepo 标记而不是错误——
// 这两件事的处置动作是「去装 / 去初始化」，界面要能原样说给用户。
func Status(dir string) (*domain.GitStatusVO, error) {
	vo := &domain.GitStatusVO{Workspace: dir, Files: []domain.GitFileVO{}}
	if !Available() {
		vo.Reason = "这台电脑上还没有 Git，装好后这里就能看到改动"
		return vo, nil
	}
	vo.Available = true
	vo.Git = Version()

	inside, err := insideWorkTree(dir)
	if err != nil {
		return nil, err
	}
	if !inside {
		vo.Reason = "这个工作目录还不是 Git 仓库，初始化后这里就能看到改动"
		return vo, nil
	}
	vo.IsRepo = true

	if res, err := run(dir, statusTimeout, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		vo.Branch = strings.TrimSpace(res.out)
	}
	vo.Ahead, vo.Behind = aheadBehind(dir)
	files, err := changedFiles(dir)
	if err != nil {
		return nil, err
	}
	vo.Files = files
	return vo, nil
}

// Diff 读取 dir 下 rel 文件的差异；rel 为空表示整个工作区。路径必须在 dir 之内。
func Diff(dir, rel string) (*domain.GitDiffVO, error) {
	if !Available() {
		return nil, pkg.New("1015", "这台电脑上还没有 Git，装好后就能看改动", "")
	}
	full := dir
	if strings.TrimSpace(rel) != "" {
		joined, err := pkg.SafeJoin(dir, rel)
		if err != nil {
			return nil, err
		}
		full = joined
		if !pkg.FileExists(full) && !isTracked(dir, rel) {
			return nil, pkg.New("1005", "文件不存在", rel)
		}
	} else if !pkg.DirExists(dir) {
		return nil, pkg.New("1003", "工作目录不存在", dir)
	}
	// 路径验完再探仓库：没初始化的目录上 git diff 只会甩一段本地化的 fatal，
	// 探一次换成一句人话，用户知道该去初始化。
	inside, err := insideWorkTree(dir)
	if err != nil {
		return nil, err
	}
	if !inside {
		return nil, pkg.New("1015", "这个工作目录还不是 Git 仓库，初始化后才能看改动", "")
	}

	vo := &domain.GitDiffVO{Path: rel}
	// 未跟踪的文件不在索引里，git diff 给不出内容：把正文整篇当作新增读出来。
	if strings.TrimSpace(rel) != "" && !isTracked(dir, rel) {
		return untrackedDiff(full, rel)
	}
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "-U3"}
	if hasHead(dir) {
		args = append(args, "HEAD")
	}
	// 路径段必须收尾：空 pathspec 会被 git 判为非法参数，整份差异会静默为空。
	args = append(args, "--")
	if strings.TrimSpace(rel) != "" {
		args = append(args, rel)
	} else {
		args = append(args, ".")
	}
	res, err := run(dir, diffTimeout, args...)
	if err != nil {
		return nil, err
	}
	lines, dropped := parseDiff(res.out)
	vo.Lines, vo.Dropped, vo.Truncated = lines, dropped, dropped > 0
	return vo, nil
}

// changedFiles 列出本目录下被改动的文件，并补上增删行数。
// 命令限定在当前目录：工作目录只是仓库的一角时，不该把仓库别处的改动算进来。
func changedFiles(dir string) ([]domain.GitFileVO, error) {
	res, err := run(dir, statusTimeout, "status", "--porcelain", "--", ".")
	if err != nil {
		return nil, err
	}
	counts := numstat(dir)
	out := make([]domain.GitFileVO, 0, 32)
	for _, line := range strings.Split(res.out, "\n") {
		if len(line) < 4 {
			continue
		}
		x, y, name := line[0], line[1], strings.TrimSpace(line[3:])
		name = unquotePath(name)
		if idx := strings.Index(name, " -> "); idx >= 0 {
			name = strings.TrimSpace(name[idx+4:])
		}
		staged, status := classify(x, y)
		if status == "" {
			continue
		}
		rel := filepath.ToSlash(name)
		out = append(out, domain.GitFileVO{
			Path:      rel,
			Status:    status,
			Staged:    staged,
			Additions: counts[rel].add,
			Deletions: counts[rel].del,
		})
	}
	return out, nil
}

// classify 把 porcelain 的两位状态码折成一个界面标签与「是否已进待提交区」。
func classify(x, y byte) (staged bool, status string) {
	if x == '?' && y == '?' {
		return false, domain.GitChangeUntracked
	}
	if x != ' ' && x != '?' {
		return true, string(rune(x))
	}
	return false, string(rune(y))
}

type counters struct{ add, del int }

// numstat 取各文件的增删行数；二进制文件那两列是 `-`，按 0 计。
// 按 tab 切分而不是按空白：路径里带空格是常态，Fields 会把路径切碎。
func numstat(dir string) map[string]counters {
	counts := make(map[string]counters)
	args := []string{"diff", "--numstat"}
	if hasHead(dir) {
		args = append(args, "HEAD")
	}
	args = append(args, "--", ".")
	res, err := run(dir, statusTimeout, args...)
	if err != nil {
		return counts
	}
	for _, line := range strings.Split(res.out, "\n") {
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}
		path := unquotePath(filepath.ToSlash(strings.TrimSpace(cols[len(cols)-1])))
		add, _ := strconv.Atoi(strings.TrimSpace(cols[0]))
		del, _ := strconv.Atoi(strings.TrimSpace(cols[1]))
		counts[path] = counters{add: add, del: del}
	}
	return counts
}

// aheadBehind 与上游分支的领先 / 落后数；没有上游时两者都是 0。
func aheadBehind(dir string) (int, int) {
	if !hasHead(dir) {
		return 0, 0
	}
	res, err := run(dir, statusTimeout, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(res.out)
	if len(fields) != 2 {
		return 0, 0
	}
	ahead, _ := strconv.Atoi(fields[0])
	behind, _ := strconv.Atoi(fields[1])
	return ahead, behind
}

// hasHead 报告仓库有没有第一次提交：没有 HEAD 的仓库 `git diff HEAD` 会直接失败。
func hasHead(dir string) bool {
	res, err := run(dir, statusTimeout, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return false
	}
	return strings.TrimSpace(res.out) != ""
}

// isTracked 判断 rel 是否已被 Git 跟踪。
func isTracked(dir, rel string) bool {
	res, err := run(dir, statusTimeout, "ls-files", "--error-unmatch", "--", rel)
	return err == nil && strings.TrimSpace(res.out) != ""
}

// untrackedDiff 把还没被跟踪的文件整篇当作新增行读出来。
func untrackedDiff(full, rel string) (*domain.GitDiffVO, error) {
	info, err := os.Stat(full)
	if err != nil {
		return nil, pkg.Wrap("1005", "读取文件失败", err)
	}
	// 读入前先看大小：os.ReadFile 是整文件进内存，超限要给出可执行的替代路径
	if info.Size() > maxUntrackedBytes {
		return nil, pkg.New("1013", "这个新文件太大（超过 8MB），这里看不了全文，直接用编辑器打开它", rel)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, pkg.Wrap("1005", "读取文件失败", err)
	}
	vo := &domain.GitDiffVO{Path: rel}
	vo.Lines = append(vo.Lines, domain.GitDiffLineVO{Kind: domain.GitLineFile, Text: rel})
	total := 0
	lines := strings.Split(string(data), "\n")
	for i, text := range lines {
		if i == len(lines)-1 && text == "" {
			break
		}
		total++
		if len(vo.Lines) >= maxLines {
			continue
		}
		vo.Lines = append(vo.Lines, domain.GitDiffLineVO{Kind: domain.GitLineAdd, NewNo: i + 1, Text: text})
	}
	vo.Dropped = total - (len(vo.Lines) - 1)
	vo.Truncated = vo.Dropped > 0
	return vo, nil
}

// parseDiff 把统一差异文本折成行结构；超出上限的行被丢掉并计入 dropped。
func parseDiff(text string) ([]domain.GitDiffLineVO, int) {
	out := make([]domain.GitDiffLineVO, 0, 128)
	oldNo, newNo, dropped := 0, 0, 0
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineFile, Text: diffPath(line)})
			continue
		case strings.HasPrefix(line, "@@"):
			oldNo, newNo = hunkPos(line)
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineHunk, Text: line})
			continue
		case strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") ||
			strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "new file ") ||
			strings.HasPrefix(line, "deleted file ") || strings.HasPrefix(line, "similarity ") ||
			strings.HasPrefix(line, "rename "):
			continue
		}
		if len(out) >= maxLines {
			dropped++
			continue
		}
		// @@ 头给出的就是首行行号，先落号再推进，否则整块行号整体偏移一行。
		body := line
		switch {
		case strings.HasPrefix(line, "+"):
			body = line[1:]
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineAdd, NewNo: newNo, Text: body})
			newNo++
		case strings.HasPrefix(line, "-"):
			body = line[1:]
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineDel, OldNo: oldNo, Text: body})
			oldNo++
		default:
			if len(line) > 0 {
				body = line[1:]
			}
			out = append(out, domain.GitDiffLineVO{Kind: domain.GitLineCtx, OldNo: oldNo, NewNo: newNo, Text: body})
			oldNo++
			newNo++
		}
	}
	// 扫描提前中止（超长行）必须算进丢弃数：静默丢掉的差异比截断更糟
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
	oldNo := atoiPrefix(strings.TrimPrefix(fields[1], "-"))
	newNo := atoiPrefix(strings.TrimPrefix(fields[2], "+"))
	return oldNo, newNo
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

// unquotePath 去掉 git 因空格 / 非 ASCII 给路径加的引号与转义。
func unquotePath(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 1 && s[0] == '"' && s[len(s)-1] == '"' {
		if unquoted, err := strconv.Unquote(s); err == nil {
			return unquoted
		}
		return s[1 : len(s)-1]
	}
	return s
}

// result 是一次 git 调用的输出；code 是退出码，errOut 留给调用方判断失败原因。
type result struct {
	out    string
	errOut string
	code   int
}

// run 执行一条 git 只读命令。「是不是仓库」由调用方先探（insideWorkTree）；探过后仍出现的 128
// 按仓库损坏报，原文进 details（git 的 128 覆盖多种成因且错误原文随系统语言变，不能按原文分流）。
func run(dir string, timeout time.Duration, args ...string) (result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// 解析出的绝对路径优先；没解析到（没装）时退回 "git"，让标准错误形态不变——
	// 调用方在到达这里之前都已用 Available 挡过。
	name := "git"
	if exe := resolveGit(); exe != "" {
		name = exe
	}
	cmd := exec.CommandContext(ctx, name, args...)
	hideConsole(cmd)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := result{out: stdout.String(), errOut: stderr.String()}
	if err == nil {
		return res, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		res.code = ee.ExitCode()
	}
	if ctx.Err() != nil {
		return res, pkg.Wrap("1015", "读取版本信息超时，仓库可能太大", err)
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = err.Error()
	}
	return res, pkg.Wrap("1015", "读取版本信息失败："+msg, err)
}
