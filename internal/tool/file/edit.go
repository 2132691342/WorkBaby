package file

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// ===== file_edit =====
//
// 精确字符串替换编辑：模型改文件的正确姿势是「局部替换」而不是整篇重写——
// 写错率与 token 消耗都低一个量级。替换前后生成 unified diff 回执，
// 模型据此确认改动是否符合预期，用户在变更面板看到同一份 diff。

// EditTool 精确替换编辑工具（file_edit）。
type EditTool struct {
	resolve  func(context.Context) string
	recorder Recorder
}

// NewEdit 构造 file_edit；resolver / defRoot 语义同 NewRead。
func NewEdit(resolver tool.RootResolver, defRoot string) *EditTool {
	return &EditTool{resolve: tool.ResolveRoot(resolver, defRoot)}
}

// WithRecorder 注入变更记录器（写前备份 + diff 登记），语义同 WriteTool。
func (t *EditTool) WithRecorder(r Recorder) *EditTool {
	t.recorder = r
	return t
}

func (t *EditTool) Name() string              { return "file_edit" }
func (t *EditTool) RiskLevel() tool.RiskLevel { return tool.RiskWriteLocal }
func (t *EditTool) Description() string {
	return "对工作区内文件做精确字符串替换（局部编辑）。必须先 file_read 拿到准确原文；old_string 需包含足够上下文保证唯一匹配。返回 unified diff。"
}

func (t *EditTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: json.RawMessage(`{
			"type": "object",
			"required": ["path", "old_string", "new_string"],
			"properties": {
				"path": {"type": "string", "description": "工作区内文件路径"},
				"old_string": {"type": "string", "description": "要替换的原文（必须与文件内容逐字一致）"},
				"new_string": {"type": "string", "description": "替换后的新文本"},
				"replace_all": {"type": "boolean", "description": "true=替换全部匹配，缺省仅允许唯一匹配"}
			}
		}`),
	}
}

// Meta 声明：文件路径参数 + diff 呈现。
func (t *EditTool) Meta() tool.ToolMeta {
	return tool.ToolMeta{
		Group: tool.GroupFile, PathParams: []string{"path"}, UIHint: "diff",
		Category: tool.CategoryEdit, ActivityDesc: "编辑文件",
	}
}

type editReq struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

// ActivityDescription 时间线文案。
func (t *EditTool) ActivityDescription(args json.RawMessage) string {
	var req editReq
	if json.Unmarshal(args, &req) != nil || req.Path == "" {
		return ""
	}
	return "正在编辑 " + shortPath(req.Path)
}

func (t *EditTool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	var req editReq
	if err := json.Unmarshal(args, &req); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4004, "file_edit args parse failed", err)}
	}
	if req.OldString == "" {
		return tool.ToolResult{Err: pkg.New(4001, "old_string 不能为空（空内容替换用 file_write）", req.Path)}
	}
	if req.OldString == req.NewString {
		return tool.ToolResult{Err: pkg.New(4001, "old_string 与 new_string 相同，无需编辑", req.Path)}
	}
	if len([]byte(req.NewString)) > maxWriteBytes {
		return tool.ToolResult{Err: pkg.New(4008, "file_edit new_string too large", "")}
	}
	p, err := safePath(t.resolve(ctx), req.Path)
	if err != nil {
		return tool.ToolResult{Err: err}
	}
	// 写前必须读：old_string 与磁盘不一致时「匹配到别处」会改坏文件。
	// 只在本轮 run 内校验（跨轮文件可能已被外部改动），无 run 身份时不设限。
	if _, serr := os.Stat(p); serr == nil && !tool.FileWasRead(ctx, p) {
		return tool.ToolResult{Err: pkg.New(4001,
			"编辑前必须先 file_read 该文件：old_string 需与磁盘内容逐字一致，凭记忆拼接会改错位置", req.Path)}
	}
	bs, err := os.ReadFile(p)
	if err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4007, "read file failed（编辑前必须先 file_read 确认内容）", err)}
	}
	before := string(bs)
	count := strings.Count(before, req.OldString)
	if count == 0 {
		return tool.ToolResult{Err: pkg.New(4001,
			"old_string not found: 原文与文件内容不一致，先用 file_read 重新读取该文件（可能已被修改或存在空白差异）", req.Path)}
	}
	if count > 1 && !req.ReplaceAll {
		return tool.ToolResult{Err: pkg.New(4001,
			fmt.Sprintf("old_string 匹配了 %d 处：请扩大上下文使其唯一，或传 replace_all=true 明确替换全部", count), req.Path)}
	}
	after := before
	if req.ReplaceAll {
		after = strings.ReplaceAll(before, req.OldString, req.NewString)
	} else {
		after = strings.Replace(before, req.OldString, req.NewString, 1)
	}
	if err := os.WriteFile(p, []byte(after), 0o644); err != nil {
		return tool.ToolResult{Err: pkg.Wrap(4007, "write file failed", err)}
	}
	if t.recorder != nil {
		t.recorder.RecordWrite(ctx, p, true, bs, []byte(after))
	}
	diff := unifiedDiff(before, after)
	return tool.ToolResult{
		Content: "edited. unified diff:\n" + diff,
		Meta:    map[string]string{"path": p},
		Data:    map[string]any{"diff": diff},
	}
}

// ===== 行级 diff（edit 回执用） =====

// unifiedDiff 生成简化 unified diff：裁掉公共前后缀行后对中间差异区做 LCS，
// 保留前后 3 行上下文。edit 替换场景下差异区极小，LCS 成本可忽略。
func unifiedDiff(before, after string) string {
	const ctxLines = 3
	const maxDiffRegion = 500
	b := strings.Split(strings.TrimRight(before, "\n"), "\n")
	a := strings.Split(strings.TrimRight(after, "\n"), "\n")

	// 裁公共前缀 / 后缀，把差异收敛到中间小区域
	pre := 0
	for pre < len(b) && pre < len(a) && b[pre] == a[pre] {
		pre++
	}
	suf := 0
	for suf < len(b)-pre && suf < len(a)-pre && b[len(b)-1-suf] == a[len(a)-1-suf] {
		suf++
	}
	mb, ma := b[pre:len(b)-suf], a[pre:len(a)-suf]
	if len(mb) == 0 && len(ma) == 0 {
		return "(no changes)"
	}
	if len(mb)+len(ma) > maxDiffRegion {
		return fmt.Sprintf("(diff too large to render: %d old lines / %d new lines changed)", len(mb), len(ma))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "@@ -%d +%d @@\n", pre+1, pre+1)
	for _, ln := range b[max(0, pre-ctxLines):pre] {
		sb.WriteString("  " + ln + "\n")
	}
	for _, ln := range lcsDiffLines(mb, ma) {
		sb.WriteString(ln)
	}
	for _, ln := range a[len(a)-suf : min(len(a), len(a)-suf+ctxLines)] {
		sb.WriteString("  " + ln + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// lcsDiffLines LCS 行对齐，输出 "+ "/"- " 行。
func lcsDiffLines(oldLines, newLines []string) []string {
	n, m := len(oldLines), len(newLines)
	// 滚动数组求 LCS 长度 + Hirschberg 式回溯成本高；edit 场景区域小，直接全矩阵
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case oldLines[i] == newLines[j]:
			out = append(out, "  "+oldLines[i]+"\n")
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			out = append(out, "- "+oldLines[i]+"\n")
			i++
		default:
			out = append(out, "+ "+newLines[j]+"\n")
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, "- "+oldLines[i]+"\n")
	}
	for ; j < m; j++ {
		out = append(out, "+ "+newLines[j]+"\n")
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
