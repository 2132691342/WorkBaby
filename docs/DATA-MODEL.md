# 数据模型

## 两条边界

| 属于谁 | 存在哪 | 内容 |
|---|---|---|
| **PI** | `~/.pi/agent/`（或 `PI_CODING_AGENT_DIR`） | 会话 JSONL、`settings.json`、`models.json`、`mcp.json`、技能、扩展、包 |
| **WorkBaby** | `%APPDATA%\WorkBaby\` | 只有界面偏好（KV 表）、内置运行时、终端入口、日志、背景图副本 |

会话、消息、用量都不在 WorkBaby 的库里：它们是 PI 写的文件，WorkBaby 只读。

## PI 的存储（真相源）

```
~/.pi/agent/
├── settings.json      # 默认模型、思考档、工具集、压缩参数、packages、extensions
├── models.json        # 服务商与模型（明文 apiKey，PI 就是这么读的）
├── mcp.json           # MCP 服务（全局）
├── auth.json          # /login 存下的凭据
├── sessions/--<cwd>--/<ts>_<id>.jsonl   # 一条会话一个文件
├── skills/            # 用户技能（停用的移到 skills.disabled/）
├── extensions/        # 用户扩展
└── npm/node_modules/  # pi install 装下来的包
```

条目链（会话文件）里 WorkBaby 真正读的字段：

| 条目 | 用途 |
|---|---|
| `session` | 会话 id 与 cwd（工作目录） |
| `message`（role=user/assistant/toolResult） | 消息投影与用量（`usage` 在 assistant 上） |
| `message`（role=system） | 工具声明的增量补丁（`toolsAdded` / `toolsRemoved`）：工具页读它 |
| `model_change` / `thinking_level_change` | 当前模型与思考档 |
| `compaction` / `branch_summary` | 压缩与分支摘要提示条 |
| `session_info` | 会话显示名 |
| `usage` | 不参与上下文但计入花费的用量（例如缓存保温） |

## 消息投影的字段形态

一条 assistant 消息的 `content` 是**有序块数组**（`thinking` / `text` / `toolCall` / `image`）。
投影成 `MessageVO` 时按数组顺序产出 `blocks`，界面就按这个顺序就地渲染：

```go
type MessageBlock struct {
    Kind       string         // text / thinking / tool
    Text       string         // text 与 thinking 的正文
    ToolCallID string         // tool：对上不存在的 tool 消息（结果在它之后一条）
    ToolName   string
    Label      string         // 中文短名；未知工具用原名
    Args       map[string]any // 工具参数：在这里，不在结果消息上
}
```

`content` 与 `thinking` 两个字段仍然保留，但它们只是 `blocks` 的拼接结果（复制、
旧响应兜底用）。**渲染只读 `blocks`**，顺序为什么是契约见 [ARCHITECTURE](ARCHITECTURE.md)
「消息顺序」；没有 `blocks` 的旧响应按 thinking → 正文 → 工具 兜底。

工具调用与它的结果分属两条消息：声明在 assistant 的块里，结果在紧随其后的
`tool` 消息上（按 `tool_call_id` 对上号）。所以界面是一张卡，数据是两行。

`MessageVO` 上另有两个渲染开关：`hidden` 是扩展注入的隐藏内容（不渲染成气泡），
`notice` 非空时渲染成分隔提示条（已压缩 / 换模型 / 调整思考档）而不是气泡。

## 用量的字段口径

上游把提示词拆成三段报，`UsageVO` 原样存三段：

| 字段 | 含义 |
|---|---|
| `input` | **未命中**上游缓存的那部分提示词 |
| `cached` | 命中缓存的部分（`cacheRead`） |
| `cache_write` | 本次新写进缓存的部分 |
| `context` | 一次调用的提示词总量 = `input + cached + cache_write` |
| `total` | `context + output`（上游给的总量小于它时以它为准） |

两个容易算错的地方：

- **缓存命中率 = `cached / context`**，不是 `cached / input`。分母只取 `input` 时读数会超过
  100%，再钳到 100% 就成了「这个读数永远是对的」。
- **`input` 不含命中量**：Anthropic / OpenAI 系都是分字段报，把它们相加才是发出去的提示词。

## WorkBaby 自己的库

SQLite 只有一个业务表：

```go
type SettingDO struct {
    Key       string `gorm:"primaryKey;size:64"`
    Value     string `gorm:"type:text"`
    UpdatedAt int64  `gorm:"autoUpdateTime:milli"`
}
func (SettingDO) TableName() string { return "settings" }
```

| 键 | 说明 |
|---|---|
| `theme` / `font_family` / `font_size` / `show_thinking` | 外观 |
| `minimize_to_tray` / `send_on_enter` / `workspace` | 行为 |
| `bg_image` / `bg_opacity` / `bg_blur` | 自定义背景（图片复制进数据目录） |
| `language` | 界面语言（zh-CN / en，前端 i18n 与 Element Plus 文案同步） |

**为什么只留一张表**：会话与消息在 PI 的文件里，再存一份就会分叉，症状是
「界面里有的对话，命令行里看不到」。单一真相源的纪律见 [ARCHITECTURE](ARCHITECTURE.md)「两个真相源」。

迁移纪律是**只增不删**：`Migrate` 只创建 `settings` 一张表，不做任何 DROP。
数据目录里可能残留历史版本写过的表，它们不被读写也不被清理——留着不影响任何路径，
而一次错误的 DROP 会把用户的对话抹掉。

## 内存里的索引

| 索引 | 位置 | 失效方式 |
|---|---|---|
| 会话元信息 | `pi.sessionCache`（路径 + size + mtime） | 改名 / 删除 / 发送后由上层显式失效 |
| 技能注册表 | `skill.Registry` | 换工作目录 / 启停 / 导入后 `Reload` |
| 运行态（running / runID / 提问表） | `service.ChatService` | 进程退出、`chat:done`、删会话 |

会话列表不落库也不持久缓存：扫码几百个文件在毫秒级，落库只会多一处需要失效的地方。
