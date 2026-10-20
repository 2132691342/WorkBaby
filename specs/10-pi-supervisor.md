# 10 · PI 监督

## 定位

`backend/pi` 不实现 Agent：它运行、观测并控制外部 `pi --mode rpc` 进程。
工具、压缩、重试、缓存、编辑、MCP、扩展、子代理全由 PI 演进，本层只做
**发现 / 拉起 / 守护 / 协议编解码 / 事件翻译 / 会话文件索引**。

## 进程模型

```
一个工作目录 = 一个 PI 进程
  ├─ spawn: node <pi>/dist/bundle/cli.js --mode rpc --append-system-prompt <persona.md>（cwd = 工作目录；提交信息草稿等短调用用 --no-session）
  │        npm 的 pi.cmd / npm.cmd 先还原成 node + 入口脚本，且子进程一律 CREATE_NO_WINDOW
  ├─ stdout: 按 LF 切分 → 记录分类 → 事件翻译
  ├─ stderr: 只进日志尾部缓冲，不解析为协议
  └─ 退出: 关 stdin → 等 agent_settled 或 5s → kill
```

`cwd` 是进程级属性，所以跨工作目录的会话不能共用一个进程；
同一目录的多个会话通过 `switch_session` 在同一个进程里切换（串行）。

**入口不走 shell**：npm 在 Windows 上装的 `pi.cmd` / `npm.cmd` 是批处理 shim，
经 `cmd.exe /c` 起进程每调一次就闪一个控制台窗口，且 `CREATE_NO_WINDOW`
只作用于 cmd.exe 本身，它再拉起的 node 仍会带窗口。因此入口一律解析成
`node.exe` + 包内 `cli.js`（shim 同目录的 node 优先，其次 PATH），
并给本层全部子进程设 `CREATE_NO_WINDOW`。

**界面文本剥掉 ANSI**：内核与扩展按终端习惯给状态文字上色
（`theme.fg("dim", "Cache 0.0%")` 经 RPC 原样送达，形如 `\x1b[39m\x1b[2m…\x1b[22;39m`）。
GUI 没有终端语义，翻译层对全部面向用户的文本（增量 / 状态 / 小部件 / 通知 /
提问 / 重试原因 / 工具输出 / 历史消息）统一剥掉转义序列。

## 协议记录分类

| 记录 | 判据 | 处置 |
|---|---|---|
| 命令响应 | `type == "response"` | 按 `id` 唤醒等待者 |
| 会话事件 | 已知事件名 | 进翻译层 |
| 扩展 UI | `type == "extension_ui_request"` | `confirm`/`select`/`input`/`editor` 转 `chat:ask`；`notify`/`setStatus`/`setWidget`/`setTitle` 转只读反馈事件（文本剥 ANSI）；其余忽略 |
| 未知 | 其余 | debug 日志后丢弃 |

## 事件翻译

| PI 事件 | WorkBaby 事件 |
|---|---|
| `agent_start` | `chat:start` |
| `message_update` / `text_delta` | `chat:delta`（kind=text） |
| `message_update` / `thinking_delta` | `chat:delta`（kind=thinking） |
| `tool_execution_start` | `chat:tool_start` |
| `message_update` / `toolcall_end` | `chat:tool_start`（参数到这里才完整） |
| `tool_execution_end` | `chat:tool_end` |
| `turn_end` | `chat:context` |
| `compaction_end` | `chat:compressed` |
| `auto_retry_start` | `chat:retry` |
| `agent_settled` | `chat:done` |
| `extension_ui_request`（confirm/select/input/editor） | `chat:ask` |
| `extension_ui_request`（notify） | `chat:notify`（info/warning/error，无需应答） |
| `extension_ui_request`（setStatus） | `chat:status`（状态条，空 text 即清除） |
| `extension_ui_request`（setWidget） | `chat:widget`（文本行小部件，空行即清除） |
| `extension_ui_request`（setTitle） | `chat:title` |

### 提问在界面上的形态

协议只有四个提问基元（`confirm` / `select` / `input` / `editor`），界面**不自己发明第五种**：

| 基元 | 界面 | 回写 |
|---|---|---|
| `confirm` | 标题 + 正文 + 「确认 / 拒绝」 | `{id, confirmed}` |
| `select` | 选项清单（整行原文换行显示，↑↓ / 数字 / 回车选中） | `{id, value}` |
| `input` | 单行输入 + 扩展给的 `placeholder` | `{id, value}` |
| `editor` | 多行输入 + `prefill` 预填 | `{id, value}` |

多选与「输入自己的想法」不是协议特性，而是扩展自己的回退路径：
`select` 末尾追加一条哨兵行 → 用户点它 → 扩展再发一条 `input`；多选题则直接把
带编号的选项清单折进 `input` 的标题、格式说明放在 `placeholder` 里。
所以 `placeholder` / `prefill` / `timeout` 必须原样透传——丢了它们，
用户只能猜该按什么格式回答。界面也不把那张清单改画成复选框：
PI 认不出我们编出来的 `"1,3"` 之外的形态，重写出一个漂亮的复选框反而是假支持。

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
| 提问只按四个基元渲染 | 多选与自定义答案是扩展层的回退组合，界面自造控件会产出一个 PI 认不出的回答 |

## 取舍

- **JSONL over stdio**：进程隔离干净、可离线测试（`pitest` 就是走真管道的假 PI）；代价是没有请求多路复用之外的背压手段。
- **按工作目录分进程**：工具落到对的目录；代价是跨目录会话不能同时跑。
- **不自动重启重发**：绝不重复副作用；代价是用户要手动点「重试」。
