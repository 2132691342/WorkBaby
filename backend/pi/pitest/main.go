// 假 PI：按脚本回应 RPC 命令并吐事件，用于让 pi 包的协议链路在离线环境跑通。
// 它不是 mock：走真实 stdin/stdout 与 LF 分帧，测的是编解码与生命周期本身。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// 这些都是测试脚本要的固定值，不是配置项。
const (
	testSessionID   = "0a1b2c3d-test-session"
	testSessionFile = "/tmp/pi-test-session.jsonl"
)

func main() {
	scenario := "basic"
	if len(os.Args) > 1 {
		for i, a := range os.Args {
			if a == "--scenario" && i+1 < len(os.Args) {
				scenario = os.Args[i+1]
			}
		}
	}

	// 首帧就发扩展 UI 请求：真实装扩展的环境就是这样，
	// 未知记录必须被忽略而不是报错。
	emit(map[string]any{
		"type": "extension_ui_request", "id": "ext-1",
		"method": "setStatus", "statusKey": "lsp",
	})
	// 一条带 U+2028 的记录：它必须被当作普通字符而不是记录分隔符。
	fmt.Fprint(os.Stdout, `{"type":"extension_ui_request","id":"ext-2","method":"setStatus","statusKey":"u2028","statusText":"a`+"\u2028"+`b"}`+"\n")

	if scenario == "crash" {
		// 立刻退出且非零：验证「进程死了要报错、不自动重发」。
		os.Exit(3)
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<22)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var cmd struct {
			ID      string `json:"id"`
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			// 坏记录不该中断读循环（与真 PI 的宽容度一致）。
			continue
		}
		handle(scenario, cmd.ID, cmd.Type, cmd.Message)
	}
	// stdin 关闭即有序退出。
}

func handle(scenario, id, typ, message string) {
	switch typ {
	case "get_state":
		respond(id, "get_state", map[string]any{
			"model":         map[string]any{"id": "test-model", "provider": "test"},
			"thinkingLevel": "medium",
			"isStreaming":   false,
			"sessionId":     testSessionID,
			"sessionFile":   testSessionFile,
			"messageCount":  0,
		})
	case "get_session_stats":
		respond(id, "get_session_stats", map[string]any{
			"totalTokens": 1200, "input": 1000, "output": 200, "messages": 4, "context": 1000,
		})
	case "new_session", "switch_session", "set_model", "set_thinking_level",
		"set_session_name", "compact":
		respond(id, typ, map[string]any{})
	case "clear_queue":
		respond(id, "clear_queue", map[string]any{
			"steering": []string{"排队的这句话"}, "followUp": []string{},
		})
	case "abort":
		respond(id, "abort", nil)
		emit(map[string]any{"type": "agent_settled", "aborted": true})
	case "fork":
		respond(id, "fork", map[string]any{
			"cancelled":   false,
			"sessionId":   "forked-session",
			"sessionFile": "/tmp/forked.jsonl",
		})
	case "prompt":
		// prompt 的响应只表示「已接受」，不代表答完。
		respond(id, "prompt", map[string]any{"disposition": "started"})
		goScript(scenario, message)
	case "steer", "follow_up":
		respond(id, typ, map[string]any{"disposition": "queued"})
	default:
		respond(id, typ, map[string]any{})
	}
}

// goScript 吐一轮完整的事件序列：agent_end 之后还有一条自动重试，最后才 agent_settled。
// done 必须挂在 agent_settled 上，挂早了会在还在跑的时候显示「完成」。
func goScript(scenario, message string) {
	emit(map[string]any{"type": "agent_start"})
	emit(map[string]any{"type": "turn_start"})
	emit(map[string]any{"type": "message_start", "message": map[string]any{
		"role": "assistant", "content": []any{}, "stopReason": "pending", "timestamp": 1,
	}})
	emit(map[string]any{
		"type":  "message_update",
		"usage": map[string]any{"input": 100, "output": 2, "totalTokens": 102},
		"assistantMessageEvent": map[string]any{
			"type": "text_delta", "contentIndex": 0, "delta": "你好",
		},
	})
	emit(map[string]any{
		"type":  "message_update",
		"usage": map[string]any{"input": 100, "output": 2, "totalTokens": 102},
		"assistantMessageEvent": map[string]any{
			"type": "thinking_delta", "contentIndex": 1, "delta": "想一下",
		},
	})
	if scenario == "tools" {
		emit(map[string]any{
			"type":       "tool_execution_start",
			"toolCallId": "call-1", "toolName": "read", "args": map[string]any{"path": "a.txt"},
		})
		emit(map[string]any{
			"type":       "tool_execution_end",
			"toolCallId": "call-1", "toolName": "read",
			"result":  map[string]any{"content": []any{map[string]any{"type": "text", "text": "文件内容"}}},
			"isError": false, "durationMs": 12,
		})
	}
	if scenario == "approval" {
		// 审批闸门：扩展发 confirm，WorkBaby 回 extension_ui_response 后才继续。
		emit(map[string]any{
			"type": "extension_ui_request", "id": "confirm-1", "method": "confirm",
			"title": "允许 write?", "message": "要写入文件 a.txt",
		})
	}
	emit(map[string]any{"type": "message_end", "message": map[string]any{
		"role": "assistant", "stopReason": "toolUse", "timestamp": 2,
		"usage": map[string]any{"input": 100, "output": 20, "totalTokens": 120},
	}})
	emit(map[string]any{"type": "turn_end"})
	if scenario == "compact" {
		emit(map[string]any{
			"type": "compaction_end", "reason": "threshold",
			"result": map[string]any{
				"summary": "之前聊了什么", "firstKeptEntryId": "e1",
				"tokensBefore": 9000, "estimatedTokensAfter": 2000,
			},
		})
	}
	emit(map[string]any{"type": "agent_end", "willRetry": true})
	emit(map[string]any{
		"type": "auto_retry_start", "attempt": 1, "maxAttempts": 3,
		"delayMs": 10, "errorMessage": "529 overloaded",
	})
	emit(map[string]any{"type": "auto_retry_end", "success": true, "attempt": 1})
	emit(map[string]any{"type": "agent_settled", "aborted": false})
}

func respond(id, command string, data any) {
	rec := map[string]any{
		"type": "response", "command": command, "success": true,
	}
	if id != "" {
		rec["id"] = id
	}
	if data != nil {
		rec["data"] = data
	}
	emit(rec)
}

func emit(v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	// LF 是唯一分隔符。
	fmt.Fprint(os.Stdout, string(raw)+"\n")
	_ = os.Stdout.Sync()
}
