package tool

import (
	"context"
	"encoding/json"
)

// 审批风险语义（与前端 ChatStreamDecoder.ApprovalRequest.risk 对齐）：
const (
	// RiskApprovalNeeds 白名单外但可恢复：同命令批准过一次后免审（进程内记忆）。
	RiskApprovalNeeds = "needs_approval"
	// RiskApprovalIrrev 命中危险正则（不可逆操作）：每次都问，永不免审。
	RiskApprovalIrrev = "irreversible"
	// RiskApprovalInput 补充输入：模型向用户提问（非安全审批），跳过即让用户自行假设。
	RiskApprovalInput = "input_required"
)

// Approver 高风险命令审批门：阻塞等待用户决策后放行/拒绝。
// 接口定义在 tool 包，实现位于 service 层（tool 不感知事件总线与前端）。
type Approver interface {
	// Approve 阻塞等待用户决策；返回 true = 放行执行，false = 拒绝 / 超时 / 取消。
	// ctx 携带 harness 注入的 runID/sessionID（RunIDFromCtx），用于事件路由。
	Approve(ctx context.Context, command string, risk string) bool
}

// RiskClassifier 按本次调用参数计算风险与可读描述（工具可选实现）。
// 供 runner 策略门做统一裁决；返回空 description 表示退回工具级静态风险。
type RiskClassifier interface {
	ClassifyArgs(args json.RawMessage) (description string, risk string)
}

// guardChainKey 标记「runner 护栏链已对本调用完成策略裁决与审批」。
type guardChainKey struct{}

// WithGuardChain 标记 ctx：本次工具调用已经过 runner 护栏链（策略门 + 审批）。
// 工具内部据此跳过自有审批兜底，消除双层闸门的重复询问。
func WithGuardChain(ctx context.Context) context.Context {
	return context.WithValue(ctx, guardChainKey{}, true)
}

// GuardChainActive 报告本次调用是否来自 runner 护栏链。
func GuardChainActive(ctx context.Context) bool {
	v, _ := ctx.Value(guardChainKey{}).(bool)
	return v
}
