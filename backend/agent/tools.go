package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sort"
	"strconv"
	"sync"
	"time"

	"WorkBaby/backend/llm"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/tool"
)

// llmToolDefs 把工具转成上游声明；按名字稳定排序，便于提示词可复现。
func llmToolDefs(tools []tool.Tool) []llm.ToolDef {
	byName := map[string]tool.Tool{}
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name())
		byName[t.Name()] = t
	}
	sort.Strings(names)
	defs := make([]llm.ToolDef, 0, len(names))
	for _, n := range names {
		t := byName[n]
		params := t.Parameters()
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		defs = append(defs, llm.ToolDef{Name: t.Name(), Description: t.Description(), Parameters: params})
	}
	return defs
}

// plan 是 prepare 阶段的产物：idx 记住它在调用序列中的位置，保证结果按序回填。
type plan struct {
	idx  int
	call llm.ToolCall
	tool tool.Tool
}

// executeTools 执行一批工具调用：prepare 串行保序，execute 并发，结果严格按调用顺序回填。
func (l *Loop) executeTools(ctx context.Context, calls []llm.ToolCall) {
	plans, results := l.prepare(ctx, calls)

	sequential := len(plans) <= 1
	for _, p := range plans {
		if p.tool.ExecutionMode() == tool.ExecutionSequential {
			sequential = true
			break
		}
	}

	if sequential {
		for _, p := range plans {
			results[p.idx] = l.runOneSafe(ctx, p)
		}
	} else {
		l.runParallel(ctx, plans, results)
	}

	// 结果按调用顺序回填：顺序错位会让上游在下一轮直接 400。
	for i, r := range results {
		if r == nil {
			continue
		}
		l.appendMessage(llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: calls[i].ID,
			Content:    r.Content,
			IsError:    r.IsError,
		})
	}
}

// prepare 串行跑参数校验与闸门，保证工具开始事件的顺序与调用顺序一致。
func (l *Loop) prepare(ctx context.Context, calls []llm.ToolCall) ([]plan, []*tool.Result) {
	plans := make([]plan, 0, len(calls))
	results := make([]*tool.Result, len(calls))

	for i, call := range calls {
		t, ok := l.lookup(call.Name)
		if !ok {
			results[i] = &tool.Result{
				Content: "没有这个工具：" + call.Name,
				Title:   "工具不存在",
				IsError: true,
			}
			l.emitSkipped(call, "工具不存在", "没有这个工具："+call.Name)
			continue
		}
		if err := tool.ValidateArgs(t, call.Args); err != nil {
			results[i] = &tool.Result{
				Content: "参数不正确：" + err.Error(),
				Title:   "参数不正确",
				IsError: true,
			}
			l.emitSkipped(call, "参数不正确", "参数不正确："+err.Error())
			continue
		}
		if l.isRepeated(call) {
			msg := "这个调用已经试了 " + itoa(repeatCallLimit) + " 次没有进展，换个做法吧。"
			results[i] = &tool.Result{
				Content: msg,
				Title:   "重复调用",
				IsError: true,
			}
			l.emitSkipped(call, "重复调用", msg)
			continue
		}
		if l.cfg.Gate != nil {
			blocked, reason := l.cfg.Gate(ctx, &call)
			if blocked {
				results[i] = &tool.Result{
					Content: "这个操作没有执行：" + reason,
					Title:   "已跳过",
					IsError: true,
				}
				l.emitSkipped(call, "已跳过", reason)
				continue
			}
		}
		l.markCalled(call)
		plans = append(plans, plan{idx: i, call: call, tool: t})
	}
	return plans, results
}

// emitSkipped 给预执行失败的调用补一对开始 / 结束事件。
// 落库侧只在 tool_start 里写声明，缺了开始事件，重载历史后协议配对会断。
func (l *Loop) emitSkipped(call llm.ToolCall, title, output string) {
	l.emit(Event{Kind: EventToolStart, ToolCall: &call, ToolTitle: title})
	l.emit(Event{Kind: EventToolEnd, ToolCall: &call, ToolOK: false,
		ToolBlocked: true, ToolTitle: title, ToolOutput: output})
}

// callKey 用工具名加参数做去重键；参数按 JSON 编码保证键稳定。
func callKey(call llm.ToolCall) string {
	raw, err := json.Marshal(call.Args)
	if err != nil {
		return call.Name + "|" + fmt.Sprint(call.Args)
	}
	return call.Name + "|" + string(raw)
}

// isRepeated 判断同一调用是否已经用满执行额度；计数在整个 run 内累计不衰减——
// 同一工具同一参数反复出现就是模型在打转，中间穿插别的调用不代表它换了方案。
// 去重表没有锁：只有 prepare 串行读写它。搬进并行路径时必须同时给它加锁。
func (l *Loop) isRepeated(call llm.ToolCall) bool {
	return l.repeated[callKey(call)] >= repeatCallLimit
}

func (l *Loop) markCalled(call llm.ToolCall) {
	l.repeated[callKey(call)]++
}

func itoa(n int) string { return strconv.Itoa(n) }

// runParallel 并发执行：信号量限制并发度，结果按原始下标回填。
// 信号量必须在 go 之前获取，否则并发度的真上界是调用数而不是 cfg.Parallel。
func (l *Loop) runParallel(ctx context.Context, plans []plan, results []*tool.Result) {
	limit := l.cfg.Parallel
	if limit <= 0 {
		limit = 4
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, p := range plans {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			// 这个下标必须有结果：assistant 声明了几个调用就得回填几个。
			results[p.idx] = l.runOneSafe(ctx, p)
			continue
		}
		wg.Add(1)
		go func(p plan) {
			defer wg.Done()
			defer func() { <-sem }()
			results[p.idx] = l.runOneSafe(ctx, p)
		}(p)
	}
	wg.Wait()
}

// runOneSafe 在 runOne 之外兜 panic：工具 panic 不该把整个进程带走。
// 串行与并行两条路径共用这一层——recover 必须直接包住 runOne 的调用栈才拿得到值。
func (l *Loop) runOneSafe(ctx context.Context, p plan) (res *tool.Result) {
	defer func() {
		if rec := recover(); rec != nil {
			pkg.Errorf("tool: %s 内部崩溃: %v\n%s", p.tool.Name(), rec, debug.Stack())
			res = panicResult(p.tool.Label(), rec)
		}
	}()
	return l.runOne(ctx, p)
}

// panicResult 把工具 panic 转成一条给模型看的失败结果。
// panic 说明有 bug，但代价不该是整进程退出。
func panicResult(label string, rec any) *tool.Result {
	return &tool.Result{
		Content: fmt.Sprintf("执行失败：%s 内部错误（%v）", label, rec),
		Title:   label + "失败了",
		IsError: true,
	}
}

// runOne 执行单个工具并发出开始 / 结束事件。
// 写类工具的串行保证来自 executeTools 的分批，这里不需要额外的锁。
func (l *Loop) runOne(ctx context.Context, p plan) *tool.Result {
	call := p.call
	l.emit(Event{Kind: EventToolStart, ToolCall: &call, ToolTitle: p.tool.Label()})

	start := time.Now()
	res, err := p.tool.Execute(ctx, tool.Input{
		Args:      call.Args,
		Workspace: l.cfg.Workspace,
		Deps:      l.cfg.Deps,
	})
	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		res = &tool.Result{
			Content: "执行失败：" + err.Error(),
			Title:   p.tool.Label() + "失败了",
			IsError: true,
		}
	}
	if res == nil {
		res = &tool.Result{Content: "", Title: p.tool.Label()}
	}
	// Detail 是给界面与日志的那份。工具只给了 Content（失败结果、空结果）时补上：
	// 不补的话工具卡展开是空白，日志里也只剩一句 ok=false，谁都查不出为什么。
	if res.Detail == "" {
		res.Detail = res.Content
	}
	l.emit(Event{Kind: EventToolEnd, ToolCall: &call, ToolOK: !res.IsError,
		ToolTitle: res.Title, ToolOutput: res.Detail, DurationMs: elapsed})
	return res
}

// lookup 按名字取已启用的工具。
func (l *Loop) lookup(name string) (tool.Tool, bool) {
	for _, t := range l.cfg.Tools {
		if t.Name() == name {
			return t, true
		}
	}
	return nil, false
}
