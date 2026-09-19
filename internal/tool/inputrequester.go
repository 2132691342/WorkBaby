package tool

import "context"

// InputRequester 模型主动向用户要信息的能力；实现位于 service.ApprovalService，经 ctx 注入。
type InputRequester interface {
	// RequestInput 阻塞等待用户回复；(回答, true)。超时/取消 ("", false)。
	RequestInput(ctx context.Context, question string) (string, bool)
}

// inputRequesterCtxKey 私有键类型，避免与调用方 ctx 值冲突。
type inputRequesterCtxKey struct{}

// WithInputRequester 把补充输入能力注入 ctx（service 在 run 开始时注入）。
func WithInputRequester(ctx context.Context, r InputRequester) context.Context {
	return context.WithValue(ctx, inputRequesterCtxKey{}, r)
}

// InputRequesterFromCtx 取补充输入能力（未注入返回 nil）。
func InputRequesterFromCtx(ctx context.Context) InputRequester {
	r, _ := ctx.Value(inputRequesterCtxKey{}).(InputRequester)
	return r
}
