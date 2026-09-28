package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"WorkBaby/internal/llm"
	"WorkBaby/internal/tool"
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
			results[p.idx] = l.runOne(ctx, p)
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
			continue
		}
		if err := tool.ValidateArgs(t, call.Args); err != nil {
			results[i] = &tool.Result{
				Content: "参数不正确：" + err.Error(),
				Title:   "参数不正确",
				IsError: true,
			}
			continue
		}
		if l.isRepeated(call) {
			results[i] = &tool.Result{
				Content: "同样的调用已经连续试了 " + itoa(repeatCallLimit) + " 次，换个做法吧。",
				Title:   "重复调用",
				IsError: true,
			}
			l.emit(Event{Kind: EventToolEnd, ToolCall: &call, ToolOK: false,
				ToolBlocked: true, ToolTitle: "重复调用"})
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
				l.emit(Event{Kind: EventToolEnd, ToolCall: &call, ToolOK: false,
					ToolBlocked: true, ToolTitle: "已跳过", ToolOutput: reason})
				continue
			}
		}
		l.markCalled(call)
		plans = append(plans, plan{idx: i, call: call, tool: t})
	}
	return plans, results
}

// callKey 用工具名加参数做去重键；参数按 JSON 编码保证键稳定。
func callKey(call llm.ToolCall) string {
	raw, err := json.Marshal(call.Args)
	if err != nil {
		return call.Name + "|" + fmt.Sprint(call.Args)
	}
	return call.Name + "|" + string(raw)
}

func (l *Loop) isRepeated(call llm.ToolCall) bool {
	return l.repeated[callKey(call)] >= repeatCallLimit-1
}

func (l *Loop) markCalled(call llm.ToolCall) {
	l.repeated[callKey(call)]++
}

func itoa(n int) string { return strconv.Itoa(n) }

// runParallel 并发执行：信号量限制并发度，结果按原始下标回填。
func (l *Loop) runParallel(ctx context.Context, plans []plan, results []*tool.Result) {
	limit := l.cfg.Parallel
	if limit <= 0 {
		limit = 4
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, p := range plans {
		wg.Add(1)
		go func(p plan) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[p.idx] = l.runOne(ctx, p)
		}(p)
	}
	wg.Wait()
}

// runOne 执行单个工具并发出开始 / 结束事件；写类工具按目标路径串行，避免同文件并发互踩。
func (l *Loop) runOne(ctx context.Context, p plan) *tool.Result {
	call := p.call
	l.emit(Event{Kind: EventToolStart, ToolCall: &call, ToolTitle: p.tool.Label()})

	if p.tool.ExecutionMode() == tool.ExecutionSequential {
		l.writeMu.Lock()
		defer l.writeMu.Unlock()
	}

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
