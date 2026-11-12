package agent

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/backend/pkg"
)

// maxReadBytes 是单次读取的上限。工具输出直接进模型上下文，
// 整份塞进去比读不到更糟：它会挤掉真正的对话。
const maxReadBytes = 512 << 10

// readTool 读文件，带行号返回。行号是必需品：编辑与差异都以行为锚点。
type readTool struct{}

// Spec 工具声明。
func (t *readTool) Spec() ToolSpec {
	return ToolSpec{
		Name:        "read",
		Description: "读取工作区内一个文本文件的内容，按行号返回。修改文件前先用它看清原文。",
		Schema: schemaOf(map[string]any{
			"path":  "文件路径，相对工作区或绝对路径",
			"start": "可选，起始行号（从 1 开始）",
			"end":   "可选，结束行号（含）",
		}, "path"),
	}
}

// Run 执行读取。
func (t *readTool) Run(_ context.Context, reg *Registry, args map[string]any) Result {
	file, err := reg.resolve(stringArg(args, "path"))
	if err != nil {
		return errorf("%s", pkg.As(err).Message)
	}
	rel := reg.relative(file)
	info, statErr := os.Stat(file)
	if statErr != nil {
		return errorf("文件不存在：%s", rel)
	}
	if info.IsDir() {
		return errorf("%s 是目录，用 ls 查看目录内容", rel)
	}
	if info.Size() > maxReadBytes {
		return errorf("文件太大（%d KB），超过单次读取上限 %d KB", info.Size()/1024, maxReadBytes/1024)
	}
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		return errorf("读取失败：%v", readErr)
	}
	text := string(raw)
	if strings.ContainsRune(text, 0) {
		return errorf("%s 看起来是二进制文件，无法按文本读取", rel)
	}
	lines := pkg.SplitLines(text)
	total := len(lines)
	start := intArg(args, "start", 1)
	end := intArg(args, "end", total)
	if start < 1 {
		start = 1
	}
	if end > total || end < start {
		end = total
	}
	var out strings.Builder
	for i := start; i <= end && i <= total; i++ {
		out.WriteString(itoa(i))
		out.WriteString("\t")
		out.WriteString(lines[i-1])
		out.WriteString("\n")
	}
	header := rel + "（共 " + itoa(total) + " 行"
	if start != 1 || end != total {
		header += "，本次 " + itoa(start) + "-" + itoa(end)
	}
	header += "）\n"
	return Result{Text: header + out.String()}
}

// writeTool 覆盖写整个文件。
type writeTool struct{}

// Spec 工具声明。
func (t *writeTool) Spec() ToolSpec {
	return ToolSpec{
		Name:        "write",
		Description: "把内容整体写入工作区内的一个文件，父目录不存在会自动建。改已有文件的局部内容请用 edit。",
		Schema: schemaOf(map[string]any{
			"path":    "文件路径，相对工作区或绝对路径",
			"content": "要写入的完整内容",
		}, "path", "content"),
	}
}

// Run 执行写入。
func (t *writeTool) Run(_ context.Context, reg *Registry, args map[string]any) Result {
	file, err := reg.resolve(stringArg(args, "path"))
	if err != nil {
		return errorf("%s", pkg.As(err).Message)
	}
	content := stringArg(args, "content")
	if err := pkg.WriteFileAtomic(file, []byte(content)); err != nil {
		return errorf("写入失败：%v", err)
	}
	rel := reg.relative(file)
	lines := len(pkg.SplitLines(content))
	return Result{
		Text:    "已写入 " + rel + "（" + itoa(lines) + " 行，" + itoa(len(content)) + " 字节）",
		Details: map[string]any{"file_path": rel},
	}
}

// editTool 用精确文本替换做局部修改。
type editTool struct{}

// Spec 工具声明。
func (t *editTool) Spec() ToolSpec {
	return ToolSpec{
		Name: "edit",
		Description: "把文件里一段原文替换成新文本。old_text 必须在文件里唯一出现，" +
			"请带上足够的上下文让它唯一；改之前先用 read 看清原文。",
		Schema: schemaOf(map[string]any{
			"path":     "文件路径，相对工作区或绝对路径",
			"old_text": "要被替换的原文，必须唯一出现",
			"new_text": "替换后的文本",
		}, "path", "old_text", "new_text"),
	}
}

// Run 执行替换。
func (t *editTool) Run(_ context.Context, reg *Registry, args map[string]any) Result {
	file, err := reg.resolve(stringArg(args, "path"))
	if err != nil {
		return errorf("%s", pkg.As(err).Message)
	}
	oldText := stringArg(args, "old_text")
	newText := stringArg(args, "new_text")
	if oldText == "" {
		return errorf("old_text 不能为空")
	}
	if oldText == newText {
		return errorf("old_text 与 new_text 相同，没有可改的内容")
	}
	raw, readErr := os.ReadFile(file)
	if readErr != nil {
		return errorf("读取失败：%v", readErr)
	}
	text := string(raw)
	switch strings.Count(text, oldText) {
	case 0:
		return errorf("文件里找不到这段原文，请先 read 确认（注意缩进与换行）")
	case 1:
	default:
		return errorf("这段原文在文件里出现多次，请带上更多上下文让它唯一")
	}
	updated := strings.Replace(text, oldText, newText, 1)
	if err := pkg.WriteFileAtomic(file, []byte(updated)); err != nil {
		return errorf("写入失败：%v", err)
	}
	rel := reg.relative(file)
	return Result{
		Text:    "已修改 " + rel,
		Details: map[string]any{"file_path": rel, "diff": unifiedDiff(rel, text, updated)},
	}
}

// lsTool 列目录。
type lsTool struct{}

// Spec 工具声明。
func (t *lsTool) Spec() ToolSpec {
	return ToolSpec{
		Name:        "ls",
		Description: "列出工作区内一个目录的内容，目录在前、文件在后，带大小。不传路径就列工作区根。",
		Schema: schemaOf(map[string]any{
			"path": "可选，目录路径；默认工作区根",
		}),
	}
}

// Run 执行列目录。
func (t *lsTool) Run(_ context.Context, reg *Registry, args map[string]any) Result {
	raw := stringArg(args, "path")
	if strings.TrimSpace(raw) == "" {
		raw = "."
	}
	dir, err := reg.resolve(raw)
	if err != nil {
		return errorf("%s", pkg.As(err).Message)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		return errorf("目录读不了：%v", readErr)
	}
	dirs := make([]string, 0, len(entries))
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			dirs = append(dirs, name+"/")
			continue
		}
		size := int64(0)
		if info, infoErr := entry.Info(); infoErr == nil {
			size = info.Size()
		}
		files = append(files, name+"  ("+humanSize(size)+")")
	}
	sort.Strings(dirs)
	sort.Strings(files)
	header := reg.relative(dir) + "（" + itoa(len(dirs)) + " 个目录，" + itoa(len(files)) + " 个文件）\n"
	return Result{Text: header + strings.Join(append(dirs, files...), "\n")}
}

// humanSize 把字节数变成人读的大小。
func humanSize(size int64) string {
	switch {
	case size >= 1<<20:
		return itoa(int(size/(1<<20))) + " MB"
	case size >= 1<<10:
		return itoa(int(size/(1<<10))) + " KB"
	default:
		return itoa(int(size)) + " B"
	}
}

// itoa 是数字转字符串：工具输出里到处要它，单独引 strconv 不值得。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	digits := make([]byte, 0, 12)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

// isTextPath 判断扩展名是否是文本文件（grep / find 用）。
func isTextPath(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".bmp",
		".zip", ".gz", ".tar", ".7z", ".rar", ".exe", ".dll", ".so", ".dylib",
		".pdf", ".docx", ".xlsx", ".pptx", ".woff", ".woff2", ".ttf", ".mp4", ".mp3":
		return false
	}
	return true
}
