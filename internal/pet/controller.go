package pet

import "time"

// State 桌宠状态。
type State string

const (
	StateIdle     State = "idle"
	StateHappy    State = "happy"
	StateWorking  State = "working"
	StateSleeping State = "sleeping"
)

// Controller 桌宠状态机：事件驱动转换，happy 3s 自动回 idle。
// 不依赖 Wails；状态变更经 onChange 回调由 api 层转发前端。
type Controller struct {
	current  State
	onChange func(State)
}

// NewController 构造；onChange 为状态变更回调（可为 nil）。
func NewController(onChange func(State)) *Controller {
	return &Controller{current: StateIdle, onChange: onChange}
}

// Current 当前状态。
func (c *Controller) Current() State { return c.current }

// OnEvent 处理状态事件（agent.run.start/done/error、user.idle、user.active）。
func (c *Controller) OnEvent(kind string) {
	switch kind {
	case "agent.run.start":
		c.set(StateWorking)
	case "agent.run.done":
		c.set(StateHappy)
		time.AfterFunc(3*time.Second, func() {
			if c.current == StateHappy {
				c.set(StateIdle)
			}
		})
	case "agent.error":
		c.set(StateIdle)
	case "user.idle.30min":
		c.set(StateSleeping)
	case "user.active":
		c.set(StateIdle)
	}
}

func (c *Controller) set(s State) {
	if c.current == s {
		return
	}
	c.current = s
	if c.onChange != nil {
		c.onChange(s)
	}
}
