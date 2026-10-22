// 假 PI：按脚本回应 RPC 并吐事件（走真实 stdin/stdout 与 LF 分帧，也写会话 JSONL）。
// 场景由 --scenario 选：basic / tools / approval / compact / notify / crash。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	testSessionID   = "01JFAKESESSION0000000000A1"
	testProvider    = "test-provider"
	testModel       = "test-model"
	testContextSize = 128000
)

var (
	mu          sync.Mutex
	sessionID   = testSessionID
	sessionFile = ""
	// busy 模拟「有 run 在跑」：steer / follow_up 只有空闲时才会变成一轮新的 prompt。
	busy atomic.Bool
)

func main() {
	// --scenario 是命令行覆盖；PI_FAKE_SCENARIO 是环境变量默认值，供上层测试
	// （走 PiCommand 拉起、拿不到额外参数）也能选到带扩展事件的场景。
	scenario := os.Getenv("PI_FAKE_SCENARIO")
	if scenario == "" {
		scenario = "basic"
	}
	for i, a := range os.Args {
		if a == "--scenario" && i+1 < len(os.Args) {
			scenario = os.Args[i+1]
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
			ID          string `json:"id"`
			Type        string `json:"type"`
			Message     string `json:"message"`
			SessionPath string `json:"sessionPath"`
		}
		if err := json.Unmarshal([]byte(line), &cmd); err != nil {
			// 坏记录不该中断读循环（与真 PI 的宽容度一致）。
			continue
		}
		if cmd.SessionPath != "" {
			lastSwitchPath = cmd.SessionPath
		}
		handle(scenario, cmd.ID, cmd.Type, cmd.Message)
	}
	// stdin 关闭即有序退出。
}

func handle(scenario, id, typ, message string) {
	switch typ {
	case "get_state":
		mu.Lock()
		empty := sessionFile == ""
		mu.Unlock()
		if empty {
			startSession("")
		}
		mu.Lock()
		sid, file := sessionID, sessionFile
		mu.Unlock()
		respond(id, "get_state", map[string]any{
			"model": map[string]any{
				"id": testModel, "provider": testProvider,
				"contextWindow": testContextSize, "maxTokens": 8192,
			},
			"thinkingLevel": "medium",
			"isStreaming":   false,
			"sessionId":     sid,
			"sessionFile":   file,
			"messageCount":  0,
		})
	case "get_available_models":
		respond(id, "get_available_models", map[string]any{
			"models": []any{
				map[string]any{
					"id": testModel, "provider": testProvider, "name": "Test Model",
					"contextWindow": testContextSize, "maxTokens": 8192,
					"reasoning": true, "input": []string{"text", "image"},
				},
			},
		})
	case "get_session_stats":
		// 形状照 PI 1.4：tokens / contextUsage 都是嵌套对象。
		respond(id, "get_session_stats", map[string]any{
			"totalMessages": 4, "userMessages": 2, "assistantMessages": 2,
			"tokens": map[string]any{
				"input": 1000, "output": 200, "cacheRead": 0, "cacheWrite": 0, "total": 1200,
			},
			"cost": 0.02,
			"contextUsage": map[string]any{
				"tokens": 1000, "contextWindow": 200000, "percent": 1,
			},
		})
	case "new_session":
		startSession("")
		respond(id, typ, map[string]any{})
	case "switch_session":
		startSession(lastSwitchPath)
		respond(id, typ, map[string]any{})
	case "set_model", "set_thinking_level", "set_session_name", "compact":
		respond(id, typ, map[string]any{})
	case "clear_queue":
		respond(id, "clear_queue", map[string]any{
			"steering": []string{"排队的这句话"}, "followUp": []string{},
		})
	case "abort":
		respond(id, "abort", nil)
		abortMarker(id)
		emit(map[string]any{"type": "agent_settled", "aborted": true})
	case "fork":
		respond(id, "fork", map[string]any{
			"cancelled": false, "sessionId": "forked-session", "sessionFile": "/tmp/forked.jsonl",
		})
	case "get_commands":
		// 三种来源各一条：扩展命令只有进程知道，磁盘扫描看不到。
		respond(id, "get_commands", map[string]any{
			"commands": []any{
				map[string]any{"name": "review", "description": "跑一遍审查", "source": "extension"},
				map[string]any{"name": "weekly", "description": "写周报", "source": "prompt"},
				map[string]any{"name": "skill:demo", "description": "演示技能", "source": "skill"},
			},
		})
	case "export_html":
		respond(id, "export_html", map[string]any{"path": "C:/tmp/session.html"})
	case "abort_retry":
		respond(id, "abort_retry", nil)
	case "prompt":
		// prompt 的响应只表示「已接受」，不代表答完。
		respond(id, "prompt", map[string]any{"disposition": "started"})
		if sessionFile == "" {
			startSession("")
		}
		appendEntry(map[string]any{
			"type": "message", "id": "u1", "parentId": nil,
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"message": map[string]any{
				"role": "user", "content": message, "timestamp": time.Now().UnixMilli(),
			},
		})
		goScript(scenario, message)
	case "steer", "follow_up":
		// 空闲时没有 run 在消费队列：与真 PI 一致，等价于一轮新的 prompt。
		if !busy.Load() {
			respond(id, typ, map[string]any{"disposition": "started"})
			if sessionFile == "" {
				startSession("")
			}
			appendEntry(map[string]any{
				"type": "message", "id": "u1", "parentId": nil,
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"message": map[string]any{
					"role": "user", "content": message, "timestamp": time.Now().UnixMilli(),
				},
			})
			go goScript(scenario, message)
			return
		}
		respond(id, typ, map[string]any{"disposition": "queued"})
	default:
		respond(id, typ, map[string]any{})
	}
}

// lastSwitchPath 是 switch_session 请求里给的路径（socket 里没有回读通道，直接从参数里记）。
var lastSwitchPath string

// startSession 建（或接管）一条会话文件：真 PI 也是从这一步开始落盘的。
// 落盘位置跟 PI 一致：<PI_CODING_AGENT_DIR>/sessions/--<cwd>--/，界面扫的就是这里。
func startSession(file string) {
	mu.Lock()
	defer mu.Unlock()
	if file == "" {
		sessionID = fmt.Sprintf("01JFAKE%d%d", os.Getpid(), time.Now().UnixNano())
		file = filepath.Join(sessionsDir(), sessionID+".jsonl")
	}
	sessionFile = file
	if sessionID == "" || !strings.HasPrefix(sessionID, "01JFAKE") {
		sessionID = strings.TrimSuffix(filepath.Base(file), ".jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(file, nil, 0o644)
	appendRaw(map[string]any{
		"type": "session", "version": 3, "id": sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "cwd": cwd(),
	})
	// 首帧系统消息带工具声明：界面上的「工具」页读的就是这一段。
	appendRaw(map[string]any{
		"type": "message", "id": "sys1", "parentId": nil,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"message": map[string]any{
			"role": "system", "content": "",
			"toolsAdded": []map[string]any{
				{"name": "read", "description": "Read a file"},
				{"name": "write", "description": "Write a file"},
				{"name": "bash", "description": "Run a command"},
				{"name": "ask_user_question", "description": "Ask the user"},
			},
		},
	})
}

// sessionsDir 计算会话目录（与真 PI 的布局一致）。
func sessionsDir() string {
	agent := os.Getenv("PI_CODING_AGENT_DIR")
	if agent == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.TempDir()
		}
		agent = filepath.Join(home, ".pi", "agent")
	}
	escaped := strings.NewReplacer(":", "-", "\\", "-", "/", "-").Replace(cwd())
	escaped = strings.TrimPrefix(escaped, "-")
	return filepath.Join(agent, "sessions", "--"+escaped+"--")
}

func cwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// goScript 吐一轮完整的事件序列：agent_end 之后还有一条自动重试，最后才 agent_settled。
// done 必须挂在 agent_settled 上，挂早了会在还在跑的时候显示「完成」。
func goScript(scenario, message string) {
	busy.Store(true)
	defer busy.Store(false)
	emit(map[string]any{"type": "agent_start"})
	emit(map[string]any{"type": "turn_start"})
	// 状态条带终端色码：内核与扩展按终端习惯上色，GUI 没有终端语义，
	// 不剥就是消息流里的 [39m[2mIndexing[22;39m 这种乱码。
	emit(map[string]any{"type": "extension_ui_request", "id": "s1", "method": "setStatus",
		"statusKey": "lsp", "statusText": "\x1b[39m\x1b[2mIndexing workspace...\x1b[22;39m"})
	if scenario == "notify" {
		// fire-and-forget 四种：通知 / 状态条 / 小部件 / 标题。无需应答，客户端只能展示或忽略。
		// 文字里带终端色码：内核与扩展按终端习惯上色，GUI 没有终端语义，原样显示就是乱码。
		emit(map[string]any{"type": "extension_ui_request", "id": "n1", "method": "notify",
			"message": "命令被用户拒绝", "notifyType": "warning"})
		emit(map[string]any{"type": "extension_ui_request", "id": "n2", "method": "setStatus",
			"statusKey": "my-ext", "statusText": "\x1b[39m\x1b[2mTurn 3 running...\x1b[22;39m"})
		emit(map[string]any{"type": "extension_ui_request", "id": "n3", "method": "setWidget",
			"widgetKey": "my-ext", "widgetLines": []any{"--- My Widget ---", "Line 1"},
			"widgetPlacement": "aboveEditor"})
		emit(map[string]any{"type": "extension_ui_request", "id": "n4", "method": "setTitle",
			"title": "pi - demo"})
	}
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
		// 审批闸门（confirm）：扩展发 confirm，客户端回 extension_ui_response 后才继续。
		emit(map[string]any{
			"type": "extension_ui_request", "id": "confirm-1", "method": "confirm",
			"title": "允许 write?", "message": "要写入文件 a.txt",
		})
		// 多选 / 自定义答案在 RPC 里就是一条带编号清单的 input：
		// placeholder 与 timeout 必须原样传到界面，丢了它们用户只能猜格式。
		emit(map[string]any{
			"type": "extension_ui_request", "id": "input-1", "method": "input",
			"title": "要改哪几项？", "placeholder": "1,3; 自定义答案", "timeout": 30000,
		})
	}
	emit(map[string]any{"type": "message_end", "message": map[string]any{
		"role": "assistant", "stopReason": "stop", "timestamp": 2,
		"usage": map[string]any{"input": 100, "output": 20, "totalTokens": 120},
	}})
	appendEntry(map[string]any{
		"type": "message", "id": "a1", "parentId": "u1",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"message": map[string]any{
			"role": "assistant", "content": []any{map[string]any{"type": "text", "text": "你好"}},
			"provider": testProvider, "model": testModel, "stopReason": "stop",
			"timestamp": time.Now().UnixMilli(),
			"usage": map[string]any{
				"input": 100, "output": 20, "totalTokens": 120,
				"cost": map[string]any{"total": 0.01},
			},
		},
	})
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

// abortMarker 把“收到 abort”写进会话文件：上层据此断言停止真的送到了内核，
// 而不是只在界面上广播了一句 chat:stopped（内核还在跑）。
func abortMarker(id string) {
	appendEntry(map[string]any{
		"type": "custom", "customType": "aborted", "id": "abort-" + id,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
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

// appendEntry 往当前会话文件追加一行（会话索引读的就是它）。
func appendEntry(v any) {
	mu.Lock()
	defer mu.Unlock()
	appendRaw(v)
}

// appendRaw 是加锁后的落盘实现。
func appendRaw(v any) {
	if sessionFile == "" {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	f, err := os.OpenFile(sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(raw, '\n'))
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
