// Package parser 把 pi 的 JSONL 会话文件读成条目树与活动分支。
// 会话文件是唯一真相：界面状态重启后由这里回放重建。
package parser

import (
	"encoding/json"
	"os"
	"strings"

	"WorkBaby/backend/pkg"
)

// Entry 是会话文件里的一条条目；字段名与引擎落盘格式一致。
type Entry struct {
	// Type 条目种类：message / compaction / model_change / session_info / usage ...
	Type string `json:"type"`
	// Id 条目 ID（活动分支与隐藏台账的稳定光标）
	Id string `json:"id"`
	// ParentId 父条目；树形回放靠它
	ParentId string `json:"parentId"`
	// Timestamp 条目时刻（ISO 串，引擎侧格式，读进来立刻换算成毫秒）
	Timestamp string `json:"timestamp"`
	// Message 消息条目才有
	Message *Message `json:"message"`
	// Provider 模型切换条目的服务商
	Provider string `json:"provider"`
	// ModelId 模型切换条目的模型
	ModelId string `json:"modelId"`
	// Summary 压缩条目的摘要
	Summary string `json:"summary"`
	// TokensBefore 压缩前的 token 数
	TokensBefore int `json:"tokensBefore"`
	// Usage 用量条目
	Usage *Usage `json:"usage"`
	// Name 改名条目的新名字
	Name string `json:"name"`
	// Content 自定义消息条目的正文
	Content string `json:"content"`
	// Display 自定义消息是否显示在会话里
	Display bool `json:"display"`
}

// Message 是引擎消息；只取壳消费的字段。
type Message struct {
	// Role user / assistant / toolResult
	Role string `json:"role"`
	// Content 正文：字符串或内容块数组
	Content json.RawMessage `json:"content"`
	// Model 产生这条的模型
	Model string `json:"model"`
	// Provider 产生这条的服务商
	Provider string `json:"provider"`
	// Usage 这条的用量
	Usage *Usage `json:"usage"`
	// StopReason 停止原因；error 表示模型调用失败
	StopReason string `json:"stopReason"`
	// ErrorMessage 失败原话
	ErrorMessage string `json:"errorMessage"`
	// ToolCallId 工具结果对应的调用 ID
	ToolCallId string `json:"toolCallId"`
	// ToolName 工具名
	ToolName string `json:"toolName"`
	// IsError 工具是否失败
	IsError bool `json:"isError"`
	// Details 工具结果的附加信息（diff 在这里）
	Details map[string]any `json:"details"`
}

// Usage 是引擎的用量对象。
type Usage struct {
	// Input 输入 token
	Input int `json:"input"`
	// Output 输出 token
	Output int `json:"output"`
	// CacheRead 缓存命中
	CacheRead int `json:"cacheRead"`
	// CacheWrite 缓存写入
	CacheWrite int `json:"cacheWrite"`
	// TotalTokens token 合计
	TotalTokens int `json:"totalTokens"`
	// Cost 花费明细
	Cost *Cost `json:"cost"`
}

// Cost 是花费明细（美元）。
type Cost struct {
	// Total 合计
	Total float64 `json:"total"`
}

// ReadEntries 读全部条目；首行的会话头不算树节点。
// 坏行直接跳过：中途断电会留下残行，一个残行不该让整份历史读不出来。
func ReadEntries(file string) ([]Entry, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeSessionIO, "会话文件读取失败", err)
	}
	lines := pkg.SplitLines(string(raw))
	out := make([]Entry, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
			continue
		}
		if entry.Type == "session" {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

// Header 是会话文件的头。
type Header struct {
	// Id 会话 ID
	Id string
	// Cwd 会话绑定的工作目录
	Cwd string
	// Timestamp 创建时刻（毫秒）
	Timestamp int64
}

// ReadHeader 读首行会话头；读不到给零值。
func ReadHeader(file string) Header {
	raw, err := os.ReadFile(file)
	if err != nil {
		return Header{}
	}
	for _, line := range pkg.SplitLines(string(raw)) {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "{") {
			continue
		}
		var head struct {
			Type      string `json:"type"`
			Id        string `json:"id"`
			Cwd       string `json:"cwd"`
			Timestamp string `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(trimmed), &head); err != nil || head.Type != "session" {
			return Header{}
		}
		return Header{Id: head.Id, Cwd: head.Cwd, Timestamp: ParseTime(head.Timestamp)}
	}
	return Header{}
}

// ActivePath 取活动分支：从最后一条条目沿 ParentId 走到根，再反转成时间序。
// 环状引用（坏文件）由 guard 集合兜住，绝不无限循环。
func ActivePath(entries []Entry) []Entry {
	if len(entries) == 0 {
		return nil
	}
	byId := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		byId[entry.Id] = entry
	}
	chain := make([]Entry, 0, len(entries))
	guard := make(map[string]struct{}, len(entries))
	cur, ok := byId[entries[len(entries)-1].Id]
	for ok {
		if _, seen := guard[cur.Id]; seen {
			break
		}
		guard[cur.Id] = struct{}{}
		chain = append(chain, cur)
		if cur.ParentId == "" {
			break
		}
		cur, ok = byId[cur.ParentId]
	}
	// 反转成时间序。
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// ContentBlock 是消息内容块。
type ContentBlock struct {
	// Type text / thinking / toolCall
	Type string `json:"type"`
	// Text 文本（text 用）
	Text string `json:"text"`
	// Thinking 思考（thinking 用）
	Thinking string `json:"thinking"`
	// Id 工具调用 ID
	Id string `json:"id"`
	// Name 工具名
	Name string `json:"name"`
	// Arguments 工具入参
	Arguments json.RawMessage `json:"arguments"`
}

// Blocks 把消息正文解析成内容块数组；字符串形态转成一个 text 块。
func (m *Message) Blocks() []ContentBlock {
	if len(m.Content) == 0 {
		return nil
	}
	var asText string
	if err := json.Unmarshal(m.Content, &asText); err == nil {
		return []ContentBlock{{Type: "text", Text: asText}}
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil
	}
	return blocks
}

// Text 取消息的纯文本：字符串直接用，块数组拼 text 块。
func (m *Message) Text() string {
	blocks := m.Blocks()
	if len(blocks) == 0 {
		if len(m.Content) == 0 {
			return ""
		}
		var asText string
		if err := json.Unmarshal(m.Content, &asText); err == nil {
			return asText
		}
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}
