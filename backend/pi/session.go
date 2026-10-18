// PI 会话文件（JSONL）的读取：会话目录即真相，界面看到的会话、用量与工具声明都从它来。
// 这里只解析不投影——把一行翻译成界面视图是上层的事。
package pi

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/pkg"
)

// SessionTool 是工具声明里界面关心的两项。
type SessionTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SessionEntry 是会话文件的一行。
type SessionEntry struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	ParentID  string `json:"parentId"`
	Timestamp string `json:"timestamp"`
	// 会话头字段
	Version int    `json:"version"`
	Cwd     string `json:"cwd"`
	// 会话信息 / 压缩 / 分支
	Name       string `json:"name"`
	Summary    string `json:"summary"`
	CustomType string `json:"customType"`
	// 模型与思考档
	Provider      string `json:"provider"`
	ModelID       string `json:"modelId"`
	ThinkingLevel string `json:"thinkingLevel"`
	// 用量条目
	Usage *Usage          `json:"usage"`
	Data  json.RawMessage `json:"data"`
	// 消息载荷：只保留需要投影与统计的字段
	Message json.RawMessage `json:"message"`
}

// SessionMessage 是 entry.message 的形状。
type SessionMessage struct {
	Role         string          `json:"role"`
	Content      json.RawMessage `json:"content"`
	Timestamp    int64           `json:"timestamp"`
	Usage        *Usage          `json:"usage"`
	StopReason   string          `json:"stopReason"`
	ToolCallID   string          `json:"toolCallId"`
	ToolName     string          `json:"toolName"`
	IsError      bool            `json:"isError"`
	Provider     string          `json:"provider"`
	Model        string          `json:"model"`
	ToolsAdded   []SessionTool   `json:"toolsAdded"`
	ToolsRemoved []struct {
		Name string `json:"name"`
	} `json:"toolsRemoved"`
}

// SessionMeta 是会话列表需要的全部信息。
type SessionMeta struct {
	ID            string
	File          string
	Cwd           string
	Name          string
	Provider      string
	Model         string
	ThinkingLevel string
	MessageCount  int
	UserMessages  int
	TotalTokens   int
	Cost          float64
	CreatedAt     int64
	UpdatedAt     int64
	FirstUser     string
	Tools         []SessionTool
}

// Session 是一条会话文件的完整解析结果。
type Session struct {
	Meta    SessionMeta
	Entries []SessionEntry
}

// Title 是会话语义名：PI 里设过名字就用它，否则用首条用户消息。
func (m SessionMeta) Title() string {
	if strings.TrimSpace(m.Name) != "" {
		return m.Name
	}
	text := strings.Join(strings.Fields(m.FirstUser), " ")
	if text == "" {
		return "新对话"
	}
	if len([]rune(text)) > 40 {
		text = string([]rune(text)[:40]) + "…"
	}
	return text
}

// LoadSession 完整读一条会话文件（详情页与用量统计用）。
func LoadSession(file string) (*Session, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, pkg.Wrap("9715", "读不到会话记录", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, pkg.Wrap("9715", "读不到会话记录", err)
	}

	data := &Session{Meta: SessionMeta{File: file}}
	tools := map[string]SessionTool{}
	order := []string{}

	err = eachLine(f, func(line []byte) {
		var entry SessionEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			// 单行坏掉不该让整条会话打不开。
			return
		}
		data.Entries = append(data.Entries, entry)
		m := &data.Meta
		switch entry.Type {
		case "session":
			m.ID = entry.ID
			m.Cwd = entry.Cwd
			m.CreatedAt = IsoMillis(entry.Timestamp)
		case "message":
			var msg SessionMessage
			if err := json.Unmarshal(entry.Message, &msg); err != nil {
				return
			}
			switch msg.Role {
			case "system":
				// 工具声明的增量补丁：重放后就是这一轮模型能调到的工具。
				for _, t := range msg.ToolsAdded {
					if _, ok := tools[t.Name]; !ok {
						order = append(order, t.Name)
					}
					tools[t.Name] = t
				}
				for _, t := range msg.ToolsRemoved {
					delete(tools, t.Name)
				}
			case "user":
				m.UserMessages++
				m.MessageCount++
				if m.FirstUser == "" {
					m.FirstUser = firstText(msg.Content)
				}
			case "assistant":
				m.MessageCount++
				if msg.Usage != nil {
					m.TotalTokens += msg.Usage.TotalTokens
					m.Cost += msg.Usage.Cost.Total
				}
			}
		case "model_change":
			m.Provider = entry.Provider
			m.Model = entry.ModelID
		case "thinking_level_change":
			m.ThinkingLevel = entry.ThinkingLevel
		case "usage":
			if entry.Usage != nil {
				m.TotalTokens += entry.Usage.TotalTokens
				m.Cost += entry.Usage.Cost.Total
			}
		case "session_info":
			m.Name = entry.Name
		}
	})
	if err != nil {
		return nil, pkg.Wrap("9715", "读不到会话记录", err)
	}
	data.Meta.UpdatedAt = info.ModTime().UnixMilli()
	data.Meta.Tools = make([]SessionTool, 0, len(order))
	for _, name := range order {
		data.Meta.Tools = append(data.Meta.Tools, tools[name])
	}
	return data, nil
}

// ListSessions 扫描会话目录，按最近更新倒序返回全部会话的元信息。
// 目录结构是 sessions/--<cwd>--/<ts>_<id>.jsonl，一层目录一条会话。
func ListSessions(dir string) ([]SessionMeta, error) {
	out := []SessionMeta{}
	if dir == "" || !pkg.DirExists(dir) {
		return out, nil
	}
	files := []string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".jsonl") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, pkg.Wrap("9715", "扫描会话目录失败", err)
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		if hit, ok := cache.items[file]; ok && hit.size == info.Size() && hit.mtime == info.ModTime().UnixMilli() {
			out = append(out, hit.meta)
			continue
		}
		data, err := LoadSession(file)
		if err != nil {
			continue
		}
		cache.items[file] = cachedMeta{size: info.Size(), mtime: data.Meta.UpdatedAt, meta: data.Meta}
		out = append(out, data.Meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

// InvalidateSessionCache 丢掉某个文件的缓存（改名/删除/新增条目后由上层调用）。
func InvalidateSessionCache(file string) {
	cache.mu.Lock()
	delete(cache.items, file)
	cache.mu.Unlock()
}

// sessionCache 按「路径 + 大小 + mtime」缓存列表用的元信息：
// 会话文件只会追加，三者不变就说明内容没变。
type sessionCache struct {
	mu    sync.Mutex
	items map[string]cachedMeta
}

type cachedMeta struct {
	size  int64
	mtime int64
	meta  SessionMeta
}

var cache = &sessionCache{items: map[string]cachedMeta{}}

// eachLine 逐行喂给回调；单行可能很长（含 base64 图片），按 Reader 而不是 Scanner 读。
func eachLine(r io.Reader, fn func([]byte)) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := strings.TrimSpace(string(line))
			if trimmed != "" {
				fn([]byte(trimmed))
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// firstText 从消息内容里取第一段文本（内容可能是字符串或内容块数组）。
func firstText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
		return ""
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			return b.Text
		}
	}
	return ""
}

// IsoMillis 把 ISO 时间转成毫秒整数；解不出返回 0。
func IsoMillis(s string) int64 {
	if s == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}
