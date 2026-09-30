// Anthropic Messages 协议的编码硬约束：任何一条违反，多轮对话第二轮起
// 就会被网关以 invalid_request_error 拒绝——表现为「助手答完一轮就卡死」。
package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	"WorkBaby/internal/llm"
)

func bodyOf(t *testing.T, req llm.Request) requestBody {
	t.Helper()
	c := New(llm.ClientConfig{APIKey: "k"})
	raw, err := json.Marshal(c.encode(req))
	if err != nil {
		t.Fatal(err)
	}
	var body requestBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestEncodeDropsUnsignedThinking(t *testing.T) {
	body := bodyOf(t, llm.Request{Model: "m", Messages: []llm.Message{
		{Role: llm.RoleUser, Content: "你好"},
		{Role: llm.RoleAssistant, Thinking: "思考过程", Content: "回答"},
		{Role: llm.RoleUser, Content: "继续"},
	}})
	for _, m := range body.Messages {
		for _, b := range m.Content {
			if b.Type == "thinking" {
				t.Fatal("无签名的 thinking 块被回传：网关会以 invalid_request_error 拒绝")
			}
		}
	}
}

func TestEncodeMergesSameRoleAndKeepsAlternation(t *testing.T) {
	body := bodyOf(t, llm.Request{Model: "m", Messages: []llm.Message{
		{Role: llm.RoleUser, Content: "看下桌面"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "t1", Name: "ls", Args: map[string]any{"path": "c:/"}}}},
		{Role: llm.RoleTool, ToolCallID: "t1", Content: "a.txt"},
		// 插话注入：紧跟在 tool 结果（role=user）后面，不能出现连续两条 user
		{Role: llm.RoleUser, Content: "顺便看看大小"},
	}})
	prev := ""
	for i, m := range body.Messages {
		if i > 0 && m.Role == prev {
			t.Fatalf("第 %d 条出现连续同角色消息 %q：严格交替的兼容层会拒绝", i, m.Role)
		}
		prev = m.Role
	}
	if body.Messages[0].Role != "user" {
		t.Fatal("首条消息必须是 user")
	}
}

func TestEncodeRejectsEmptyTextAndToolResult(t *testing.T) {
	body := bodyOf(t, llm.Request{Model: "m", Messages: []llm.Message{
		{Role: llm.RoleUser, Content: ""},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "t1", Name: "ls"}}},
		{Role: llm.RoleTool, ToolCallID: "t1", Content: ""},
	}})
	for _, m := range body.Messages {
		for _, b := range m.Content {
			if b.Type == "text" && strings.TrimSpace(b.Text) == "" {
				t.Fatal("空 text 块被发出")
			}
			if b.Type == "tool_result" && strings.TrimSpace(b.Content) == "" {
				t.Fatal("空 tool_result 被发出")
			}
		}
	}
}
