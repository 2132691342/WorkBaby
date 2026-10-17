# 23 · 运行观测与检查点

## 定位

回答三个问题：**这一轮干了什么、花了多少、走错了从哪回去**。

对话页只看得到「此刻」——正在流式输出的正文、正在跑的工具、当前水位。一旦这一轮结束、
或者用户切到别的会话，那段过程就只剩一条消息。运行观测把每次 run 落成一行账，
把工具动作从条目链投影成时间线，并在每一轮开始前打一个检查点。

它不是新运行时，也不是第二份历史：

| 数据 | 来源 | 谁写 |
|---|---|---|
| 正文与工具结果 | PI 的 JSONL（唯一真相源） | PI |
| 时间线 | 从 JSONL 的 entries 投影 | 读时投影，不落库 |
| 用量 | PI 的 `usage` entry + `message_end` | 翻译层落 `token_usages` |
| run 的账 | `runs` | 翻译层在 `agent_settled` 收尾 |
| 上下文快照 | `context_snapshots` | 轮末 `CheckpointService.Touch` |
| 检查点 | `checkpoints` | run 开始前 `CheckpointService.Mark` |

**WorkBaby 不再持有条目链**。它只记账、只投影，不写对话内容。

## 设计

| 决策 | 选择 | 为什么 | 代价 |
|---|---|---|---|
| 账与正文分离 | `runs` 只存指标与位点，不存正文 | 正文存两份就要在两处保持一致；条目链已经是唯一真相源 | 看详情要 join 一次条目链 |
| 时间线读时投影 | 从 `entries` 算出 `{chars, steps[]}` | 运行期维护第二份账要在每个事件点同步，漏一处就永久少一段 | 长会话投影有线性开销（单会话几百条可忽略） |
| 两种状态分列 | `status`（结局）+ `stop_reason`（模型为什么停） | 合成一列就分不清「被用户停掉」与「正常说完」——这两件事的处置动作完全不同 | 多一列 |
| 检查点存位点而非快照 | PI 的 entry id | 会话是树，回到某轮 = 让 PI 从这个 id 分叉；拷消息快照等于把树压平，分支就没了 | 回到某轮会新建一条 JSONL |
| 每轮一张水位快照 | `context_snapshots` | 「第几轮涨到多少、压过几次」是压缩调参的唯一依据，只在收尾记一个数看不出来 | 每条消息一行级别的表会涨（按会话删除时一起清） |
| 归档只删逐字链 | 保留 `runs` / `token_usages` / 评估 | 用量账与评估指标跨会话存活，一起删就是毁账 | 运行记录会指向一个不存在的会话（`session_id` 摘空） |

### 状态机

```
         起跑                收尾（正常/异常）
   ─────────────▶ running ──────────────▶ done
                     │                ├──▶ failed
                     │                └──▶ aborted（用户停止）
                     └── 进程退出 ─────────▶ aborted（reason=interrupted，启动时收口）
```

`running` 只存在于进程活着的时候。重启后残留的行由 `RunsService.ReapStale` 统一收口成
`aborted`：执行句柄是内存态，没人办得完，不清理就永远显示「一直在跑」——
既不能看结论，也不会自己消失。

## 契约

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/runs?session_id=&limit=` | 运行记录列表（不给 `session_id` 就是跨会话，最多 200 条） |
| GET | `/api/v1/runs/:id` | 详情：记录 + 时间线 + 上下文快照 + 检查点 |
| GET | `/api/v1/checkpoints?session_id=` | 一个会话的全部检查点（按轮次升序） |
| POST | `/api/v1/checkpoints/restore/:id` | 回到这一刻 `{session_id}` |
| POST | `/api/v1/checkpoints/fork/:id` | 从这一刻开一条新会话 `{session_id}` → `SessionVO` |
| POST | `/api/v1/archive` | 手动按保留期归档 → `{retention_days, sessions, detached_runs}` |

`RunVO`：`{id, session_id, session_title, provider_id, model, status, stop_reason, turns,
tokens, input, output, cached, context, max_tokens, compressed, started_at, finished_at,
duration_ms, can_resume}`。

`RunDetailVO` 在 `RunVO` 上追加 `timeline`（`{chars, steps[]{kind, name, label, at, ms, ok, detail}}`）、
`snapshots`（`{turn, used, window, compressed}`）、`checkpoints`（`{id, pi_entry_id, turn, prompt}`）。

`can_resume` 在 `failed` / `aborted` 时为真：界面据此给「继续」入口——这一轮没收完，
用户想接着做是合理处置。

错误码：`9601` 检查点不属于这个会话；`9602` 这次运行不存在（可能已被归档）；
`9603` 这个检查点不存在。

## 关键流程

### 一次 run 的记账

1. `agent_start` → `Runs.Start` 落一行 `running`
2. 同一处 `Checkpoints.Mark` 打一个检查点（`pi_entry_id` = 本轮起点的 PI 条目 id，
   `prompt` = 本轮的 prompt 文本，截断 200 字）
3. 每条 `message_end` → 记 usage；每轮 `turn_end` → `Touch` 覆盖写实时水位 + 落一张快照
4. `agent_settled` → `Runs.Finish`（成功与失败都走）+ `Evals.Auto` + `Archive.Maybe`

旁听者（`service/observe.go`）在这些事件之间采时间线与错误原因。它是只读的：
不改数据、不放行审批、不碰 PI 的任何状态。

### 回到某一轮

```
POST /checkpoints/restore/:id {session_id}
  → 校验检查点属于该会话（9601）
  → PI 的 fork（entryId = 检查点里的 pi_entry_id）
  → 拿到新 JSONL → 建一条新的 WorkBaby 会话行（标题加「（回退）」）
  → 广播 chat:switched，前端跳过去
```

**不是移动指针，而是分叉。** PI 的 leaf 是它内部状态，协议上没有“改 leaf”这回事。
原会话完整保留——用户想再看回去，切回原来的会话就行。

代价：回退一次多一个 JSONL 文件。这与 PI 自己的 `/fork` 行为一致，界面上
它是「从这一刻重开」而不是「撤销」。

### 从某一轮分叉

```
POST /checkpoints/fork/:id {session_id}
  → 新建会话：沿用标题（+「（分支）」）、工作目录、模型、权限、叶子位点
  → 原会话完整保留
```

### 归档

隐藏会话（子代理的载体）按 `orchestra_retention_days` 回收（默认 30 天，0 = 永久）：

| 归档删掉 | 归档留下 |
|---|---|
| 逐字消息链（`entries`）、挂起的审批、会话行 | `runs` 的账、`token_usages` 用量、评估记录 |

归档后把 `runs.session_id` 摘空：**入口消失**，而不是留着让用户点了拿 404。
还在跑的会话不碰——那一轮正要往里写。

触发点：启动时异步一次 + 每次 run 收尾检查（进程内 24 小时节流）+ 手动 `POST /archive`。

## 不变量

| 不变量 | 锁死原因 |
|---|---|
| 正文只存一份 | `runs` 与快照都不存正文；复制一份就要在两处保持一致，不一致时无法判断哪份生效 |
| 时间线必须从条目链投影 | 运行期维护第二份账，漏一个事件点就永久少一段，且没人会发现 |
| `status` 与 `stop_reason` 分开 | 合并就分不清「被停」与「说完」，两者的处置动作不同 |
| 进程退出残留的 `running` 必须收口 | 没人办得完的行会永远显示「运行中」，既不能看结论也不会自己消失 |
| 回到某轮不删任何内容 | 删了就没有「另一条分支」，用户再想回去就没了 |
| 回到某轮必须清前端运行态 | 前端按旧状态显示「运行中」，用户点停止也停不掉 |
| 检查点必须校验会话归属 | 检查点 id 全局唯一，但「回到这里」是针对某个会话的动作；不校验就是跨会话改别人的链 |
| 归档绝不删 `token_usages` | 仪表盘聚合与评估指标都从它来，一起删就是毁账 |
| 归档后 `session_id` 必须摘空 | 留着就是给用户一个点进去 404 的入口 |
| 还在跑的会话不归档 | 那一轮正往里写，删表与写链并发会让这一轮直接失败 |
| 运行记录 append-only | 「上次为什么这么做」要能翻回来，覆盖写即失忆 |
| 记账失败不拦对话 | 观测是旁路，它坏了不该让用户发不出消息（只告警） |

## 取舍

- **不落时间线**：读时投影省一张表、省一套同步逻辑；代价是长会话（上千条）打开详情
  要多一次线性扫描。单会话几百条是舒适上限，与前端无虚拟列表的取舍同源。
- **检查点按轮打点而非按消息**：一轮是一个自然的「回到这里」单位；按消息打点会让列表
  长到没法读，而用户想回去的念头总是「刚才那句之前」。
- **快照只记水位不记内容**：记内容等于把上下文存两遍（一遍在条目链，一遍在快照），
  而「当时窗口里塞了什么」是压缩器的输入，不是用户要看的东西。
- **归档以隐藏会话为单位**：子代理的逐字链是最大的那块死数据，而结论与账不依赖它，
  回收时两条互不牵连。用户自己的会话永不自动归档——那是他的东西。
