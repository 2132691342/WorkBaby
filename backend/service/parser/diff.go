package parser

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// maxDiffLineLen 是单行差异的字符上限。
// 压缩过的 JS / 单行 JSON 会产出一行几 MB 的差异：整行塞进 DOM 会把界面直接卡死，
// 而这种文件的差异本来也读不出信息，截断比原样摆出来有用。
const maxDiffLineLen = 2000

// ParseDiff 把统一 diff 文本拆成差异行：file 是文件头、hunk 是 @@ 定位行、
// add/del/ctx 是正文三态。拆错会让用户以为文件里真有以 + 开头的行。
func ParseDiff(text string) []domain.DiffLineVO {
	lines := pkg.SplitLines(text)
	out := make([]domain.DiffLineVO, 0, len(lines))
	var oldNo, newNo int
	for _, line := range lines {
		line = capLine(line)
		switch {
		case isDiffFileHeader(line):
			out = append(out, domain.DiffLineVO{LineKind: "file", LineText: line})
		case strings.HasPrefix(line, "@@"):
			oldNo, newNo = parseHunkHeader(line)
			out = append(out, domain.DiffLineVO{LineKind: "hunk", LineText: line})
		case strings.HasPrefix(line, "+"):
			out = append(out, domain.DiffLineVO{LineKind: "add", NewNo: newNo, LineText: line})
			newNo++
		case strings.HasPrefix(line, "-"):
			out = append(out, domain.DiffLineVO{LineKind: "del", OldNo: oldNo, LineText: line})
			oldNo++
		default:
			out = append(out, domain.DiffLineVO{LineKind: "ctx", OldNo: oldNo, NewNo: newNo, LineText: line})
			oldNo++
			newNo++
		}
	}
	return out
}

// capLine 把超长行截到上限，并明确标出被截断：静默截断会让人以为文件里就这么多。
func capLine(line string) string {
	if len(line) <= maxDiffLineLen {
		return line
	}
	// 按 rune 边界回退，不把多字节字符切成半个。
	cut := maxDiffLineLen
	for cut > 0 && !utf8.RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + " …（本行过长，已截断）"
}

// isDiffFileHeader 判断是不是文件级头部行。
func isDiffFileHeader(line string) bool {
	for _, prefix := range []string{
		"diff --git", "index ", "--- ", "+++ ",
		"new file mode", "deleted file mode", "rename from", "rename to",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// parseHunkHeader 从 `@@ -1,4 +1,6 @@` 里取出两边的起始行号。
func parseHunkHeader(line string) (int, int) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return 0, 0
	}
	return hunkStart(fields[1]), hunkStart(fields[2])
}

// hunkStart 解析 `-12,3` / `+5` 这类起点。
func hunkStart(token string) int {
	token = strings.TrimLeft(token, "-+")
	if idx := strings.IndexByte(token, ','); idx >= 0 {
		token = token[:idx]
	}
	n, err := strconv.Atoi(token)
	if err != nil {
		return 0
	}
	return n
}
