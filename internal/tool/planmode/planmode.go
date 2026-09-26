// Package planmode 计划模式：进入后 run 内所有有副作用的工具被拦截，
// 只允许只读工具做调研；退出计划必须经用户批准。
package planmode

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// Store 会话级计划模式状态（内存态：计划模式是 run 内的工作方式，不跨重启恢复）。
type Store struct {
	mu       sync.RWMutex
	active   map[string]bool
	readonly func(toolName string) bool // 工具名 → 是否只读（装配方注入 registry 查询）
}

// NewStore 构造；readonly 判定函数由装配方注入（基于 tool registry 的 Meta）。
func NewStore(readonly func(toolName string) bool) *Store {
	return &Store{active: map[string]bool{}, readonly: readonly}
}

// Active 会话是否处于计划模式。
func (s *Store) Active(sessionID string) bool {
	if s == nil || sessionID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active[sessionID]
}

// Enable / Disable 切换状态。
func (s *Store) Enable(sessionID string) { s.set(sessionID, true) }

// Disable 关闭计划模式。
func (s *Store) Disable(sessionID string) { s.set(sessionID, false) }

func (s *Store) set(sessionID string, v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[sessionID] = v
}

// Guard 拦截钩子：计划模式中非只读工具一律拒绝（执行器级硬拦，不靠 prompt 自觉）。
// 返回空串表示放行，非空串为拒绝原因（走 refused 语义回填模型）。
func (s *Store) Guard(sessionID, toolName string) string {
	if !s.Active(sessionID) {
		return ""
	}
	if toolName == exitToolName {
		return "" // exit 本身放行（其内部走审批）
	}
	if s.readonly != nil && s.readonly(toolName) {
		return ""
	}
	return "plan mode active: 只有只读工具可用。请完成调研后用 todo(plan) 记录计划，" +
		"再调用 exit_plan_mode 请求用户批准后执行。"
}

// ===== 工具定义 =====

const (
	enterToolName = "enter_plan_mode"
	exitToolName  = "exit_plan_mode"
)

// EnterTool 进入计划模式（只读调研 + 出计划）。
type EnterTool struct{ store *Store }

// NewEnter 构造 enter_plan_mode。
func NewEnter(store *Store) *EnterTool { return &EnterTool{store: store} }

func (t *EnterTool) Name() string              { return enterToolName }
func (t *EnterTool) RiskLevel() tool.RiskLevel { return tool.RiskReadOnly }

// ToolExecutionMode 计划模式入口整批串行：状态机切换需保序。
func (t *EnterTool) ToolExecutionMode() tool.ExecutionMode { return tool.ExecutionSequential }
func (t *EnterTool) Description() string {
	return "进入计划模式：先只用只读工具调研并产出计划，用户批准后才开始执行。适合改动面大或方向未定的任务。"
}

func (t *EnterTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

func (t *EnterTool) Execute(ctx context.Context, _ json.RawMessage) tool.ToolResult {
	sid := agent.SessionIDFromCtx(ctx)
	if sid == "" {
		return tool.ToolResult{Err: pkg.New(4001, "plan mode: no session context", "")}
	}
	t.store.Enable(sid)
	return tool.ToolResult{
		Content: "已进入计划模式：接下来只有只读工具可用，执行类调用会被拒绝。" +
			"请完成调研后产出计划（建议用 todo(plan) 记录步骤），然后调用 exit_plan_mode 请求用户批准。",
		Meta: map[string]string{"plan_mode": "on"},
	}
}

// ExitTool 退出计划模式：必须经用户批准（审批拒绝则保持计划模式继续调研）。
type ExitTool struct {
	store    *Store
	approver tool.Approver
}

// NewExit 构造 exit_plan_mode；approver 由装配方注入（审批门）。
func NewExit(store *Store, approver tool.Approver) *ExitTool {
	return &ExitTool{store: store, approver: approver}
}

// WithApprover 注入审批门。
func (t *ExitTool) WithApprover(a tool.Approver) *ExitTool { t.approver = a; return t }

func (t *ExitTool) Name() string              { return exitToolName }
func (t *ExitTool) RiskLevel() tool.RiskLevel { return tool.RiskWriteLocal }

// ToolExecutionMode 计划模式出口需审批，整批串行避免审批门并发触发。
func (t *ExitTool) ToolExecutionMode() tool.ExecutionMode { return tool.ExecutionSequential }
func (t *ExitTool) Description() string {
	return "退出计划模式：把计划提交给用户审批，批准后恢复全部工具并开始执行。"
}

func (t *ExitTool) Schema() tool.ToolSchema {
	return tool.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

// Meta 计划模式状态供前端识别。
func (t *ExitTool) Meta() tool.ToolMeta { return tool.ToolMeta{Group: tool.GroupAgent} }

func (t *ExitTool) Execute(ctx context.Context, _ json.RawMessage) tool.ToolResult {
	sid := agent.SessionIDFromCtx(ctx)
	if sid == "" {
		return tool.ToolResult{Err: pkg.New(4001, "plan mode: no session context", "")}
	}
	if !t.store.Active(sid) {
		return tool.ToolResult{Content: "当前不在计划模式中，无需退出。"}
	}
	// 退出即「计划交付」：必须用户批准。
	// 拒绝是用户行为而非模型能自行补齐的信息缺口：审批门只回传 bool，模型拿不到任何反馈，
	// 继续跑只能在只读模式里猜测或反复重申同一个计划。故拒绝即终局，把控制权交回用户。
	if t.approver != nil {
		if !t.approver.Approve(ctx, "exit_plan_mode: 批准计划并开始执行？", tool.RiskApprovalNeeds) {
			return tool.ToolResult{
				Content: "用户未批准该计划，本次运行到此结束。计划模式仍然开启（工具仍为只读）；" +
					"用户的下一条消息会说明要改什么，收到后再修订计划并重新提交。",
				Meta: map[string]string{
					"plan_mode":        "on",
					tool.MetaTerminate: "1",
				},
			}
		}
	}
	t.store.Disable(sid)
	return tool.ToolResult{
		Content: "计划已获批准，计划模式已退出：全部工具恢复可用，按计划开始执行。",
		Meta:    map[string]string{"plan_mode": "off"},
	}
}

// Trim 供外部展示用的短名（plan_mode_ 前缀工具名在白名单里逐名列出）。
func Trim(name string) string { return strings.TrimPrefix(name, "plan_mode_") }
