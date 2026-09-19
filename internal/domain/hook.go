// Package domain 本文件：UserHook 聚合根（user_hooks 表）——用户自定义生命周期钩子。
// 事件：SessionStart / UserPromptSubmit / PreToolUse / PermissionRequest / PostToolUse /
// PostToolUseFailure / Stop；子进程协议 stdin JSON in / stdout JSON out，退出码 2 = 阻断。
package domain

import (
	"encoding/json"

	"gorm.io/gorm"

	"WorkBaby/internal/pkg"
)

// Hook 事件枚举（七类；工具类事件支持 matcher 过滤，其余事件 matcher 一律忽略）。
const (
	HookEventSessionStart       = "SessionStart"       // 首轮模型请求前：注入项目约束 / 操作说明
	HookEventUserPromptSubmit   = "UserPromptSubmit"   // 用户提交后、模型调用前：补充上下文或阻断本次请求
	HookEventPreToolUse         = "PreToolUse"         // 工具执行前：allow / ask / deny
	HookEventPermissionRequest  = "PermissionRequest"  // 仅当权限结果需要询问时：自动允许或拒绝
	HookEventPostToolUse        = "PostToolUse"        // 工具成功：追加模型可见上下文
	HookEventPostToolUseFailure = "PostToolUseFailure" // 工具失败：追加恢复建议 / 诊断
	HookEventStop               = "Stop"               // 模型准备结束：返回 block 可让循环继续一轮
)

// HookEvents 全部合法事件（设置页下拉与后端校验共用）。
var HookEvents = []string{
	HookEventSessionStart, HookEventUserPromptSubmit, HookEventPreToolUse,
	HookEventPermissionRequest, HookEventPostToolUse, HookEventPostToolUseFailure,
	HookEventStop,
}

// HookEventUsesMatcher 该事件是否参与 matcher 过滤。
// UserPromptSubmit 与 Stop 不使用 matcher（即使填写也执行），与工具类事件区分。
func HookEventUsesMatcher(event string) bool {
	switch event {
	case HookEventPreToolUse, HookEventPermissionRequest, HookEventPostToolUse, HookEventPostToolUseFailure:
		return true
	}
	return false
}

// hookEventAliases 旧事件名 → 现行事件名：读库时归一，让既有配置继续生效。
var hookEventAliases = map[string]string{
	"run_start":   HookEventSessionStart,
	"before_tool": HookEventPreToolUse,
	"after_tool":  HookEventPostToolUse,
	"run_end":     HookEventStop,
}

// NormalizeHookEvent 把历史事件名归一为现行事件名；未知值原样返回（由校验层拒绝）。
func NormalizeHookEvent(event string) string {
	if v, ok := hookEventAliases[event]; ok {
		return v
	}
	return event
}

// UserHookDO 用户钩子持久化实体（user_hooks 表是唯一真相源）。
type UserHookDO struct {
	ID        string         `gorm:"primaryKey;size:64" json:"id"`
	Name      string         `gorm:"size:128" json:"name"`            // 展示名
	Event     string         `gorm:"size:32;index" json:"event"`      // 触发事件
	Matcher   string         `gorm:"size:256" json:"matcher"`         // 工具名匹配（空 / * = 全部；名单；正则）
	Command   string         `gorm:"size:1024" json:"command"`        // 子进程命令行（经 cmd /c 解析）
	TimeoutMs int            `gorm:"default:10000" json:"timeout_ms"` // 超时（毫秒），上限 60000
	Enabled   bool           `gorm:"default:true" json:"enabled"`     // 启用开关
	Sort      int            `gorm:"default:0" json:"sort"`           // 执行顺序（小者先）
	CreatedAt int64          `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt int64          `gorm:"autoUpdateTime:milli" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 固定表名。
func (UserHookDO) TableName() string { return "user_hooks" }

// UserHookRESP 出参。
type UserHookRESP struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Event     string `json:"event"`
	Matcher   string `json:"matcher"`
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms"`
	Enabled   bool   `json:"enabled"`
	Sort      int    `json:"sort"`
}

// UserHookREQ 创建/更新入参（按 ID upsert，ID 空 = 新建）。
type UserHookREQ struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Event     string `json:"event"`
	Matcher   string `json:"matcher"`
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms"`
	Enabled   *bool  `json:"enabled"` // nil = 保留现值
	Sort      *int   `json:"sort"`
}

// HookPayload 子进程 stdin 载荷（一行 JSON + 换行）。
type HookPayload struct {
	SessionID      string          `json:"session_id,omitempty"`
	Cwd            string          `json:"cwd,omitempty"`
	PermissionMode string          `json:"permission_mode,omitempty"`
	HookEventName  string          `json:"hook_event_name"`
	Source         string          `json:"source,omitempty"` // SessionStart：startup / resume / compact
	Prompt         string          `json:"prompt,omitempty"` // UserPromptSubmit
	ToolName       string          `json:"tool_name,omitempty"`
	ToolInput      json.RawMessage `json:"tool_input,omitempty"`
	ToolUseID      string          `json:"tool_use_id,omitempty"`
	ToolResponse   string          `json:"tool_response,omitempty"` // PostToolUse
	Error          string          `json:"error,omitempty"`         // PostToolUseFailure
	IsInterrupt    bool            `json:"is_interrupt,omitempty"`
	// StopHookActive 本次 Stop 由「上一次 Stop 的 block 续跑」触发；用于防无限续跑。
	StopHookActive       bool   `json:"stop_hook_active,omitempty"`
	LastAssistantMessage string `json:"last_assistant_message,omitempty"`
	// RunID 本地扩展：与 run 事件日志对账。
	RunID string `json:"run_id,omitempty"`
}

// HookPermissionDecision PermissionRequest 的 behavior 决策体。
type HookPermissionDecision struct {
	Behavior string `json:"behavior"` // allow / deny
	Message  string `json:"message,omitempty"`
}

// HookSpecificOutput 事件归属明确的标准输出体（优先于顶层兼容字段）。
type HookSpecificOutput struct {
	HookEventName            string                  `json:"hookEventName,omitempty"`
	AdditionalContext        string                  `json:"additionalContext,omitempty"`
	PermissionDecision       string                  `json:"permissionDecision,omitempty"` // allow / ask / deny
	PermissionDecisionReason string                  `json:"permissionDecisionReason,omitempty"`
	Decision                 *HookPermissionDecision `json:"decision,omitempty"`
}

// HookDecision 子进程 stdout 决策（空 stdout = 成功且无附加效果）。
type HookDecision struct {
	// Continue 仅 UserPromptSubmit 使用：显式 false 阻断本次用户请求。
	Continue *bool  `json:"continue,omitempty"`
	Reason   string `json:"reason,omitempty"`
	// Decision 仅 Stop 使用：block 让主循环带着 Reason 再跑一轮。
	Decision           string              `json:"decision,omitempty"`
	AdditionalContext  string              `json:"additionalContext,omitempty"`
	AdditionalContext2 string              `json:"additional_context,omitempty"` // 兼容下划线写法
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`

	// ExitCode 退出码（2 = 阻断快捷方式）；不参与 JSON 反序列化。
	ExitCode int `json:"-"`
}

// ContextText 该决策要注入模型的补充上下文（标准字段优先，兼容顶层写法）。
func (d HookDecision) ContextText() string {
	if d.HookSpecificOutput != nil && d.HookSpecificOutput.AdditionalContext != "" {
		return d.HookSpecificOutput.AdditionalContext
	}
	if d.AdditionalContext != "" {
		return d.AdditionalContext
	}
	return d.AdditionalContext2
}

// ToolPermission 工具类事件的权限决策：返回 allow / ask / deny 与理由。
// 退出码 2 等价于 deny / block（无 JSON 输出也生效）。
func (d HookDecision) ToolPermission() (decision, reason string) {
	if d.HookSpecificOutput != nil {
		if s := d.HookSpecificOutput.PermissionDecision; s != "" {
			return s, d.HookSpecificOutput.PermissionDecisionReason
		}
		if pd := d.HookSpecificOutput.Decision; pd != nil {
			switch pd.Behavior {
			case "allow":
				return "allow", pd.Message
			case "deny":
				return "deny", pd.Message
			}
		}
	}
	if d.ExitCode == 2 {
		return "deny", d.Reason
	}
	if d.Decision == "deny" {
		return "deny", d.Reason
	}
	if d.Decision == "allow" {
		return "allow", d.Reason
	}
	return "", ""
}

// HookTestRESP 设置页试跑结果。
type HookTestRESP struct {
	Decision   string `json:"decision"`
	Reason     string `json:"reason"`
	Context    string `json:"context"`
	Err        string `json:"err"`
	DurationMs int64  `json:"duration_ms"`
}

// 包级错误变量；错误码段位 8600（UserHook 聚合根）。
var (
	ErrHookNotFound   = pkg.New(8601, "user hook not found", "")
	ErrHookInvalid    = pkg.New(8602, "user hook invalid", "")
	ErrHookTestFailed = pkg.New(8603, "user hook test run failed", "")
)
