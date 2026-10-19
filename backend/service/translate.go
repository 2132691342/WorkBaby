// 事件翻译：把 PI 的会话事件转成 WorkBaby 的 chat:* 事件。
// 它是唯一知道「PI 事件 → 界面事件」映射的地方。
package service

import (
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pi"
	"WorkBaby/backend/pkg"
)

// runState 收集一次 PI run 过程中的记账位点。
// 它不是会话状态：消息真身在 PI 的 JSONL 里，这里只记本轮要广播的读数。
type runState struct {
	sessionID  string
	runID      string
	model      string
	provider   string
	tokens     int
	context    int
	window     int
	maxTokens  int
	stopReason string
	usage      *domain.UsageVO
	started    int64
}

// translator 是 pi.Handler 的实现：一个工作区一个实例，按会话分派。
type translator struct {
	svc       *ChatService
	workspace string

	mu sync.Mutex
	// states 按会话 id 索引正在跑的 run。
	states map[string]*runState
	// current 是进程当前停着的会话 id：扩展在会话开始时提问时只有它有值。
	current string
}

// OnEvent 分派一条 PI 事件。
func (t *translator) OnEvent(ev pi.Event) {
	sessionID, st := t.sessionOf()
	t.svc.onEvent(t.workspace, sessionID, st, ev)
}

// OnExit 在 PI 进程退出时收口。
func (t *translator) OnExit(err error) { t.svc.onProcessExit(t.workspace, err) }

// sessionOf 取当前活跃会话与其 run 状态。
func (t *translator) sessionOf() (string, *runState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for sid, st := range t.states {
		return sid, st
	}
	return t.current, nil
}

// setSession 绑定当前活跃会话。
func (t *translator) setSession(sessionID string, st *runState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = sessionID
	if st == nil {
		delete(t.states, sessionID)
		return
	}
	t.states[sessionID] = st
}

// setCurrent 只更新当前会话指针（切会话但还没发消息时用）。
func (t *translator) setCurrent(sessionID string) {
	t.mu.Lock()
	t.current = sessionID
	t.mu.Unlock()
}

// clearSession 摘掉会话的 run 状态。
func (t *translator) clearSession(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.states, sessionID)
}

// onEvent 把一条 PI 事件翻译成界面事件。
func (c *ChatService) onEvent(workspace, sessionID string, st *runState, ev pi.Event) {
	switch ev.Kind {
	case pi.KindAgentStart:
		if st == nil {
			return
		}
		st.started = nowMillis()
		c.emit(sessionID, domain.EventChatStart, domain.StartData{
			RunID: st.runID, SessionID: sessionID,
		})

	case pi.KindMessageUpdate:
		if st == nil {
			return
		}
		c.onMessageUpdate(sessionID, st, ev)

	case pi.KindMessageEnd:
		if st != nil && (ev.Role == "assistant" || ev.Usage != nil) {
			c.onMessageEnd(st, ev)
			if ev.StopReason != "" {
				st.stopReason = ev.StopReason
			}
		}

	case pi.KindToolStart:
		c.emit(sessionID, domain.EventChatToolStart, domain.ToolStartData{
			ToolCallID: ev.ToolCallID, Tool: ev.ToolName,
			Label: labelOf(ev.ToolName), Args: ev.Args,
		})

	case pi.KindToolEnd:
		out := ""
		if ev.Result != nil {
			out = pkg.StripANSI(plainTextOf(ev.Result.Content))
		}
		c.emit(sessionID, domain.EventChatToolEnd, domain.ToolEndData{
			ToolCallID: ev.ToolCallID, OK: !ev.IsError,
			Title: labelOf(ev.ToolName), Output: out, DurationMs: ev.DurationMs,
		})

	case pi.KindTurnEnd:
		if st == nil {
			return
		}
		c.emit(sessionID, domain.EventChatContext, domain.ContextData{
			Used: st.context, Window: st.window, Known: st.window > 0,
			Ratio: ratioOf(st.context, st.window, 0),
		})

	case pi.KindCompactionEnd:
		if ev.CompactResult == nil {
			return
		}
		c.emit(sessionID, domain.EventChatCompressed, domain.CompressedData{
			TokensBefore: ev.CompactResult.TokensBefore,
			TokensAfter:  ev.CompactResult.EstimatedTokensAfter,
		})

	case pi.KindRetryStart:
		// 没有这个事件，界面在自动重试期间看上去就是卡死。
		c.emit(sessionID, domain.EventChatRetry, domain.RetryData{
			Attempt: ev.Attempt, Max: ev.MaxAttempts, Message: pkg.StripANSI(ev.ErrorMessage),
		})

	case pi.KindAgentEnd:
		if st != nil {
			st.stopReason = stopReasonOf(ev)
		}

	case pi.KindAgentSettled:
		if st != nil {
			c.finishSettled(sessionID, st, ev.Aborted)
		}

	case pi.KindExtensionUI:
		// 需要用户回答的方法进问答桥；fire-and-forget 方法转成只读的界面反馈。
		switch ev.Method {
		case "confirm", "select", "input", "editor":
			c.openAsk(workspace, sessionID, ev)
		case "notify":
			c.emit(sessionID, domain.EventChatNotify, domain.NotifyData{
				Level: notifyLevelOf(ev.NotifyType), Message: pkg.StripANSI(ev.Body),
			})
		case "setStatus":
			c.emit(sessionID, domain.EventChatStatus, domain.StatusData{
				Key: ev.StatusKey, Text: pkg.StripANSI(ev.StatusText),
			})
		case "setWidget":
			lines := []string{}
			if ev.WidgetLines != nil {
				lines = ev.WidgetLines
			}
			clean := make([]string, 0, len(lines))
			for _, ln := range lines {
				clean = append(clean, pkg.StripANSI(ln))
			}
			c.emit(sessionID, domain.EventChatWidget, domain.WidgetData{
				Key: ev.ID, Lines: clean, Placement: ev.WidgetPlacement,
			})
		case "setTitle":
			c.emit(sessionID, domain.EventChatTitle, domain.TitleData{Title: pkg.StripANSI(ev.Title)})
		}
	}
}

// notifyLevelOf 把 PI 的 notify 级别归一到前端取值；未知级别按 info。
func notifyLevelOf(t string) string {
	switch t {
	case "warning", "error":
		return t
	default:
		return "info"
	}
}

// onMessageUpdate 处理流式增量。
func (c *ChatService) onMessageUpdate(sessionID string, st *runState, ev pi.Event) {
	switch ev.DeltaType {
	case "text_delta":
		if ev.Delta == "" {
			return
		}
		c.emit(sessionID, domain.EventChatDelta, domain.DeltaData{
			EntryID: st.runID, Kind: "text", Delta: pkg.StripANSI(ev.Delta),
		})
	case "thinking_delta":
		if ev.Delta == "" {
			return
		}
		c.emit(sessionID, domain.EventChatDelta, domain.DeltaData{
			EntryID: st.runID, Kind: "thinking", Delta: pkg.StripANSI(ev.Delta),
		})
	case "toolcall_end":
		if ev.ToolCall == nil {
			return
		}
		// 参数到这里才完整：toolcall_delta 只给半截 JSON，发出去界面也解析不了。
		c.emit(sessionID, domain.EventChatToolStart, domain.ToolStartData{
			ToolCallID: ev.ToolCall.ID, Tool: ev.ToolCall.Name,
			Label: labelOf(ev.ToolCall.Name), Args: ev.ToolCall.Arguments,
		})
	}
	// 流式期间的 usage 是累计读数：拿它实时更新水位。
	if ev.Usage != nil {
		st.tokens = ev.Usage.TotalTokens
		st.context = ev.Usage.Input + ev.Usage.CacheRead
	}
}

// onMessageEnd 记一次用量读数。
func (c *ChatService) onMessageEnd(st *runState, ev pi.Event) {
	if ev.Usage == nil {
		return
	}
	u := &domain.UsageVO{
		Input: ev.Usage.Input, Output: ev.Usage.Output,
		Cached: ev.Usage.CacheRead, Total: ev.Usage.TotalTokens,
		Context: st.context, Cost: ev.Usage.Cost.Total,
	}
	// 上游给的总量小于两者之和时以两者之和为准：仪表盘上「总量比输入+输出还小」只会让人以为统计坏了。
	if u.Total < u.Input+u.Output {
		u.Total = u.Input + u.Output
	}
	// 命中量不可能超过输入量；上游按「未命中口径」报 input 时补回总量。
	if u.Cached > u.Input {
		u.Input = u.Cached
	}
	st.usage = u
	if st.context < u.Input {
		st.context = u.Input
	}
}

// finishSettled 在 agent_settled 时收尾：这是唯一「真的结束」信号。
func (c *ChatService) finishSettled(sessionID string, st *runState, aborted bool) {
	reason := st.stopReason
	if aborted {
		reason = "aborted"
	}
	c.emit(sessionID, domain.EventChatDone, domain.DoneData{
		EntryID: st.runID, StopReason: reason, Usage: st.usage, MaxTokens: st.maxTokens,
	})
	c.release(sessionID)
	c.translatorOfWorkspace(sessionWorkspace(c, sessionID)).clearSession(sessionID)
}

// onProcessExit 在 PI 进程退出时收口：本轮标记失败，但不自动重发。
func (c *ChatService) onProcessExit(workspace string, err error) {
	// 不自动重发是纪律：工具可能已经写过文件，重发等于执行两次。
	tr := c.translatorOfWorkspace(workspace)
	tr.mu.Lock()
	ids := make([]string, 0, len(tr.states))
	for sid := range tr.states {
		ids = append(ids, sid)
	}
	tr.mu.Unlock()
	for _, sid := range ids {
		pkg.Warnf("chat: PI 进程退出，会话 %s 的本轮按失败收口: %v", sid, err)
		c.emit(sid, domain.EventChatError, domain.ErrorData{
			Code: pkg.CodeOf(pi.ErrProcessDown), Message: "助手进程退出了，这一轮没做完",
		})
		c.release(sid)
		tr.clearSession(sid)
		c.closeAsksOf(sid, "gone")
	}
}

// sessionWorkspace 取会话所在工作目录；查不到返回空串。
func sessionWorkspace(c *ChatService, sessionID string) string {
	if c.sessions == nil {
		return ""
	}
	return c.workspaceOf(sessionID)
}

// labelOf 给工具名一个中文短名；未知工具用原名（编一个好听的反而对不上）。
func labelOf(name string) string {
	switch name {
	case "read", "ffread":
		return "读文件"
	case "write":
		return "写文件"
	case "edit":
		return "改文件"
	case "bash", "powershell":
		return "跑命令"
	case "python":
		return "跑脚本"
	case "grep", "ffgrep":
		return "搜内容"
	case "find", "ls", "fffind":
		return "找文件"
	case "web_search":
		return "搜网页"
	case "fetch_content", "web_fetch":
		return "开网页"
	case "codemode":
		return "写代码调用工具"
	case "tool_search":
		return "找工具"
	case "ask_user_question":
		return "征求意见"
	default:
		if strings.HasPrefix(name, "mcp__") {
			parts := strings.Split(name, "__")
			if len(parts) >= 3 {
				return parts[1] + " / " + parts[2]
			}
		}
		return name
	}
}

// plainTextOf 把内容块数组拼成纯文本（工具输出展示用）。
func plainTextOf(blocks []pi.ContentBlock) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type == "text" && blk.Text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

// stopReasonOf 取 PI 的收尾原因。
func stopReasonOf(ev pi.Event) string {
	if ev.StopReason != "" {
		return ev.StopReason
	}
	return "stop"
}

// ratioOf 换算成整数百分比，分母是可用预算（窗口减输出余量）。
func ratioOf(used, window, reserve int) int {
	if window <= 0 || used <= 0 {
		return 0
	}
	usable := window - reserve
	if usable <= 0 {
		usable = window / 2
	}
	r := used * 100 / usable
	if r > 100 {
		return 100
	}
	return r
}

// nowMillis 是毫秒时间戳的单一取法。
func nowMillis() int64 { return time.Now().UnixMilli() }
