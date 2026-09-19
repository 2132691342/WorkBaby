package tool

import "context"

// Delegator 子 Agent 委派能力。
//
// 抽象定义在 tool 包（delegate_task 依赖），实现位于 service（coreDelegator）——
// 与 Approver 同一套路：能力域之间只经接口，不互相感知实现。
// 委派只回传摘要（不把子 Agent 的正文流混进父回答），避免上下文污染。
type Delegator interface {
	// Delegate 以指定 Agent 定义跑一个子任务，返回摘要文本。
	// agent 为空或未知时回退默认 Agent；ctx 携带父 run 身份用于事件路由。
	Delegate(ctx context.Context, agent, task string) (string, error)
}

// delegatorCtxKey 私有键类型，避免与调用方 ctx 值冲突。
type delegatorCtxKey struct{}

// WithDelegator 把委派能力注入 ctx（harness 在 run 开始时注入 Runner 自身）。
//
// 深层工具（delegate_task）无需持有 Runner 引用即可发起子 Agent。
func WithDelegator(ctx context.Context, d Delegator) context.Context {
	return context.WithValue(ctx, delegatorCtxKey{}, d)
}

// DelegatorFromCtx 取委派能力（未注入返回 nil）。
func DelegatorFromCtx(ctx context.Context) Delegator {
	d, _ := ctx.Value(delegatorCtxKey{}).(Delegator)
	return d
}
