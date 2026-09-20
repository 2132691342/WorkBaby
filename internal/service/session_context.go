package service

import (
	"context"
	"path/filepath"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/runtime"
)

// SessionContext 会话级路径解析：工作区根与过程数据目录的唯一数据源。
// 独立于 ChatService，供工具沙箱、快照、文件面板与 chat 共用（避免多份目录规则）。
type SessionContext struct {
	home     string
	sessions *repo.ChatSessionRepo
}

// NewSessionContext 构造会话路径解析器；home 为数据根（paths.Home）。
func NewSessionContext(home string, sessions *repo.ChatSessionRepo) *SessionContext {
	return &SessionContext{home: home, sessions: sessions}
}

// WorkspaceRoot 会话绑定的工作区根；未绑定或查询失败回落到 defRoot。
func (c *SessionContext) WorkspaceRoot(ctx context.Context, sessionID, defRoot string) string {
	if sessionID == "" || c.sessions == nil {
		return defRoot
	}
	row, err := c.sessions.GetByID(ctx, sessionID)
	if err != nil || row == nil {
		if err != nil {
			pkg.L.Warn("workspace root resolve failed, fallback to default", "session", sessionID, "err", err.Error())
		}
		return defRoot
	}
	if p := strings.TrimSpace(row.WorkspacePath); p != "" {
		return p
	}
	return defRoot
}

// DataDirs 会话的记忆文件与快照目录：绑定本地工作区 → {dir}/.workbaby/ 下（过程数据跟工作区走）；
// 未绑定 → {home} 下。
func (c *SessionContext) DataDirs(ctx context.Context, sessionID string) (memoryFile, snapshotDir string) {
	memoryFile = filepath.Join(c.home, "memory", sessionID, "MEMORY.md")
	snapshotDir = filepath.Join(c.home, "snapshots", sessionID)
	if sessionID == "" || c.sessions == nil {
		return
	}
	row, err := c.sessions.GetByID(ctx, sessionID)
	if err != nil || row == nil {
		return
	}
	if wp := strings.TrimSpace(row.WorkspacePath); wp != "" {
		sb := runtime.SandboxOf(wp)
		memoryFile = filepath.Join(sb.Memory, sessionID, "MEMORY.md")
		snapshotDir = filepath.Join(sb.Snapshots, sessionID)
	}
	return
}

// MemoryEnabled 记忆全局开关（设置缺失或读取失败时按开启处理）。
func MemoryEnabled(ctx context.Context, setRepo *repo.SystemSettingRepo) bool {
	if setRepo == nil {
		return true
	}
	row, err := setRepo.Get(ctx, domain.SettingKeyMemoryEnabled)
	if err != nil || row == nil {
		return true
	}
	return strings.TrimSpace(row.V) != "false"
}
