// Package service 是业务编排层：设置、运行时、会话、对话、插件、技能、MCP、文件、git。
// 它只依赖 repo / pkg / domain / engine，不感知 HTTP 与 Wails。
package service

import (
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/service/parser"
	"WorkBaby/backend/sse"
)

// 设置键名集中在这里：界面、托盘、首启读的是同一份名字，
// 散在调用处迟早会出现「设置页写了 A，聊天读的是 B」。
const (
	// KeyWorkspace 当前工作区
	KeyWorkspace = "workspace.current"
	// KeyWorkspaces 工作区历史（JSON 数组，最新在前）
	KeyWorkspaces = "workspace.history"
	// KeyLastSession 上次打开的会话文件
	KeyLastSession = "prefs.last_session_file"
	// KeyRestoreSession 启动时恢复上次会话
	KeyRestoreSession = "prefs.restore_session"
	// KeyCloseAction 点关闭按钮时做什么：tray / quit
	KeyCloseAction = "prefs.close_action"
	// KeyTrayHintShown 是否已解释过「收进托盘」
	KeyTrayHintShown = "prefs.tray_hint_shown"
	// KeyHideLedger 隐藏轮台账：`hide:<会话文件>` → JSON 数组
	KeyHideLedger = "chat.hide:"

	// CloseActionTray 收进托盘
	CloseActionTray = "tray"
	// CloseActionQuit 直接退出
	CloseActionQuit = "quit"
)

// Settings 是设置与工作区编排。
type Settings struct {
	// Store 设置库
	Store *repo.Store
	// Layout 沙箱布局
	Layout pkg.Layout
	// Hub 事件广播
	Hub *sse.Hub
	// Git git 编排：工作区行要带分支与改动数
	Git *Git
}

// All 读全部设置键值。
func (s *Settings) All() map[string]string {
	return s.Store.All()
}

// Get 读一个设置项。
func (s *Settings) Get(key string) string {
	value, _ := s.Store.Get(key)
	return value
}

// Save 批量保存设置；工作区变化会触发引擎重建，由调用方决定何时调。
func (s *Settings) Save(kv map[string]string) (map[string]string, error) {
	if err := s.Store.Save(kv); err != nil {
		return nil, err
	}
	return s.Store.All(), nil
}

// Workspace 当前工作区。
func (s *Settings) Workspace() string {
	return s.Get(KeyWorkspace)
}

// maxWorkspaces 是工作区历史容量。工作区不是标签页：十几条就够切换，
// 留太多会让一份短名单变成需要搜索的长名单。
const maxWorkspaces = 24

// SetWorkspace 切工作区：校验目录、写设置、记历史、广播。
func (s *Settings) SetWorkspace(dir string) (string, error) {
	target := strings.TrimSpace(dir)
	if target == "" {
		return "", pkg.Err(pkg.CodeBadRequest, "工作区路径不能为空")
	}
	if !pkg.IsDir(target) {
		return "", pkg.Err(pkg.CodeFileNotFound, "工作区不存在或不是目录", target)
	}
	if abs, err := filepath.Abs(target); err == nil {
		target = abs
	}
	rows := s.Workspaces()
	next := []domain.WorkspaceRowVO{{
		WorkspacePath:   target,
		WorkspaceExists: true,
		UpdatedAt:       pkg.NowMs(),
	}}
	for _, row := range rows {
		// 同一个目录只留最新那条，历史才不至于被重复项淹没。
		if strings.EqualFold(row.WorkspacePath, target) {
			continue
		}
		next = append(next, row)
	}
	if len(next) > maxWorkspaces {
		next = next[:maxWorkspaces]
	}
	if err := s.Store.Save(map[string]string{
		KeyWorkspace:  target,
		KeyWorkspaces: marshalRows(next),
	}); err != nil {
		return "", err
	}
	s.Hub.Broadcast("workspace:changed", sse.EngineTag(map[string]any{"workspace": target}))
	return target, nil
}

// ForgetWorkspace 从历史里去掉一个目录（不删磁盘）。
func (s *Settings) ForgetWorkspace(dir string) ([]domain.WorkspaceRowVO, error) {
	rows := s.Workspaces()
	kept := make([]domain.WorkspaceRowVO, 0, len(rows))
	for _, row := range rows {
		if strings.EqualFold(row.WorkspacePath, dir) {
			continue
		}
		kept = append(kept, row)
	}
	if err := s.Store.Save(map[string]string{KeyWorkspaces: marshalRows(kept)}); err != nil {
		return nil, err
	}
	return kept, nil
}

// Workspaces 工作区历史；每条都带上「目录还在不在」以及仓库与会话概况。
// 一次扫描把界面要的全部事实算齐：分开拉会让每一行各发一次 git 请求。
func (s *Settings) Workspaces() []domain.WorkspaceRowVO {
	rows := unmarshalRows(s.Get(KeyWorkspaces))
	for i := range rows {
		rows[i].WorkspaceExists = pkg.IsDir(rows[i].WorkspacePath)
		if !rows[i].WorkspaceExists {
			continue
		}
		rows[i].SessionCount = s.sessionCountOf(rows[i].WorkspacePath)
		if s.Git == nil {
			continue
		}
		rows[i].WorkspaceIsRepo, rows[i].WorkspaceBranch, rows[i].WorkspaceDirty = s.Git.ProbeOf(rows[i].WorkspacePath)
	}
	return rows
}

// sessionCountOf 数一个工作区目录下的会话文件。
// 目录编码规则与引擎一致，所以直接按编码目录找就行，不必扫全部会话。
func (s *Settings) sessionCountOf(workspace string) int {
	dir := filepath.Join(s.Layout.SessionsDir(), parser.EncodedWorkspace(workspace))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			count++
		}
	}
	return count
}

// WorkspaceList 工作区清单（含当前工作区），供工作区页默认打开的那一屏。
func (s *Settings) WorkspaceList() domain.WorkspaceListRESP {
	return domain.WorkspaceListRESP{
		Workspace:     s.Workspace(),
		WorkspaceRows: s.Workspaces(),
		ActiveEngine:  domain.EnginePi,
	}
}

// Prefs 组装设置页要的全部开关状态。
func (s *Settings) Prefs() domain.AppPrefsVO {
	restore, _ := s.Store.Get(KeyRestoreSession)
	hint, _ := s.Store.Get(KeyTrayHintShown)
	_ = hint
	return domain.AppPrefsVO{
		PrefValues:      s.Store.All(),
		AutoStart:       autoStartEnabled(),
		RestoreSession:  restore == "1" || restore == "true",
		LastSessionFile: s.Get(KeyLastSession),
		CloseAction:     s.closeAction(),
		DataDir:         s.Layout.DataDir,
		LogDir:          s.Layout.LogDir,
	}
}

// SetRestoreSession 写「启动时恢复上次会话」。
func (s *Settings) SetRestoreSession(enabled bool) (domain.AppPrefsVO, error) {
	if err := s.Store.Save(map[string]string{KeyRestoreSession: boolString(enabled)}); err != nil {
		return domain.AppPrefsVO{}, err
	}
	return s.Prefs(), nil
}

// SetCloseAction 写关窗行为。
func (s *Settings) SetCloseAction(action string) (domain.AppPrefsVO, error) {
	if action != CloseActionQuit {
		action = CloseActionTray
	}
	if err := s.Store.Save(map[string]string{KeyCloseAction: action}); err != nil {
		return domain.AppPrefsVO{}, err
	}
	return s.Prefs(), nil
}

// CloseAction 读关窗行为；没有记录时默认收托盘（正在跑的对话不该被一键弄死）。
func (s *Settings) CloseAction() string {
	return s.closeAction()
}

func (s *Settings) closeAction() string {
	if value := s.Get(KeyCloseAction); value == CloseActionQuit {
		return CloseActionQuit
	}
	return CloseActionTray
}

// MarkTrayHintShown 记住已经解释过「收进托盘」。
func (s *Settings) MarkTrayHintShown() {
	s.Store.Save(map[string]string{KeyTrayHintShown: "1"})
}

// TrayHintPending 还没解释过收托盘。
func (s *Settings) TrayHintPending() bool {
	value, ok := s.Store.Get(KeyTrayHintShown)
	return !ok || value != "1"
}

// RememberSession 记住上次会话。
func (s *Settings) RememberSession(file string) {
	if file == "" {
		return
	}
	s.Store.Save(map[string]string{KeyLastSession: file})
}

// lastSession 上次会话文件。
func (s *Settings) LastSession() string {
	return s.Get(KeyLastSession)
}

// boolString 布尔转设置值。
func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
