// 子代理聚合根：一次委派的入参（specs/16）。
package domain

import "WorkBaby/backend/pkg"

var (
	// ErrSubagentOff 是「子代理能力没装配」。它只在装配错误时出现，正常路径不会命中。
	ErrSubagentOff = pkg.New("5103", "子代理没有装配，这一步做不了", "重启程序后再试")
)

// SubagentREQ 一次子代理委派。Task 是它看到的全部背景——
// 子代理看不到当前对话，缺的上下文必须写进 Task。
type SubagentREQ struct {
	Task  string   `json:"task"`
	Label string   `json:"label"`
	Tools []string `json:"tools"` // 空 = 沿用当前启用集
	Model string   `json:"model"` // 空 = 跟随当前会话
}

// SubagentVO 一次子代理的执行结果，供运行观测页展开。
type SubagentVO struct {
	SessionID string `json:"session_id"`
	Task      string `json:"task"`
	Output    string `json:"output"`
	Tokens    int    `json:"tokens"`
	Failed    bool   `json:"failed"`
	Reason    string `json:"reason"`
}
