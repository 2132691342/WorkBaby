package tool

import "context"

// RootResolver 解析会话的工具根目录（工作区）。
//
// 工作区是会话级概念：用户可以为不同会话绑定不同本地目录，
// 因此工具不能在注册期持有固定根，必须在执行期按 run 所属会话解析。
type RootResolver func(sessionID string) string

// runIdentity 注入 ctx 的 run 身份。
type runIdentity struct {
	runID     string
	sessionID string
}

type runCtxKey struct{}

// ResolveRoot 把「按会话解析」包装成「按 ctx 解析」，并带默认根回落。
//
// 解析失败（会话未绑定目录 / ctx 无身份）一律回落 defRoot：工具宁可在默认
// 工作区里返回「文件不存在」，也不能因拿不到根而让整个 run 中断。
func ResolveRoot(r RootResolver, defRoot string) func(context.Context) string {
	return func(ctx context.Context) string {
		if r != nil {
			if root := r(SessionIDFromCtx(ctx)); root != "" {
				return root
			}
		}
		return defRoot
	}
}

// ResolveForSession 直接按 sessionID 解析根（非工具链路用，如工作区文件面板）。
//
// 与 ResolveRoot 同一套回落语义，供 service 层复用，避免两处各写一遍兜底。
func ResolveForSession(r RootResolver, defRoot, sessionID string) string {
	if r != nil {
		if root := r(sessionID); root != "" {
			return root
		}
	}
	return defRoot
}

// WithRunIdentity 把 runID / sessionID 注入 ctx（harness 在 run 开始时注入）。
//
// 工具借此解析本会话的工作区根、在审批事件里带上身份，无需把身份参数
// 一路穿透 Tool 接口签名。
func WithRunIdentity(ctx context.Context, runID, sessionID string) context.Context {
	return context.WithValue(ctx, runCtxKey{}, runIdentity{runID: runID, sessionID: sessionID})
}

// RunIDFromCtx 取 runID（缺省空串）。
func RunIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(runCtxKey{}).(runIdentity)
	return id.runID
}

// SessionIDFromCtx 取 sessionID（缺省空串）。
func SessionIDFromCtx(ctx context.Context) string {
	id, _ := ctx.Value(runCtxKey{}).(runIdentity)
	return id.sessionID
}
