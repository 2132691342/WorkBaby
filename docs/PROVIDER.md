# 模型接入 · PROVIDER

把三族 wire API 统一成一个 `ModelClient` 契约：入参是消息序列 + 工具 schema +
采样参数，出参是归一化的 `ModelEvent` 流。配置与凭据自管。

## wire API 三族

| api | 覆盖 | 关键点 |
|---|---|---|
| openai-completions | OpenAI / DeepSeek / Moonshot / OpenRouter / 各类网关 | `stream_options:{include_usage:true}`；cacheRead 取 `prompt_tokens_details.cached_tokens`；reasoning 取 `completion_tokens_details.reasoning_tokens`（是 output 的子集） |
| anthropic-messages | Anthropic 及兼容网关 | thinking 块带签名（回放必须原样带上，否则校验失败）；usage 的 cacheRead / cacheWrite 取 cache_creation / input_tokens 系列字段；beta 头按模型能力加 |
| google-generative-ai | Gemini | system_instruction 独立传；思考档位映射；usageMetadata |

流式统一经 reqwest + eventsource-stream 解析 SSE。
空闲超时 60 秒，超时按可重试错误处理（交给内核重试）。

## ModelEvent

```
Start | TextDelta(idx, s) | ThinkingDelta(idx, s) | ToolCallDelta(idx, s)
| ToolCallEnd(idx, ToolCall) | Done{usage, stop_reason} | Error(msg)
```

`idx` 是同一消息内的块序号——一个回复可以同时有正文、思考与多个工具调用，
idx 用来把它们分开再按原序拼回去。

`stop_reason` 归一化为 `stop` / `tool_use` / `length` / `other`。
内核只看这三个：`stop` 收尾、`tool_use` 继续循环、`length` 触发重试（参数可能被截断）。

## 工具调用的分片累积

模型流式返回工具调用时，函数名与参数会被切成多片：

```json
{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":"}}]}}
{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a\"}"}}]}}
{"delta":{},"finish_reason":"tool_calls"}
```

累积器 `Acc` 按 index 归并各片，收到 `finish_reason=tool_calls` 时才吐出一条完整的
`ToolCallEnd`。**提前吐出会拿到半个 JSON**，参数校验必然失败。

## 配置与凭据

| 文件 | 内容 |
|---|---|
| `models.json` | providers 键值：`{name?, baseUrl?, api?, apiKey?, models:[{id,name?,reasoning?,contextWindow?,maxTokens?,cost?}]}` |
| `auth.json` | `{providerId: {type:"api_key", key} | {type:"oauth", refresh, access, expires}}` |

- `apiKey` 支持 `$ENV_NAME` 形式，从环境变量读，避免密钥落盘
- `auth.json` 与 `models.json` 分开：前者只存凭据，后者只存服务描述
- OAuth 走 refresh token 换 access token，access 过期时自动刷新
- 模型清单支持手工填写，也可从服务商接口拉取（写进 `models.json`）

## 上下文窗口

每个模型可声明 `contextWindow` 与 `maxTokens`。压缩决策用这两个值：

- `contextWindow` 决定什么时候触发压缩
- `maxTokens` 决定单次生成上限（也影响压缩的 reserve 计算）

没声明时用保守默认值（上下文 128K、单次 8K）——宁可早压缩，不要被服务端拒绝。

## 错误分类

| 情况 | 分类 | 处理 |
|---|---|---|
| 网络错误、连接重置 | 可重试 | 内核退避重试 |
| 429 限流 | 可重试 | 退避时长按响应头提示 |
| 5xx | 可重试 | 退避重试 |
| 401 / 403 | 不可重试 | 直接失败，提示检查密钥 |
| 400 参数错误 | 不可重试 | 直接失败，把上游消息透出 |
| 流中断 | 看情况 | 已产出内容则重试，否则失败 |

**上游错误消息要提取**：服务商返回的常常是 JSON（`{"error":{"message":"bad key"}}`），
直接把整坨 JSON 甩给用户不友好——提取其中的 message 字段。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 三族 wire API 而非统一走 OpenAI 兼容层 | Anthropic 的 thinking 签名与 cache 语义在兼容层里会丢 | 维护三套解析 |
| 自己管配置与凭据 | 不依赖任何外部账号体系 | 用户要自己配 key |
| 手写 SSE 解析 | 需要控制错误分类与重试时机 | 解析代码要跟三家协议 |
| 流式累加器自己写 | 工具调用分片归并是核心正确性点 | 边界情况要测 |
