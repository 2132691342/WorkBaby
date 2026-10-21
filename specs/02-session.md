# 02 · 会话

## 真相源：PI 的会话文件

```
~/.pi/agent/sessions/--<cwd>--/<timestamp>_<session-id>.jsonl
```

一条会话一个文件，一个目录一个 cwd。WorkBaby **不存会话表**，列表与详情都从这些文件读：

| 界面要的 | 从文件里取 |
|---|---|
| 会话编号 | 首行 `session.id` |
| 工作目录 | 首行 `session.cwd` |
| 标题 | 最新的 `session_info.name`，没有就用首条用户消息的前 40 字 |
| 模型 / 思考档 | 最新的 `model_change` / `thinking_level_change`，模型另有 assistant 消息上的 `provider` / `model` |
| 消息数 / token / 花费 | message 与 usage 条目累加 |
| 工具清单 | `role=system` 消息里的 `toolsAdded` / `toolsRemoved` 增量补丁 |
| 消息流 | 逐条投影（user / assistant / toolResult + compaction / 分支摘要提示条） |

因此**终端里用 `pi` 聊过的会话，界面里同样看得到，点开就能接着聊**。

## 索引与缓存

`pi.ListSessions(dir)` 全量扫一遍，按「路径 + 大小 + mtime」缓存元信息（文件只会追加）。
改名 / 删除 / 发送之后显式失效对应文件的缓存。会话数量在几百量级，扫盘是毫秒级，
因此不落库、也不做持久缓存——多一处缓存就多一处需要失效的地方。

## 新建：懒创建

点「新对话」不建文件：PI 要等第一条消息才落盘。界面切到草稿态（`currentId = null`），
第一次发送时后端 `new_session` → `get_state` → 把新编号与文件路径带回给前端。

好处是列表里不会堆一串空文件，也不需要「先建再删」的清理逻辑。

## 改名 / 删除 / 换模型

| 动作 | 落点 | 约束 |
|---|---|---|
| 改名 | PI 的 `set_session_name`（写进会话文件，`pi /resume` 也认） | 要切到那条会话上，所以同目录有对话在跑时拒绝（5005） |
| 删除 | 直接删那个 JSONL（PI 的会话就是这么删的） | 运行中先中止；进程正停在它上面时先 `new_session` |
| 换模型 / 换思考档 | `set_model` / `set_thinking_level` | 同上：切会话会动进程的当前指针。新对话（还没落盘）时改的是 PI 的默认模型 / 默认思考档（`settings.json`），下一条会话直接用它 |

## 一个工作区一个当前会话

PI 的进程模型是「一个 cwd 一个进程」，进程的当前会话只有一个。
因此：

- 同目录第二条对话要等第一条跑完（返回 5005），不同目录各自一个进程可以并行；
- 切会话（改名 / 换模型 / 导出）前先确认该目录空闲。

这不是界面的限制，是内核约束；把它藏起来只会得到「两个对话的回答串在一起」。

## 投影规则

| 条目 | 投影成 |
|---|---|
| `message`（user / assistant） | 气泡；assistant 带 usage 脚注（入 / 出 / 缓存命中 / 上下文） |
| `message`（toolResult） | 工具卡，挂在声明它的那条 assistant 上（按 `tool_call_id` 对上号） |
| `compaction` | 「已压缩上下文」提示条 + 摘要 |
| `branch_summary` | 「分支摘要」提示条 |
| `model_change` / `thinking_level_change` | 「换了模型 / 调整了思考档」提示条 |
| `session` / `usage` / `custom` / `label` / `session_info` / `context_edit` | 不产消息 |

坏行只跳过自己：一行 JSON 解析失败不该让整条会话打不开。

### 内容块保留原始顺序

assistant 消息的 `content` 是一个有序块数组（`thinking` / `text` / `toolCall`）。投影时按数组顺序
产出 `blocks`，界面就按这个顺序就地渲染：思考段、正文、工具卡谁先谁后就按谁先谁后。
`content` 与 `thinking` 两个字段仍然保留，但它们只是 `blocks` 的拼接结果（复制、历史缓存兜底用）。

把「正文 / 思考 / 工具」按字段类型分组渲染，会把一轮里所有工具卡顶到正文前面——顺序一丢，
用户就没法从消息流里读出助手到底先干了什么。

## 取舍

- **文件即真相**：与 `pi` 天然一致，不会出现「界面里有、命令行里没有」；代价是每次列表要扫盘（毫秒级）。
- **改名/换模型要切会话**：用 PI 自己的写入路径，永远不手写 JSONL；代价是该目录必须空闲。
- **草稿态不建文件**：列表干净；代价是前端要处理 `session_id` 为空的发送路径。
