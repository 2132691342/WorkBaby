# 数据模型

四处存储，分工明确：**会话 JSONL 是对话真相，workbaby.json 是壳的设置台账，
workbaby.db 是可查询的派生数据，mcp.json 是 MCP 服务器清单**。
每一类对象只有一个真相，壳只在其上做台账。

## 1. 设置台账（`%LOCALAPPDATA%\WorkBaby\workbaby.json`）

单个 JSON 文件，启动整读进内存，每次变更原子整写（临时文件 + 换名），
进程内互斥锁串行化。条目量级是几十条，不值得为它引数据库：

```json
{
  "settings": { "ui.theme": "dark", "ui.locale": "zh-CN" },
  "skill_toggles": { "pdf": false }
}
```

| 段 | 形状 | 用途 |
|---|---|---|
| `settings` | 键值对（值一律字符串，复杂结构自己 JSON） | 壳的全部键值（见 §2） |
| `skill_toggles` | 技能目录名 → 是否启用 | 技能开关台账 |

**为什么只有两段**：其余对象的真相都不在这个文件里。会话是 JSONL，
服务商与模型在 SQLite，技能是目录位置，MCP 是 `mcp.json`。
把这些再抄进台账就多出第二份真相，而第二份一定会不同步。

文件损坏不堵启动：解析失败时挪到旁边（`workbaby.json.bad-<ts>`）留证，
从空白重新开始——半截 JSON 多半来自断电或杀进程。

## 2. 设置键（`settings` 段）

| 键 | 值 | 消费方 |
|---|---|---|
| `ui.theme` / `ui.reduce_motion` / `ui.font_scale` / `ui.locale` | 界面偏好 | 前端 |
| `prefs.restore_session` | `'1'` / `'0'` | 启动恢复上次会话 |
| `prefs.last_session_file` | 会话文件路径 | 恢复目标 |
| `prefs.close_action` | `'tray'` / `'quit'` | 关窗行为 |
| `prefs.tray_hint_shown` | `'1'` / `'0'` | 「收进托盘」提示是否已解释过 |
| `workspace.current` | 路径 | 当前工作区 |
| `workspace.history` | JSON 数组（≤ 24 条） | 工作区历史 |
| `chat.hide:<会话路径>` | JSON 数组（条目 id） | 单轮隐藏台账 |
| `model.active_provider` / `model.active_model` | 服务商名 / 模型 ID | 当前生效的模型 |
| `model.auto_compaction` | `'0'` / `'1'` | 自动压缩开关 |
| `web.search_kind` | `duckduckgo` / `brave` / `exa` / `tavily` | 联网搜索服务 |
| `web.search_key` | 搜索服务密钥 | 联网搜索服务 |

键值一律字符串，复杂结构自己编码 JSON——KV 表不做类型推断，
加一列不如加一个键。

## 3. 会话 JSONL（对话真相）

位置：`data\sessions\--<工作区编码>--\SES_<ULID>.jsonl`

工作区编码规则：剥掉一个头部分隔符，把 `/` `\` `:` 空格换成 `-`，两端包 `--`。
引擎建会话目录与清单扫会话目录用的是同一个函数（`pkg.EncodedWorkspace`），
两边各写一套的话，新建的会话会落在清单扫不到的目录里。

首行是会话头，其后每行一个条目，条目以 `id` / `parent_id` 构成**树**：

```
{"type":"session","version":1,"id":"SES_...","timestamp":...,"cwd":"D:\\proj"}
{"type":"message","id":"SES_...","parent_id":"","timestamp":...,"message":{...}}
{"type":"message",...}          ← 工具结果同样是一条 message（role=tool）
{"type":"session_info","name":"我自己起的名字"}
{"type":"compaction","tokens_before":12345}
{"type":"custom","custom_type":"plan-mode","data":{"active":true}}
```

**会话头只在创建时写一次，之后永不重写。** 整份重写在断电时会把唯一真相写坏。
标题因此是**派生**的：改名条目优先，其次首条用户消息截断。

### 回放规则（`service/parser`）

1. **活动分支**：从最后一个条目沿 `parent_id` 走到根，反转成时间序。
   分叉时天然取叶子所在分支。环状引用由已访问集合兜住，绝不无限循环。
2. **轮次分组**：`message` 条目按角色分组——user 开新轮；assistant 成一轮
   （内容块：`text` → 文本块，`thinking` → 思考块，`toolCall` → 工具块）；
   `toolResult` 按 `tool_call_id` 挂回最近的工具块（填 output / is_error /
   差异文本），挂不上则落成独立工具轮；system 与 usage 不进历史视图。
3. **特殊条目**：`compaction` 转成引擎标记轮（显示压缩前 token 数）；
   `session_info` 的 `name` 作为会话标题；`custom` 转标记轮，
   但 `plan-mode` 这类开关不画进对话流——它不是用户说的话。
4. **健壮性**：坏行直接跳过（中途断电会留下残行，一行残行不该让整份历史读不出来）。
5. **隐藏台账**：`chat.hide:<会话路径>` 存设置台账；回放时剔除隐藏轮，
   会话文件本身不动。
6. **待办派生**：待办清单本身只在引擎内存里活着，但每次 `todo` 工具调用的结果
   `details.tasks` 都带全量表并落进会话文件——活动分支上最近一次成功调用
   就是当前待办。
7. **计划模式派生**：活动分支上最后一条 `plan-mode` 自定义条目就是当前状态。

### 内容块形状

消息正文有两种落法：纯文本走字符串，助手消息与工具结果走内容块数组。

```json
{"type":"message","message":{"role":"user","content":"问题原文"}}
{"type":"message","message":{"role":"assistant","content":[
  {"type":"text","text":"回答"},
  {"type":"thinking","thinking":"思考"},
  {"type":"toolCall","id":"call_1","name":"read","arguments":{"path":"a.txt"}}
]}}
{"type":"message","message":{"role":"tool","toolCallId":"call_1","toolName":"read",
  "content":[{"type":"text","text":"文件内容"}],"details":{"tidied":true}}}
```

`details` 跟着工具结果落盘：待办清单、折叠标记、差异文本都在里面，
回放层靠它把清单复原出来——只记正文的话界面还原不出清单。

## 4. SQLite 派生数据（`%LOCALAPPDATA%\WorkBaby\workbaby.db`）

七张表。**除服务商与模型外都是派生数据**：删掉可以从真相重建，
写入失败只降级检索与统计能力，不影响对话本身。

| 表 | 行类型 | 内容 |
|---|---|---|
| `provider` | `ProviderDO` | 服务商：接口地址、协议、认证方式、密钥原文、排序位 |
| `provider_model` | `ProviderModelDO` | 模型：所属服务商 + 模型 ID 复合主键、上下文窗口、输出上限、是否支持思考、输入种类 |
| `session_index` | `SessionIndexDO` | 会话索引：标题、路径、消息数、工具调用数、累计 token、时间戳 |
| `usage_record` | `UsageRecordDO` | 用量账：每轮回复的输入 / 输出 / 缓存 token 与时间 |
| `tool_call_record` | `ToolCallRecordDO` | 工具调用画像：工具名、是否失败、耗时 |
| `search_doc` | `SearchDocDO` | 检索文档：工作区、路径、种类、符号名、行号、正文摘要、命中次数 |
| `plan_state` | `PlanStateDO` | 计划模式状态：按会话记是否只读 |

**密钥原文只在本进程内流转**，界面读到的是掩码（`ApiKeySecret` 的 json tag 是 `-`，
不参与序列化）。`api_key` 传空串的语义是「保留原值」——
界面拿到的是掩码，当成新值存会把密钥写坏。

驱动用纯 Go 实现（`glebarez/sqlite`），不引 cgo。
外键与 WAL 都开：前者保证行不会指向不存在的东西，
后者让「读的同时后台在写索引」不互相阻塞。

## 5. MCP 清单（`data\mcp.json`）

| 作用域 | 文件 | 内容 |
|---|---|---|
| 用户级 | `data\mcp.json` | `{"mcpServers": {"<名>": {...}}}` |
| 项目级 | `<工作区>\.pi\mcp.json` | 同形；同名时项目条目覆盖用户条目 |

条目形状：

```json
{"mcpServers": {
  "filesystem": {
    "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem"],
    "env": {"KEY": "VALUE"}, "cwd": "C:\\work", "timeout": 30, "enabled": true,
    "description": "一句话说明"
  },
  "remote": {"url": "https://example.com/mcp", "enabled": true}
}}
```

`command` 与 `url` 互斥：stdio 类用命令行（拆成 `command` + `args`），
HTTP 类用 `url`。`enabled` 缺省视为启用。

删除只动界面显示的那一条：项目级遮蔽用户级时，不连带删除被遮蔽的用户级条目。

## 6. 技能目录

技能的身份是**目录位置**，不是配置项：

| 目录 | 状态 |
|---|---|
| `data\skills\<名字>\` | 启用 |
| `data\skills_disabled\<名字>\` | 停用 |
| `<工作区>\.pi\skills\<名字>\` | 项目技能（归那个仓库管，壳里的开关对它无效） |

开关 = 搬目录（`os.Rename`），真相是目录在哪个区。
`workbaby.json` 的 `skill_toggles` 只做记账与核对，不是真相。

随包原件在 `assets\skills\<名字>\`，装与重置都从那里复制。
目录内容只落在数据目录里——导入是复制不是引用，
源目录之后删改都不影响壳内副本。

技能的身份与「是否内置」由 `service.RecommendedSkills` 这张推荐清单定义：
它在 `assets\skills` 与磁盘清单之间做双向咬合（少一边就是死条目）。

## 7. 目录一览

```
%LOCALAPPDATA%\WorkBaby\          （开发态 -dev 后缀）
├── workbaby.json                 # 设置与技能开关台账
├── workbaby.db                   # SQLite：服务商 / 模型 / 索引 / 用量
├── cache\                        # 壳自己的缓存
├── logs\                         # 按天滚动
├── webview\                      # WebView2 用户数据
├── sessions\                     # 会话 JSONL（唯一持久真相）
└── skills\ skills_disabled\      # 技能启用 / 停用区
```

`webview\` 绑定的是这一次安装，卸载时无条件清掉（可再生）。
其余目录是否清空由用户在卸载询问里自己选，默认保留。
