# 10 · LLM 适配层

## 定位

`backend/llm` 定义协议无关的归一化层；三家协议各自实现 `Streamer`。
新增一家服务通常只是新增一行 switch。

## 归一化模型

```go
type Message struct {
    Role, Content, Thinking string
    ToolCalls []ToolCall
    ToolCallID string
    IsError bool
    Images []Image            // 图片附件（base64 + MIME），随用户消息落库
}
type Event struct {
    Type string               // EventDelta / EventThinking / EventToolCall / EventDone / EventError
    Delta string; ToolCall *ToolCall
    StopReason string; Usage *Usage; Err error
}
type Streamer interface {
    Stream(ctx, Request) (<-chan Event, error)
}
```

### 图片附件

粘贴或附加的图片走 `Message.Images`。发送前有一道闸门：模型能力画像里
`Vision=false` 时直接拒绝（错误码 3113）——不拦截的话图片会被上游静默丢掉，
用户以为发过去了。三个适配层各自把图片编码成上游格式（openai 的
`image_url` data URI、anthropic 的 `image` block、ollama 透传 base64）。

## 三家差异收口

| 维度 | openai | anthropic | ollama |
|---|---|---|---|
| 流格式 | SSE `data:` | SSE 事件流 | JSON Lines |
| 工具调用 | `tool_calls` 增量拼装 | `tool_use` 块 | `tool_calls`（整体） |
| 思考 | reasoning_content（若有） | thinking 块 | message.thinking（若有） |
| 鉴权 | Bearer | x-api-key | 无 |
| 输出上限字段 | 按模型家族二选一（见下） | `max_tokens` 必填 | `options.num_predict` |
| 采样参数 | 普通模型下发；推理家族拒绝 | 均可下发 | 均可下发 |
| 用量字段 | `prompt_tokens` 含缓存；命中取 `prompt_tokens_details.cached_tokens` 与顶层 `cache_read_input_tokens` 的较大者（两种写法都出现过） | `input_tokens` **只含未命中**，与 `cache_read_input_tokens`、`cache_creation_input_tokens` 相加才是输入总量 | `prompt_eval_count` / `eval_count`，无缓存计量 |

适配器只做「协议 → 归一化」，业务字段（snake_case）出归一化层后统一。

### 采样参数的下发纪律

`MaxTokens` 必须显式下发。0 的语义是「调用方没准备好」，属于上层 bug，两条协议
都必须把它兜住，而且不能各兜各的：

| 协议 | 0 会怎样 | 兜底 |
|---|---|---|
| openai | `max_tokens,omitempty` 把字段整个吃掉 → 上级不知道上限，实际由网关自己定 | `llm.DefaultMaxTokens` |
| anthropic | `max_tokens` 是必填字段 | 同上 |

openai 兼容面内部还要按模型家族二选一（`reasoningOnly`）：o1/o3/o4 与 gpt-5 系
**只认 `max_completion_tokens`，且拒绝 `temperature` / `top_p`**，用旧字段直接 400。
判定按模型名（网关前缀不影响）——这不是猜能力，是协议差异。

**兜底值不能小**：推理型模型把思考算进同一份预算，给小值会让它「想完就没词」，
正文与工具调用一起断在 `finish_reason=length`（界面表现是「助手只思考，什么都没做」）。
上游报的 `length` 只是表象——预算是我们自己给的，问题出在下发的数值上；
真正的预算由 service 层派生（算法见 spec 02），这里的兜底常量只在调用方漏传时生效。

用量口径由适配层负责如实上报，`llm.Usage.Normalize` 再兜一道（见 `14`）：
网关转发别家协议时，两家的字段语义会串台，只靠适配层判不准。

两类「上游拒绝」由内核按错误文本识别后降级重试，见 `02`：

| 谓词 | 判据 | 内核动作 |
|---|---|---|
| `llm.IsContextOverflow` | 上下文超限文案 | 强制压缩后重试 |
| `llm.IsOutputLimit` | 输出预算被拒文案（`max_tokens` / `max_completion_tokens`） | `llm.OutputLimitFrom(err, 当前值)` 取新预算后重试 |

两者都不在 `RetryingStreamer` 里重试——重发同样的请求只会同样失败，
必须由掌握上下文与预算的内核换参数再发。

`Temperature` / `TopP` 相反：用指针表达「没设置就不下发」，因为 0 是合法取值
（刻意要确定性输出），不能让 0 与「未设置」在同一个字段里撞车。
持久化层的「未设置」用 `domain.SamplingUnset = -1`（负值在协议层面非法，正好当哨兵）；
service 只在用户显式配置过时才下发采样参数——用一个拍脑袋的默认温度覆盖所有模型，
会在推理型与思考型模型上撞上游约束。

### 空闲看门狗

上游超过空闲阈值没有数据就掐断（缺省 5 分钟）：推理模型思考期几分钟不发字节属正常，
给 2 分钟会把正常回答误判成断线。阈值经 `llm.SetStreamIdleTimeout` 运行期可改，
`service.applyStreamIdle` 在每次 run 装配时按 `stream_idle_seconds` 设置同步，
夹取 5 秒 ~ 30 分钟。三个适配器的错误分支都必须显式 `feed.Close()`——
feed 的 cancel 与 events 通道不关就是一次泄漏。

## 重试

`RetryingStreamer`：限流 / 过载 / 传输中断退避重试，最多 2 次，
基准 500ms、指数退避带 ≤25% 抖动、上限 8 秒；配额与鉴权类错误直接透传。

判据优先级：**HTTP 状态码 > 错误文本**。适配器用 `llm.StatusError` 把状态码
和 `Retry-After` 带出错误链，`Retryable` 先 `errors.As` 取它按码分流
（408/409/429 与 5xx 可重试，4xx 其余不可），取不到（连接层直接报错）才回落到
文本特征表。三个适配器共用 `llm.MapStatus` 映射 AppError 码段 3103~3107。

服务端给了 `Retry-After`（毫秒头优先、秒头兜底、也吃 HTTP-date）就照它等，
上限 60 秒；没有再走本地指数退避。429 场景下这是决定性的——服务端明说
「等 30 秒」，本地还按 1s/2s/4s 连撞三次，只会把限流窗口拉得更长。

## 工厂

```go
factory.NewStreamer(cfg ClientConfig) (llm.Streamer, error)
factory.SetOverride(api, f)   // 测试注入口：不联网跑全链路
factory.HasOverride(api) bool // 服务层放行测试型 api 名
```

## 凭据与连通性

- API Key AES-256-GCM 加密落库，出参只给 `has_key`
- 连通测试发一条最小请求（不设输出预算：部分上游对过小的 `max_tokens` 直接 400，
  探测会被误判成「配置错误」；模型在一句答复后自然停止），成功返回实际模型名
- 模型列表：openai 兼容 `/models`；anthropic `/v1/models`；ollama `/api/tags`

## 取舍

三家协议各自独立实现而不是统一中间层：归一化只发生在 `llm.go` 的事件出口，
协议 DTO 忠实上游字段名。统一抽象层省下的代码量，抵不上调试「归一化层
吃掉了某家特有字段」的时间。重试只在「一个字都没产出」时进行——
重复发送半截正文比失败本身更糟。
