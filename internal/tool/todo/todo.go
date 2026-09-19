// Package todo 提供「计划」工具：Agent 显式维护会话内待办清单。
// 先列计划、逐步勾选，长任务不跑偏；工具结果回填模型，前端进度卡消费同一状态快照。
// 状态存取经 Store 接口注入（service 侧为持久化实现，跨重启可回放）。
package todo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// Store 会话计划存取（由装配方注入）。
type Store interface {
	Load(ctx context.Context, sessionID string) ([]domain.TodoItem, error)
	Save(ctx context.Context, sessionID string, items []domain.TodoItem) error
}

// Tool LLM 可调用的计划工具（readonly 风险级：不触碰本地文件/网络）。
type Tool struct {
	store Store
	sess  func(ctx context.Context) string // 从 ctx 取 sessionID（harness 注入的身份不在 tool 可读范围，由装配方桥接）
}

// New 构造计划工具；store/sess 为 nil 时工具返回配置缺失错误（模型可见反馈）。
func New(store Store, sess func(ctx context.Context) string) *Tool {
	return &Tool{store: store, sess: sess}
}

var paramsSchema = json.RawMessage(`{
	"type": "object",
	"required": ["action"],
	"properties": {
		"action": {"type": "string", "enum": ["plan", "list", "mark_done", "mark_undone"]},
		"items":  {"type": "array", "items": {"type": "object", "required": ["title"], "properties": {"title": {"type": "string"}}}},
		"id":     {"type": "string"}
	}
}`)

func (t *Tool) Name() string { return "todo" }
func (t *Tool) Description() string {
	return "管理本会话待办计划。动作：plan(items=[{title}] 列新计划) / list / mark_done(id) / mark_undone(id)。长任务先 todo(plan) 再逐步 mark_done。"
}
func (t *Tool) Schema() tool.ToolSchema {
	return tool.ToolSchema{Name: "todo", Description: t.Description(), Parameters: paramsSchema}
}
func (t *Tool) RiskLevel() tool.RiskLevel { return tool.RiskReadOnly }

// Execute 执行动作并返回最新计划（文本 + 结构化 Data）。
func (t *Tool) Execute(ctx context.Context, args json.RawMessage) tool.ToolResult {
	fail := func(msg string, err error) tool.ToolResult {
		return tool.ToolResult{Content: "todo: " + msg, Err: err}
	}
	if t.store == nil || t.sess == nil {
		return fail("tool not configured", fmt.Errorf("todo: tool not configured"))
	}
	sessionID := t.sess(ctx)
	if sessionID == "" {
		return fail("no active session", fmt.Errorf("todo: no active session"))
	}

	var req struct {
		Action string `json:"action"`
		Items  []struct {
			Title string `json:"title"`
		} `json:"items"`
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return fail("invalid args: "+err.Error(), err)
	}

	items, err := t.store.Load(ctx, sessionID)
	if err != nil {
		return fail("load failed", err)
	}

	switch req.Action {
	case "plan":
		items = items[:0]
		for _, it := range req.Items {
			title := strings.TrimSpace(it.Title)
			if title == "" {
				continue
			}
			items = append(items, domain.TodoItem{ID: pkg.NewID(domain.IDTodo), Title: title})
		}
	case "mark_done", "mark_undone":
		if req.ID == "" {
			return fail("id required for "+req.Action, fmt.Errorf("todo: id required"))
		}
		found := false
		for i := range items {
			if items[i].ID == req.ID {
				items[i].Done = req.Action == "mark_done"
				found = true
				break
			}
		}
		if !found {
			return fail("item not found: "+req.ID, fmt.Errorf("todo: item not found"))
		}
	case "list":
		// 只读
	case "":
		return fail("action required", fmt.Errorf("todo: action required"))
	default:
		return fail("unknown action "+req.Action, fmt.Errorf("todo: unknown action"))
	}

	if err := t.store.Save(ctx, sessionID, items); err != nil {
		return fail("save failed", err)
	}
	return render(items)
}

// render 组装文本与结构化快照。
func render(items []domain.TodoItem) tool.ToolResult {
	var sb strings.Builder
	done := 0
	sb.WriteString("当前计划：\n")
	for i, it := range items {
		mark := "[ ]"
		if it.Done {
			mark = "[x]"
			done++
		}
		fmt.Fprintf(&sb, "%d. %s %s (id=%s)\n", i+1, mark, it.Title, it.ID)
	}
	fmt.Fprintf(&sb, "进度：%d/%d 完成", done, len(items))
	state := domain.TodoStateRESP{Items: items, DoneCount: done, Total: len(items)}
	return tool.ToolResult{Content: sb.String(), Data: map[string]any{"session_todo": state}}
}
