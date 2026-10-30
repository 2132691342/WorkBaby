# 模型接入 · PROVIDER

把模型服务的 wire 协议统一成一个 `ModelClient` 契约：入参是消息序列 + 工具 schema +
采样参数，出参是归一化的 `ModelEvent` 流。配置与凭据自管。

## wire api

`models.json` 里每个 provider 的 `api` 字段决定走哪套 wire 实现。

| api | 状态 | 覆盖 |
|---|---|---|
| openai-completions | 已实现 | OpenAI / DeepSeek / Moonshot / OpenRouter / 各类网关 |
| anthropic-messages | 未实现 | 选它会在建客户端时报 3005 |
| google-generative-ai | 未实现 | 同上 |

**当前只落地 openai-completions 一族**。三族共存的收益是能覆盖不同厂商的独有语义
（思考块签名、cache token 口径、system_instruction 独立传），代价是三套解析代码要各自跟进协议。
这个取舍的前提是「先跑通最通用的一族」——绝大多数网关都兼容 openai-completions，
先把协议解析、错误分类与重试时机做扎实，比一次铺三套半成品更有价值。

选择未实现的 api 时，`create_client` 直接报错并指明原因，不做静默降级——
静默降级会让用户以为在用 Anthropic，实际发出去的是 openai 格式的请求。

## ModelEvent

```
Start | TextDelta(idx, s) | ThinkingDelta(idx, s) | ToolCallDelta(idx, s)
| ToolCallEnd(idx, ToolCallDone) | Done(usage, stop_reason) | Error(msg)
```

`idx` 是同一消息内的块序号——一个回复可以同时有正文、思考与多个工具调用，
idx 用来把它们分开再按原序拼回去。

`stop_reason` 归一化为 `stop` / `tool_use` / `length` / `other`。
内核只看这几个：`stop` 收尾、`tool_use` 继续循环、`length` 触发重试（参数可能被截断）。

### 一帧多事件

上游一帧可能同时带正文、思考与多个工具调用的分片，所以解析结果是**事件序列**而非单值。
按「一帧一个事件」处理会静默丢掉同帧里的其它内容——典型表现是工具调用参数少一半，
或者正文被工具分片挤掉。

### 工具调用的分片累积

模型流式返回工具调用时，函数名与参数会被切成多片：

```json
{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":"}}]}}
{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a\"}"}}]}}
{"delta":{},"finish_reason":"tool_calls"}
```

累积器 `Acc` 按 index 归并各片，收到 `finish_reason=tool_calls` 时才吐出完整的
`ToolCallEnd`。**提前吐出会拿到半个 JSON**，参数校验必然失败。

并行调用各占一个 index，定稿时按 index 序逐个吐出——漏掉一个就是丢掉一次调用，
模型会以为那件事做过了，实际没做。

### 停止原因

`finish_reason` 归一化成 `StopReason` 后再交给内核：

| 上游 | 归一化 | 含义 |
|---|---|---|
| `stop` | Stop | 正常收尾 |
| `tool_calls` / `function_call` | ToolUse | 要调工具，内核继续循环 |
| `length` | Length | 输出被截断，参数可能不完整 |
| 其它 | Stop | 收尾 |

**正文收尾必须是 Stop**。把它当成 ToolUse 会让内核在正文写完后继续空转，
反复请求模型——token 白烧，用户看到的界面还在「运行中」。

流式统一经 reqwest + eventsource-stream 解析 SSE。
空闲超时 90 秒，超时按可重试错误处理（交给内核重试）。

## 配置与凭据

| 文件 | 内容 |
|---|---|
| `models.json` | providers 键值：`{name?, baseUrl?, api?, apiKey?, headers?, models:[{id,name?,reasoning?,vision?,contextWindow?,maxTokens?}]}` |
| `auth.json` | `{providerId: {type:"api_key", key} \| {type:"oauth", refresh, access, expires}}` |

- `models.json` 与 `auth.json` 分开：前者只存服务描述，后者只存凭据
- 密钥取用有级联：`auth.json` 优先（面板「保存密钥」落这里），
  其次取 `models.json` 的 `apiKey`；后者支持 `$ENV_NAME` 形式从环境变量读，避免密钥落盘
- `auth.json` 的 `oauth` 形态是数据结构上的预留，当前没有刷新流程
- 模型清单支持手工填写，也可从服务商接口拉取（写进 `models.json`）

## 上下文窗口

每个模型可声明 `contextWindow` 与 `maxTokens`。压缩决策用这两个值：

- `contextWindow` 决定什么时候触发压缩
- `maxTokens` 决定单次生成上限（也影响压缩的 reserve 计算）

没声明时用保守默认值（上下文 128K、单次 16K）——宁可早压缩，不要被服务端拒绝。

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
非 JSON 错误体原样带出前 200 字符，不吞掉内容。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 只落地 openai-completions 一族 | 先把协议解析与错误分类做扎实 | 用不了 Anthropic 原生思考签名与 cache 语义 |
| 未实现的 api 报错而非降级 | 静默降级会让用户误判协议 | 换厂商要改代码 |
| 自己管配置与凭据 | 不依赖任何外部账号体系 | 用户要自己配 key |
| 手写 SSE 解析 | 需要控制错误分类与重试时机 | 解析代码要跟协议变化 |
| 流式累加器自己写 | 工具调用分片归并是核心正确性点 | 边界情况要测 |
