package core

import "context"

// run 身份在 ctx 中的载体。工具与审计经 RunIDFromCtx / SessionIDFromCtx 取用，
// 避免每个工具签名都带上 run 元信息。
type ctxKey struct{ name string }

var (
	runIDKey     = ctxKey{"run_id"}
	sessionIDKey = ctxKey{"session_id"}
)

// WithRunContext 注入 run 身份。
func WithRunContext(ctx context.Context, runID, sessionID string) context.Context {
	ctx = context.WithValue(ctx, runIDKey, runID)
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

// RunIDFromCtx 取 run id；无则空串。
func RunIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(runIDKey).(string)
	return v
}

// SessionIDFromCtx 取 session id；无则空串。
func SessionIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(sessionIDKey).(string)
	return v
}

// resumedKey 续跑标记的私有载体。
type resumedKey struct{}

// WithResumedRun 标记本次调用来自续跑：同一工具调用会被重放，
// 审批门据此改查已决记录而不是重新问用户。
func WithResumedRun(ctx context.Context) context.Context {
	return context.WithValue(ctx, resumedKey{}, true)
}

// IsResumedRun 报告本次调用是否来自续跑。
func IsResumedRun(ctx context.Context) bool {
	v, _ := ctx.Value(resumedKey{}).(bool)
	return v
}
