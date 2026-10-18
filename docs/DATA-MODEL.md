# 数据模型

## 两条边界

| 属于谁 | 存在哪 | 内容 |
|---|---|---|
| **PI** | `~/.pi/agent/`（或 `PI_CODING_AGENT_DIR`） | 会话 JSONL、`settings.json`、`models.json`、`mcp.json`、技能、扩展、包 |
| **WorkBaby** | `%APPDATA%\WorkBaby\` | 只有界面偏好（KV 表）、内置运行时、日志、背景图副本 |

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
| `context_window` | 手填覆盖 `models.json` 里的窗口 |

**为什么只留一张表**：会话与消息在 PI 的文件里，再存一份就是两本账——两本账迟早分叉，
症状是「界面里有的对话，命令行里看不到」。单一真相源让这类分叉不可能发生。

旧版本留下的表（sessions / providers / token_usages / approvals / runs / evals /
knowledge_docs / memories …）不再迁移也不再读写；迁移函数只创建 `settings`。
「只增不删」的迁移纪律在这里体现为：**不主动 DROP**，旧表留在库里不影响任何路径。

## 内存里的索引

| 索引 | 位置 | 失效方式 |
|---|---|---|
| 会话元信息 | `pi.sessionCache`（路径 + size + mtime） | 改名 / 删除 / 发送后由上层显式失效 |
| 技能注册表 | `skill.Registry` | 换工作目录 / 启停 / 导入后 `Reload` |
| 运行态（running / runID / 提问表） | `service.ChatService` | 进程退出、`chat:done`、删会话 |

会话列表不落库也不持久缓存：扫码几百个文件在毫秒级，落库只会多一处需要失效的地方。
