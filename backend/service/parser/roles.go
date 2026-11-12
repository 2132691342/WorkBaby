package parser

// 消息角色名。引擎按 OpenAI 的约定落 tool 角色（"tool"），
// 早期版本按 pi 的约定落 "toolResult"——两种都要认，
// 否则用户升级后旧会话的工具输出会整个消失。

const (
	// roleUser 用户消息
	roleUser = "user"
	// roleAssistant 助手消息
	roleAssistant = "assistant"
	// roleToolResult 工具结果消息
	roleToolResult = "tool"
	// roleLegacyToolResult 早期版本的工具结果角色名
	roleLegacyToolResult = "toolResult"
)

// isToolResult 判断一个角色是不是工具结果。
func isToolResult(role string) bool {
	return role == roleToolResult || role == roleLegacyToolResult
}
