// Package service 是业务编排层。
package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/service/parser"
)

// Sessions 是会话编排：清单、详情、改名、删除、隐藏台账与历史轮。
type Sessions struct {
	// Settings 设置（工作区、隐藏台账）
	Settings *Settings
	// Store 设置库
	Store *repo.Store
	// Layout 沙箱布局
	Layout pkg.Layout
}

// NewSessions 造一个会话门面。
func NewSessions(settings *Settings, store *repo.Store, layout pkg.Layout) *Sessions {
	return &Sessions{Settings: settings, Store: store, Layout: layout}
}

// List 扫会话清单。给了工作区只扫它的目录，否则扫全部。
func (s *Sessions) List(workspace string) domain.SessionListRESP {
	root := s.Layout.SessionsDir()
	dirs := s.scanDirs(root, workspace)
	rows := make([]domain.SessionRowVO, 0, len(dirs))
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			// 单个坏文件不该让整张清单读不出来。
			rows = append(rows, parser.Summarize(filepath.Join(dir, entry.Name())))
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].UpdatedAt > rows[j].UpdatedAt })
	return domain.SessionListRESP{Sessions: rows}
}

// scanDirs 决定这次要扫哪些目录。
func (s *Sessions) scanDirs(root, workspace string) []string {
	if workspace != "" {
		dir := filepath.Join(root, parser.EncodedWorkspace(workspace))
		if pkg.IsDir(dir) {
			return []string{dir}
		}
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	return dirs
}

// Detail 会话详情：清单行 + 逐条消息。
func (s *Sessions) Detail(file string) (domain.SessionDetailRESP, error) {
	resolved, err := parser.GuardSession(s.Layout.SessionsDir(), file)
	if err != nil {
		return domain.SessionDetailRESP{}, err
	}
	return domain.SessionDetailRESP{
		Session: parser.Summarize(resolved),
		Items:   parser.DetailItems(resolved),
	}, nil
}

// Delete 删除会话文件。
func (s *Sessions) Delete(file string) error {
	resolved, err := parser.GuardSession(s.Layout.SessionsDir(), file)
	if err != nil {
		return err
	}
	if err := os.Remove(resolved); err != nil {
		return pkg.WrapErr(pkg.CodeSessionIO, "会话删除失败", err)
	}
	return nil
}

// Rename 改名：往会话文件追加一条 session_info 条目。
// 这是引擎自己的改名方式，直接改文件头会让引擎认不出会话。
func (s *Sessions) Rename(file, title string) error {
	resolved, err := parser.GuardSession(s.Layout.SessionsDir(), file)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(title)
	if name == "" {
		return pkg.Err(pkg.CodeBadRequest, "会话标题不能为空")
	}
	entries, err := parser.ReadEntries(resolved)
	if err != nil {
		return err
	}
	leafId := ""
	if len(entries) > 0 {
		leafId = entries[len(entries)-1].Id
	}
	// 条目 ID 用壳的 ULID：引擎接受任意稳定唯一串，而 ULID 天然可排序。
	entry := map[string]any{
		"type":      "session_info",
		"id":        pkg.NewID(pkg.PrefixSession),
		"parentId":  leafId,
		"timestamp": nowISO(),
		"name":      name,
	}
	raw, err := pkg.MarshalJSONLine(entry)
	if err != nil {
		return pkg.WrapErr(pkg.CodeSessionIO, "会话改名失败", err)
	}
	f, err := os.OpenFile(resolved, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return pkg.WrapErr(pkg.CodeSessionIO, "会话改名失败", err)
	}
	defer f.Close()
	if _, err := f.WriteString(raw + "\n"); err != nil {
		return pkg.WrapErr(pkg.CodeSessionIO, "会话改名失败", err)
	}
	return nil
}

// History 回放当前会话的历史轮（已剔除隐藏的轮）。
func (s *Sessions) History(file string) []domain.HistoryTurnVO {
	if file == "" {
		return nil
	}
	return parser.BuildHistory(file, s.hiddenSet(file))
}

// hideKey 隐藏台账的键。
func (s *Sessions) hideKey(file string) string {
	return KeyHideLedger + file
}

// hiddenSet 当前会话的隐藏集合。
func (s *Sessions) hiddenSet(file string) map[string]struct{} {
	raw, _ := s.Store.Get(s.hideKey(file))
	out := map[string]struct{}{}
	for _, id := range unmarshalStrings(raw) {
		out[id] = struct{}{}
	}
	return out
}

// Hide 隐藏一轮。
func (s *Sessions) Hide(file, turnId string) error {
	if file == "" || turnId == "" {
		return pkg.Err(pkg.CodeBadRequest, "会话与轮次都不能为空")
	}
	raw, _ := s.Store.Get(s.hideKey(file))
	ids := unmarshalStrings(raw)
	for _, id := range ids {
		if id == turnId {
			return nil
		}
	}
	ids = append(ids, turnId)
	return s.Store.Save(map[string]string{s.hideKey(file): marshalStrings(ids)})
}

// Unhide 取消隐藏一轮。
func (s *Sessions) Unhide(file, turnId string) error {
	if file == "" || turnId == "" {
		return pkg.Err(pkg.CodeBadRequest, "会话与轮次都不能为空")
	}
	raw, _ := s.Store.Get(s.hideKey(file))
	ids := unmarshalStrings(raw)
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != turnId {
			kept = append(kept, id)
		}
	}
	return s.Store.Save(map[string]string{s.hideKey(file): marshalStrings(kept)})
}

// Export 导出会话为自包含 HTML。
func (s *Sessions) Export(file string) (domain.ChatExportRESP, error) {
	resolved, err := parser.GuardSession(s.Layout.SessionsDir(), file)
	if err != nil {
		return domain.ChatExportRESP{}, err
	}
	history := parser.BuildHistory(resolved, s.hiddenSet(resolved))
	title := strings.TrimSuffix(filepath.Base(resolved), ".jsonl")
	return domain.ChatExportRESP{
		FileName:    title + ".html",
		FileContent: renderSessionHTML(title, history),
	}, nil
}

// Count 会话总数（健康面板用）。
func (s *Sessions) Count() int {
	return len(s.List("").Sessions)
}
