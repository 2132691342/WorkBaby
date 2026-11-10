package parser

// 计划模式派生：pi-plan-mode 每次开关都会往会话文件追加一条 custom 条目
// （customType = "plan-mode"，data.active 是开关后的状态）。活动分支上
// 最后一条就是当前状态——与插件自己 session_start 时的 restore 判据一致。

// PlanModeFromEntries 从活动分支派生「计划模式是否开启」。
// 没有任何 plan-mode 条目时给 false：插件默认就是关的。
func PlanModeFromEntries(entries []Entry) bool {
	active := false
	for _, entry := range ActivePath(entries) {
		if entry.Type != "custom" || entry.CustomType != "plan-mode" {
			continue
		}
		if flag, ok := entry.Data["active"].(bool); ok {
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
