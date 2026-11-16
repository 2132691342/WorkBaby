# 服务商兼容性

`api_protocol` 有三个取值，但**只有一条通路能真的聊起来**。本文按代码给出：
每种协议在「列模型」与「聊天」两条路上各支持什么，以及一个服务商配成什么样才会端到端可用。

---

## 1. 两条完全独立的路径

| 路径 | 代码 | 支持范围 |
|---|---|---|
| 列模型 / 测试连接 | `backend/service/model_probe.go`（`Probe`、`probeRequest`） | 三种协议各有端点与鉴权头 |
| 聊天（流式） | `backend/agent/provider_openai.go`（`Stream`、`buildWireRequest`） | **只有 OpenAI 兼容 `/chat/completions`** |

两者不共享判定：探测通过不保证聊天能通，聊天能通也不需要探测通过。
这解释了最常见的困惑 —— **配成 `anthropic` / `google` 能「测试连接通过」，一发言就失败**。

---

## 2. 列模型（`POST /providers/models`、`/providers/test`）

同一个 handler（`api/routes_data.go` 的 `providerProbe`），入参 `ProviderProbeREQ{provider_name,base_url,api_protocol,api_key}`；
空字段从已保存的服务商配置补（`resolveProbe`），`api_protocol` 空则按 `openai`。

| `api_protocol` | 请求 | 鉴权 | 解析 |
|---|---|---|---|
| `anthropic` | `GET <base>/v1/models`（base 已带 `/v1` 则不再补） | 头 `x-api-key: <key>` + `anthropic-version: 2023-06-01` | `data[].id`，其次 `models[].name`，再次裸数组 |
| `google` | `GET <base>/v1beta/models`（base 已带 `/v1beta` 则不再补） | 头 `x-goog-api-key: <key>`（密钥走头不进 URL：URL 会进错误原话与日志） | 同上；`models/gemini-1.5-pro` 取最后一段作模型 ID |
| 其它（含 `openai`、空、任意字符串） | `GET <pkg.OpenAIEndpoint(base, "models")>` | 头 `Authorization: Bearer <key>` | 同上 |

约束与行为：

- 地址归一与拼接收敛在一处：`pkg.NormalizeApiBase`（去首尾空白与末尾斜杠）+ `pkg.OpenAIEndpoint`（探测与聊天共用，见 §3）。`base_url` 带 `/v1` 尾巴就不再补 `/v1`，不带则自动补——两种填法收敛到同一个真实端点，探测与聊天同真同假。
- `google` 协议同口径：带 `/v1beta` 尾巴不再补。
- 超时 20s（`probeTimeout`），不复用业务客户端。
- 响应体最多读 4 MB。
- **失败不报错**：回 `probe_ok:false` + `probe_message`（如「连不上：…」「上游返回 401：…」），HTTP 仍是 200 信封成功。错误原话里的密钥会换成掩码（`redactSecret`）。
- `probe_message` 成功时是「连通，可见 N 个模型」。
- 模型名取 `displayName`，缺失时回落成 ID。
- 协议字段**不校验**：任意非空字符串都原样存下（`Upsert` 只做 `defaultIfEmpty(..., "openai")`）。

---

## 3. 聊天（唯一适配器）

`backend/agent/provider_openai.go` 无条件发：

```
POST <pkg.OpenAIEndpoint(base_url, "chat/completions")>
Content-Type: application/json
Accept: text/event-stream
Authorization: Bearer <api_key>          # 密钥非空才带
```

地址规则与探测一致（`pkg.OpenAIEndpoint`）：`base_url` 带 `/v1` 尾巴直接拼 `chat/completions`，不带则补成 `<base>/v1/chat/completions`——探测与聊天两条路收敛到同一约定。

请求体（`wireRequest`）：

| 字段 | 何时发 | 说明 |
|---|---|---|
| `model` | 恒发 | 模型的 `model_id` |
| `messages` | 恒发 | 系统提示作第一条 `system`，其余按角色 |
| `tools` | 有工具时 | `type:"function"`，JSON Schema 原样透传 |
| `stream` | 恒为 `true` | 引擎只走流式 |
| `stream_options.include_usage` | 恒发 | 让上游在末尾补一帧用量 |
| `reasoning_effort` | **仅 `low` / `medium` / `high`** | `off`、`minimal`、未知档都不发 |
| `max_tokens` | 模型配了才发 | 0 表示用上游默认 |
| `temperature` | 有值才发 | 引擎当前不设，故通常不带 |

### 图片

只有一条二进制通道：用户消息里出现图片块时，`content` 变成内容块数组，
图片以 **data URL** 形式内联（`userContent`）：

```json
{"type":"image_url","image_url":{"url":"data:image/png;base64,<...>"}}
```

纯文本消息仍发字符串形态的 `content` —— 部分兼容端点不认识内容块数组。
所以**图片支持与否取决于上游**：引擎总是这么发，上游不认就报错或忽略。

### 工具调用

助手消息的 `tool_calls[]`（`id` / `function.name` / `function.arguments` 字符串）与
工具结果的 `role:"tool"` + `tool_call_id` 是标准 OpenAI 形状。
一条被折叠过的工具输出进上下文时是「头 + 标记 + 尾」，界面与会话文件里仍是全文。

### 响应处理

- 只认 SSE 的 `data:` 行；`event` / `id` / 注释行（心跳）一律跳过。
- `data: [DONE]` 结束。
- 缓冲 1 MB：单帧可能是一整段长文本。
- 非 200 时读最多 8 KB 响应体，回「接口返回 N：<原文>」。
- 不设整体超时：一轮回复可能几分钟，只能靠 `ctx` 中止。

---

## 4. 端到端可用的判定

「能用」= 上游实现 OpenAI 兼容的 `POST /chat/completions` 流式 + `tools` + `tool_calls`。

| 配置 | 列模型 | 聊天 | 结论 |
|---|---|---|---|
| `api_protocol = openai`（或空、或任意其它值）+ OpenAI 兼容上游 | 通 | 通 | **端到端可用**（唯一的完整路径） |
| `api_protocol = anthropic` | 通（`/v1/models` + `x-api-key`） | **不通**：请求发到 `/chat/completions` 且用 Bearer | 只可探测，不可聊天 |
| `api_protocol = google` | 通（`/v1beta/models` + `x-goog-api-key`） | **不通**：请求发到 OpenAI 兼容端点且用 Bearer | 只可探测，不可聊天 |
| 反向代理 / 网关把 `/chat/completions` 转成厂商协议 | 通 | 通 | 可用：对引擎而言就是 OpenAI 兼容 |
| 自建推理服务（vLLM / Ollama / 兼容层） | 通（若实现 `/v1/models`） | 通（若实现 `/chat/completions` 流式与 tools） | 可用；不支持 `tools` 时工具轮次会失败 |

实用口径：**协议字段只影响探测，聊天一律按 OpenAI 兼容发。**
要接非 OpenAI 协议的上游，就在前面放一层转换，把 `api_protocol` 留成 `openai`。

常见服务商的填写（`base_url` 带 `/v1` 与不带均可，`pkg.OpenAIEndpoint` 收敛到同一端点）：

| 服务商 | `base_url` | `api_protocol` | 说明 |
|---|---|---|---|
| DeepSeek | `https://api.deepseek.com` | `openai` | 上游即 OpenAI 兼容，`/v1/chat/completions` 与不带 `/v1` 的路径都通 |
| Moonshot（Kimi） | `https://api.moonshot.cn/v1` | `openai` | OpenAI 兼容 |
| 智谱（GLM） | `https://open.bigmodel.cn/api/paas/v4` | `openai` | 兼容层路径不带 `/v1`，填全即可——本产品只在末尾收敛 `/v1`，不重写中间路径 |
| Ollama 本地 | `http://127.0.0.1:11434/v1` | `openai` | 实现 `/v1/chat/completions` 流式；不支持 `tools` 的模型工具轮次会失败 |
| 自建 vLLM | `http://<host>:8000/v1` | `openai` | 同上 |

---

## 5. 模型条目的字段怎么影响行为

| 字段 | 谁读 | 影响 |
|---|---|---|
| `model_id` | 请求体 `model` | 上游认的模型名；必填 |
| `model_name` | 界面 | 只影响显示，缺失回落成 `model_id` |
| `api_protocol` | 探测 | 空则继承服务商的，再空则 `openai` |
| `context_window` | 上下文水位与自动压缩、上下文裁剪 | 0 表示未知：不显示占比、不触发自动压缩、不按窗口裁剪（`Session.ContextUsage` / `autoCompactionDue` / `trimToWindow`） |
| `max_tokens` | 请求体 `max_tokens` | 0 表示不发，用上游默认 |
| `supports_reasoning` | 界面展示 | 引擎**不据此门控** `reasoning_effort`：填了 `low/medium/high` 就发（`buildWireRequest` 只看档位）。标签与实际发送可能不一致，以档位为准 |
| `input_kinds` | 界面展示 | 引擎**不据此门控**图片：图片总是以 data URL 发。上游不支持就失败 |

后两行是当前实现的事实：这两个字段是标签，不是闸门。

---

## 6. 密钥语义

- 存储：SQLite `provider.api_key_secret`，JSON 标签是 `-`，永不出接口。
- 读出：恒为掩码（空 → 空；≤ 8 字符 → `••••`；否则前 4 + `••••` + 后 4）。
- 写入：**空串 = 保留原值**（`Upsert`）。界面回传的必然是掩码，当成新值存会把密钥写坏，所以这条语义必须由调用方遵守。
- 探测：`api_key` 为空时用已保存的原文（`resolveProbe`）。
- 删除服务商或改指向：`DropActiveIf` 会同时清 `model.active_*` 与 `model.default_*` 里的坏指针。

---

## 7. 排障入口

| 现象 | 先看 |
|---|---|
| 探测失败 | `probe_message` 原文：连不上 / 返回 4xx 原文 / 地址不合法 |
| 探测通过但一聊就失败 | 就是 §1 那张表：协议选了 `anthropic` / `google`，或上游不是 OpenAI 兼容 |
| 聊到要工具就失败 | 上游不支持 `tools` / `tool_calls`（自建服务的常见缺项） |
| 思考档没效果 | 档位是 `off` / `minimal` 时不发 `reasoning_effort`；上游也可能不认这个字段 |
| 图片发送失败 | 上游不接受 data URL 形式的 `image_url`；`input_kinds` 只是标签 |
| 上下文水位不显示 | 模型没填 `context_window` |
| 发送报「还没有填密钥」 | `3000`；看 `/providers` 的 `has_key` |

错误码含义与下一步见 `docs/TROUBLESHOOTING.md`；接口字段见 `docs/API.md` §3.5。
