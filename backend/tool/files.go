package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"WorkBaby/backend/pkg"
	"golang.org/x/text/unicode/norm"
)

// readTool 读文件：按行带行号返回，便于模型精确定位后再改。
type readTool struct{}

func (readTool) Name() string                { return "read" }
func (readTool) Label() string               { return "读文件" }
func (readTool) ExecutionMode() ExecutionMode { return ExecutionParallel }
func (readTool) RequiresApproval() bool      { return false }
func (readTool) Description() string {
	return "读取文件文本内容。用 offset 与 limit 分页读大文件，不要一次读全部。"
}
func (readTool) PromptSnippet() string { return "读取文件内容" }
func (readTool) PromptGuidelines() []string {
	return []string{
		"用 read 看文件，不要用命令行的 cat / type。",
		"大文件先读一部分，确认结构后再决定要不要继续读。",
	}
}

func (readTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":   map[string]any{"type": "string", "description": "文件路径，相对工作目录或绝对路径"},
			"offset": map[string]any{"type": "integer", "description": "起始行号，从 1 开始，默认 1"},
			"limit":  map[string]any{"type": "integer", "description": "读取行数，默认 2000"},
		},
		"required": []string{"path"},
	}
}

func (t readTool) Execute(ctx context.Context, in Input) (*Result, error) {
	path := Str(in.Args, "path")
	if path == "" {
		return nil, ErrMissingArg
	}
	full, st, err := resolveReadable(in.Workspace, path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, pkg.New(1008, "这是目录，不是文件", full)
	}

	raw, err := os.ReadFile(full)
	if err != nil {
		return nil, pkg.Wrap(1005, "读取文件失败", err)
	}
	if bytesAreBinary(raw) {
		return &Result{
			Content: "这是二进制文件，无法按文本读取。",
			Title:   "读文件（二进制，已跳过）",
			Detail:  full,
			IsError: true,
		}, nil
	}
	if in.Deps.Reads != nil {
		in.Deps.Reads.MarkRead(full)
	}
	rel := pkg.RelPath(in.Workspace, full)
	if len(raw) == 0 {
		return &Result{Content: "（空文件，没有任何内容）", Title: fmt.Sprintf("读了 %s（空文件，0 字节）", rel)}, nil
	}

	lines := strings.Split(strings.TrimPrefix(string(raw), "\uFEFF"), "\n")
	offset := Int(in.Args, "offset", 1)
	limit := Int(in.Args, "limit", MaxLines)
	if offset < 1 {
		offset = 1
	}
	if limit <= 0 {
		limit = MaxLines
	}
	end := offset - 1 + limit
	if end > len(lines) {
		end = len(lines)
	}
	if offset-1 > len(lines) {
		offset, end = len(lines)+1, len(lines)
	}

	var b strings.Builder
	for i := offset - 1; i < end; i++ {
		fmt.Fprintf(&b, "%6d\t%s\n", i+1, lines[i])
	}
	out := b.String()
	truncated := end < len(lines)
	content, title := out, fmt.Sprintf("读了 %s（%d 行，%d 字节）", rel, end-offset+1, len(raw))
	if truncated {
		content += fmt.Sprintf("……（共 %d 行，已显示第 %d-%d 行；用 offset=%d 继续读）", len(lines), offset, end, end+1)
		title = fmt.Sprintf("读了 %s（%d-%d 行，%d 字节）", rel, offset, end, len(raw))
	}
	return &Result{Content: content, Title: title, Detail: out}, nil
}

// resolveReadable 定位要读的文件。复制来的路径常与磁盘上的字节不一致，
// 主路径落空时按常见 Unicode 变体重试一次，全失败才报「找不到这个文件」。
func resolveReadable(ws, path string) (string, os.FileInfo, error) {
	var lastErr error
	for _, candidate := range append([]string{path}, pathVariants(path)...) {
		full, err := pkg.SafeJoin(ws, candidate)
		if err != nil {
			return "", nil, err
		}
		st, err := os.Stat(full)
		if err == nil {
			return full, st, nil
		}
		lastErr = err
	}
	return "", nil, pkg.Wrap(1005, "找不到这个文件", lastErr)
}

// quotePair 是一对引号的写法；同一文件里的引号必须成对，不能左右混搭。
type quotePair struct{ dOpen, dClose, sOpen, sClose rune }

// quotePairs 覆盖文件名里真实出现过的写法：直引号、左弯+右弯、左弯+左弯、
// 右弯+右弯、右弯+左弯。Windows 不允许直引号进文件名，但复制粘贴与输入法
// 常把磁盘上的弯引号变成直的，所以两个方向都要试。
var quotePairs = []quotePair{
	{0x22, 0x22, 0x27, 0x27},
	{0x201C, 0x201D, 0x2018, 0x2019},
	{0x201C, 0x201C, 0x2018, 0x2018},
	{0x201D, 0x201D, 0x2019, 0x2019},
	{0x201D, 0x201C, 0x2019, 0x2018},
}

// pathVariants 列出「肉眼相同、字节不同」的常见变体：NFD 分解形式与成对引号写法。
func pathVariants(p string) []string {
	out := make([]string, 0, len(quotePairs)+1)
	if nfd := norm.NFD.String(p); nfd != p {
		out = append(out, nfd)
	}
	for _, q := range quotePairs {
		if v := applyQuotePair(p, q); v != p {
			out = append(out, v)
		}
	}
	return out
}

// applyQuotePair 把串里的 ASCII 引号按出现顺序整体替换成同一对弯引号。
func applyQuotePair(p string, q quotePair) string {
	dOpen, sOpen := false, false
	return strings.Map(func(r rune) rune {
		switch r {
		case 0x22:
			if dOpen = !dOpen; dOpen {
				return q.dOpen
			}
			return q.dClose
		case 0x27:
			if sOpen = !sOpen; sOpen {
				return q.sOpen
			}
			return q.sClose
		}
		return r
	}, p)
}

// writeTool 整体写入文件。写前必读，避免模型凭空覆盖用户文件。
type writeTool struct{}

func (writeTool) Name() string                { return "write" }
func (writeTool) Label() string               { return "写文件" }
func (writeTool) ExecutionMode() ExecutionMode { return ExecutionSequential }
func (writeTool) RequiresApproval() bool      { return true }
func (writeTool) Description() string {
	return "整体写入文件（覆盖原内容）。会自动创建不存在的父目录。"
}
func (writeTool) PromptSnippet() string { return "写入文件" }
func (writeTool) PromptGuidelines() []string {
	return []string{
		"写已存在的文件前，必须先用 read 读过它。",
		"改一小处用 edit，不要为了改一行而重写整个文件。",
	}
}

func (writeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "文件路径"},
			"content": map[string]any{"type": "string", "description": "文件完整内容"},
		},
		"required": []string{"path", "content"},
	}
}

func (t writeTool) Execute(ctx context.Context, in Input) (*Result, error) {
	path := Str(in.Args, "path")
	content := Str(in.Args, "content")
	if path == "" {
		return nil, ErrMissingArg
	}
	full, err := pkg.SafeJoin(in.Workspace, path)
	if err != nil {
		return nil, err
	}
	// 覆盖是危险操作，先探一次目标是否存在，结果里必须写清是新建还是覆盖。
	_, statErr := os.Stat(full)
	exists := statErr == nil
	rel := pkg.RelPath(in.Workspace, full)
	if exists && in.Deps.Reads != nil && !in.Deps.Reads.HasRead(full) {
		return nil, pkg.New(1009, "请先用 read 读一遍这个文件，再决定要不要覆盖", rel)
	}
	if err := pkg.WriteText(full, content); err != nil {
		return nil, err
	}
	if in.Deps.Reads != nil {
		in.Deps.Reads.MarkRead(full)
	}
	action := "新建了"
	if exists {
		action = "覆盖了"
	}
	return &Result{
		Content: fmt.Sprintf("已%s %s（%d 字节）", action, rel, len(content)),
		Title:   fmt.Sprintf("%s %s", action, rel),
		Detail:  content,
	}, nil
}

// editTool 精确替换：old_text 必须唯一，避免多处替换造成不可预期修改。
type editTool struct{}

func (editTool) Name() string                { return "edit" }
func (editTool) Label() string               { return "改文件" }
func (editTool) ExecutionMode() ExecutionMode { return ExecutionSequential }
func (editTool) RequiresApproval() bool      { return true }
func (editTool) Description() string {
	return "按精确文本替换修改文件。old_text 必须在文件中唯一，否则会报错。"
}
func (editTool) PromptSnippet() string { return "精确替换文件中的一段文字" }
func (editTool) PromptGuidelines() []string {
	return []string{
		"同一处文字在文件里出现多次时，把 old_text 写长一点直到唯一。",
		"一次 edits 可以改多处，但各段不能互相重叠。",
	}
}

func (editTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "文件路径"},
			"edits": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"old_text": map[string]any{"type": "string"},
						"new_text": map[string]any{"type": "string"},
					},
					"required": []string{"old_text", "new_text"},
				},
			},
		},
		"required": []string{"path", "edits"},
	}
}

func (t editTool) Execute(ctx context.Context, in Input) (*Result, error) {
	path := Str(in.Args, "path")
	if path == "" {
		return nil, ErrMissingArg
	}
	full, err := pkg.SafeJoin(in.Workspace, path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return nil, pkg.Wrap(1005, "读取文件失败", err)
	}
	rel := pkg.RelPath(in.Workspace, full)
	if in.Deps.Reads != nil && !in.Deps.Reads.HasRead(full) {
		return nil, pkg.New(1009, "请先用 read 读一遍这个文件，再修改", rel)
	}
	bom, eol, current := splitFileText(raw)

	edits := parseEdits(in.Args["edits"])
	if len(edits) == 0 {
		return nil, ErrBadArg
	}

	var diff strings.Builder
	for _, e := range edits {
		if e.Old == e.New {
			return nil, pkg.New(1010, "old_text 和 new_text 相同，这次什么也没改", rel)
		}
		// 唯一性按已应用的前序改动判定：第二条要能在第一条改完之后的文本里唯一定位。
		count := strings.Count(current, e.Old)
		if count == 0 {
			return nil, pkg.New(1010, "文件里找不到这段内容", truncateForMsg(e.Old))
		}
		if count > 1 {
			return nil, pkg.New(1010, fmt.Sprintf("这段内容在文件里出现了 %d 次，请把 old_text 写得更完整", count), truncateForMsg(e.Old))
		}
		diff.WriteString(renderDiff(current, e.Old, e.New))
		current = strings.Replace(current, e.Old, e.New, 1)
	}

	if err := pkg.WriteText(full, bom+strings.ReplaceAll(current, "\n", eol)); err != nil {
		return nil, err
	}
	return &Result{
		Content: fmt.Sprintf("已修改 %s\n%s", rel, diff.String()),
		Title:   fmt.Sprintf("改了 %s（%d 处）", rel, len(edits)),
		Detail:  diff.String(),
	}, nil
}

// splitFileText 剥掉 BOM、把行尾统一成 LF 供替换用；写回时再按原样还原。
// 行尾被顺手改掉会让整个文件在用户的版本库里变成一次大改。
func splitFileText(raw []byte) (bom, eol, text string) {
	bom, eol = "", "\n"
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		bom, raw = "\uFEFF", raw[3:]
	}
	if crlf, lf := countEOL(raw); crlf > lf {
		eol = "\r\n"
	}
	return bom, eol, strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// countEOL 统计 CRLF 与裸 LF 的数量，取多数作为原文件的行尾风格。
func countEOL(b []byte) (crlf, lf int) {
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\r':
			if i+1 < len(b) && b[i+1] == '\n' {
				crlf++
				i++
			}
		case '\n':
			lf++
		}
	}
	return crlf, lf
}

type editItem struct{ Old, New string }

// 内联快速解析；不暴露为领域类型。
func parseEdits(v any) []editItem {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]editItem, 0, len(list))
	for _, raw := range list {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		oldText, _ := m["old_text"].(string)
		newText, _ := m["new_text"].(string)
		if oldText == "" {
			continue
		}
		out = append(out, editItem{Old: oldText, New: newText})
	}
	return out
}

// renderDiff 输出改动处前后 3 行的上下文，够看懂又不占篇幅。
func renderDiff(src, oldText, newText string) string {
	idx := strings.Index(src, oldText)
	if idx < 0 {
		return ""
	}
	before := src[:idx]
	startLine := strings.Count(before, "\n")
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	all := strings.Split(src, "\n")
	from := startLine - 3
	if from < 0 {
		from = 0
	}
	to := startLine + len(oldLines) + 3
	if to > len(all) {
		to = len(all)
	}
	var b strings.Builder
	b.WriteString("```diff\n")
	for i := from; i < startLine; i++ {
		b.WriteString("  " + all[i] + "\n")
	}
	for _, l := range oldLines {
		b.WriteString("- " + l + "\n")
	}
	for _, l := range newLines {
		b.WriteString("+ " + l + "\n")
	}
	for i := startLine + len(oldLines); i < to; i++ {
		b.WriteString("  " + all[i] + "\n")
	}
	b.WriteString("```\n")
	return b.String()
}

func truncateForMsg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 120 {
		return CutBytes(s, 120) + "…"
	}
	return s
}

// bytesAreBinary 用 NUL 字节判断，避免把乱码塞进上下文。
func bytesAreBinary(b []byte) bool {
	n := len(b)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}
