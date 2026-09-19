// Package toolcall 提供 NormalizedToolCall 与各 Provider 互转（含流式累积器）。
// 协议差异：OpenAI 的 arguments 是流式字符串片段，Anthropic 与 Ollama 是一次性完整块。
package toolcall

import (
	"encoding/json"
	"strings"
	"sync"

	"WorkBaby/internal/llm"
)

// OpenAIToolCall OpenAI 兼容协议的 tool_call 形态。
type OpenAIToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIFunctionCall `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// FromOpenAI 非流式 OpenAI → 归一化。
func FromOpenAI(in []OpenAIToolCall) []llm.NormalizedToolCall {
	out := make([]llm.NormalizedToolCall, 0, len(in))
	for _, tc := range in {
		args := tc.Function.Arguments
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		out = append(out, llm.NormalizedToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: json.RawMessage(args),
		})
	}
	return out
}

// ToOpenAI 归一化 → OpenAI 形态。
func ToOpenAI(in []llm.NormalizedToolCall) []OpenAIToolCall {
	out := make([]OpenAIToolCall, 0, len(in))
	for i, tc := range in {
		out = append(out, OpenAIToolCall{
			Index: i, ID: tc.ID, Type: "function",
			Function: OpenAIFunctionCall{Name: tc.Name, Arguments: string(tc.Arguments)},
		})
	}
	return out
}

// Accumulator OpenAI 流式 tool_calls 累积器。
type Accumulator struct {
	mu    sync.Mutex
	calls map[int]*pending
}

type pending struct {
	id        string
	name      string
	args      strings.Builder
	finalized bool
}

func NewAccumulator() *Accumulator { return &Accumulator{calls: make(map[int]*pending)} }

func (a *Accumulator) Feed(d OpenAIToolCall) (completed []llm.NormalizedToolCall) {
	a.mu.Lock()
	defer a.mu.Unlock()
	p, ok := a.calls[d.Index]
	if !ok {
		p = &pending{id: d.ID}
		a.calls[d.Index] = p
	}
	if d.ID != "" {
		p.id = d.ID
	}
	if d.Function.Name != "" {
		p.name = d.Function.Name
	}
	if d.Function.Arguments != "" {
		p.args.WriteString(d.Function.Arguments)
	}
	if isLikelyCompleteJSON(p.args.String()) {
		if !p.finalized {
			p.finalized = true
			completed = append(completed, llm.NormalizedToolCall{
				ID:        p.id,
				Name:      p.name,
				Arguments: json.RawMessage(p.args.String()),
			})
		}
	}
	return
}

func (a *Accumulator) Dump() []llm.NormalizedToolCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]llm.NormalizedToolCall, 0)
	for i := 0; i < len(a.calls); i++ {
		p := a.calls[i]
		if p == nil || p.finalized {
			continue
		}
		args := p.args.String()
		if args == "" {
			args = "{}"
		}
		out = append(out, llm.NormalizedToolCall{ID: p.id, Name: p.name, Arguments: json.RawMessage(args)})
		p.finalized = true
	}
	return out
}

func isLikelyCompleteJSON(s string) bool {
	if s == "" {
		return false
	}
	depth := 0
	inStr := false
	escaped := false
	for _, r := range s {
		if escaped {
			escaped = false
			continue
		}
		switch r {
		case '\\':
			if inStr {
				escaped = true
			}
		case '"':
			inStr = !inStr
		default:
			if !inStr {
				switch r {
				case '{', '[':
					depth++
				case '}', ']':
					depth--
				}
			}
		}
	}
	return depth == 0 && !inStr
}
