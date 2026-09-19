// Package core 是 WorkBaby 的 Agent 执行内核：一个 ReAct 主循环 + 一条可插拔护栏中间件链。
//
// 边界：core 不依赖 api / service / server / wails；持久化与审批经接口注入；
// 护栏以 Middleware 组合（对齐 go-micro ToolWrapper），主循环只保留「请求 → 执行 → 回填」三件事。
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// Call 一次工具调用的完整上下文；中间件按它裁决，执行器按它执行。
type Call struct {
	ID      string          // tool_call id，回填时配对用
	Name    string          // 工具名
	Args    json.RawMessage // 原始 JSON 参数
	Tool    tool.Tool       // 解析出的工具实例（未命中为 nil）
	RunID   string
	Turn    int
	Trusted bool // 本次调用已获信任放行（计划模式等外部标记）
}

// Handler 工具执行器；Middleware 层层包裹它，最内层落到 tool.Tool.Execute。
type Handler func(ctx context.Context, call Call) tool.ToolResult

// Middleware 护栏中间件：包裹下一个 Handler，可短路、改写结果或放行。
//
// 契约：拒绝必须返回 tool.ToolResult{Refused: true}，让模型能改道续跑；
// 不得返回 Err——错误留给真正的执行失败。
type Middleware func(next Handler) Handler

// Chain 按序组装中间件：索引 0 为最外层（最先裁决，最先短路）。
func Chain(ms ...Middleware) Middleware {
	return func(next Handler) Handler {
		for i := len(ms) - 1; i >= 0; i-- {
			next = ms[i](next)
		}
		return next
	}
}

// ExecOptions 执行器参数。
type ExecOptions struct {
	Timeout        time.Duration // 单次调用超时；0 取默认 5 分钟
	MaxResultChars int           // 结果回填前截断长度；0 取默认 32k rune
}

func (o ExecOptions) withDefaults() ExecOptions {
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Minute
	}
	if o.MaxResultChars <= 0 {
		o.MaxResultChars = 32_000
	}
	return o
}

// Executor 最内层执行器：施加超时、隔离 panic、截断结果、标记护栏链。
func Executor(opt ExecOptions) Handler {
	opt = opt.withDefaults()
	return func(ctx context.Context, call Call) (res tool.ToolResult) {
		if call.Tool == nil {
			return tool.ToolResult{Content: "工具未注册: " + call.Name, Err: pkg.New(4001, "工具未注册", call.Name)}
		}
		ctx = tool.WithGuardChain(ctx)
		ctx, cancel := context.WithTimeout(ctx, opt.Timeout)
		defer cancel()

		start := time.Now()
		defer func() {
			if r := recover(); r != nil {
				res = tool.ToolResult{
					Content: "工具执行异常: " + call.Name,
					Err:     pkg.New(4002, "工具执行异常", fmt.Sprint(r)),
				}
			}
			if res.Meta == nil {
				res.Meta = map[string]string{}
			}
			res.Meta["duration_ms"] = strconv.FormatInt(time.Since(start).Milliseconds(), 10)
			res.Content = pkg.TruncateRunes(res.Content, opt.MaxResultChars)
		}()
		return call.Tool.Execute(ctx, call.Args)
	}
}
