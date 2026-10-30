# 会话存储 · SESSIONS

会话是「一个 JSONL 文件 + 一棵内存树」。JSONL 是真相源，界面不另存状态——
重启后从文件回放，一切如旧。

## 文件布局

```
{数据目录}\sessions\{编码后工作目录}\{起建毫秒时间}_{SESS_ULID}.jsonl
```

工作目录编码：绝对路径中的 `\` `/` `:` 全部替换为 `-`。
按工作区分目录是因为会话与工作区一一对应——换工作区就是换一批会话。

文件在首条消息到达时才创建——空会话不落盘，避免垃圾文件。

## 首行 header

```json
{"type":"session","version":1,"session_id":"SESS_01J…","created_at":1730000000000,"workspace":"D:\\demo"}
```

## 条目

每行一条条目，公共基座 + 类型专有字段：

```json
{"type":"message","entry_id":"ENT_01J…","parent_entry_id":"ENT_01J…|null","timestamp":1730000000123,
 "payload":{"message":{"role":"assistant","content":[{"type":"text","text":"…"}],
            "provider_id":"openai","model_id":"…","usage":{…},"stop_reason":"stop","timestamp":…}}}
```

| type | 专有字段 | 进上下文 |
|---|---|---|
| message | message | 是 |
| model_change | provider_id, model_id | 否（改状态） |
| thinking_change | thinking_level | 否 |
| compaction | summary, first_kept_entry_id, tokens_before | 是（摘要形态） |
| usage | kind, usage | 否 |
| custom | custom_type, data | 否 |
| context_edit | target_entry_id, replacement（内容或 null） | 间接 |
| session_info | session_name? | 否 |

载荷统一放在 `payload` 字段里，`type` 标记条目种类。

## 消息形态

`message.role` = user / assistant / toolResult：

- **user**：`content`（文本）+ `timestamp`
- **assistant**：`content`（text / thinking / toolCall 块的有序数组）
  + `provider_id` / `model_id` / `message_id` / `usage` / `stop_reason` / `timestamp` / `duration_ms`
- **toolResult**：`tool_call_id` / `tool_name` / `content` / `is_error` / `timestamp` / `duration_ms`

内容块类型：`text` / `thinking` / `toolCall`（`{tool_call_id, name, arguments}`）。

**块有序**：正文、思考、工具调用谁先谁后，按原序存按原序渲染。
按字段类型分组会把「先说一句话，再调用工具」的顺序弄丢。

`Usage` 结构：`{input, output, cache_read, cache_write, reasoning}`。
`total` 与 `context` 在转成界面 VO 时才算出来（input+output+cache_read+cache_write），
`cost` 当前恒为 0——没有接价格表。

## 回放（leaf → root）

1. 从当前 leaf 沿 parent 指针收集到根，反转成时间序
2. 套用路径上所有 `context_edit`：`replacement` 为 null 则该条目从上下文剔除
3. 找最新的 `compaction`：取其摘要作为前缀，跳过被它覆盖的早期条目
4. 剩下的 message 条目转成消息序列

**为什么是 leaf→root 而不是全量顺序**：分支场景下会话可以有多条路径，
「当前上下文」取决于从哪个 leaf 回溯。改 leaf 指针即切分支，历史不用复制。

**摘要以 user 消息形态进入上下文**：回放遇到 compaction 条目时，
把它替换成一条 user 消息（内容是摘要文本）。更早的摘要会折叠进这条消息，
不会层层叠加。

## 索引与统计

会话文件本身不适合做列表查询（要遍历全量文件），所以 `wb-repo` 维护两张表：

| 表 | 内容 | 更新时机 |
|---|---|---|
| `session_index` | id / 名称 / 工作目录 / 模型 / 消息数 / 最后活动 / 摘要预览 | 会话文件变动后增量刷新 |
| `usage_daily` | 天 × 模型 × 会话粒度的 token 与成本 | assistant 消息定稿时累加 |

摘要预览取最后一条正文的前 80 字——列表里不展开全文。

## 不变量

- **只追加**：文件不重写，历史不可变
- **toolResult 紧随其 toolCall**：顺序错乱会让模型看到悬空结果
- **不做文件锁**：单进程单内核写同一会话
- **毫秒整数时间戳**：与全仓跨边界时间一致
- **JSONL 而非 SQLite 存消息**：追加语义天然匹配；人类可读可拷走；分支树免 schema 迁移
- **坏行跳过而非整体失败**：加载时逐行解析，跳过坏行并记警告——
  一个损坏的条目不该让整段会话打不开

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| JSONL 而非 SQLite 存消息 | 追加语义天然匹配；人类可读可拷走；分支树免 schema 迁移 | 列表要索引表配合 |
| 毫秒整数时间戳 | 与全仓跨边界时间一致 | 文件内不如 ISO 可读（有 header 毫秒换算） |
| 不做文件锁 | 单进程单内核写同一会话 | 手动改文件有风险（文档声明只读） |
| 摘要预览存索引表而非读文件 | 列表页要一次拿全部会话的预览 | 索引表要与文件保持同步 |
| 坏行跳过而非整文件报错 | 单条损坏不该让整段会话不可读 | 损坏内容会静默丢失，靠日志发现 |
