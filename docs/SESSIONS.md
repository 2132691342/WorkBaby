# 会话存储 · SESSIONS

会话是「一个 JSONL 文件 + 一棵内存树」。JSONL 是真相源，界面不另存状态——
重启后从文件回放，一切如旧。

## 文件布局

```
{数据目录}\sessions\{编码后工作目录}\{起建毫秒时间}_{SESS_ULID}.jsonl
```

工作目录编码：绝对路径中的 `\` `/` `:` 全部替换为 `-`。
按工作区分目录是因为会话与工作区一一对应——换工作区就是换一批会话。

文件在首条**任意条目**追加时才创建——空会话不落盘，避免垃圾文件。
对已有会话，改模型 / 思考档位 / 重命名这类编辑同样落条目，也会触发建文件。

## 首行 header

```json
{"type":"session","version":1,"session_id":"SESS_01J…","created_at":1730000000000,"workspace":"D:\\demo"}
```

## 条目

每行一条条目：`type` / `entry_id` / `parent_entry_id` / `timestamp` 四个公共
字段 + 类型专有字段。专有字段经 serde flatten 直接铺在行顶层，与公共字段平级：

```json
{"type":"message","entry_id":"ENT_01J…","parent_entry_id":"ENT_01J…|null","timestamp":1730000000123,
 "message":{"role":"assistant","content":[{"type":"text","text":"…"}],
            "provider_id":"openai","model_id":"…","usage":{…},"stop_reason":"stop","timestamp":…}}
```

`compaction` 行的专有字段同样是顶层平铺：`summary` / `first_kept_entry_id` / `tokens_before`。

压缩触发与保留预算的两个常量：RESERVE_TOKENS = 16384（触发阈值扣减量）、
KEEP_RECENT_TOKENS = 20000（切点预算），含义见 `docs/KERNEL.md`。

| type | 专有字段 | 进上下文 |
|---|---|---|
| message | message | 是 |
| model_change | provider_id, model_id | 否（改状态） |
| thinking_change | thinking_level | 否 |
| compaction | summary, first_kept_entry_id, tokens_before | 是（摘要形态） |
| usage | kind, usage | 否 |
| custom | custom_type, data | 否 |
| context_edit | target_entry_id, replacement | 间接（仅 null 形态被回放消费） |
| session_info | session_name? | 否 |

`context_edit` 当前仅实现 `replacement = null`——把目标条目摘出回放；
非 null 的替换值在类型层有定义，回放仅消费 null 形态，生产代码当前不写
context_edit 条目。

## 消息形态

`message.role` = user / assistant / tool_result：

- **user**：`content`（文本）+ `timestamp`
- **assistant**：`content`（text / thinking / tool_call 块的有序数组）
  + `provider_id` / `model_id` / `message_id` / `usage` / `stop_reason` / `timestamp` / `duration_ms`
- **tool_result**：`tool_call_id` / `tool_name` / `content` / `is_error` / `timestamp` / `duration_ms`

内容块类型：`text` / `thinking` / `tool_call`（`{tool_call_id, name, arguments}`）。

**块有序**：正文、思考、工具调用谁先谁后，按原序存按原序渲染。
按字段类型分组会把「先说一句话，再调用工具」的顺序弄丢。

`Usage` 结构：`{input, output, cache_read, cache_write, reasoning}`。
`total` 与 `context` 在转成界面 VO 时才算出来（input+output+cache_read+cache_write），
`cost` 当前恒为 0——没有接价格表。

## 回放（leaf → root）

1. 从当前 leaf 沿 parent 指针收集到根，反转成时间序
2. 套用路径上所有 `context_edit`：`replacement` 为 null 则该条目从上下文剔除
3. 找路径上**最后一条** `compaction`：它之前的全部条目直接丢弃（不读
   `first_kept_entry_id`），摘要替代一切——全量折叠
4. compaction 替换成一条 user 消息，其后的 message 条目转成消息序列

**摘要消息的内容是固定英文包装 + 摘要正文**：

```text
The conversation history before this point was compacted into the following summary:

<summary>
{summary}
</summary>
```

更早的 compaction 经这层包装成为普通 user 消息，随全量回放进入下一次
压缩的摘要输入——不会层层叠加。

**为什么是 leaf→root 而不是全量顺序**：parent 链 + leaf 指针构成树形结构，
append 恒以当前 leaf 为父；当前没有切 leaf 的入口，实际恒为线性链。
按指针回溯让存储天然支持分支语义，历史不用复制。

## 索引与统计

会话文件本身不适合做列表查询（要遍历全量文件），所以 `wb-repo` 维护两张表：

| 表 | 内容 | 更新时机 |
|---|---|---|
| `session_index` | id / 名称 / 工作目录 / 模型 / 消息数 / 最后活动 / 摘要预览 | 会话文件变动后增量刷新 |
| `usage_daily` | 天 × 模型 × 会话粒度的 token 与成本 | 一轮运行结束时按 run 总量聚合入账；启动时回填历史会话 |

摘要预览取最后一条正文的前 80 字——列表里不展开全文。

## 不变量

- **只追加**：文件不重写，历史不可变
- **工具结果紧随其工具调用**：顺序错乱会让模型看到悬空结果
- **不做文件锁**：单进程单内核写同一会话
- **毫秒整数时间戳**：与全仓跨边界时间一致
- **JSONL 而非 SQLite 存消息**：追加语义天然匹配；人类可读可拷走；分支树免 schema 迁移
- **坏行跳过而非整体失败，header 除外**：加载时逐行解析，正文行损坏则跳过并记警告——
  一个损坏的条目不该让整段会话打不开；header 行损坏是硬失败（错误码 9701），
  没有会话身份与工作区信息，文件无法继续使用

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| JSONL 而非 SQLite 存消息 | 追加语义天然匹配；人类可读可拷走；分支树免 schema 迁移 | 列表要索引表配合 |
| 毫秒整数时间戳 | 与全仓跨边界时间一致 | 文件内不如 ISO 可读（有 header 毫秒换算） |
| 不做文件锁 | 单进程单内核写同一会话 | 手动改文件有风险（文档声明只读） |
| 摘要预览存索引表而非读文件 | 列表页要一次拿全部会话的预览 | 索引表要与文件保持同步 |
| 坏行跳过而非整文件报错 | 单条损坏不该让整段会话不可读 | 损坏内容会静默丢失，靠日志发现（header 损坏除外，硬失败） |
