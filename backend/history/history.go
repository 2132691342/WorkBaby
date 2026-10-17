// Package history 只读扫描本机其他 AI 工具留下的会话历史（specs/12）。
// 不同步、不常驻、不写库；「接着聊」由 service 层新建 WorkBaby 会话并注入上下文。
package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 扫描硬上限：大户的 projects 目录有几千个文件，全量解析会把页面挂死。
const (
	maxFiles   = 500
	maxDepth   = 6
	scanWindow = 5 * time.Second
)

// 来源标识。
const (
	SourceClaudeCode = "claude-code"
	SourceCodex      = "codex"
	SourceCustom     = "custom"
)

// Root 一个扫描根：来源 + 目录。
type Root struct {
	Source string
	Dir    string
}

// DefaultRoots 已知工具的默认位置。用户目录探测不到就跳过——
// 没装那个工具不该让扫描报错。
func DefaultRoots(home string) []Root {
	var out []Root
	if home != "" {
		out = append(out,
			Root{SourceClaudeCode, filepath.Join(home, ".claude", "projects")},
			Root{SourceCodex, filepath.Join(home, ".codex", "sessions")},
		)
	}
	return out
}

// Scan 扫描全部根目录。超时（scanWindow）时返回已扫到的部分并置 TimedOut——
// 结果不全必须说，不能拿半份当全份。
func Scan(roots []Root, extra map[string]string) domain.HistoryScanRESP {
	// Entries 先给空切片：nil 出网就是前端的 null，列表页会直接崩。
	res := domain.HistoryScanRESP{Entries: []domain.HistoryEntryVO{}}
	deadline := time.Now().Add(scanWindow)
	seen := map[string]bool{}
	for _, r := range roots {
		dir := r.Dir
		if s, ok := extra[r.Source]; ok && s != "" {
			dir = s // 自定义文件夹覆盖默认位置
		}
		if dir == "" || !pkg.DirExists(dir) {
			continue
		}
		files := collectFiles(dir, deadline, &res)
		for _, f := range files {
			if seen[f] {
				continue
			}
			seen[f] = true
			e, err := ParseFile(r.Source, f)
			if err != nil || len(e.Messages) == 0 {
				res.Skipped++
				continue
			}
			res.Entries = append(res.Entries, e)
		}
	}
	sort.Slice(res.Entries, func(i, j int) bool { return res.Entries[i].StartedAt > res.Entries[j].StartedAt })
	if time.Now().After(deadline) {
		res.TimedOut = true
	}
	return res
}

// collectFiles 限时 / 限量 / 限深地收集 JSONL 文件。
func collectFiles(dir string, deadline time.Time, res *domain.HistoryScanRESP) []string {
	var out []string
	var walk func(d string, depth int)
	walk = func(d string, depth int) {
		if len(out) >= maxFiles || depth > maxDepth || time.Now().After(deadline) {
			if len(out) >= maxFiles {
				res.TimedOut = true
			}
			return
		}
		ents, err := os.ReadDir(d)
		if err != nil {
			return
		}
		for _, e := range ents {
			if len(out) >= maxFiles {
				res.TimedOut = true
				return
			}
			name := e.Name()
			if e.IsDir() {
				walk(filepath.Join(d, name), depth+1)
				continue
			}
			if strings.HasSuffix(strings.ToLower(name), ".jsonl") {
				out = append(out, filepath.Join(d, name))
			}
		}
	}
	walk(dir, 0)
	return out
}

// ParseFile 解析一个 JSONL 会话文件。按来源选解析器，未知形状走兜底。
func ParseFile(source, path string) (domain.HistoryEntryVO, error) {
	e := domain.HistoryEntryVO{ID: source + ":" + path, Source: source, Path: path}
	f, err := os.Open(path)
	if err != nil {
		return e, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		m, ts, proj := extract(raw)
		if m.Role == "" || m.Text == "" {
			continue
		}
		if e.StartedAt == 0 {
			e.StartedAt = ts
		}
		if e.Project == "" && proj != "" {
			e.Project = proj
		}
		e.Messages = append(e.Messages, m)
	}
	if len(e.Messages) == 0 {
		return e, pkg.New("9303", "解析不出消息", path)
	}
	if e.Project == "" {
		e.Project = filepath.Base(filepath.Dir(path))
	}
	first := e.Messages[0].Text
	e.Title = firstLine(first, 40)
	return e, nil
}

// extract 从一行 JSON 里抽消息：先按已知来源形状取 text / content，其余按 role+content 兜底。
// 变体太多，兜底形状优先保证「有文字就能看」——具体形状见 specs/12。
func extract(raw map[string]any) (domain.HistoryMessageVO, int64, string) {
	var m domain.HistoryMessageVO
	var ts int64
	var proj string
	if v, ok := raw["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			ts = t.UnixMilli()
		}
	}
	if v, ok := raw["cwd"].(string); ok {
		proj = v
	}
	// 形状 2：payload 包裹
	if p, ok := raw["payload"].(map[string]any); ok {
		if t, ok := p["type"].(string); ok && t != "message" {
			return m, ts, proj // reasoning / function_call 等非消息条目
		}
		if v, ok := p["role"].(string); ok {
			m.Role = normalizeRole(v)
		}
		m.Text = textOf(p["content"])
		return m, ts, proj
	}
	// 形状 1：message 包裹
	if msg, ok := raw["message"].(map[string]any); ok {
		if v, ok := msg["role"].(string); ok {
			m.Role = normalizeRole(v)
		}
		m.Text = textOf(msg["content"])
		return m, ts, proj
	}
	// 形状 3：平铺
	if v, ok := raw["role"].(string); ok {
		m.Role = normalizeRole(v)
	}
	if t, ok := raw["text"].(string); ok {
		m.Text = t
	} else {
		m.Text = textOf(raw["content"])
	}
	return m, ts, proj
}

// normalizeRole 归一到三种角色；工具结果归 tool（界面上折叠展示）。
func normalizeRole(r string) string {
	switch strings.ToLower(r) {
	case "user", "human":
		return domain.RoleUser
	case "assistant", "ai":
		return domain.RoleAssistant
	case "tool", "function":
		return domain.RoleTool
	default:
		return ""
	}
}

// textOf 把 content（字符串或内容块数组）折成纯文本。
func textOf(v any) string {
	switch c := v.(type) {
	case string:
		return strings.TrimSpace(c)
	case []any:
		var parts []string
		for _, item := range c {
			blk, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t, ok := blk["text"].(string); ok && t != "" {
				parts = append(parts, t)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
}

// firstLine 取首行做标题：长文第一行常常就是主题。
func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return string(r)
}
