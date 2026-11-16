// Package service 是业务编排层：设置、会话、对话、技能、MCP、文件、git、联网。
// 它只依赖 repo / pkg / domain / engine，不感知 HTTP 与 Wails。
package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
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
	// KeyActiveProvider 当前选中的服务商
	KeyActiveProvider = "model.active_provider"
	// KeyActiveModel 当前选中的模型
	KeyActiveModel = "model.active_model"
	// KeyAutoCompaction 自动压缩开关（"0" 为关）
	KeyAutoCompaction = "model.auto_compaction"
	// KeyDefaultProvider 新会话的默认服务商
	KeyDefaultProvider = "model.default_provider"
	// KeyDefaultModel 新会话的默认模型
	KeyDefaultModel = "model.default_model"
	// KeyDefaultThinkingLevel 新会话的默认思考档
	KeyDefaultThinkingLevel = "model.default_thinking_level"
	// KeyThinkingLevel 当前思考档
	KeyThinkingLevel = "model.thinking_level"
	// KeyWebSearchKind 联网搜索服务的种类
	KeyWebSearchKind = "web.search_kind"
	// KeyWebSearchKey 联网搜索服务的密钥
	KeyWebSearchKey = "web.search_key"
	// KeyMentionSessions @ 引用里列出最近几条会话
	KeyMentionSessions = "ui.mention_sessions"
	// KeyFontScale 界面字号缩放
	KeyFontScale = "ui.font_scale"
	// KeyReduceMotion 是否减弱动效
	KeyReduceMotion = "ui.reduce_motion"
	// KeyLocale 界面语言
	KeyLocale = "ui.locale"
	// KeyBashConfirm 危险命令确认闸门（"0" 为关，默认开）
	KeyBashConfirm = "agent.bash_confirm"

	// CloseActionTray 收进托盘
	CloseActionTray = "tray"
	// CloseActionQuit 直接退出
	CloseActionQuit = "quit"
)

// defaultSettings 是界面类设置的默认值。首启写入一份：
// 界面读到的就是明确的值，不必各自兜底一个数字。
var defaultSettings = map[string]string{
	KeyMentionSessions: "5",
	KeyFontScale:       "1",
	KeyReduceMotion:    "0",
	KeyLocale:          "zh-CN",
	KeyBashConfirm:     "1",
}

// EnsureDefaults 把缺失的界面设置补成默认值，返回是否有写入。
// 只在缺键时写：用户改过的值一个字不动。
func (s *Settings) EnsureDefaults() bool {
	missing := map[string]string{}
	for key, value := range defaultSettings {
		if _, ok := s.Store.Get(key); !ok {
			missing[key] = value
		}
	}
	if len(missing) == 0 {
		return false
	}
	if err := s.Store.Save(missing); err != nil {
		pkg.Warn("界面默认设置写入失败：%v", err)
		return false
	}
	return true
}

// BashConfirmEnabled 危险命令确认闸门是否开启；只有显式 "0" 才关。
// 缺键/坏值一律按开处理：安全开关的兜底方向必须是更严的那边。
func (s *Settings) BashConfirmEnabled() bool {
	return strings.TrimSpace(s.Get(KeyBashConfirm)) != "0"
}

// DefaultThinkingLevel 是新会话起步的思考档：读 `model.default_thinking_level`，
// 非法或缺失时给兜底档。当前档与会话档分开存，所以这条只在「用户没选过」时生效。
func (s *Settings) DefaultThinkingLevel() string {
	if level := strings.TrimSpace(s.Get(KeyDefaultThinkingLevel)); thinkingLevels[level] {
		return level
	}
	return fallbackThinkingLevel
}

// Settings 是设置与工作区编排。
type Settings struct {
	// Store 设置库
	Store *repo.Store
	// Layout 目录布局
	Layout pkg.Layout
	// Hub 事件广播
	Hub *sse.Hub
	// Git git 编排：工作区行要带分支与改动数
	Git *Git
}

// All 读全部设置键值；密钥类键在出口处换成掩码。
// 读接口是界面唯一的设置来源，原样回显等于把联网密钥交给渲染进程。
func (s *Settings) All() map[string]string {
	return maskSettings(s.Store.All())
}

// maskSettings 把密钥类键的值换成掩码；其余键原样返回。
// 掩码只用于读回显：写入路径不经过它，空串语义仍是「保留原值」。
func maskSettings(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	if value := out[KeyWebSearchKey]; value != "" {
		out[KeyWebSearchKey] = "••••"
	}
	return out
}

// reservedSettings 是由专门接口写入的设置键：普通保存入口不接受它们。
// 工作区变更要校验目录并同步历史，绕过它就能把文件接口的根指到任意位置。
var reservedSettings = map[string]string{
	KeyWorkspace:  "工作区请走 /settings/workspace（要校验目录并同步历史）",
	KeyWorkspaces: "工作区历史由 /settings/workspace 维护，不能直接写",
}

// Get 读一个设置项。
func (s *Settings) Get(key string) string {
	value, _ := s.Store.Get(key)
	return value
}

// Save 批量保存设置。
// 保留键在这里就拒：工作区是文件接口的根，绕过 SetWorkspace 的目录校验与历史同步
// 等于把 1002 路径守卫连根拔掉。拒而不是静默丢弃，否则界面以为改上了。
func (s *Settings) Save(kv map[string]string) (map[string]string, error) {
	for key := range kv {
		if message, reserved := reservedSettings[key]; reserved {
			return nil, pkg.Err(pkg.CodeBadRequest, message)
		}
	}
	// 界面类键在入口收敛：手改过的坏值不该一路带进界面，
	// 也不该让 @ 引用因为一个 0 而一条会话都不列。
	s.normalize(kv)
	if err := s.Store.Save(kv); err != nil {
		return nil, err
	}
	return s.All(), nil
}

// normalize 把界面类设置收敛到合法范围；就地改写入映射。
func (s *Settings) normalize(kv map[string]string) {
	if raw, ok := kv[KeyMentionSessions]; ok {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 1 {
			n = 1
		}
		if n > 20 {
			n = 20
		}
		kv[KeyMentionSessions] = strconv.Itoa(n)
	}
}

// SaveSettings 是 Save 的别名：调用方拿的是业务语义而不是存储细节。
func (s *Settings) SaveSettings(kv map[string]string) error {
	return s.Store.Save(kv)
}

// Workspace 当前工作区。
func (s *Settings) Workspace() string {
	return s.Get(KeyWorkspace)
}

// maxWorkspaces 是工作区历史容量。工作区不是标签页：十几条就够切换，
// 留太多会让一份短名单变成需要搜索的长名单。
const maxWorkspaces = 24

// SetWorkspace 切工作区：校验目录、写设置、记历史、广播。
func (s *Settings) SetWorkspace(ctx context.Context, dir string) (string, error) {
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
	rows := s.Workspaces(ctx)
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
func (s *Settings) ForgetWorkspace(ctx context.Context, dir string) ([]domain.WorkspaceRowVO, error) {
	rows := s.Workspaces(ctx)
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
func (s *Settings) Workspaces(ctx context.Context) []domain.WorkspaceRowVO {
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
		rows[i].WorkspaceIsRepo, rows[i].WorkspaceBranch, rows[i].WorkspaceDirty = s.Git.ProbeOf(ctx, rows[i].WorkspacePath)
	}
	return rows
}

// sessionCountOf 数一个工作区目录下的会话文件。
// 目录编码规则与引擎一致，所以直接按编码目录找就行，不必扫全部会话。
func (s *Settings) sessionCountOf(workspace string) int {
	dir := filepath.Join(s.Layout.SessionsDir, pkg.EncodedWorkspace(workspace))
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
func (s *Settings) WorkspaceList(ctx context.Context) domain.WorkspaceListRESP {
	return domain.WorkspaceListRESP{
		Workspace:     s.Workspace(),
		WorkspaceRows: s.Workspaces(ctx),
		ActiveEngine:  domain.EngineNative,
	}
}

// Prefs 组装设置页要的全部开关状态。
func (s *Settings) Prefs(ctx context.Context) domain.AppPrefsVO {
	restore, _ := s.Store.Get(KeyRestoreSession)
	hint, _ := s.Store.Get(KeyTrayHintShown)
	_ = hint
	return domain.AppPrefsVO{
		PrefValues:      s.All(),
		AutoStart:       autoStartEnabled(ctx),
		RestoreSession:  restore == "1" || restore == "true",
		LastSessionFile: s.Get(KeyLastSession),
		CloseAction:     s.closeAction(),
		DataDir:         s.Layout.DataDir,
		LogDir:          s.Layout.LogDir,
	}
}

// SetRestoreSession 写「启动时恢复上次会话」。
func (s *Settings) SetRestoreSession(ctx context.Context, enabled bool) (domain.AppPrefsVO, error) {
	if err := s.Store.Save(map[string]string{KeyRestoreSession: boolString(enabled)}); err != nil {
		return domain.AppPrefsVO{}, err
	}
	return s.Prefs(ctx), nil
}

// SetCloseAction 写关窗行为。
func (s *Settings) SetCloseAction(ctx context.Context, action string) (domain.AppPrefsVO, error) {
	if action != CloseActionQuit {
		action = CloseActionTray
	}
	if err := s.Store.Save(map[string]string{KeyCloseAction: action}); err != nil {
		return domain.AppPrefsVO{}, err
	}
	return s.Prefs(ctx), nil
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
