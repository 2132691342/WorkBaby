package service

import (
	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// 计划模式：只读不改。
// 它是引擎级的能力——闸门挂在工具注册表上，
// 一次实现就管住全部写工具；逐工具判断的话每个写工具都得自己记得检查。

// PlanMode 当前会话是否处于计划模式。
// 真相在会话文件里（custom 条目），界面状态与引擎闸门都从它派生。
func (c *Chat) PlanMode() bool {
	file := c.CurrentFile()
	if file == "" {
		return false
	}
	return c.Sessions.PlanMode(file)
}

// SetPlanMode 开关计划模式。
// 状态要落进会话文件：它是会话的事实，不是界面的一次性内存状态——
// 重开应用、切回旧会话都得还原成同一个样子。
func (c *Chat) SetPlanMode(enabled bool) error {
	session, err := c.BindSession("")
	if err != nil {
		return err
	}
	if _, err := session.AppendCustom("plan-mode", map[string]any{"active": enabled}); err != nil {
		return err
	}
	// 闸门与界面横幅都读这一处：状态分成两份就会有一份先过期。
	c.Tools.Policy.SetReadOnly(session.Id(), enabled)
	c.Hub.Broadcast("chat:plan", map[string]any{
		"plan_enabled": enabled,
		"session_id":   session.Id(),
	})
	if enabled {
		pkg.Info("进入计划模式（会话 %s）", session.Id())
	} else {
		pkg.Info("退出计划模式（会话 %s）", session.Id())
	}
	return nil
}

// applyPlanState 把会话文件里的计划模式状态同步到引擎闸门。
// 每次开新会话或切会话都要调：闸门是进程内的一份状态，
// 而真相在文件里——不同步就会出现「界面说在计划模式，工具却在写」。
func (c *Chat) applyPlanState(sessionID string) {
	enabled := c.Sessions.PlanMode(c.CurrentFile())
	c.Tools.Policy.SetReadOnly(sessionID, enabled)
}

// planDeniedToolNames 列出计划模式下会被拦下的工具。
// 判定规则在引擎包里：上层只拿名单，不该知道「什么算写工具」。
func (c *Chat) planDeniedToolNames() []string {
	return agent.BlockedToolNames(c.Tools)
}

// currentPlanState 组装计划模式状态。
func (c *Chat) currentPlanState() domain.PlanStateVO {
	return domain.PlanStateVO{
		PlanEnabled:  c.PlanMode(),
		BlockedTools: c.planDeniedToolNames(),
	}
}

// TogglePlan 开关计划模式并返回生效后的状态。
// 界面拿它来更新横幅：返回里带上被拦的工具名，界面能直接显示「哪些不能用了」。
func (c *Chat) TogglePlan(enabled bool) (domain.ChatPlanRESP, error) {
	if err := c.SetPlanMode(enabled); err != nil {
		return domain.ChatPlanRESP{}, err
	}
	state := c.currentPlanState()
	message := "已退出计划模式，可以正常修改文件。"
	if state.PlanEnabled {
		message = "已进入计划模式：只能读，不能改。写工具会被拦下。"
	}
	return domain.ChatPlanRESP{
		PlanEnabled:  state.PlanEnabled,
		BlockedTools: state.BlockedTools,
		PlanMessage:  message,
	}, nil
}
