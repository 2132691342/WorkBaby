// Package memory 长期记忆：一份 MEMORY.md + 派生 FTS5 索引。
//
// 边界：MEMORY.md 是唯一真相源（用户可直接编辑）；FTS5 是从文件重建的派生索引，
// 丢失只影响检索速度，不影响记忆本身。不做「情景 / 语义 / 程序」分类——
// 分类只增加「该往哪里写」的决策成本，检索效果取决于内容而非标签。
package memory

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// 分节标题固定枚举：避免模型自由发挥把文件结构写乱，也让注入时能按节取舍。
const (
	SectionPreference = "用户偏好"
	SectionFact       = "事实"
	SectionProcedure  = "流程"
	SectionProject    = "项目约定"
)

// maxEntries 条目上限：记忆不是日志，写满就该人工整理一次。
const maxEntries = 200

// maxEntryRunes 单条上限：一条记忆应当是一句可复述的话，不是一段文档。
const maxEntryRunes = 400

// Service 记忆服务。
type Service struct {
	file string
	idx  *index
	mu   sync.Mutex
}

// NewService 构造；db 为 nil 时退化为纯文件检索（无索引）。
func NewService(file string, db *gorm.DB) *Service {
	return &Service{file: file, idx: newIndex(db)}
}

// File 记忆文件路径（设置页「打开文件」用）。
func (s *Service) File() string { return s.file }

// Entry 单条记忆。
type Entry struct {
	Section string `json:"section"`
	Text    string `json:"text"`
}

// Read 读全文；文件不存在返回空串（首次使用不算错误）。
func (s *Service) Read() string {
	bs, err := os.ReadFile(s.file)
	if err != nil {
		return ""
	}
	return string(bs)
}

// Entries 解析为结构化条目。
func (s *Service) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return parseEntries(s.Read())
}

// Search 检索记忆条目：优先 FTS5，短查询或索引不可用时退化为子串扫描。
func (s *Service) Search(query string, k int) []Hit {
	s.mu.Lock()
	all := parseEntries(s.Read())
	s.mu.Unlock()
	return s.idx.search(query, k, all)
}

// Append 追加一条记忆到指定分节（分节不存在则创建）。
// section 为空时归入「事实」；重复条目直接跳过，不制造噪声。
func (s *Service) Append(section, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len([]rune(text)) > maxEntryRunes {
		text = string([]rune(text)[:maxEntryRunes])
	}
	section = NormalizeSection(section)

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := parseEntries(s.Read())
	for _, e := range entries {
		if e.Text == text {
			return nil // 已有同一条：幂等
		}
	}
	if len(entries) >= maxEntries {
		// 写满时丢掉最旧的一条（文件顶部最旧）：长期记忆要能自我收敛。
		entries = entries[1:]
	}
	entries = append(entries, Entry{Section: section, Text: text})
	return s.writeLocked(entries)
}

// Replace 整篇覆盖（人工编辑或 API 直接写入）。
func (s *Service) Replace(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLocked(parseEntries(text))
}

// Remove 删除完全匹配的一条。
func (s *Service) Remove(section, text string) error {
	section = NormalizeSection(section)
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := parseEntries(s.Read())
	out := entries[:0]
	removed := false
	for _, e := range entries {
		if !removed && e.Section == section && e.Text == text {
			removed = true
			continue
		}
		out = append(out, e)
	}
	if !removed {
		return nil
	}
	return s.writeLocked(out)
}

// writeLocked 落盘并重建索引。
func (s *Service) writeLocked(entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.file), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.file, []byte(formatEntries(entries)), 0o644); err != nil {
		return err
	}
	s.idx.rebuild(entries)
	return nil
}

// NormalizeSection 归一化分节名：未知分节归入「事实」，避免模型自由发挥写乱结构。
func NormalizeSection(section string) string {
	switch strings.TrimSpace(section) {
	case SectionPreference, "偏好", "preference":
		return SectionPreference
	case SectionProcedure, "程序", "procedure":
		return SectionProcedure
	case SectionProject, "项目", "project":
		return SectionProject
	default:
		return SectionFact
	}
}

// formatEntries 渲染为 Markdown：分节按固定顺序输出，条目按写入序。
func formatEntries(entries []Entry) string {
	grouped := map[string][]string{}
	for _, e := range entries {
		grouped[e.Section] = append(grouped[e.Section], e.Text)
	}
	var b strings.Builder
	b.WriteString("# 长期记忆\n")
	b.WriteString("\n<!-- 由 WorkBaby 维护；可直接编辑，改动下次运行时生效。 -->\n")
	for _, sec := range sectionsInOrder(grouped) {
		b.WriteString("\n## " + sec + "\n")
		for _, t := range grouped[sec] {
			b.WriteString("- " + t + "\n")
		}
	}
	return b.String()
}

// sectionsInOrder 固定分节顺序，未知分节排在最后（保持稳定，便于人工阅读）。
func sectionsInOrder(grouped map[string][]string) []string {
	preferred := []string{SectionPreference, SectionFact, SectionProcedure, SectionProject}
	var out []string
	for _, s := range preferred {
		if len(grouped[s]) > 0 {
			out = append(out, s)
		}
	}
	var extra []string
	for s := range grouped {
		if !containsStr(preferred, s) {
			extra = append(extra, s)
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

// parseEntries 解析 Markdown：`## ` 起分节，`- ` 起条目；其余内容忽略。
func parseEntries(text string) []Entry {
	var out []Entry
	section := SectionFact
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "## "):
			section = NormalizeSection(strings.TrimPrefix(trimmed, "## "))
		case strings.HasPrefix(trimmed, "- "):
			if t := strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")); t != "" {
				out = append(out, Entry{Section: section, Text: t})
			}
		}
	}
	return out
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
