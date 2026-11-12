package parser

// 计划模式派生：开关每次都会往会话文件追加一条 custom 条目
// （customType = "plan-mode"，data.active 是开关后的状态）。活动分支上
// 最后一条就是当前状态——它是会话文件里的真相，
// 所以重开应用、切回旧会话都能还原。

import "WorkBaby/backend/domain"

// PlanModeFromEntries 从活动分支派生「计划模式是否开启」。
// 没有任何 plan-mode 条目时给 false：计划模式默认就是关的。
func PlanModeFromEntries(entries []Entry) bool {
	active := false
	for _, entry := range ActivePath(entries) {
		if entry.EntryType != domain.SessionEntryCustom || entry.EntryCustomType != "plan-mode" {
			continue
		}
		if entry.EntryData == nil {
			continue
		}
		if flag, ok := entry.EntryData["active"].(bool); ok {
			active = flag
		}
	}
	return active
}

// PlanModeFromFile 读一个会话文件并派生计划模式状态；读不了给 false。
func PlanModeFromFile(file string) bool {
	if file == "" {
		return false
	}
	entries, err := ReadEntries(file)
	if err != nil {
		return false
	}
	return PlanModeFromEntries(entries)
}
