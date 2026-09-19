package service

import (
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/memory"
	"WorkBaby/internal/pkg"
)

// MemoryService 记忆读写编排（API 层入口）。
// 业务语义只有一条：MEMORY.md 是唯一真相源，索引是可重建的派生数据。
type MemoryService struct {
	mem *memory.Service
}

// NewMemoryService 构造。
func NewMemoryService(mem *memory.Service) *MemoryService {
	return &MemoryService{mem: mem}
}

// Overview 记忆概览。
func (s *MemoryService) Overview() domain.MemoryOverviewRESP {
	if s.mem == nil {
		return domain.MemoryOverviewRESP{}
	}
	entries := s.mem.Entries()
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Section]++
	}
	out := domain.MemoryOverviewRESP{File: s.mem.File(), Entries: len(entries)}
	for _, sec := range []string{memory.SectionPreference, memory.SectionFact, memory.SectionProcedure, memory.SectionProject} {
		if counts[sec] > 0 {
			out.Sections = append(out.Sections, domain.MemorySectionRESP{Section: sec, Count: counts[sec]})
		}
	}
	return out
}

// List 条目列表；section 非空时按分节过滤。
func (s *MemoryService) List(section string) domain.MemoryListRESP {
	if s.mem == nil {
		return domain.MemoryListRESP{Items: []domain.MemoryEntryRESP{}}
	}
	section = strings.TrimSpace(section)
	out := domain.MemoryListRESP{File: s.mem.File(), Items: []domain.MemoryEntryRESP{}}
	for _, e := range s.mem.Entries() {
		if section != "" && e.Section != section {
			continue
		}
		out.Items = append(out.Items, domain.MemoryEntryRESP{Section: e.Section, Text: e.Text})
	}
	return out
}

// Search 检索记忆。
func (s *MemoryService) Search(query string, k int) []domain.MemoryEntryRESP {
	if s.mem == nil {
		return []domain.MemoryEntryRESP{}
	}
	hits := s.mem.Search(query, k)
	out := make([]domain.MemoryEntryRESP, 0, len(hits))
	for _, h := range hits {
		out = append(out, domain.MemoryEntryRESP{Section: h.Section, Text: h.Text, Score: h.Score})
	}
	return out
}

// Read 读全文（设置页编辑器用）。
func (s *MemoryService) Read() string {
	if s.mem == nil {
		return ""
	}
	return s.mem.Read()
}

// Append 追加一条。
func (s *MemoryService) Append(req *domain.MemoryWriteREQ) error {
	if s.mem == nil {
		return pkg.New(6001, "记忆服务未就绪", "")
	}
	if strings.TrimSpace(req.Content) == "" {
		return pkg.New(6001, "记忆内容不能为空", "")
	}
	return s.mem.Append(req.Section, req.Content)
}

// Delete 删除一条（分节 + 正文精确匹配）。
func (s *MemoryService) Delete(req *domain.MemoryDeleteREQ) error {
	if s.mem == nil {
		return pkg.New(6001, "记忆服务未就绪", "")
	}
	return s.mem.Remove(req.Section, req.Text)
}

// Replace 整篇覆盖。
func (s *MemoryService) Replace(req *domain.MemoryReplaceREQ) error {
	if s.mem == nil {
		return pkg.New(6001, "记忆服务未就绪", "")
	}
	return s.mem.Replace(req.Text)
}
