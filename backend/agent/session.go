package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// EntryType 是会话条目的种类；与落盘形态共用一套词汇。
type EntryType = domain.SessionEntryType

const (
	// EntryMessage 一条对话消息
	EntryMessage = domain.SessionEntryMessage
	// EntrySessionInfo 会话改名
	EntrySessionInfo = domain.SessionEntrySessionInfo
	// EntryModelChange 切换模型
	EntryModelChange = domain.SessionEntryModelChange
	// EntryThinkingChange 切换思考档
	EntryThinkingChange = domain.SessionEntryThinkingChange
	// EntryUsage 一次独立计费（缓存预热之类）
	EntryUsage = domain.SessionEntryUsage
	// EntryCompaction 上下文压缩
	EntryCompaction = domain.SessionEntryCompaction
)

// Session 是一份打开着的会话：文件是唯一真相，内存只做追加缓冲与叶子指针。
type Session struct {
	// mu 串行化写入：并发追加会交错成半行，整份会话就回放不出来
	mu sync.Mutex
	// file 会话文件绝对路径
	file string
	// id 会话 ID
	id string
	// cwd 绑定的工作目录
	cwd string
	// title 当前标题
	title string
	// leafId 活动分支末端条目
	leafId string
	// entries 已落盘的条目（内存镜像，供组装上下文）
	entries []domain.SessionEntryDO
}

// Store 是会话目录的读写门面。
type Store struct {
	// Root 会话根目录
	Root string
	// Workspace 当前工作区；打开新会话时写进会话头
	Workspace func() string
}

// NewStore 造一个会话存储。
func NewStore(root string, workspace func() string) *Store {
	return &Store{Root: root, Workspace: workspace}
}

// Dir 是某个工作区对应的会话目录。
func (s *Store) Dir(workspace string) string {
	return pkg.SessionDirOf(s.Root, workspace)
}

// Create 新建一份会话文件并返回句柄。
func (s *Store) Create(workspace string) (*Session, error) {
	dir := s.Dir(workspace)
	if err := pkg.EnsureDir(dir); err != nil {
		return nil, pkg.WrapErr(pkg.CodeSessionIO, "会话目录创建失败", err)
	}
	id := pkg.NewID(pkg.PrefixSession)
	file := filepath.Join(dir, id+".jsonl")
	head := domain.SessionHeaderDO{
		HeaderType:      "session",
		HeaderVersion:   1,
		HeaderId:        id,
		HeaderTimestamp: pkg.NowISO(),
		HeaderCwd:       workspace,
	}
	raw, err := json.Marshal(head)
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeSessionIO, "会话头编码失败", err)
	}
	if err := os.WriteFile(file, append(raw, '\n'), 0o644); err != nil {
		return nil, pkg.WrapErr(pkg.CodeSessionIO, "会话文件创建失败", err)
	}
	return &Session{file: file, id: id, cwd: workspace}, nil
}

// Open 打开一份已有会话。
func (s *Store) Open(file string) (*Session, error) {
	guarded, err := Guard(s.Root, file)
	if err != nil {
		return nil, err
	}
	entries, err := ReadEntries(guarded)
	if err != nil {
		return nil, err
	}
	session := &Session{file: guarded, entries: entries}
	if head, ok := readHeader(guarded); ok {
		session.id = head.HeaderId
		session.cwd = head.HeaderCwd
		session.title = head.HeaderTitle
	}
	if len(entries) > 0 {
		session.leafId = entries[len(entries)-1].EntryId
	}
	// 标题不在会话头里——头只在创建时写一次，之后永不重写。
	// 标题由改名条目派生：它是活动分支上的一条事实，不是文件的属性。
	session.title = titleFromEntries(entries)
	return session, nil
}

// File 会话文件路径。
func (s *Session) File() string { return s.file }

// Id 会话 ID。
func (s *Session) Id() string { return s.id }

// Cwd 会话绑定的工作目录。
func (s *Session) Cwd() string { return s.cwd }

// Title 当前标题。
func (s *Session) Title() string { return s.title }

// Entries 已落盘条目的副本。
func (s *Session) Entries() []domain.SessionEntryDO {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.SessionEntryDO, len(s.entries))
	copy(out, s.entries)
	return out
}

// AppendMessage 追加一条消息条目并推进叶子指针。
func (s *Session) AppendMessage(msg Message) (domain.SessionEntryDO, error) {
	if msg.CreatedAt == 0 {
		msg.CreatedAt = pkg.NowMs()
	}
	return s.append(domain.SessionEntryDO{
		EntryType:    domain.SessionEntryMessage,
		EntryMessage: messageToDO(msg),
	})
}

// Rename 追加一条改名条目。
func (s *Session) Rename(title string) (domain.SessionEntryDO, error) {
	name := strings.TrimSpace(title)
	if name == "" {
		return domain.SessionEntryDO{}, pkg.Err(pkg.CodeBadRequest, "会话标题不能为空")
	}
	entry, err := s.append(domain.SessionEntryDO{
		EntryType: domain.SessionEntrySessionInfo,
		EntryName: name,
	})
	if err != nil {
		return domain.SessionEntryDO{}, err
	}
	s.mu.Lock()
	s.title = name
	s.mu.Unlock()
	return entry, nil
}

// AppendUsage 追加一条独立用量条目（不对应任何消息）。
func (s *Session) AppendUsage(usage Usage) (domain.SessionEntryDO, error) {
	return s.append(domain.SessionEntryDO{
		EntryType: domain.SessionEntryUsage,
		EntryUsage: &domain.SessionUsageDO{
			UsageInput:       usage.InputTokens,
			UsageOutput:      usage.OutputTokens,
			UsageCacheRead:   usage.CacheReadTokens,
			UsageCacheWrite:  usage.CacheWriteTokens,
			UsageTotalTokens: usage.TotalTokens(),
		},
	})
}

// CompactHint 在会话里标记一次压缩。
// 引擎不做摘要重写：原文一律不删——把历史悄无声息地换掉，
// 用户下次翻回去就找不到自己说过的话了。
func (s *Session) CompactHint(instructions string) (domain.SessionEntryDO, error) {
	entry, err := s.append(domain.SessionEntryDO{EntryType: domain.SessionEntryCompaction})
	if err != nil {
		return domain.SessionEntryDO{}, err
	}
	_ = instructions
	return entry, nil
}

// AppendCustom 追加一条自定义条目：引擎用它表达自己的状态（如计划模式开关）。
func (s *Session) AppendCustom(customType string, data map[string]any) (domain.SessionEntryDO, error) {
	return s.append(domain.SessionEntryDO{
		EntryType:       domain.SessionEntryCustom,
		EntryCustomType: customType,
		EntryData:       data,
	})
}

// append 是唯一的写入口：分配 ID 与父指针、整行落盘、推进内存镜像。
// 整段持锁：两路并发追加会写出交错行，那是整份会话读不出来的唯一原因。
func (s *Session) append(entry domain.SessionEntryDO) (domain.SessionEntryDO, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.EntryId == "" {
		entry.EntryId = pkg.NewID(pkg.PrefixSession)
	}
	entry.EntryParentId = s.leafId
	if entry.EntryTimestamp == "" {
		entry.EntryTimestamp = pkg.NowISO()
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return domain.SessionEntryDO{}, pkg.WrapErr(pkg.CodeSessionIO, "会话条目编码失败", err)
	}
	handle, err := os.OpenFile(s.file, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return domain.SessionEntryDO{}, pkg.WrapErr(pkg.CodeSessionIO, "会话文件打开失败", err)
	}
	defer handle.Close()
	if _, err := handle.Write(append(raw, '\n')); err != nil {
		return domain.SessionEntryDO{}, pkg.WrapErr(pkg.CodeSessionIO, "会话写入失败", err)
	}
	s.entries = append(s.entries, entry)
	s.leafId = entry.EntryId
	// 首条用户消息即标题：界面清单不该显示一串 ULID。
	if entry.EntryType == domain.SessionEntryMessage && entry.EntryMessage != nil &&
		entry.EntryMessage.MessageRole == string(RoleUser) && s.title == "" {
		if title := TitleOf(entry.EntryMessage.MessageContent); title != "" {
			s.title = title
		}
	}
	return entry, nil
}

// TitleOf 把一段文本压成一行标题。
func TitleOf(content any) string {
	text := contentTextOf(content)
	clean := strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if clean == "" {
		return ""
	}
	runes := []rune(clean)
	if len(runes) <= 24 {
		return clean
	}
	return string(runes[:24]) + "…"
}

// titleFromEntries 从活动分支派生当前标题。
// 改名条目优先，其次首条用户消息：两者都是会话文件里的事实，
// 所以重开应用、切回旧会话都能还原出同一个标题。
func titleFromEntries(entries []domain.SessionEntryDO) string {
	title := ""
	for _, entry := range entries {
		switch entry.EntryType {
		case domain.SessionEntrySessionInfo:
			if name := strings.TrimSpace(entry.EntryName); name != "" {
				title = name
			}
		case domain.SessionEntryMessage:
			if title != "" || entry.EntryMessage == nil {
				continue
			}
			if entry.EntryMessage.MessageRole != string(RoleUser) {
				continue
			}
			if text := TitleOf(entry.EntryMessage.MessageContent); text != "" {
				title = text
			}
		}
	}
	return title
}

// readHeader 读会话头。
func readHeader(file string) (domain.SessionHeaderDO, bool) {
	handle, err := os.Open(file)
	if err != nil {
		return domain.SessionHeaderDO{}, false
	}
	defer handle.Close()
	buffer := make([]byte, 8<<10)
	n, err := handle.Read(buffer)
	if n <= 0 {
		return domain.SessionHeaderDO{}, false
	}
	var head domain.SessionHeaderDO
	if json.Unmarshal(buffer[:n], &head) != nil || head.HeaderType != "session" {
		return domain.SessionHeaderDO{}, false
	}
	return head, true
}

// ReadEntries 读会话文件里的全部条目，跳过会话头与坏行。
// 坏行直接跳过：中途断电会留下残行，一行残行不该让整份历史读不出来。
func ReadEntries(file string) ([]domain.SessionEntryDO, error) {
	lines, err := pkg.ReadSessionLines(file)
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeSessionIO, "会话文件读取失败", err)
	}
	out := make([]domain.SessionEntryDO, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
			continue
		}
		var entry domain.SessionEntryDO
		if json.Unmarshal([]byte(trimmed), &entry) != nil {
			continue
		}
		// 会话头只有 type 一个可辨识字段，其余按条目语义读进来就是空条目。
		if entry.EntryType == "" || entry.EntryType == "session" {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

// Guard 校验会话文件落在会话根内；越界路径一律拒。
func Guard(root, file string) (string, error) {
	clean := filepath.Clean(file)
	if !pkg.Exists(clean) {
		return "", pkg.Err(pkg.CodeNotFound, "会话文件不存在", clean)
	}
	if !pkg.Inside(root, clean) {
		return "", pkg.Err(pkg.CodeSessionIO, "会话路径越界", clean)
	}
	return clean, nil
}
