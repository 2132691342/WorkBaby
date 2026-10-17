# 17 · PI 监督进程

WorkBaby 不实现 Agent 内核。内核是外部 PI 进程（`pi --mode rpc`），
本机 Go 侧只做四件事：**发现/安装**、**拉起与守护**、**协议编解码**、**事件翻译**。

定位一句话：PI 负责跑，WorkBaby 负责让你看见并控制它怎么跑。

## 1. 为什么是子进程而不是重写内核

| 取 | 舍 |
|---|---|
| 内核能力（工具、压缩、重试、缓存、编辑、MCP、扩展、子代理）由 PI 演进，本机不追版本 | 依赖用户环境里有 Node ≥ 22.19 与 PI，启动前必须过环境闸门 |
| PI 的会话是纯 JSONL 文件，WorkBaby 可以只读投影，不复制状态 | 两套持久化并存：SQLite 存 WorkBaby 自己的账，PI 存对话真身 |
| 协议是 JSONL over stdin/stdout，语言无关 | 每次 spawn 有固定冷启动（实测 ~1s，首次含扩展加载 ~6s） |
| 崩溃隔离：PI 挂了不带走桌面进程 | 需要一套重启与"会话丢失"的对账逻辑 |

重写内核意味着永久追 PI 的每次工具与协议变更，而收益只是省一个 Node 依赖。

## 2. 生命周期

```text
Bootstrap（一次）
  ├─ 定位 Node：内置 > 系统 PATH > 向导安装
  ├─ 定位 PI：内置 node_modules > 全局 PATH
  ├─ 环境闸门通过 → Supervisor.Start
  └─ 未通过 → 前端进「环境向导」，不 spawn

Supervisor.Start(workspace)
  ├─ spawn(pi, --mode rpc --no-session=[true|false], cwd=workspace)
  ├─ stdout 协程按 LF 切分 → 记录分类
  ├─ stderr 协程 → 日志尾部缓冲（不解析为协议）
  └─ 就绪判定 = 首个 response 或首个 session event

Supervisor.Stop
  ├─ 关闭 stdin（PI 的有序关闭信号）
  ├─ 等 agent_settled 或 5s 超时
  └─ 超时则 kill
```

一个 Supervisor 进程对应**一个工作区**。不同工作区的会话不能共用一个 PI 进程：
PI 的 `cwd` 在进程启动时固定，工作区是进程级属性。

## 3. 协议编解码

### 3.1 分帧铁律

按 LF（`\n`）切分，允许前置 `\r`。**禁止**用会同时按 U+2028 / U+2029 切分的行读取器
（JSON 字符串里这两个字符合法）。Go 侧 `bufio.Reader.ReadBytes('\n')` 满足要求。

stdout 只走协议记录；诊断与日志走 stderr。两侧都必须持续读到 EOF——
停止读取会让管道缓冲写满并阻塞 PI。

### 3.2 记录分类

| 记录 | 判据 | 处置 |
|---|---|---|
| 命令响应 | `type == "response"` | 按 `id` 唤醒等待者；`success == false` 转 AppError |
| 会话事件 | 无 `id` 的已知事件名 | 进事件翻译 |
| 扩展 UI | `type == "extension_ui_request"` | `confirm`/`select` 转 `chat:approval` 并回响应；其余忽略 |
| 未知 | 其余 | 记 debug 日志后丢弃 |

**未知记录必须忽略而不是失败**：PI 的扩展会在首帧就发 `extension_ui_request`（实测
`setStatus` / `setWidget`），把未知类型当错误会让每个装了扩展的环境都起不来。

`confirm` / `select` 是唯一需要回响应的两种记录，它们就是审批闸门的通道——
详见 [27](27-approval-bridge.md)。

### 3.3 命令与关联

每个命令可带字符串 `id`，响应原样回带。命令处理是异步的，**必须按 id 关联，不能按顺序**。

```go
type Client struct {
    cmd   io.WriteCloser
    mu    sync.Mutex
    waits map[string]chan Response  // id → 一次性等待
    seq   atomic.Uint64
}

func (c *Client) Call(ctx context.Context, cmd Command) (json.RawMessage, error)
```

无响应的命令（`steer` / `abort` / 事件）也必须带 id，否则超时无法区分是谁超的。

### 3.4 输出背压

写 stdin 必须尊重背压：`cmd.Write` 返回不足即视为失败并重启，
半条 JSON 写进去会让 PI 的解析器吞掉后续命令。

## 4. 事件翻译（核心）

WorkBaby 保留自己的 SSE 词表，把 PI 事件翻进去。前端的 `chat:*` 契约不变。

| PI 事件 | WorkBaby 事件 | 说明 |
|---|---|---|
| `agent_start` | `chat:start` | 生成 runID，写运行记录 |
| `turn_start` | （内部） | 重置本轮累积 |
| `message_start`（assistant） | （内部） | 落空壳 assistant 条目 |
| `message_update` / `text_delta` | `chat:delta`（kind=text） | 逐字 |
| `message_update` / `thinking_delta` | `chat:delta`（kind=thinking） | 逐字 |
| `message_update` / `toolcall_start` | `chat:tool_start` | 参数已完整时才有 args |
| `tool_execution_end` | `chat:tool_end` | `isError` 取反进 `ok` |
| `message_end`（assistant） | （内部） | 回写条目、记 usage |
| `turn_end` | `chat:context` + `chat:done` 的用量部分 | 本轮 usage 与上下文水位 |
| `compaction_end` | `chat:compressed` | before/after |
| `auto_retry_start` | `chat:retry` | 新增事件：前端显示"重试中" |
| `agent_end` | （内部） | 记 stopReason |
| `agent_settled` | `chat:done` | 唯一"真的结束了"信号 |
| `extension_ui_request` | 忽略 | — |

### 4.1 `agent_end` 不是结束

PI 的重试、溢出恢复、压缩重试、插话、跟进都可能接在 `agent_end` 之后。
**只有 `agent_settled` 表示「不会自动继续了」**。把 `chat:done` 挂在 `agent_end`
会让界面在自动续跑时先显示"完成"再继续吐字。

代价：`agent_settled` 之前必须一直持着会话锁，用户在此期间看到的是"还在跑"。

### 4.2 会话锁与 PI 的 streamingBehavior

PI 在流式中拒绝无 `streamingBehavior` 的 prompt。所以：

| 用户动作 | WorkBaby 调用的命令 |
|---|---|
| 空闲发消息 | `prompt` |
| 流式中发消息并选「插话」 | `steer` |
| 流式中发消息并选「排队」 | `follow_up` |
| 停止 | `abort`（先 `clear_queue` 取回未消费文本还给输入框） |

`abort` 会等待会话空闲后才响应，因此它本身是阻塞命令，必须有超时。

## 5. 会话持久化

**PI 的 JSONL 是对话真身，SQLite 只存 WorkBaby 自己的账。**

| 数据 | 位置 | 理由 |
|---|---|---|
| 消息、工具结果、压缩摘要、分支 | `~/.pi/agent/sessions/**/*.jsonl` | PI 写的，只有它能正确写 |
| 会话标题、工作区、模型、权限 | SQLite `sessions` | WorkBaby 的列表与筛选靠它 |
| token 用量、运行记录、检查点 | SQLite | 仪表盘与观测是自己的功能 |
| 技能、知识库、记忆、MCP | SQLite + 文件 | 拓展模块 |

列表页读 SQLite（快、可按工作区筛），打开会话读 PI 的 JSONL 并投影成消息视图。

SQLite 的 `sessions` 表新增两列：`pi_session_id`（PI 的 uuid）与 `pi_session_file`（绝对路径）。
`message_count` / `total_tokens` 从 PI 的 `get_session_stats` 同步。

### 5.1 为什么读 JSONL 而不是每次 `get_messages`

`get_messages` 只返回当前 leaf 的上下文链，**分支结构、压缩摘要、被 context_edit
剔除的条目都拿不到**——这些恰好是"看见它怎么跑的"最需要的东西。
JSONL 是完整的、离线的、崩溃后仍可读的。

## 6. 崩溃与重启

| 场景 | 处置 |
|---|---|
| PI 进程退出码非 0 | 广播 `chat:error`（code 9701），会话锁释放，进程表标记 dead |
| 用户下一条消息 | 按需 respawn，然后用 `switch_session` 接回原 JSONL |
| PI 在本轮中途死 | 本轮标记 failed，已落库的条目保留，不自动重发（重发可能重复副作用） |
| 应用退出 | 先关 stdin，再等 `agent_settled`，最后 kill；残留的「运行中」记录在下次启动收口 |

**不自动重发**是同一条纪律：工具可能已经写过文件，自动重发等于执行两次。

## 7. 不变量

| 不变量 | 锁死原因 |
|---|---|
| stdout 分帧只按 LF | U+2028/U+2029 在 JSON 字符串里合法，按 Unicode 行分隔切分会截断记录 |
| 扩展 UI 未知方法忽略不报错 | PI 的扩展会在首帧就发 `setStatus`，当错误处理则处处起不来 |
| 命令按 id 关联 | PI 异步处理，响应顺序与发送顺序无保证 |
| `chat:done` 只在 `agent_settled` | 自动重试/压缩/插话都挂在 `agent_end` 之后，提前完成会让界面自相矛盾 |
| 一个工作区一个进程 | `cwd` 是进程级属性，跨工作区会话共进程会让工具写到错的目录 |
| 停止必先 `clear_queue` | 不取回就 `abort`，用户排队的那句话凭空消失 |
| 进程死亡不自动重发本轮 | 工具可能已有副作用，重发等于执行两次 |
| stderr 不解析为协议 | PI 把诊断写 stderr，误解析会把日志当命令 |
| stdin 必须完整写入 | 半条 JSON 会让 PI 的解析器吞掉后续命令，表现为"命令全部超时" |
| 事件出口仍按会话串行化 | 沿用 `service.Emitter`：并行工具的表现层仍在同一会话里竞争 seq |

## 8. 代价

- **启动多一跳**：首次 spawn 含扩展加载，实测 ~6s。用「启动时预热一个空闲进程」抵掉。
- **协议是 PI 的私有契约**：PI 改事件名时要跟。翻译层集中在一个文件，改动面可控。
- **两套存储**：删会话要同时删 SQLite 行与 JSONL 文件；只删一边会留孤儿。
- **依赖 Node**：内置 node 后这一条对最终用户消失，但 exe 体积 +36MB。
