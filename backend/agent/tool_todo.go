package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// TodoStatus 是待办任务的状态。
type TodoStatus string

const (
	// TodoPending 还没开始
	TodoPending TodoStatus = "pending"
	// TodoInProgress 正在做
	TodoInProgress TodoStatus = "in_progress"
	// TodoCompleted 已完成
	TodoCompleted TodoStatus = "completed"
)

// TodoItem 是一条待办任务。
type TodoItem struct {
	// TodoId 任务号；同一份清单内唯一且稳定
	TodoId int `json:"id"`
	// TodoSubject 一句话说明这条要做什么
	TodoSubject string `json:"subject"`
	// TodoStatus 当前状态
	TodoStatus TodoStatus `json:"status"`
	// TodoActiveForm 进行中的现在进行时描述（如「正在跑测试」）
	TodoActiveForm string `json:"activeForm,omitempty"`
}

// TodoStore 是一份待办清单。
// 清单本身只活在内存里，但每次改动都会随工具结果落进会话文件——
// 回放时从会话文件派生，所以重开应用能还原成同一份清单。
type TodoStore struct {
	// mu 保护清单：工具可能并发被调
	mu sync.Mutex
	// items 任务列表，按任务号升序
	items []TodoItem
}

// NewTodoStore 造一份空清单。
func NewTodoStore() *TodoStore { return &TodoStore{} }

// Items 取清单副本。
func (s *TodoStore) Items() []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TodoItem, len(s.items))
	copy(out, s.items)
	return out
}

// Replace 整份替换清单：模型一次给全量，比逐条增删更不容易走偏。
// 任务号按给出的顺序重排：模型不认得自己的旧编号，重排比保留更可预期。
func (s *TodoStore) Replace(items []TodoItem) []TodoItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TodoItem, 0, len(items))
	for i, item := range items {
		item.TodoId = i + 1
		if item.TodoStatus == "" {
			item.TodoStatus = TodoPending
		}
		out = append(out, item)
	}
	s.items = out
	return out
}

// todoTool 管待办清单。
type todoTool struct {
	// store 清单本体；由注册表持有，全局一份
	store *TodoStore
}

// Spec 工具声明。
func (t *todoTool) Spec() ToolSpec {
	return ToolSpec{
		Name: "todo",
		Description: "管理这份任务的待办清单。传入 tasks 数组整份替换清单，" +
			"每项写清 subject（要做什么）、status（pending / in_progress / completed）与 activeForm（进行中的现在进行时）。" +
			"开始做某项时把它标成 in_progress 并写好 activeForm，做完标成 completed。" +
			"不要为已经做好的事建任务，只维护还没做完的。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tasks": map[string]any{
					"type":        "array",
					"description": "完整的待办清单，按顺序给出",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"subject":    map[string]any{"type": "string", "description": "要做什么，一句话"},
							"status":     map[string]any{"type": "string", "description": "pending / in_progress / completed"},
							"activeForm": map[string]any{"type": "string", "description": "进行中的现在进行时描述"},
						},
						"required": []string{"subject"},
					},
				},
			},
			"required": []string{"tasks"},
		},
	}
}

// Run 执行清单更新。
func (t *todoTool) Run(_ context.Context, _ *Registry, args map[string]any) Result {
	raw, ok := args["tasks"].([]any)
	if !ok {
		return errorf("tasks 必须是数组")
	}
	items := make([]TodoItem, 0, len(raw))
	for _, entry := range raw {
		task, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		subject := strings.TrimSpace(stringArg(task, "subject"))
		if subject == "" {
			continue
		}
		status := TodoStatus(strings.TrimSpace(stringArg(task, "status")))
		switch status {
		case TodoPending, TodoInProgress, TodoCompleted:
		default:
			status = TodoPending
		}
		items = append(items, TodoItem{
			TodoSubject:    subject,
			TodoStatus:     status,
			TodoActiveForm: strings.TrimSpace(stringArg(task, "activeForm")),
		})
	}
	saved := t.store.Replace(items)
	done := 0
	active := ""
	for _, item := range saved {
		switch item.TodoStatus {
		case TodoCompleted:
			done++
		case TodoInProgress:
			active = item.TodoSubject
		}
	}
	// 全量表随结果落盘：回放层靠它派生待办面板，
	// 只记「改了什么」的话，面板复原不出完整清单。
	details := map[string]any{"tasks": tasksForDetails(saved)}
	text := fmt.Sprintf("待办清单已更新：共 %d 项，完成 %d 项", len(saved), done)
	if active != "" {
		text += "，正在做：" + active
	}
	return Result{Text: text, Details: details}
}

// tasksForDetails 把清单转成落盘形状。
// 形状固定为 tasks 数组：回放层按这个键读，两边不能各写一套。
func tasksForDetails(items []TodoItem) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"id":         item.TodoId,
			"subject":    item.TodoSubject,
			"status":     string(item.TodoStatus),
			"activeForm": item.TodoActiveForm,
		})
	}
	return out
}

// sortedTodos 按任务号排序；清单在界面上的顺序要稳定。
func sortedTodos(items []TodoItem) []TodoItem {
	out := make([]TodoItem, len(items))
	copy(out, items)
	sort.Slice(out, func(i, j int) bool { return out[i].TodoId < out[j].TodoId })
	return out
}
