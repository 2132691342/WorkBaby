# 07 · 会话

一个会话 = 一个 PI session（JSONL 文件）+ 一行 WorkBaby 元数据。
两者用 `pi_session_id` 绑定，谁都不是对方的副本。

## 1. 两处存储的分工

| 数据 | 位置 | 谁写 |
|---|---|---|
| 消息、工具结果、压缩摘要、分支树、context_edit | PI JSONL | 只有 PI |
| 标题、工作区、模型、权限档、隐藏标记 | SQLite `sessions` | 只有 WorkBaby |
| token 用量、运行记录、检查点 | SQLite | 只有 WorkBaby |

WorkBaby **绝不写 PI 的 JSONL**。格式是 PI 的私有契约，写坏了 PI 自己都读不回来。

## 2. SessionDO

```go
type SessionDO struct {
    ID            string // ULID，WorkBaby 自己的 id，界面与路由用它
    PiSessionID   string // PI 的 uuid
    PiSessionFile string // JSONL 绝对路径
    Title         string
    Workspace     string
    Model         string // "provider/modelId"
    ThinkingLevel string
    Permission    string // auto / ask / readonly
    Hidden        bool
    MessageCount  int
    TotalTokens   int
    CreatedAt     int64
    UpdatedAt     int64
}
```

`ID` 与 `PiSessionID` 分开：JSONL 文件名里带 PI 的 uuid 与时间戳，
WorkBaby 不能保证提前知道它（`new_session` 之后才拿到），也不能保证从不重建。
用 WorkBaby 自己的 id 做外部标识，PI 的 id 只用于接回那个文件。

## 3. 创建

```text
POST /sessions
  ├─ 建 Shell 会话（pi_session_id 为空，界面立刻可见可点）
  ├─ 首次发送消息时：
  │    └─ ensureProcess(workspace) → new_session → get_state
  │         └─ 拿到 sessionId 与 sessionFile，回写 SQLite
  └─ 之后每轮复用同一个进程
```

**懒创建**的理由：用户点「新对话」十次只有一次真的发消息。每次都 spawn PI 建 JSONL
会留下十个空文件，还会拉长每次点击的等待。

## 4. 载入

```text
GET /sessions/:id
  ├─ 读 SQLite 元数据
  ├─ 读 PI JSONL（只读） → 解析每条 entry
  └─ 投影成 MessageVO[]：只出 message / compaction / branch_summary / custom_message
```

投影要点：

| JSONL entry | 出参 |
|---|---|
| `session` | 不输出（头信息进 SessionVO） |
| `message`（user / assistant / toolResult） | 一条 MessageVO |
| `compaction` | 一条「已压缩」提示条 + summary 正文 |
| `branch_summary` | 一条「分支摘要」提示条 |
| `custom_message` | 一条 MessageVO（`display=false` 时标记为隐藏） |
| `model_change` / `thinking_level_change` | 一条分隔提示条（时间线锚点） |
| `usage` / `custom` / `label` / `session_info` / `context_edit` | 不出消息（`label` 进后续的收藏功能） |

**不做上下文重建**。WorkBaby 只显示"文件里有什么"，不决定"模型看到什么"——
后者是 PI 的职责，重建错了会让界面与模型的实际输入不一致。

## 5. 列表

列表读 SQLite，不碰 JSONL：进列表页读一百个 JSONL 文件就是一百次磁盘 IO。

| 排序 | 依据 |
|---|---|
| 默认 | `updated_at` 倒序 |
| 按工作区筛 | `workspace` 精确匹配 |
| 隐藏会话 | `hidden = false` 才出（子代理的会话不进列表） |

## 6. 改名与删除

| 动作 | 落点 |
|---|---|
| 改名 | SQLite `title` + PI 的 `set_session_name`（两处都写，界面与 PI 各自读自己那份） |
| 删除 | SQLite 行 + JSONL 文件（两者都删，只删一边会留孤儿） |

删除时若该会话有进程在跑：先 `abort`，等 `agent_settled`，再删。
不等就删会让 PI 往已删除的文件继续追加。

## 7. 分支

PI 的分支是 JSONL 内的树（`id` / `parentId`）。WorkBaby 读树，用 `switch_session` +
`get_tree` 展示，用 `fork` / `clone` 落成新会话。

| 动作 | PI 命令 | 结果 |
|---|---|---|
| 从某条消息重开 | `fork`（带 entryId） | 新建一条 JSONL，`parentSession` 指向原文件 |
| 整条复制 | `clone` | 同上，但保留全部历史 |

**没有「回到那个时刻继续」**。PI 的树只有一个 leaf，往回走就是切 leaf，
切了之后原分支的消息不再进上下文——WorkBaby 的旧实现（`BranchSession` 改 leaf 指针）
在 PI 上没有对应的写入能力。界面用「分叉成新会话」表达同一意图，语义更清楚。

## 8. 工作区

工作区是**进程级**属性：PI 的 `cwd` 在 spawn 时固定。

| 动作 | 处置 |
|---|---|
| 会话首次发消息 | 按会话的 workspace spawn 对应进程（同工作区复用） |
| 改会话工作区 | 结束当前进程的会话上下文，按新工作区重新 spawn |
| 无工作区 | 用全局默认工作区 |

同一工作区的多个会话共用一个 PI 进程：进程内的 `switch_session` 切换当前会话。
**代价**：两个会话不能同时跑（一个 PI 进程只有一个 streaming 状态）。
并发跑两个会话需要两个进程，这期不做——桌面端单人使用，串行足够。

## 9. 不变量

| 不变量 | 锁死原因 |
|---|---|
| 绝不写 PI 的 JSONL | 格式是 PI 私有，写坏它 PI 都读不回来 |
| `ID` 与 `PiSessionID` 分开 | JSONL 文件名里的 uuid 要 `new_session` 之后才有，不能做提前标识 |
| 删除必须同时删 SQLite 行与 JSONL | 只删一边会留孤儿文件或孤儿记录 |
| 列表页不读 JSONL | 进列表读一百个文件就是一百次 IO，且每次都会重新解析 |
| 会话运行中不删 | 不等 `agent_settled` 就删，PI 会往已删文件继续追加 |
| 一个工作区一个进程 | `cwd` 进程级固定，跨工作区共进程会让工具写错目录 |
| 载入只投影不重建上下文 | 重建错了界面与模型实际输入不一致，比不显示更糟 |
| 分支走 fork 而非改 leaf | PI 的 leaf 是内部状态，外部改 leaf 没有协议面 |

## 10. 代价

- 同一工作区的会话不能并发跑。
- 删除要两步（行 + 文件），任何一步失败都要靠下次启动的对账扫尾。
- 分支语义从「回去继续」变成「分叉新会话」，与 PI 原生 TUI 一致但与旧实现不同。
