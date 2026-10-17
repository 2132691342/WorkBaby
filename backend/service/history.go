// 通用会话历史的编排：扫描、读取、目录配置与搬到本机会话接着聊。
package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/history"
	"WorkBaby/backend/pkg"
)

// HistoryService 通用会话历史：扫描翻看 + 接着聊（specs/12）。
// 自定义文件夹走 settings KV（history_folders），不建表——翻看不需要持久化。
type HistoryService struct {
	env      *Env
	sessions *SessionService
	chat     *ChatService
}

// NewHistoryService 构造历史服务。
func NewHistoryService(env *Env, sessions *SessionService, chat *ChatService) *HistoryService {
	return &HistoryService{env: env, sessions: sessions, chat: chat}
}

// historyFoldersKey 自定义文件夹设置键：JSON 对象 {source: dir}。
const historyFoldersKey = "history_folders"

// Scan 扫描全部来源。
func (s *HistoryService) Scan() (domain.HistoryScanRESP, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	extra := s.folders()
	return history.Scan(history.DefaultRoots(home), extra), nil
}

// Read 读一段会话的完整消息（只读）。
func (s *HistoryService) Read(id string) (*domain.HistoryEntryVO, error) {
	source, path, ok := splitEntryID(id)
	if !ok {
		return nil, pkg.New("9301", "这条会话记录已过期", "刷新列表后再试")
	}
	e, err := history.ParseFile(source, path)
	if err != nil {
		return nil, pkg.Wrap("9303", "这条会话读不出来：可能被移动或删除了", err)
	}
	return &e, nil
}

// Folders 已注册的自定义文件夹。
func (s *HistoryService) Folders() (map[string]string, error) {
	return s.folders(), nil
}

// AddFolder 注册自定义文件夹：目录必须存在（不存在的注册项只会让每次扫描都白跑）。
func (s *HistoryService) AddFolder(source, dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return pkg.New("9302", "文件夹路径不能为空", "")
	}
	if !pkg.DirExists(dir) {
		return pkg.New("9302", "这个文件夹不存在", dir)
	}
	if source == "" {
		source = history.SourceCustom
	}
	cur := s.folders()
	cur[source] = dir
	return s.saveFolders(cur)
}

// RemoveFolder 删除注册。
func (s *HistoryService) RemoveFolder(source string) error {
	cur := s.folders()
	delete(cur, source)
	return s.saveFolders(cur)
}

// Continue 接着聊：新建会话 + 首条上下文注入原文摘要（注明来源与时间），
// 返回会话 id 与预填的下一句（用户补的那句）。
func (s *HistoryService) Continue(entryID, next string) (*domain.SessionVO, error) {
	e, err := s.Read(entryID)
	if err != nil {
		return nil, err
	}
	summary := historySummary(e)
	sess, err := s.sessions.Create(domain.CreateSessionREQ{
		Title: "接着聊：" + e.Title,
	})
	if err != nil {
		return nil, err
	}
	ctxMsg := "下面是我之前在「" + sourceLabel(e.Source) + "」里的一段对话（" +
		formatWhen(e.StartedAt) + "），接下去我们继续这件事：\n\n" + summary
	if _, err := s.chat.Send(sess.ID, ctxMsg, nil); err != nil {
		return nil, err
	}
	if next != "" {
		if _, err := s.chat.Send(sess.ID, next, nil); err != nil {
			return nil, err
		}
	}
	vo := sess
	return vo, nil
}

// historySummary 把一段历史折成注入用的摘要：上限 40 条 / 8000 字。
// 注入全文会把窗口一次吃光；原文随时能在原文件里看。
func historySummary(e *domain.HistoryEntryVO) string {
	const maxMsgs = 40
	const maxChars = 8000
	msgs := e.Messages
	if len(msgs) > maxMsgs {
		msgs = msgs[len(msgs)-maxMsgs:]
	}
	var sb strings.Builder
	for _, m := range msgs {
		if m.Role == domain.RoleTool {
			continue // 工具原始输出对「接上话」没有信息量
		}
		label := "我"
		if m.Role == domain.RoleAssistant {
			label = "助手"
		}
		sb.WriteString(label + "：" + m.Text + "\n")
	}
	out := sb.String()
	if len([]rune(out)) > maxChars {
		out = string([]rune(out)[:maxChars]) + "\n……（更早的内容省略）"
	}
	return out
}

func sourceLabel(s string) string {
	switch s {
	case history.SourceClaudeCode:
		return "Claude Code"
	case history.SourceCodex:
		return "Codex"
	default:
		return "外部工具"
	}
}

func formatWhen(ms int64) string {
	if ms <= 0 {
		return "时间未知"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04")
}

func (s *HistoryService) folders() map[string]string {
	out := map[string]string{}
	raw, _ := s.env.Repo.GetSetting(historyFoldersKey)
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func (s *HistoryService) saveFolders(m map[string]string) error {
	b, err := json.Marshal(m)
	if err != nil {
		return pkg.Wrap("2006", "保存文件夹设置失败", err)
	}
	return s.env.Repo.SetSetting(historyFoldersKey, string(b))
}

// splitEntryID 拆 "source:path"。路径里可能带冒号（盘符），只拆第一段。
func splitEntryID(id string) (source, path string, ok bool) {
	i := strings.Index(id, ":")
	if i <= 0 || i == len(id)-1 {
		return "", "", false
	}
	return id[:i], filepath.FromSlash(id[i+1:]), true
}
