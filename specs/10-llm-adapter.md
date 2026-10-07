# 10 · LLM 适配层

## 定位

`internal/llm` 定义协议无关的归一化层；三家协议各自实现 `Streamer`。
新增一家服务通常只是新增一行 switch。

## 归一化模型

```go
type Message struct {
    Role, Content, Thinking string
    ToolCalls []ToolCall
    ToolCallID string
    IsError bool
}
type Event struct {
    Type EventKind            // Delta / ThinkingDelta / ToolCall / Done / Error
    Delta string; ToolCall *ToolCall
    StopReason string; Usage *Usage; Err error
}
type Streamer interface {
    Stream(ctx, Request) (<-chan Event, error)
}
```

## 三家差异收口

| 维度 | openai | anthropic | ollama |
|---|---|---|---|
| 流格式 | SSE `data:` | SSE 事件流 | JSON Lines |
| 工具调用 | `tool_calls` 增量拼装 | `tool_use` 块 | `tool_calls`（整体） |
| 思考 | reasoning_content（若有） | thinking 块 | 无 |
| 鉴权 | Bearer | x-api-key | 无 |

适配器只做「协议 → 归一化」，业务字段（snake_case）出归一化层后统一。

## 重试

`RetryingStreamer`：限流 / 过载 / 传输中断退避重试，最多 2 次，
基准 500ms、指数退避带 ≤25% 抖动、上限 8 秒；配额与鉴权类错误直接透传。

一条硬规则：**已经产出过任何内容就不再重试**——否则用户会看到重复的前半段正文。

## 工厂

```go
factory.NewStreamer(cfg ClientConfig) (llm.Streamer, error)
factory.SetOverride(api, f)   // 测试注入口：不联网跑全链路
factory.HasOverride(api) bool // 服务层放行测试型 api 名
```

## 凭据与连通性

- API Key AES-256-GCM 加密落库，出参只给 `has_key`
- 连通测试发一条 1 token 的最小请求，成功返回实际模型名
- 模型列表：openai 兼容 `/models`；anthropic `/v1/models`；ollama `/api/tags`

## 取舍

三家协议各自独立实现而不是统一中间层：归一化只发生在 `llm.go` 的事件出口，
协议 DTO 忠实上游字段名。统一抽象层省下的代码量，抵不上调试「归一化层
吃掉了某家特有字段」的时间。重试只在「一个字都没产出」时进行——
重复发送半截正文比失败本身更糟。
