# 03 · LLM 适配层

适配层的职责：把 OpenAI / Anthropic / Ollama 三种差异很大的协议，归一成内核唯一依赖的一组结构，并把「流式、工具调用、思维链、用量、错误」五类差异消化在这一层。

## 统一抽象

```go
type Provider interface {
    Name() string
    Kind() ProviderKind                                                  // openai / anthropic / ollama
    Chat(ctx, *ChatRequest) (*ChatResponse, error)
    Stream(ctx, *ChatRequest) (<-chan StreamChunk, error)
    Models(ctx) ([]ModelInfo, error)
    Ping(ctx) error
}
```

**请求** `ChatRequest`：`Model / Messages / Tools / Temperature / TopP / MaxTokens / Stop / Thinking / ExtraBody / SessionID`
`SessionID` 在 OpenAI 侧用作 `prompt_cache_key`（仅 `api.openai.com` 下发）。

**消息** `Message`：`Role(system|user|assistant|tool) / Content / Parts(多模态) / Thinking / ToolCalls / ToolCallID`
**响应** `ChatResponse`：`Message / ToolCalls []NormalizedToolCall / Usage / StopReason`
**流帧** `StreamChunk`：`Delta / ToolCall / FinalUsage / FinishReason / Err`
**用量** `TokenUsage`：`Input / Output / CacheRead / CacheWrite / Total`

`registry.Registry` 持有全部 Provider 实例：`Build(providers, 解密函数)` 启动时构造，`Reload` 单条热更新，`Get(id)` 未就绪返回 3001，`Status()` 给设置页展示就绪状态与原因。

## 三家协议差异

| | OpenAI | Anthropic | Ollama |
|---|---|---|---|
| 端点 | `POST {base}/chat/completions` | `POST {base}/v1/messages` | `POST {base}/api/chat` |
| 鉴权 | `Authorization: Bearer` | `x-api-key` + `anthropic-version` | 无 |
| 流式帧 | SSE `data:` + `[DONE]` | SSE 事件流（`content_block_*` / `message_*`） | ndjson，每行一对象 |
| 工具调用 | arguments 是流式字符串片段，按 index 累积，括号闭合即发射 | `input_json_delta` 累积，`content_block_stop` 时发射 | 一次性完整块 |
| 思维链 | 五种方言（见下） | `thinking:{type:"enabled",budget_tokens}` | `message.thinking` 直读 |
| 多模态 | `content:[{type:text\|image_url}]` | `source:{type:base64,media_type,data}` | `images:[裸 base64]` |
| 提示缓存 | `prompt_cache_key` | system/tools/末条消息打 `cache_control: ephemeral` | 只读 `cache_read_count` |
| 模型列表 | 真实拉取 | 静态列表兜底 | 真实拉取 |

**工具调用归一化**：`llm/toolcall` 的 `Accumulator` 负责把分片拼接成完整调用，`isLikelyCompleteJSON` 用括号深度 + 转义判定闭合即发射，`Dump()` 在流结束时兜底。内核只见 `NormalizedToolCall{ID, Name, Arguments json.RawMessage}`。

## 思维链方言

`ThinkingStyle`：`""(auto) / none / adaptive / enabled / reasoning_effort / enable_thinking`
`DetectThinkingStyle(baseURL, model)` 按 host 子串 + 模型断言探测；`ResolveThinkingStyle(指定值, baseURL, model)` 显式优先。
`ThinkingFromEffort(off|low|medium|high)` → `disabled / 2048 / 8192 / 16384`。

## 参数三级合并

`ResolveParams(req, provider, defaults)`：请求级 > Provider 级 > 默认值；`ExtraBody` 浅合并；显式 disabled 的 Thinking 不被默认值覆盖。Provider 侧把 `Temperature==0 / TopP==0 / ThinkingEffort==""` 视为「未设置」。

## 错误与重试

错误码段 3000–3009：`3001 未就绪 / 3002 密钥 / 3003 429 / 3004 5xx / 3005 超时 / 3006 4xx / 3007 模型 / 3008 上下文 / 3009 不支持`。`UpstreamError.Hint()` 给出可操作建议。

`ClassifyError` 把错误归为 `transient / auth / context / request / cancelled / unknown`——先看错误码，再扫描文本内的 `[3003]` 段位标记（`pkg.Wrap` 不保留 cause 链，靠标记传递）。

`RetryPolicy{MaxAttempts:3, BaseDelay:800ms, MaxDelay:8s}`：`Backoff` 优先采纳响应头的 `Retry-After`，否则指数退避 + 0~25% 抖动并封顶；>120s 视为越界。只有 transient 才重试。

HTTP 客户端：连接 10s、响应头 30s，**不设 `Client.Timeout`**——否则会截断长流。

## 一致性契约

`internal/llm/conformance_test.go` 钉住三家实现必须共同满足的三条契约：错误分类一致（含 Wrap 后仍可分类）、重试策略一致（Retry-After 优先、退避区间有界）、参数优先级一致。新增 Provider 必须过这套测试。
