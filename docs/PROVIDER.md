# 模型接入 · PROVIDER

把模型服务的 wire 协议统一成一个 `ModelClient` 契约：入参是消息序列 + 工具 schema +
采样参数，出参是归一化的 `ModelEvent` 流。配置与凭据自管。

## wire api

`models.json` 里每个 provider 的 `api` 字段决定走哪套 wire 实现。

| api | 状态 | 覆盖 |
|---|---|---|
| `openai-completions` | 已实现（缺省值） | OpenAI / DeepSeek / Moonshot / OpenRouter / 各类兼容网关 |
| `anthropic-messages` | 已实现 | Anthropic 官方与兼容网关 |
| `google-generative-ai` | 已实现 | Gemini 官方与兼容网关 |

三族都落地了，因为它们各有一块别的族覆盖不到的语义：

| 族 | 独有语义 | 踩过的坑 |
|---|---|---|
| openai-completions | 绝大多数网关都兼容 | 无 |
| anthropic-messages | `system` 独立传；工具结果叫 `tool_result` 装在 user 消息里；思考块与工具调用各有独立块类型 | 工具参数走 `input_json_delta` 分片；开思考时 `max_tokens` 必须大于 `thinking.budget_tokens` |
| google-generative-ai | `systemInstruction` 独立传；工具叫 `functionCall` / `functionResponse` 且都在 user 角色；思考是 part 里的 `thought` 标记 | JSON Schema 很严，`additionalProperties` 这类关键字会直接 400，要先剥掉 |

选择未实现的 api 时 `create_client` 直接报错并指明原因，不做静默降级——
静默降级会让用户以为在用 Anthropic，实际发出去的是 openai 格式的请求。

## ModelEvent

```
Start | TextDelta(idx, s) | ThinkingDelta(idx, s) | ToolCallDelta(idx, s)
| ToolCallEnd(idx, ToolCallDone) | Done(usage, stop_reason) | Error(retryable, msg)
```

`idx` 是同一消息内的块序号——一个回复可以同时有正文、思考与多个工具调用，
`idx` 用来把它们分开再按原序拼回去。

`stop_reason` 归一化为 `stop` / `tool_use` / `length` / `error` / `aborted`。
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

累加器 `Acc` 按 `index` 归并各片，收到 `finish_reason=tool_calls` 时才吐出完整的
`ToolCallEnd`。**提前吐出会拿到半个 JSON**，参数校验必然失败。

并行调用各占一个 `index`，定稿时按 `index` 序逐个吐出——漏掉一个就是丢掉一次调用，
模型会以为那件事做过了，实际没做。

### 停止原因

`finish_reason` 归一化成 `StopReason` 后再交给内核：

| 上游 | 归一化 | 含义 |
|---|---|---|
| `stop` | `Stop` | 正常收尾 |
| `tool_calls` / `function_call` | `ToolUse` | 要调工具，内核继续循环 |
| `length` | `Length` | 输出被截断，参数可能不完整 |
| `content_filter` | `Error`（不可重试） | 上游按内容策略截断 |
| `""`（空串） | 不定状态 | 本帧还没下结论，继续累积 |
| 流错误终止 | `Error` | 本轮失败（不可续写） |
| 用户取消 | `Aborted` | 本轮中止（不计失败） |
| 其它 | `Stop` | 收尾 |

**空 `finish_reason` 是「尚无结论」，不是一个停止理由**。部分网关每个 chunk 都带
`"finish_reason":""`，把它当终止态会把紧随其后的工具调用分片全部作废——
模型明确要求调工具，内核却当成「正文到此结束」，整轮静默丢活。

**流断开时要结清手头的调用**。`[DONE]` 或直接断流而未见 `finish_reason` 的网关
并不少见，此时 `Acc::flush_tools` 把累积中的调用定稿成 `ToolUse`，
否则这些调用凭空消失。结清之后不重复吐同一批。

**有调用就执行**。个别网关把带 `tool_calls` 的帧报成 `finish_reason=stop`，
内核因此不看 `stop_reason` 单判，只要本轮解析出了调用就执行它们。

**正文收尾必须是 `Stop`**。把它当成 `ToolUse` 会让内核在正文写完后继续空转，
反复请求模型——token 白烧，用户看到的界面还在「运行中」。

流式统一经 reqwest + eventsource-stream 解析 SSE。空闲超时 90 秒，
超时按可重试错误处理（交给内核退避重试）。

## 配置与凭据

| 文件 | 内容 |
|---|---|
| `models.json` | providers 键值：`{name?, baseUrl?, api?, apiKey?, headers?, models:[{id,name?,reasoning?,vision?,contextWindow?,maxTokens?}]}`（camelCase） |
| `auth.json` | `{"providers": {providerId: {type:"api_key", key} \| {type:"o_auth", refresh, access, expires}}}` |

- `models.json` 与 `auth.json` 分开：前者只存服务描述，后者只存凭据
- 密钥级联：`auth.json` 的 ApiKey 优先（面板「保存密钥」落这里），
  为空才取 `models.json` 的 `apiKey`；后者支持 `$ENV_NAME` 形式从环境变量读，
  这样 `models.json` 可以拷贝分享而不带密钥
- `auth.json` 的 `o_auth` 形态只存数据（refresh / access / expires），没有刷新流程
- 模型清单可手工填写，也可从服务商接口拉取后写进 `models.json`

## 内置服务商预设

没有预设时首启必然失败：用户得手写 `baseUrl` / `api` / `models` 才能发出第一条消息，
而这三项恰恰是绝大多数人不知道的东西。`wb_provider::presets` 内置九家常见服务商：

| 预设码 | 说明 |
|---|---|
| `openai` | 官方接口 |
| `deepseek` | 国内直连，性价比高 |
| `moonshot` | 国内直连，长上下文 |
| `dashscope` | 阿里云百炼 / 通义 |
| `zhipu` | 智谱 GLM |
| `openrouter` | 聚合网关 |
| `anthropic` | Claude 官方接口（走 anthropic-messages wire） |
| `gemini` | Gemini 官方接口（走 google-generative-ai wire） |
| `ollama` | 本机跑模型，无需密钥 |

预设只填「地址 + 模型 + 能力（reasoning / vision / 窗口）」——**密钥永远由用户自己给**。
三条规则：

- **已存在不覆盖**：用户可能改过地址或追加过模型，重加预设不能把它们冲掉
- **无默认时才占位**：首次添加把默认指到它的第一个模型；已有默认不抢
- **默认指向与写配置是同一件事**：只写配置不指默认，用户加完预设仍然看到「未配置模型」

`auth.json` 的凭据带 `type` 判别标签，OAuth 变体的字面量是 `o_auth`
（serde snake_case）。手写成 `oauth` 会让整个文件解析失败（2001）——
判别标签不认识就整份拒收，不做部分降级。

## 上下文窗口与采样参数

每个模型可声明 `contextWindow` 与 `maxTokens`：

| 项 | 缺省 | 作用 |
|---|---|---|
| `contextWindow` | 128 000 | 触发压缩的阈值来源，也是界面水位线的分母 |
| `maxTokens` | 16 384 | 单轮生成上限，随请求的 `max_tokens` 下发 |

缺省值刻意保守：宁可早压缩，也不要被服务端拒绝。

### 思考档位

界面给七档，wire 取值一一对应，**不做静默降级**：

| 档位码 | `reasoning_effort` |
|---|---|
| `off` | 不送该字段 |
| `minimal` / `low` / `medium` / `high` / `xhigh` / `max` | 同名值 |

把未知档位默默当 `medium` 会让「我选了 max」和「它在随便跑」不可区分。
`ThinkingLevel::parse` 对未知值返回 `None`，由 service 决定如何处置；
模型不支持推理时统一关掉（送了也没用，只会让上游报参数错）。

## 提示词缓存

Anthropic 系默认开缓存：在请求体的三处下 `cache_control` 断点——
系统提示词、最后一个工具定义、历史消息末尾。

缓存按**前缀**命中：断点之前的内容完全一致才命中。所以断点要落在
「每轮都重发但不变」的地方：系统提示词与工具定义每轮重发，历史末尾
则把整条历史与工具定义一起纳入缓存前缀。

工具定义的断点必须落在**最后一个**工具上：断在中间的话后续工具不在
缓存前缀里，等于没断。

部分兼容网关不认 `cache_control` 字段，配置里 `cache: false` 可关掉
（`ProviderCfg.cache`，缺省开）。

## 用量口径归一

各家的 usage 字段含义不同，归一后才好比较与统计：

| provider | 原始字段 | 归一 |
|---|---|---|
| openai-completions | `prompt_tokens` 含缓存部分 | `input = prompt_tokens − cache_read − cache_write` |
| google-generative-ai | `promptTokenCount` 含缓存部分 | `input = promptTokenCount − cache_read`；思考 token 计入 `output` |
| anthropic-messages | `input_tokens` 不含缓存部分 | 原样使用 |

缓存命中的读数各家摆放不同，OpenAI 系依次找 `prompt_tokens_details.cached_tokens`、
顶层 `prompt_cache_hit_tokens`、顶层 `cached_tokens`。

归一后四个字段互不重叠，`total = input + output + cache_read + cache_write`
就是上游实际处理的总量。不归一就会把同一批 token 算两遍。

## 错误分类

| 情况 | 分类 | 处理 |
|---|---|---|
| 网络错误、连接重置、空闲超时 | 可重试 | 内核退避重试 |
| 429 限流 / 5xx / 520 / 524 | 可重试 | 退避按指数序列：2s 起、逐次翻倍、上限 60s |
| 401 / 403 | 不可重试 | 直接失败，提示检查密钥 |
| 400 参数错误 | 不可重试 | 直接失败，把上游消息透出 |
| `content_filter` | 不可重试 | 直接失败，说清是上游策略截断 |
| 流中断 | 看情况 | 已产出内容则重试，否则失败 |

**上游错误消息要提取**：服务商返回的常常是 JSON（`{"error":{"message":"bad key"}}`），
直接把整坨 JSON 甩给用户不友好——提取其中的 message 字段。
非 JSON 错误体原样带出前 200 字符，不吞掉内容。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 三族 wire 都原生实现 | 各有独有语义，翻译成同一套会丢信息 | 每加一家协议就要加一份实现 |
| 未实现的 api 报错而非降级 | 静默降级会让用户误判协议 | 换厂商要改代码 |
| 缓存默认开但可关 | 命中时输入按增量价计费，是净赚 | 不认该字段的网关要手动关 || 自己管配置与凭据 | 不依赖任何外部账号体系 | 用户要自己配 key |
| 手写 SSE 解析与累加器 | 需要控制错误分类、重试时机与分片归并 | 解析代码要跟协议变化 |
| 配置与凭据分文件 | 配置可分享，密钥不外流 | 两处要同时维护 |
