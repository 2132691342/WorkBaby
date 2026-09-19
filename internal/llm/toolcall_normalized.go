package llm

import "encoding/json"

// NormalizedToolCall 归一化工具调用形态；所有 Provider 经 toolcall 子包转换后落到此处。
type NormalizedToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}
