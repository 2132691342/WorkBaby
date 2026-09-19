package llm

import (
	"encoding/json"
	"strconv"
	"strings"

	"WorkBaby/internal/pkg"
)

// LLM 段错误码：3000-3999。
// Provider 适配层在转换错误时统一映射。
var (
	ErrLLMGeneric      = pkg.New(3000, "LLM generic error", "")
	ErrProviderUnready = pkg.New(3001, "provider not ready", "")
	ErrAPIKeyInvalid   = pkg.New(3002, "API key invalid (401/403)", "")
	ErrRateLimited     = pkg.New(3003, "rate limited (429)", "")
	ErrProvider5xx     = pkg.New(3004, "provider service error (5xx)", "")
	ErrTimeout         = pkg.New(3005, "response timeout", "")
	ErrBadRequest      = pkg.New(3006, "request error (4xx non-401/403/429)", "")
	ErrModelNotFound   = pkg.New(3007, "model not found or no permission", "")
	ErrContextTooLong  = pkg.New(3008, "context too long (400/413)", "")
	ErrNotImplemented  = pkg.New(3009, "provider does not support this feature", "")
)

// MapHTTPStatus 把 HTTP 状态码映射到错误码；Transport 错误统一归 3005。
func MapHTTPStatus(status int) *pkg.AppError {
	switch {
	case status == 401 || status == 403:
		return ErrAPIKeyInvalid
	case status == 404:
		return ErrModelNotFound
	case status == 408:
		return ErrTimeout
	case status == 413:
		return ErrContextTooLong
	case status == 429:
		return ErrRateLimited
	case status >= 500:
		return ErrProvider5xx
	case status >= 400:
		return ErrBadRequest
	default:
		return nil
	}
}

// NewUpstreamError 把上游非 2xx 响应封装成 AppError。
//
// Message 是可直接展示给用户的单行文案（上游 message 清洗后截断）；
// Details 放可操作的修复建议（无建议时为空），前端据此决定要不要补一行引导。
// 原始 JSON 不进这两个字段——它只在日志与 run 事件里保留。
func NewUpstreamError(prefix string, status int, raw []byte) *pkg.AppError {
	u := ParseUpstreamError(status, raw)
	code := 3100
	if mapped := MapHTTPStatus(status); mapped != nil {
		code = mapped.Code
	}
	msg := prefix + " " + strconv.Itoa(status) + "：" + u.HumanMessage()
	if u.Code != "" {
		msg += "（上游码 " + u.Code + "）"
	}
	return pkg.New(code, msg, u.Hint())
}

// UpstreamError 上游错误的可读摘要。
//
// 上游报文是给人看的诊断信息，不是给终端用户看的 UI 文案：直接整段抛到聊天界面
// 会同时带出厂商内部字段与超长 JSON，把消息流撑乱。这里把它拆成
// Message（上游说了什么）+ Hint（用户该做什么）两截，前端分段呈现。
type UpstreamError struct {
	Status  int    `json:"status"`
	Message string `json:"message"` // 上游 error.message 原文（解析失败时为清洗后的原文截断）
	Type    string `json:"type"`    // 上游 error.type
	Code    string `json:"code"`    // 上游业务码（如 2013）
}

// ParseUpstreamError 尽最大努力从上游响应体解析错误摘要；非 JSON / 未知结构退回原文截断。
func ParseUpstreamError(status int, raw []byte) UpstreamError {
	body := strings.TrimSpace(string(raw))
	if body == "" {
		return UpstreamError{Status: status}
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return UpstreamError{Status: status, Message: truncateRaw(body)}
	}
	msg := firstNonEmpty(env.Error.Message, env.Message)
	if msg == "" {
		return UpstreamError{Status: status, Message: truncateRaw(body)}
	}
	return UpstreamError{
		Status:  status,
		Message: truncateRaw(msg),
		Type:    firstNonEmpty(env.Error.Type, env.Type),
		Code:    stringifyCode(env.Error.Code, env.Code),
	}
}

// HumanMessage 生成面向用户的单行错误文案（不含上游内部字段）。
func (e UpstreamError) HumanMessage() string {
	if e.Message == "" {
		return "上游返回了一个空错误"
	}
	return e.Message
}

// Hint 根据上游错误语义给出可操作的下一步建议；无匹配建议返回空串。
//
// 只覆盖「用户能自己解决」的高频场景，避免给出无法执行的建议制造噪声。
func (e UpstreamError) Hint() string {
	m := strings.ToLower(e.Message)
	switch {
	case strings.Contains(m, "thinking.type"):
		return "该模型不接受当前的思维模式参数。请到「设置 → 模型」把该模型的「思维方言」改为 adaptive 或 none 后重试。"
	case strings.Contains(m, "thinking"), strings.Contains(m, "reasoning_effort"), strings.Contains(m, "enable_thinking"):
		return "该模型不支持当前的思维参数。请到「设置 → 模型」把「思维强度」设为关闭，或调整「思维方言」。"
	case strings.Contains(m, "context length"), strings.Contains(m, "context_length"),
		strings.Contains(m, "too long"), strings.Contains(m, "maximum context"):
		return "输入超出模型上下文上限。可点输入框旁的压缩按钮手动压缩，或换用上下文窗口更大的模型。"
	case strings.Contains(m, "tool"), strings.Contains(m, "function"):
		return "该模型可能不支持工具调用。请到「设置 → 模型」关闭「支持工具调用」，改为纯对话模式。"
	case strings.Contains(m, "api key"), strings.Contains(m, "unauthorized"), strings.Contains(m, "authentication"):
		return "API Key 无效或已过期，请在「设置 → 模型」重新填写。"
	case strings.Contains(m, "quota"), strings.Contains(m, "insufficient"), strings.Contains(m, "balance"):
		return "账户额度不足，请充值后重试。"
	case strings.Contains(m, "model"), strings.Contains(m, "does not exist"):
		return "模型名不被上游识别，请核对「设置 → 模型」里的模型标识与上游清单一致。"
	default:
		return ""
	}
}

// truncateRaw 原文截断：上游报文可能极长，聊天界面放不下也没有诊断价值。
func truncateRaw(s string) string {
	s = strings.TrimSpace(s)
	const max = 300
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func stringifyCode(vals ...any) string {
	for _, v := range vals {
		switch c := v.(type) {
		case nil:
			continue
		case string:
			if c != "" {
				return c
			}
		case float64:
			return strconv.Itoa(int(c))
		}
	}
	return ""
}
