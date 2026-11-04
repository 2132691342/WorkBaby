package parser

import (
	"strconv"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// ParseDiff 把统一 diff 文本拆成差异行：file 是文件头、hunk 是 @@ 定位行、
// add/del/ctx 是正文三态。拆错会让用户以为文件里真有以 + 开头的行。
func ParseDiff(text string) []domain.DiffLineVO {
	lines := pkg.SplitLines(text)
	out := make([]domain.DiffLineVO, 0, len(lines))
	var oldNo, newNo int
	for _, line := range lines {
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
