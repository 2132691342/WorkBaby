# 17 · PI 监督

## 定位

`backend/pi` 不实现 Agent：它运行、观测并控制外部 `pi --mode rpc` 进程。
工具、压缩、重试、缓存、编辑、MCP、扩展、子代理全由 PI 演进，本层只做
**发现 / 拉起 / 守护 / 协议编解码 / 事件翻译 / 会话文件索引**。

## 进程模型

```
一个工作目录 = 一个 PI 进程
  ├─ spawn: pi --mode rpc --append-system-prompt <persona.md>   (cwd = 工作目录)
  ├─ stdout: 按 LF 切分 → 记录分类 → 事件翻译
  ├─ stderr: 只进日志尾部缓冲，不解析为协议
  └─ 退出: 关 stdin → 等 agent_settled 或 5s → kill
```

`cwd` 是进程级属性，所以跨工作目录的会话不能共用一个进程；
同一目录的多个会话通过 `switch_session` 在同一个进程里切换（串行）。

## 协议记录分类

| 记录 | 判据 | 处置 |
|---|---|---|
| 命令响应 | `type == "response"` | 按 `id` 唤醒等待者 |
| 会话事件 | 已知事件名 | 进翻译层 |
| 扩展 UI | `type == "extension_ui_request"` | `confirm`/`select`/`input`/`editor` 转 `chat:ask`；其余忽略 |
| 未知 | 其余 | debug 日志后丢弃 |

## 事件翻译

| PI 事件 | WorkBaby 事件 |
|---|---|
| `agent_start` | `chat:start` |
| `message_update` / `text_delta` | `chat:delta`（kind=text） |
| `message_update` / `thinking_delta` | `chat:delta`（kind=thinking） |
| `message_update` / `toolcall_end` | `chat:tool_start`（参数到这里才完整） |
| `tool_execution_end` | `chat:tool_end` |
| `turn_end` | `chat:context` |
| `compaction_end` | `chat:compressed` |
| `auto_retry_start` | `chat:retry` |
| `agent_settled` | `chat:done` |
| `extension_ui_request`（confirm/select/input/editor） | `chat:ask` |

## 会话文件索引

`pi/session.go` 读 `~/.pi/agent/sessions/**/*.jsonl`：

- 列表用：id、cwd、标题、模型、思考档、消息数、token、花费、工具声明（缓存按 size+mtime）；
- 详情用：完整条目链，交给 service 投影成消息；
- 工具页用：重放 `toolsAdded` / `toolsRemoved` 得到的当前工具集。

## 不变量

| 不变量 | 锁死原因 |
|---|---|
| stdout 分帧只按 LF | U+2028/U+2029 在 JSON 字符串里合法，按 Unicode 行分隔切分会截断记录 |
| 未知记录忽略，不报错 | 装了扩展的环境首帧就是 `extension_ui_request`，当错误处理则处处起不来 |
| 命令按 id 关联 | PI 异步处理，响应顺序与发送顺序无保证 |
| `chat:done` 只在 `agent_settled` | 重试 / 压缩 / 插话都挂在 `agent_end` 之后，提前完成会让界面自相矛盾 |
| 一个工作目录一个进程 | `cwd` 是进程级属性，共进程会让工具写到错的目录 |
| 一个进程一个当前会话 | 切会话前必须确认该目录空闲，否则两个对话的回答会串 |
| 停止必先 `clear_queue` | 不取回就 `abort`，用户排队的那句话凭空消失 |
| 进程死亡不自动重发本轮 | 工具可能已有副作用，重发等于执行两次 |
| stderr 不解析为协议 | PI 把诊断写 stderr，误解析会把日志当命令 |
| stdin 必须完整写入 | 半条 JSON 会让解析器吞掉后续命令，表现为「命令全部超时」 |
| 不写 PI 的 JSONL | 格式是 PI 私有契约；改名 / 换模型一律走 RPC |
| agent 目录不覆盖 | 换成私有目录，用户的技能 / 插件 / 模型 / 会话全部看不见 |
| 会话运行中不删它的文件 | 不等 `agent_settled` 就删，PI 会往已删文件继续追加 |
| 写 `settings.json` 只改掌管键 | PI 与用户还在里面写 `packages` / `cacheWarming` / `codemode`，整体重写会抹掉它们 |

## 取舍

- **JSONL over stdio**：进程隔离干净、可离线测试（`pitest` 就是走真管道的假 PI）；代价是没有请求多路复用之外的背压手段。
- **按工作目录分进程**：工具落到对的目录；代价是跨目录会话不能同时跑。
- **不自动重启重发**：绝不重复副作用；代价是用户要手动点「重试」。
