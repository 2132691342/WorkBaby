# 会话存储 · SESSIONS

会话是「一个 JSONL 文件 + 一棵内存树」。JSONL 是真相源，界面不另存状态——
重启后从文件回放，一切如旧。

## 文件布局

```
{数据目录}\sessions\{编码后工作目录}\{起建毫秒时间}_{SESS_ULID}.jsonl
{同上}.jsonl.leaf          ← 当前叶指针（旁路文件）
```

工作目录编码：绝对路径中的 `\` `/` `:` 全部替换为 `-`。
按工作区分目录是因为会话与工作区一一对应——换工作区就是换一批会话。

文件在首条**任意条目**追加时才创建——空会话不落盘，避免垃圾文件。
对已有会话，改模型 / 思考档位 / 重命名这类编辑同样落条目，也会触发建文件。

**header 只在建文件时写一次**。加载过的会话再 append 不会补写头部：
多写一次就是往文件中间插一段 header，之后每次回放那一段都被当成坏行跳过，
会话被静默截断而界面上看不到任何错误。

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

| type | 专有字段 | 进上下文 |
|---|---|---|
| `message` | message | 是 |
| `model_change` | provider_id, model_id | 否（改状态） |
| `thinking_change` | thinking_level | 否 |
| `compaction` | summary, first_kept_entry_id, tokens_before | 是（摘要形态） |
| `usage` | kind, usage | 否 |
| `custom` | custom_type, data | 否 |
| `context_edit` | target_entry_id, replacement | 间接（仅 null 形态被回放消费） |
| `session_info` | session_name? | 否 |

`context_edit` 的 `replacement` 只在类型层支持非 null；回放只消费 `null` 形态
（把目标条目摘出上下文），生产代码目前不写这条条目。

## 消息形态

`message.role` = `user` / `assistant` / `tool_result`：

- **user**：`content`（文本）+ `timestamp`，可选 `skill`（显式调用的技能名）
  与 `images`（`[{mime, base64}]`）。技能只存名字不存正文：正文在发请求那一刻
  按名字读文件并展开成 `<skill>` 块，消息本身保持干净。
- **assistant**：`content`（text / thinking / tool_call 块的有序数组）
  + `provider_id` / `model_id` / `message_id` / `usage` / `stop_reason` / `timestamp` / `duration_ms`
- **tool_result**：`tool_call_id` / `tool_name` / `content` / `is_error` / `timestamp` /
  `duration_ms` / `file_path`? / `diff`?

内容块类型：`text` / `thinking` / `tool_call`（`{tool_call_id, name, arguments}`）。

**块有序**：正文、思考、工具调用谁先谁后，按原序存按原序渲染。
按字段类型分组会把「先说一句话，再调用工具」的顺序弄丢。

`Usage` 结构：`{input, output, cache_read, cache_write, reasoning}`。
`total` 与 `context` 在转成界面 VO 时才算出来，`cost` 当前恒为 0——没有接价格表。

## 回放（leaf → root）

前提：**叶子是本体的真相**。当前 leaf 由旁路文件 `.jsonl.leaf` 记住，
不写成树里的条目——写了它自己就会占路径一位，把回放算歪。

1. 从当前 leaf 沿 `parent_entry_id` 收集到根，反转成时间序（未被走的分支自然不参与）
2. 套用路径上所有 `context_edit`：`replacement` 为 null 则该条目从上下文剔除
3. 找路径上**最后一条** `compaction`：它之前的全部条目直接丢弃（不读
   `first_kept_entry_id`），摘要替代一切——全量折叠
4. compaction 替换成一条 user 消息，其后的 message 条目转成消息序列
5. 修复悬空工具调用（见下）

**摘要消息的内容是固定英文包装 + 摘要正文**：

```text
The conversation history before this point was compacted into the following summary:

<summary>
{summary}
</summary>
```

包装固定成英文是刻意的：它是给模型看的结构标记，不随界面语言变，
也不随轮次改写——稳定的前缀能持续命中上游的提示词缓存。

更早的 compaction 经这层包装成为普通 user 消息，随全量回放进入下一次
压缩的摘要输入——不会层层叠加。

**为什么是 leaf→root 而不是全量顺序**：parent 链 + leaf 指针构成树形结构，
append 恒以当前 leaf 为父，`fork_at` 只改叶指针、不改已有条目。
历史因此不用复制，旧分支完整保留、可随时切回。

## 悬空工具调用修复

进程死在工具执行中会留下「有 `tool_call`、没有对应 `tool_result`」的条目。
回放时如果原样发出，模型看到的是非法消息序（悬空调用）。

`session_repair` 在回放末尾补齐：为每个缺失结果的调用插一条 `is_error` 结果，
位置紧跟在所属助手消息之后。文件不改写，只修回放视图——
所以损坏由「没跑完」造成时，重开会话依然能看到当时的助手消息。

## 分支编辑

| 操作 | 接口 | 语义 |
|---|---|---|
| 列出分支点 | `session::user_entry_ids` | 只有用户消息适合作分支点（从助手回复或工具结果之后重开会留下半截对话） |
| 分叉 | `fork_at(entry_id)` | 叶指针指回去，后续追加形成新分支；旧分支条目一条不删 |
| 列出可切回的分支 | `session::alternate_leaves` | 除当前叶外、没人把它当父节点的消息条目 |
| 界面对应端点 | `POST /sessions/branch/{id}` · `POST /sessions/leaf/{id}` | 分叉与切回 |

两条切分支路径都在分叉**之前**把被放弃的那段抽成摘要（见 [KERNEL.md](KERNEL.md)
的「分支摘要」）：只把叶子指回去的话，旧分支上做过的事会从模型视野里整段消失。
摘要失败不拦住回溯，只留一条 warn 日志。

叶指针在**每次 append 后**写入 `.leaf` 旁路文件。只在分叉时写是不够的：
分叉之后的每一次追加都会把叶子往后推，磁盘上的旧指针会让下次加载从分叉前的
位置回放——分叉后的新对话整段丢掉。

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
- **坏行跳过而非整体失败，header 除外**：加载时逐行解析，正文行损坏则跳过并记警告——
  一个损坏的条目不该让整段会话打不开；header 行损坏是硬失败（9701），
  没有会话身份与工作区信息，文件无法继续使用

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| JSONL 而非 SQLite 存消息 | 追加语义天然匹配；人类可读可拷走；分支树免 schema 迁移 | 列表要索引表配合 |
| 叶指针走旁路文件 | 它不属于树，写进树会占路径一位 | 一个会话文件配一个 `.leaf` |
| 毫秒整数时间戳 | 与全仓跨边界时间一致 | 文件内不如 ISO 可读 |
| 不做文件锁 | 单进程单内核写同一会话 | 手动改文件有风险（文档声明只读） |
| 摘要预览存索引表 | 列表页要一次拿全部会话的预览 | 索引表要与文件保持同步 |
| 坏行跳过而非整文件报错 | 单条损坏不该让整段会话不可读 | 损坏内容会静默丢失，靠日志发现 |
