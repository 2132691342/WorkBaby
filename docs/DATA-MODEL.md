# 数据模型

本节总览：应用有五类持久化真相，每一类只有一个载体、一个写入方。
排障时先问「这份数据属于哪一类」，再去看那一类的写入路径——
把不同类的数据互相抄写，必然出现两份互相漂移的事实。

| # | 真相 | 载体 | 写入方 | 消费者 |
|---|---|---|---|---|
| 1 | 会话 | `data\sessions\<工作区编码>\SES_<ULID>.jsonl` | `backend/agent/session.go` | `agent`（上下文组装）、`service/parser`、`service/sessions.go`、`session_html.go` |
| 2 | 设置台账 | `data\workbaby.json` | `backend/repo/store.go` | `service/settings.go`、`service/*` 各模块、`shell`（关窗偏好）、前端 `/bootstrap` |
| 3 | 服务商目录 | `data\workbaby.db` 的 `provider` / `provider_model` | `backend/repo/provider_repo.go`（经 `service/models.go`） | 模型页、对话页下拉、`Chat.resolveModel` |
| 4 | 能力配置 | `data\mcp.json`、`<工作区>\.pi\mcp.json`、`data\skills`、`data\skills_disabled`、`<工作区>\.pi\skills`、`data\templates\*.md` | `service/mcp.go`、`service/skills.go`、`service/templates.go` | `service/mcp_runtime.go`（装载工具）、技能页与斜杠命令、`chat_state.commands()` |
| 5 | 派生数据 | `data\workbaby.db` 的 `search_doc` + 进程内索引与 frecency | `service/searcher.go` | 索引页、`index_search` 工具 |

## 0.1 目录布局（`pkg.Layout`，`backend/pkg/paths.go`）

所有路径的唯一来源。装配时解析一次，之后全项目只读。
数据根是 `%LOCALAPPDATA%\WorkBaby`（开发态目录名带 `-dev` 后缀，避免与安装态互相污染）。

| 字段 | 实际路径 | 内容 |
|---|---|---|
| `DataDir` | `%LOCALAPPDATA%\WorkBaby` | 数据根 |
| `StorePath` | `data\workbaby.json` | 设置台账（§2） |
| `DatabasePath()` | `data\workbaby.db` | SQLite（§3） |
| `McpFile()` | `data\mcp.json` | 用户级 MCP 清单（§4） |
| `SessionsDir` | `data\sessions` | 会话 JSONL（§1） |
| `SkillsDir` | `data\skills` | 启用态技能区（§5） |
| `SkillsOffDir` | `data\skills_disabled` | 停用态技能区（§5） |
| `TemplatesDir` | `data\templates` | 提示词模板（§5） |
| `LogDir` | `data\logs` | 日志，按天滚动 |
| `AssetsDir` | 安装目录 `assets`（开发态为仓库根） | 随包资产；`assets\skills` 是推荐技能原件（§5） |

`data\webview` 不在 Layout 里：装配时由 `shell` 直接拼 `DataDir\webview`，
作为 WebView2 的用户数据目录，把内核缓存收进自己的数据根而不是 `%APPDATA%`。

---

## 1. 会话 JSONL

本节讲会话文件的行格式：条目的公共字段与每一类条目的专属字段、
为什么头只写一次、以及回放侧怎么把条目读成历史。

一行一条 JSON，只追加不回填。首行是会话头，之后全是条目。
落盘形态的唯一真相在 `backend/domain/session_file.go`：引擎写、回放读，两边共用这一份结构定义。

```json
{"type":"session","version":1,"id":"SES_01J...","timestamp":"2026-01-01T00:00:00.000Z","cwd":"D:\\proj","title":"可选"}
{"type":"message","id":"SES_01J...","parentId":"","timestamp":"2026-01-01T00:00:01.234Z","message":{"role":"user","content":[{"type":"text","text":"..."}]}}
{"type":"custom","id":"SES_...","parentId":"...","timestamp":"...","customType":"plan-mode","data":{"active":true}}
{"type":"compaction","id":"SES_...","parentId":"...","timestamp":"...","summary":"...","tokensBefore":41233}
```

为什么这样设计：

- **只追加**：断电最多丢最后一行；整份重写在写到一半时会把唯一真相写坏。
- **头只写一次**：标题因此由条目派生（改名条目优先，否则首条用户消息），不必重写文件。
  改名、压缩这类「会话的现在」都是活动分支上的一条条目，不是文件属性，
  所以重开应用、切回旧会话都能从条目还原出同一个状态。
- **时间形态**：会话文件里一律是 ISO 串、UTC 毫秒精度（`2006-01-02T15:04:05.000Z`，
  `pkg.SessionTimeFormat`）；毫秒 number 只出现在进程内 DTO，禁止进文件。
- **ID 形态**：会话与每条条目都用 `SES_<ULID>`（`pkg.NewID(pkg.PrefixSession)`）；
  条目 ID 是活动分支与隐藏台账共同引用的稳定光标。
- **唯一写入口**：`Session.append` 整段持锁——分配 ID、挂父指针、落一行、推进内存镜像与叶子指针。
  两路并发追加会写出交错行，那是整份会话读不出来的唯一原因。

### 1.1 会话头（`domain.SessionHeaderDO`）

| 字段 | JSON 键 | 说明 |
|---|---|---|
| `HeaderType` | `type` | 恒为 `session` |
| `HeaderVersion` | `version` | 格式版本，写 `1` |
| `HeaderId` | `id` | 会话 ID（`SES_<ULID>`，也是文件名主干） |
| `HeaderTimestamp` | `timestamp` | 创建时刻（ISO 串，UTC 毫秒） |
| `HeaderCwd` | `cwd` | 绑定的工作目录 |
| `HeaderTitle` | `title` | 可选；界面标题不依赖它（见上） |

文件位置：`SessionsDir\EncodedWorkspace(工作区)\`，工作区路径编码规则是
剥掉头部分隔符、`/ \ :` 与空格换成 `-`、两端包 `--`（`pkg.SessionDirOf`）。
引擎建目录与清单扫目录共用这一个函数。

### 1.2 条目公共字段（`domain.SessionEntryDO`）

| JSON 键 | 说明 |
|---|---|
| `type` | 条目种类，见 1.3 全表 |
| `id` | 条目 ID（`SES_<ULID>`） |
| `parentId` | 父条目 ID；分支树靠它串起来，回放按活动分支走 |
| `timestamp` | 条目时刻（ISO 串，UTC 毫秒） |

之后每条条目按 `type` 只填自己那一族的专属字段。

### 1.3 条目类型全表

| `type` | 专属字段（JSON 键） | 写入方 | 消费者 / 语义 |
|---|---|---|---|
| `message` | `message`（`SessionMessageDO`，见 1.4） | `Session.AppendMessage` | 引擎上下文、历史回放、摘要、导出、详情 |
| `session_info` | `name` | `Session.Rename` | 清单标题优先取它；历史回放跳过（元信息不进对话流） |
| `model_change` | `provider`、`modelId` | 只有类型定义与解析，没有写入方 | `parser.Summarize` 取 `session_model`；历史回放跳过 |
| `thinking_level_change` | `thinkingLevel` | 同上 | 历史回放跳过 |
| `usage` | `usage`（`SessionUsageDO`，见 1.6） | `Session.AppendUsage`（无调用方） | `parser.Summarize` 累加 `total_tokens`/`cost_usd` |
| `compaction` | `summary`、`tokensBefore` | `Session.AppendCompaction`（`Chat.compactNow` 调用） | `Runner.history()` 以落盘序最后一条带 `summary` 的为分界：之前折成一条摘要引导消息；`Session.ContextUsage` 遇压缩条目清零读数；`parser` 渲染成 `engine-mark` 分隔 |
| `custom` | `customType`、`data` | `Session.AppendCustom` | `customType="plan-mode"` 的 `data.active` 由 `parser.PlanModeFromEntries` 派生计划模式；其它 `custom` 取 `data.text` 渲染成 `engine-mark`，plan-mode 不画进对话流 |

### 1.4 消息体（`domain.SessionMessageDO`）

| JSON 键 | 类型 | 说明 |
|---|---|---|
| `role` | `user` / `assistant` / `toolResult` | 说话的人 |
| `content` | 字符串或内容块数组 | 正文；字符串形态读取时等价于单个 text 块 |
| `model` | string | 产生这条的模型 |
| `provider` | string | 产生这条的服务商 |
| `usage` | `SessionUsageDO` | 这条的用量（见 1.6） |
| `stopReason` | string | 停止原因；`error` 表示模型调用失败 |
| `errorMessage` | string | 失败原话 |
| `toolCallId` | string | 工具结果对应的调用 ID |
| `toolName` | string | 工具结果对应的工具名 |
| `isError` | bool | 工具结果是否失败 |
| `details` | 对象 | 工具结果的附加信息（见下） |

`details` 里被读取方认识的键：

| 键 | 谁写 | 谁读 |
|---|---|---|
| `tasks` | `todo` 工具每次返回全量清单 | `parser.TodosFromEntries`：活动分支上最近一次调用的全量表就是当前待办（`status="deleted"` 的行剔除） |
| `tidied`、`original_chars` | 输出折叠发生时（`Runner.runTools`） | 回放层据此提示「已折叠，展开看全文」；折叠前的原文不进上下文 |
| `diff`、`file_path` | 文件编辑类工具（`agent/tool_fs.go`） | 回放层取 `diff` 交 `parser.ParseDiff` 渲染成逐行差异 |

### 1.5 内容块（`domain.SessionBlockDO`）

一个块只填自己那一族的字段。

| JSON 键 | 属于哪些 `type` | 说明 |
|---|---|---|
| `type` | — | `text` / `thinking` / `toolCall` / `image` |
| `text` | `text` | 正文 |
| `thinking` | `thinking` | 思考文本 |
| `id` | `toolCall` | 调用 ID，结果消息用 `toolCallId` 回填到这个块 |
| `name` | `toolCall` | 工具名 |
| `arguments` | `toolCall` | 工具入参（任意 JSON） |
| `mime` | `image` | 图片 MIME |
| `data` | `image` | 图片内容（base64） |

### 1.6 用量（`domain.SessionUsageDO` / `domain.SessionCostDO`）

| JSON 键 | 说明 |
|---|---|
| `input` | 输入 token |
| `output` | 输出 token |
| `cacheRead` | 命中缓存的输入 token |
| `cacheWrite` | 写入缓存的输入 token |
| `totalTokens` | token 合计 |
| `cost.total` | 花费合计（美元） |

### 1.7 读取路径

| 入口 | 做什么 |
|---|---|
| `pkg.ReadSessionLines` | 引擎与回放层共用的读行函数：单文件最多读 64 MB，超限只留后半段并丢掉可能残缺的首行 |
| `parser.ReadEntries` | 逐行解析；空行、坏行、会话头跳过——一个断电残行不该让整份历史读不出来 |
| `parser.ActivePath` | 从最后一条沿 `parentId` 回溯到根再反转，得到活动分支；环状引用（坏文件）由 guard 集合兜住 |
| `parser.HistoryFromEntries` | 活动分支 → 历史轮：工具结果回填进产生调用的助手轮；压缩与其它 `custom` 变 `engine-mark`；元信息条目不进对话流 |
| `parser.GuardSession` | 路径必须存在且落在 `SessionsDir` 内，否则 5003（不存在）/ 5001（越界、非法） |
| `agent.Store.Open` | 校验路径后读条目进内存镜像；`Entries()` 返回副本；标题由条目派生 |
| `Session.ContextUsage` | 落盘序里最后一条带 `usage` 的消息（input+output+cacheRead 相加）就是读数，遇压缩条目清零；一条都没有时按字符数 ÷4 估算 |
| 隐藏轮 | 不改会话文件：`chat.hide:<会话路径>` 键存条目 ID 数组（§2），回放时剔除 |

---

## 2. 设置台账 `workbaby.json`

本节讲壳的全部开关与偏好：文件形状、每个键的类型/缺省/读写方，
以及原子整写的落盘纪律。

单个 JSON，启动整读进内存，变更时整写回文件。
值一律是字符串；数组、对象这类结构自己序列化成 JSON 字符串再存。

```json
{"settings":{"ui.theme":"dark","workspace.current":"D:\\proj"},"skill_toggles":{"pdf":false}}
```

### 2.1 文件形状（`backend/repo/store.go`）

| 顶层键 | 类型 | 内容 |
|---|---|---|
| `settings` | 对象（键→字符串） | §2.3 的全部键 |
| `skill_toggles` | 对象（技能目录名→布尔） | 技能开关台账，见 §5 |

### 2.2 落盘纪律

- 每次变更都是**整份序列化 + 原子写**（同目录临时文件 → 改名替换），不做局部改写。
- 所有读-改-写串行化在一把进程内互斥锁（`Store.mu`）后：
  两处并发保存若不串行，后写的那份会拿回改前的旧表，前一次改动凭空消失。
- 解析失败不堵启动：坏文件挪成 `workbaby.json.bad-<毫秒时间戳>` 留证，从空白重新开始。
  半截 JSON 多半来自断电或杀进程，留着只会让每次启动都失败。
- 文件不存在时首启就建起来：数据目录不可写在启动当刻暴露，而不是拖到第一次保存设置。

### 2.3 设置键全表（`backend/service/settings.go` 常量）

键名散在调用处迟早会出现「设置页写了 A，聊天读的是 B」，所以全部集中在常量里。

| 键 | 值类型 | 缺省 | 写方 | 读方 |
|---|---|---|---|---|
| `workspace.current` | 绝对路径 | 无 | `Settings.SetWorkspace` | `Settings.Workspace`；files/git/searcher/chat 的守卫；`agent.NewStore` 的 cwd 取值 |
| `workspace.history` | JSON 数组（`WorkspaceRowVO`，最新在前，≤24 条） | 未写时读方拿到空表 | `SetWorkspace` / `ForgetWorkspace` | `Settings.Workspaces`（工作区页、托盘菜单） |
| `prefs.last_session_file` | 会话文件路径 | 无 | Chat 绑定、新建、复制、分叉、切换会话时（`Settings.RememberSession`） | `Prefs` 回显；前端 `restoreLastSession` |
| `prefs.restore_session` | `'1'` / `'0'` | 未写视为关 | `Settings.SetRestoreSession` | `Settings.Prefs` → 前端决定是否恢复 |
| `prefs.close_action` | `tray` / `quit` | `tray` | `Settings.SetCloseAction` | `shell/window.go` 的关窗决策、`Prefs` |
| `prefs.tray_hint_shown` | `'1'` | 未写视为未提示 | `Settings.MarkTrayHintShown` | `App.TrayHintPending`（首次关窗询问） |
| `chat.hide:<会话文件路径>` | JSON 数组（条目 ID） | 无 | `Sessions.Hide` / 取消隐藏；会话删除时清键 | `Sessions.hiddenSet` / `History` |
| `model.active_provider` | 服务商名 | 无 | `Chat.SetModel`；`Models.DropActiveIf` 在对应模型被删时清掉 | `Chat.resolveModel`、`ChatStateRESP.model` |
| `model.active_model` | 模型 ID | 无 | 同上 | 同上 |
| `model.thinking_level` | `off`/`minimal`/`low`/`medium`/`high` | `medium`（`NewChat` 兜底；不在合法集合内也回到它） | `Chat.SetThinking` | `Chat.level` → `Runner.Thinking`；上游仅 `low/medium/high` 透传 `reasoning_effort` |
| `model.auto_compaction` | `'0'` / `'1'` | 未写视为开 | `Chat.SetAutoCompaction` | `Chat.autoCompactionDue`、`ChatStateRESP.auto_compaction` |
| `model.default_provider` | 服务商名 | 无（空串与没设过等价） | `Models.SaveDefaults`（空串=清掉该键）；`DropActiveIf` 在对应服务商被删时清掉 | `Models.Defaults`（`/models/defaults` 回显） |
| `model.default_model` | 模型 ID | 同上 | `SaveDefaults`；`DropActiveIf` 清理 | 同上 |
| `model.default_thinking_level` | 思考档 | 同上 | `SaveDefaults` | 同上 |
| `web.search_kind` | `duckduckgo`/`brave`/`exa`/`tavily` | 未写按 `duckduckgo` | `Web.SaveSettings`（只认合法枚举值，空值不写） | `Web.config`（`Status`/`Search` 与适配器重建都经它），每次调用重读 |
| `web.search_key` | 密钥原文 | 无 | `Web.SaveSettings`（空串语义是「保留原值」：界面拿到的是掩码，回传的必然不是原文） | 同上 |
| `ui.locale` | `zh-CN` / `en` | `zh-CN`（`EnsureDefaults` 写入） | `/settings/save`、`EnsureDefaults` | 前端 `applyLocale`、Element Plus 语言包 |
| `ui.font_scale` | 数值字符串 | `1`（`EnsureDefaults`） | 同上 | 前端 `applyFontScale`（夹在 0.8~1.35） |
| `ui.reduce_motion` | `'0'` / `'1'` | `0`（`EnsureDefaults`） | 同上 | 前端 `applyMotion` |
| `ui.mention_sessions` | 整数 `1`~`20` | `5`（`EnsureDefaults`） | 同上（`Settings.normalize` 在 `/settings/save` 入口收敛） | 前端 `MentionBrowser`、`AppearancePanel`；读取侧同样夹在 1~20 |

关于默认值与收敛：

- `EnsureDefaults` 只在缺键时写 `ui.*` 四项；用户改过的值一个字不动。
- `SaveSettings`（各模块内部调用）直接落库，不走 `normalize`；只有 `/settings/save` 收敛 `ui.mention_sessions`。
- `ui.theme`（`light`/`dark`）由前端直接读写，后端没有对应常量——`/settings/save` 把它当普通不透明字符串原样存下。

---

## 3. SQLite `workbaby.db`

本节讲库里的两张目录表与一张索引表：行映射、索引、谁在什么时机写，
以及「索引是派生数据」带来的读取纪律。

### 3.1 打开与建表

- 装配时 `repo.OpenDB` 打开（或创建）数据库并立即完成建表（`backend/repo/db.go` 的 `migrate`）。
- 驱动是纯 Go 的 SQLite 实现（无 cgo）：静态构建与交叉编译是这套交付的前提。
- 连接串固定开三个 pragma：`foreign_keys(1)`、`journal_mode(WAL)`、`busy_timeout(5000)`。
  外键保证行不会指向不存在的东西；WAL 让「读的同时后台在写索引」互不阻塞。
- GORM 日志丢弃：发布版没有控制台，逐条 SQL 会污染终端。
- 建表用 `AutoMigrate`：只增表、增列；删列这类破坏性变更要走手写迁移。
  `migrate` 登记的表就是下面三张，数据库里没有别的表。

### 3.2 表全表（行映射在 `backend/domain/store.go`，`repo` 层用同名别名）

| 表 | 主键 | 列 | 用途 |
|---|---|---|---|
| `provider` | `provider_name` | `base_url`,`api_protocol`,`auth_kind`,`api_key_secret`,`provider_sort`,`created_at_ms`,`updated_at_ms` | 服务商定义与凭据；`auth_kind` 恒为 `api_key`；`api_key_secret` 的 JSON 标签是 `-`，密钥只在进程内流转，不出接口 |
| `provider_model` | `provider_name`+`model_id` | `model_name`,`api_protocol`,`context_window`,`max_tokens`,`supports_reasoning`,`input_kinds`,`model_sort`,`created_at_ms` | 每个服务商下的模型清单；`input_kinds` 逗号分隔；对话页下拉与 `resolveModel` 的数据源 |
| `search_doc` | `doc_id` | `workspace_path`,`doc_path`,`doc_kind`,`doc_symbol`,`doc_symbol_kind`,`doc_line`,`doc_body`,`doc_size`,`doc_mtime_ms`,`doc_hit_count`,`doc_last_hit_ms` | 预索引的行映射；只存可检索字段，正文真相仍是工作区文件 |

`search_doc` 上声明的索引列：`workspace_path`、`doc_kind`、`doc_symbol`、`doc_mtime_ms`。
`doc_id` 的形状：文件行 `F:<相对路径>`，符号行 `S:<相对路径>::<符号名>`（`backend/index`）。

### 3.3 谁写、何时写

| 表 | 写方 | 时机 |
|---|---|---|
| `provider` / `provider_model` | `ProviderRepo` 的 `Upsert`/`Delete`/`UpsertModel`/`DeleteModel`，经 `service/models.go` | 模型页保存、删除；删服务商在同一事务里带走它的全部模型行 |
| `search_doc` | `SearchRepo.ReplaceWorkspace`（整工作区替换） | 每次重建索引：手动点重建，或切工作区后第一次检索触发惰性重建（`EnsureIndexed`） |
| `search_doc` 的 `doc_hit_count`/`doc_last_hit_ms` | `SearchRepo.BumpHit` | 每次检索命中后累加 |

### 3.4 `search_doc` 的读取纪律

- 服务层只从库里读一个数：`Count`（状态卡读数）。
- 检索走进程内 `docs` 切片与进程内 frecency：`Search` 不查表。
- 「已索引」判据是内存态（`indexedAt > 0` 且工作区一致），不跨进程重启。
  所以重启后第一次检索触发一次惰性重建；`doc_hit_count`/`doc_last_hit_ms` 是纯记账。
- 这一切成立的原因是索引是派生数据：随时可以从工作区文件整体重建，
  重建失败只影响检索速度，不影响任何功能。

### 3.5 数据库里没有什么

会话清单、用量账、工具调用账、计划模式状态都没有表。这四件事的真相都在 §1：

- 会话清单由扫 `data\sessions` 下的 JSONL 派生（`parser.Summarize`）。
- 用量在每条消息自己的 `usage` 字段上；调用账就是 `toolCall` 与 `toolResult` 消息。
- 计划模式在会话文件的 `custom` 条目里。

往 SQLite 抄一份副本，只会制造第二份会漂移的事实。

---

## 4. MCP 清单 `mcp.json`

本节讲两层级清单的形状、合并与遮蔽规则，以及引擎装载时认哪些字段。

### 4.1 层级

| 层 | 路径 | 写方 |
|---|---|---|
| 用户级 | `data\mcp.json`（`Layout.McpFile()`） | `Mcp.Save`/`Toggle`/`Remove`（默认目标） |
| 项目级 | `<工作区>\.pi\mcp.json` | 同一组方法，`scope="project"` 时；未选工作区则报错 |

形状：顶层 `mcpServers`，值是「服务名 → 条目对象」的映射。

```json
{"mcpServers":{"server-name":{"command":"npx","args":["-y","@x/y"],"cwd":"D:\\proj","env":{"KEY":"v"},"timeout":30,"enabled":true,"description":"一句话"}}}
```

### 4.2 条目字段

| JSON 键 | 类型 | 说明 |
|---|---|---|
| `command` | string | stdio 服务的可执行程序；引擎只装载带这个键的条目（见 4.4） |
| `args` | 字符串数组 | 命令行参数 |
| `url` | string | HTTP 服务地址；与 `command` 互斥 |
| `cwd` | string | 服务工作目录 |
| `env` | 对象（键→值） | 子进程环境变量 |
| `timeout` | number（秒） | 装载时换算成毫秒 |
| `enabled` | bool | 缺省（键不存在）视为启用；显式 `false` 才关 |
| `description` | string | 一句话说明 |
| 其它键 | 任意 | 用户手配（如 `headers`、`oauth`、`type`），表单不认识但必须保住（见 4.4） |

### 4.3 合并、遮蔽与去重

- 清单展示（`Mcp.List`）：先列未被遮蔽的用户级条目，再列全部项目级条目；
  同名条目**项目级覆盖用户级**（用户级那条直接不显示）。
- 开关、删除、查找（`Toggle`/`Remove`/`findEntry`）都按「项目级优先，其次用户级」定位，
  且只动找到的那一条：界面上删一行就少一条，磁盘上另一层的同名条目不受牵连。
- 引擎装载（`mcpRuntime.enabledEntries`）同样先用户级后项目级合并、同名只留项目级，
  最终按服务名排序，保证清单形状稳定。
- 单侧文件坏了不影响另一侧：读取错误按条目收进 `read_errors` 给界面看。

### 4.4 写入与装载纪律

- `Save`/`Toggle`/`Remove` 都持 `Mcp.mu` 串行化两个文件的读-改-写；写盘走原子整写（`pkg.WriteJSON`）。
- 编辑已有条目时**先整份继承旧条目再覆盖表单管得到的字段**：
  手配的 headers/oauth 不在表单里，从零重建会静默丢掉它们。
- 表单字段的空值语义是「清掉」而不是「保持不动」：描述、cwd、env、超时留空会删除对应键。
- 服务名只允许字母、数字、`_`、`-`——它是引擎拼工具名（`mcp__<服务>__<工具>`）的原料。
- 装载时只认 stdio：有 `command` 才建客户端，`url` 型条目不装载。
- 每次 `Rebuild`（配置变更后后台触发）先收掉全部旧进程、清空注册表 MCP 工具段，再按清单重装；
  单个服务失败只记 Warn，其它服务照常。

---

## 5. 技能与模板目录

本节讲技能的五个目录位置及相互搬移规则、开关台账的记账方式，
以及模板文件的形状。

### 5.1 目录

| 路径 | 语义 | 壳内可写 |
|---|---|---|
| `data\skills\<dir>\SKILL.md` | 启用态：引擎扫描得到 | 是（安装/导入/新建/编辑/搬移） |
| `data\skills_disabled\<dir>\SKILL.md` | 停用态：搬进来即关闭 | 是（搬移/删除） |
| `<工作区>\.pi\skills\<dir>\SKILL.md` | 项目技能：属于那个仓库 | 否（开关与删除都拒绝） |
| `<AssetsDir>\skills\<dir>` | 随包原件：10 条推荐技能的来源 | 只读 |
| `data\templates\<name>.md` | 提示词模板 | 是 |

识别口径：

- 技能的身份是**目录名**（`skill_dir_name`）；显示名取 `SKILL.md` YAML 头的 `name`，缺省用目录名。
- 扫描三个区（`skills_scan.scanDir`）后按 `skill_group` 再按 `skill_name` 排序；
  同名技能只留启用区那份（引擎真正看到的是它）。
- 来源四分（`skill_source`）：`builtin`（推荐清单登记的，可重置）、`bundled`（在 `assets\skills` 里但清单没登记）、
  `managed`（界面新建）、`project`（工作区 `.pi\skills`）。
- 目录名清洗（`sanitizeSkillName`）保留字母数字（含中文）与短横线，其余换成 `-`；新建、导入共用这套规则。

### 5.2 搬移规则

开关不用改配置，用搬目录：目录在哪个区就是什么状态，
对所有引擎版本都成立，且不需要引擎配合。

| 动作 | 做什么 | 台账记账 |
|---|---|---|
| 开 | `skills_disabled\<dir>` → `skills\<dir>`；若两边都有（破损态），先删启用区那份再搬 | `SetSkillEnabled(dir, true)` |
| 关 | `skills\<dir>` → `skills_disabled\<dir>`；若两边都有，先删禁用区那份再搬 | `SetSkillEnabled(dir, false)` |
| 安装 | 从 `assets\skills\<dir>` 复制进启用区；已存在则拒绝 | 不记 |
| 导入 | 外部目录（必须有 `SKILL.md`）复制进启用区，目录名清洗；两区有同名则拒绝 | 不记 |
| 新建 | 启用区建 `<dir>\SKILL.md`（YAML 头写 `name`/`description`） | 不记 |
| 删除 | 删整个技能目录（参数必须是 `SKILL.md` 路径，目录路径一律拒绝）；项目技能拒绝 | 记 `true`（目录已消失，台账回到默认为开的状态） |
| 重置 | 内置技能从随包原件复制回原位，保持它此前在哪个区 | 记原状态 |

- 台账 = `workbaby.json` 的 `skill_toggles`：键是技能目录名，值是最近一次开关结果。
  **开关的真相是目录在哪个区**，台账只做记账与核对，写入失败只记 Warn，不回滚目录搬移。
- 搬移与复制都怕「中途被杀留下半个技能」：复制先落 `<目标>.copying`（重置用 `<目标>.resetting`）
  临时目录、成功后删旧换新名；换名失败宁可保旧内容。
- 每次动作都以 `skill:status` 事件广播目录名，界面刷新清单。

### 5.3 模板

- 一份模板一个文件：`data\templates\<name>.md`，YAML 头字段 `name` / `description` / `args`，
  头用两行 `---` 包住，闭合行之后的全部内容都是正文。
- 模板名清洗后只能是**小写字母、数字、连字符**（`sanitizeTemplateName`），且不能是纯点；文件名 = 模板名 + `.md`。
- 列表扫描目录里的 `.md`：`name` 缺失时退回文件名主干；**坏文件跳过**，一份手改坏的模板不该拖垮全部。
- 保存原子整写；删除幂等（不存在不算错）。
- 模板同时是斜杠命令来源：`Commands` 把每份模板列成 `source=prompt` 的命令。

---

## 6. 落盘纪律

本节汇总：每类数据只有一个写入者，每类写入只允许一种落盘形态；
每条纪律后面写清理由和被违反的后果。

### 6.1 唯一写入者矩阵

| 对象 | 唯一写入者 | 串行化手段 |
|---|---|---|
| `data\workbaby.json` | `repo.Store` | `Store.mu` 一把锁罩住全部读-改-写 |
| `data\workbaby.db` | `repo.ProviderRepo` / `repo.SearchRepo`（经 service 层） | SQLite 连接 + WAL + `busy_timeout(5000)` |
| 会话 JSONL | `agent.Session.append` | `Session.mu`：ID、父指针、落行、内存镜像整段原子 |
| `mcp.json`（两级） | `service.Mcp` | `Mcp.mu` 罩住两个文件的读-改-写；引擎侧重建另有 `rebuildMu` 信号量 |
| 技能目录 | `service.Skills` | 目录搬移/复制本身原子（见 6.2） |
| `data\templates\*.md` | `service.Templates` | 每份文件独立，原子整写即可 |

各文件互不相同，各自的锁也互不共享：台账的 `Store.mu` 与 MCP 的 `Mcp.mu` 不是同一把，因为不存在共享的文件需要它们共同守卫。

### 6.2 落盘形态纪律

| 纪律 | 理由 | 违反后果 |
|---|---|---|
| 配置类文件只允许「临时文件 + 改名替换」原子整写（`pkg.WriteFileAtomic`），不允许原地截断重写 | 截断重写把「写一半」变成常态 | 断电或杀进程留下半截文件，唯一真相被写坏 |
| 会话文件只允许 O_APPEND 追加，一行一条，永不回填 | 追加的坏窗口只有最后一行 | 整份重写若中断，毁掉的是全部历史；回填会让分支指针与隐藏台账引用的条目 ID 失效 |
| 目录复制先落 `.copying` / `.resetting` 暂存目录，成功后删旧换新名 | 目标目录要么旧要么新，不存在「半个」 | 中途被杀留下残缺技能，且它再也回不到可用状态 |
| 读台账/清单坏文件：留证挪走或按空处理，不静默半解析 | 空白起点可恢复，坏状态每次启动都失败 | 启动被自己的配置文件卡死 |
| 时间形态按载体划分：会话文件用 ISO 串（UTC 毫秒），DTO 与库表用毫秒 number，互不串门 | 文件是跨进程读的真相，格式必须稳定；内存里比较与排序要便宜 | ISO 串进 DTO 让前端各自解析；毫秒进文件让两次写入格式不一致 |
| 派生数据（`search_doc`、进程内索引与 frecency）允许随时整体重建，不作为真相源 | 随时可从文件重建 | 把派生副本当真相，就又是一份会漂移的事实 |
