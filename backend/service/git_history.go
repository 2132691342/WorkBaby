package service

import (
	"strconv"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/parser"
)

// parseDiffShared 是 git 与工具结果共用的差异切分入口。
func parseDiffShared(text string) []domain.DiffLineVO {
	return parser.ParseDiff(text)
}

// History 提交历史。
func (g *Git) History(limit int) ([]domain.GitCommitVO, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	// 用不可见分隔符排版：git 的格式化输出没有 JSON 模式，只能自己切。
	const sep = "\x1f"
	format := strings.Join([]string{
		"%H", "%h", "%s", "%an", "%at", "%D",
	}, sep)
	text, err := g.runOrFail("log", "-n", strconv.Itoa(limit),
		"--numstat", "--pretty=format:"+format+sep+"%b"+sep+"%P")
	if err != nil {
		return nil, err
	}
	return parseHistory(text, sep), nil
}

// parseHistory 把 `git log --numstat` 的输出切成提交摘要。
func parseHistory(text, sep string) []domain.GitCommitVO {
	out := make([]domain.GitCommitVO, 0, 64)
	var current *domain.GitCommitVO
	for _, line := range pkg.SplitLines(text) {
		switch {
		case strings.Count(line, sep) >= 5:
			fields := strings.Split(line, sep)
			if len(fields) < 5 {
				continue
			}
			commit := domain.GitCommitVO{
				CommitHash:    fields[0],
				ShortHash:     fields[1],
				CommitSubject: fields[2],
				AuthorName:    fields[3],
				CommitTime:    int64(atoiSafe(fields[4])) * 1000,
			}
			out = append(out, commit)
			current = &out[len(out)-1]
		case current != nil && isNumStat(line):
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) < 3 {
				continue
			}
			current.FileCount++
			current.Insertions += atoiSafe(fields[0])
			current.Deletions += atoiSafe(fields[1])
		}
	}
	return out
}

// isNumStat 判断一行是不是 `新增 删除 路径` 的统计行。
func isNumStat(line string) bool {
	fields := strings.SplitN(line, "\t", 3)
	if len(fields) < 3 {
		return false
	}
	if fields[0] != "-" && atoiSafe(fields[0]) == 0 && fields[0] != "0" {
		return false
	}
	return true
}

// CommitDetail 单个提交的完整内容。
func (g *Git) CommitDetail(hash string) (domain.GitCommitDetailRESP, error) {
	if hash == "" {
		return domain.GitCommitDetailRESP{}, pkg.Err(pkg.CodeBadRequest, "提交哈希不能为空")
	}
	const sep = "\x1f"
	format := strings.Join([]string{"%H", "%h", "%s", "%an", "%at", "%P"}, sep)
	meta, err := g.runOrFail("show", "-s", "--pretty=format:"+format+sep+"%b", hash)
	if err != nil {
		return domain.GitCommitDetailRESP{}, err
	}
	fields := strings.Split(meta, sep)
	if len(fields) < 5 {
		return domain.GitCommitDetailRESP{}, pkg.Err(pkg.CodeGitFailed, "提交信息解析失败", hash)
	}
	detail := domain.GitCommitDetailRESP{
		Commit: domain.GitCommitVO{
			CommitHash:    fields[0],
			ShortHash:     fields[1],
			CommitSubject: fields[2],
			AuthorName:    fields[3],
			CommitTime:    int64(atoiSafe(fields[4])) * 1000,
		},
	}
	if len(fields) > 5 {
		detail.CommitBody = fields[5]
	}
	if len(fields) > 6 {
		detail.ParentHashes = strings.Fields(fields[6])
	}
	files, err := g.runOrFail("show", "--numstat", "--pretty=format:", hash)
	if err == nil {
		detail.ChangedFiles = parseNumStatFiles(files)
		detail.Commit.FileCount = len(detail.ChangedFiles)
		for _, file := range detail.ChangedFiles {
			detail.Commit.Insertions += file.Insertions
			detail.Commit.Deletions += file.Deletions
		}
	}
	diffText, err := g.runOrFail("show", "--no-color", "--format=", hash)
	if err == nil {
		detail.Diff = cutDiff("", diffText)
	}
	return detail, nil
}

// parseNumStatFiles 把 `--numstat` 输出转成改动文件。
func parseNumStatFiles(text string) []domain.GitFileVO {
	out := make([]domain.GitFileVO, 0, 16)
	for _, line := range pkg.SplitLines(text) {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 {
			continue
		}
		out = append(out, domain.GitFileVO{
			FilePath:   fields[2],
			ChangeCode: "M",
			Insertions: atoiSafe(fields[0]),
			Deletions:  atoiSafe(fields[1]),
		})
	}
	return out
}

// FileHistory 单个文件的提交历史。
func (g *Git) FileHistory(path string, limit int) ([]domain.GitCommitVO, error) {
	if path == "" {
		return nil, pkg.Err(pkg.CodeBadRequest, "文件路径不能为空")
	}
	if limit <= 0 || limit > 200 {
		limit = 40
	}
	text, err := g.runOrFail("log", "-n", strconv.Itoa(limit), "--pretty=format:%H\x1f%h\x1f%s\x1f%an\x1f%at", "--", path)
	if err != nil {
		return nil, err
	}
	out := make([]domain.GitCommitVO, 0, limit)
	for _, line := range pkg.SplitLines(text) {
		fields := strings.Split(line, "\x1f")
		if len(fields) < 5 {
			continue
		}
		out = append(out, domain.GitCommitVO{
			CommitHash:    fields[0],
			ShortHash:     fields[1],
			CommitSubject: fields[2],
			AuthorName:    fields[3],
			CommitTime:    int64(atoiSafe(fields[4])) * 1000,
		})
	}
	return out, nil
}

// FileDiff 某个文件在某个提交里的差异。
func (g *Git) FileDiff(hash, path string) (domain.GitDiffRESP, error) {
	if hash == "" || path == "" {
		return domain.GitDiffRESP{}, pkg.Err(pkg.CodeBadRequest, "提交与文件路径都不能为空")
	}
	text, err := g.runOrFail("show", hash, "--no-color", "--format=", "--", path)
	if err != nil {
		return domain.GitDiffRESP{}, err
	}
	return cutDiff(path, text), nil
}

// Branches 分支清单。
func (g *Git) Branches() ([]domain.GitBranchVO, error) {
	text, err := g.runOrFail("branch", "-vv", "--no-color")
	if err != nil {
		return nil, err
	}
	out := make([]domain.GitBranchVO, 0, 16)
	for _, line := range pkg.SplitLines(text) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, parseBranchLine(line))
	}
	// 远端分支单列一遍：界面要能把本地分支推到某个远端。
	// 远端名从分支名里拆出来，不能写死 origin：用户可以有多个远端。
	if remote, err := g.runOrFail("branch", "-r", "--no-color"); err == nil {
		for _, line := range pkg.SplitLines(remote) {
			name := strings.TrimSpace(line)
			if name == "" || strings.Contains(name, "->") {
				continue
			}
			remoteName := "origin"
			if idx := strings.IndexByte(name, '/'); idx > 0 {
				remoteName = name[:idx]
			}
			out = append(out, domain.GitBranchVO{BranchName: name, RemoteName: remoteName})
		}
	}
	return out, nil
}

// parseBranchLine 解析 `* main  abc1234 [origin/main: ahead 1] subject`。
func parseBranchLine(line string) domain.GitBranchVO {
	row := domain.GitBranchVO{}
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "*") {
		row.IsCurrent = true
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "*"))
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return row
	}
	row.BranchName = fields[0]
	// 上游信息在方括号里：`[origin/main: ahead 1, behind 2]`。
	joined := strings.Join(fields, " ")
	if start := strings.Index(joined, "["); start >= 0 {
		if end := strings.Index(joined, "]"); end > start {
			row.Ahead, row.Behind = parseAheadBehind(joined[start:end])
		}
	}
	return row
}

// parseAheadBehind 从方括号内容里读 ahead / behind 数字。
func parseAheadBehind(text string) (int, int) {
	ahead, behind := 0, 0
	for _, part := range strings.Split(text, ",") {
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		switch strings.TrimSpace(fields[len(fields)-2]) {
		case "ahead":
			ahead = atoiSafe(fields[len(fields)-1])
		case "behind":
			behind = atoiSafe(fields[len(fields)-1])
		}
	}
	return ahead, behind
}
