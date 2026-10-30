# 会话存储 · SESSIONS

会话是「一个 JSONL 文件 + 一棵内存树」。JSONL 是真相源，界面不另存状态——
重启后从文件回放，一切如旧。

## 文件布局

```
{数据目录}\sessions\{编码后工作目录}\{起建毫秒时间}_{SESS_ULID}.jsonl
```

工作目录编码：绝对路径中的 `\` `/` `:` 全部替换为 `-`。
按工作区分目录是因为会话与工作区一一对应——换工作区就是换一批会话。

## 首行 header

```json
{"type":"session","version":1,"session_id":"SESS_01J…","created_at":1730000000000,"workspace":"D:\\demo"}
```

## 条目

每行一条条目，公共基座 + 类型专有字段：

```json
{"type":"message","entry_id":"ENT_01J…","parent_entry_id":"ENT_01J…|null","timestamp":1730000000123,
 "message":{"role":"assistant","content":[{"type":"text","text":"…"}],
            "provider_id":"anthropic","model_id":"…","usage":{…},"stop_reason":"stop","timestamp":…}}
```

| type | 专有字段 | 进上下文 |
|---|---|---|
| message | message | 是 |
| model_change | provider_id, model_id | 否（改状态） |
| thinking_change | thinking_level | 否 |
| compaction | summary, first_kept_entry_id, tokens_before, usage | 是（摘要形态） |
| usage | kind, provider_id, model_id, usage | 否 |
| custom | custom_type, data | 否 |
| context_edit | target_entry_id, replacement（内容或 null） | 间接 |
| session_info | session_name? | 否 |

## 消息形态

`message.role` = user / assistant / toolResult / system：

- **user**：`content`（文本或 image 块数组）
- **assistant**：`content`（text / thinking / toolCall 块的有序数组）
  + `provider_id` / `model_id` / `usage` / `stop_reason` / `duration_ms`
- **toolResult**：`tool_call_id` / `tool_name` / `content` / `is_error` / `duration_ms`

内容块类型：`text` / `thinking` / `image`（base64）/ `toolCall`（`{tool_call_id, name, arguments}`）。

**块有序**：正文、思考、工具调用谁先谁后，按原序存按原序渲染。
按字段类型分组会把「先说一句话，再调用工具」的顺序弄丢。

`Usage` 结构：`{input, output, cache_read, cache_write, reasoning, total, cost{...}}`。

## 回放（leaf → root）

1. 从当前 leaf 沿 parent 指针收集到根，反转成时间序
2. 套用路径上所有 `context_edit`：`replacement` 为 null 则该条目从上下文剔除
3. 找最新的 `compaction`：取其摘要作为前缀，跳过被它覆盖的早期条目
4. 剩下的条目转成消息序列

**为什么是 leaf→root 而不是全量顺序**：分支场景下会话可以有多条路径，
「当前上下文」取决于从哪个 leaf 回溯。改 leaf 指针即切分支，历史不用复制。

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

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| JSONL 而非 SQLite 存消息 | 追加语义天然匹配；人类可读可拷走；分支树免 schema 迁移 | 列表要索引表配合 |
| 毫秒整数时间戳 | 与全仓跨边界时间一致 | 文件内不如 ISO 可读（有 header 毫秒换算） |
| 不做文件锁 | 单进程单内核写同一会话 | 手动改文件有风险（文档声明只读） |
| 摘要预览存索引表而非读文件 | 列表页要一次拿全部会话的预览 | 索引表要与文件保持同步 |
