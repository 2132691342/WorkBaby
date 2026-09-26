package llm

// providerbase 共享工具：跨 Provider 的 JSON 序列化与指针助手。
// Provider 实现（openai/anthropic/ollama）只在此文件外承担协议差异。

import "encoding/json"

// MustMarshal JSON 序列化；忽略 error（请求构造阶段无可恢复错误，调用方负责 schema 正确）。
func MustMarshal(v any) []byte {
	bs, _ := json.Marshal(v)
	return bs
}

// IntPtr 取 int 指针（OpenAI/Anthropic max_tokens 等可选字段用 *int 表达「省略 vs 0」）。
func IntPtr(i int) *int { return &i }