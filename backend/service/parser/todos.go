package parser

// 待办派生：待办清单本身只在引擎内存里活着，但每次待办工具调用的
// 结果 details.tasks 都带全量表并落进会话文件——活动分支上最近一次
// 成功调用就是当前状态。

import "WorkBaby/backend/domain"

// TodosFromEntries 从活动分支派生当前待办；没有待办调用给空表。
func TodosFromEntries(entries []Entry) []domain.TodoVO {
	out := []domain.TodoVO{}
	for _, entry := range ActivePath(entries) {
		msg := entry.EntryMessage
		if msg == nil || !isToolResult(msg.MessageRole) || msg.MessageToolName != "todo" {
			continue
		}
		rows := todoRowsOf(msg.MessageDetails)
		if len(rows) == 0 {
			continue
		}
		// 最近一次成功调用的全量表就是当前状态：直接覆盖。
		out = rows
	}
	return out
}

// TodosFromFile 读一个会话文件并派生待办；读不了给空表。
func TodosFromFile(file string) []domain.TodoVO {
	entries, err := ReadEntries(file)
	if err != nil {
		return []domain.TodoVO{}
	}
	return TodosFromEntries(entries)
}

// todoRowsOf 把 details.tasks 解析成界面行；解析不了给空。
func todoRowsOf(details map[string]any) []domain.TodoVO {
	if details == nil {
		return nil
	}
	raw, ok := details["tasks"].([]any)
	if !ok {
		return nil
	}
	rows := make([]domain.TodoVO, 0, len(raw))
	for _, item := range raw {
		task, ok := item.(map[string]any)
		if !ok {
			continue
		}
		subject, _ := task["subject"].(string)
		status, _ := task["status"].(string)
		if subject == "" || status == "" || status == "deleted" {
			continue
		}
		id := 0
		if n, ok := task["id"].(float64); ok {
			id = int(n)
		}
		activeForm, _ := task["activeForm"].(string)
		rows = append(rows, domain.TodoVO{
			TodoId:         id,
			TodoSubject:    subject,
			TodoStatus:     status,
			TodoActiveForm: activeForm,
		})
	}
	return rows
}
