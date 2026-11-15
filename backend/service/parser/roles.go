package parser

// 消息角色名。引擎按 OpenAI 的约定落 tool 角色（"tool"），
// 但会话文件里也认 "toolResult"——那是被广泛使用的另一种写法，
// 只认一种会让另一种写法的会话文件读不出工具输出。

const (
	// roleUser 用户消息
	roleUser = "user"
	// roleAssistant 助手消息
	roleAssistant = "assistant"
	// roleToolResult 工具结果消息
	roleToolResult = "tool"
	// roleLegacyToolResult 另一种工具结果角色名的写法
	roleLegacyToolResult = "toolResult"
)

// isToolResult 判断一个角色是不是工具结果。
func isToolResult(role string) bool {
	return role == roleToolResult || role == roleLegacyToolResult
}
