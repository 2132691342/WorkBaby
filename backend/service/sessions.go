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
	// Active 当前绑定到引擎的会话文件（空表示未绑定）；改名要用它避让写入冲突
	Active func() string
}

// NewSessions 造一个会话门面。
func NewSessions(settings *Settings, store *repo.Store, layout pkg.Layout) *Sessions {
	return &Sessions{Settings: settings, Store: store, Layout: layout}
}

// List 扫会话清单。给了工作区只扫它的目录，否则扫全部。
// query 非空时按标题与正文过滤（大小写不敏感的包含匹配）。
func (s *Sessions) List(workspace, query string) domain.SessionListRESP {
	root := s.Layout.SessionsDir()
	dirs := s.scanDirs(root, workspace)
	needle := strings.ToLower(strings.TrimSpace(query))
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
			path := filepath.Join(dir, entry.Name())
			row := parser.Summarize(path)
			if needle != "" && !s.matches(row, path, needle) {
				continue
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].UpdatedAt > rows[j].UpdatedAt })
	return domain.SessionListRESP{Sessions: rows}
}

// matches 搜索判据：标题命中或正文命中；正文搜索只在标题未命中时才读文件。
func (s *Sessions) matches(row domain.SessionRowVO, file, needle string) bool {
	if strings.Contains(strings.ToLower(row.SessionTitle), needle) {
		return true
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(raw)), needle)
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
// 引擎目前正打开这份会话时拒掉：两路同时追加会写出交错的行，
// 而会话文件是唯一真相，写坏就整份回放不出来。
func (s *Sessions) Rename(file, title string) error {
	resolved, err := parser.GuardSession(s.Layout.SessionsDir(), file)
	if err != nil {
		return err
	}
	if s.Active != nil {
		if active := s.Active(); active != "" && strings.EqualFold(filepath.Clean(active), filepath.Clean(resolved)) {
			return pkg.Err(pkg.CodeEngineBusy, "这份会话正被引擎打开，切到其他会话后再改名")
		}
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

// Todos 当前待办：活动分支上最近一次 todo 工具调用派生（rpiv-todo 的回放语义）。
func (s *Sessions) Todos(file string) []domain.TodoVO {
	if file == "" {
		return []domain.TodoVO{}
	}
	resolved, err := parser.GuardSession(s.Layout.SessionsDir(), file)
	if err != nil {
		return []domain.TodoVO{}
	}
	return parser.TodosFromFile(resolved)
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
	return len(s.List("", "").Sessions)
}
