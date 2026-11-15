# 数据模型

五类真相，各有唯一载体与唯一写入方。排障时先问「这份数据属于哪一类」，
再去看那一类的写入路径——把不同类的数据互相抄写，必然出现两份互相漂移的事实。

| # | 真相 | 载体 | 写入方 | 消费者 |
|---|---|---|---|---|
| 1 | 会话 | `data\sessions\<工作区编码>\SES_<ULID>.jsonl` | `backend/agent/session.go` | `agent`（上下文组装）、`service/parser`、`service/sessions.go`、`session_html.go` |
| 2 | 设置台账 | `data\workbaby.json` | `backend/repo/store.go` | `service/settings.go`、`service/*` 各模块、`shell`（关窗偏好）、前端 `/bootstrap` |
| 3 | 服务商目录 | `data\workbaby.db` 的 `provider` / `provider_model` | `backend/repo/provider_repo.go`（经 `service/models.go`） | 模型页、对话页下拉、`Chat.resolveModel` |
| 4 | 能力配置 | `data\mcp.json`、`<工作区>\.pi\mcp.json`、`data\skills`、`data\skills_disabled`、`<工作区>\.pi\skills`、`data\templates\*.md` | `service/mcp.go`、`service/skills.go`、`service/templates.go` | `service/mcp_runtime.go`（装载工具）、技能页与斜杠命令、`chat_state.commands()` |
| 5 | 派生数据 | `data\workbaby.db` 的 `search_doc` + 进程内索引与 frecency | `service/searcher.go` | 索引页、`index_search` 工具 |

`pkg.Layout`（`backend/pkg/paths.go`）是这些路径的唯一来源：`DataDir`、`LogDir`、`CacheDir`、`SessionsDir`、
`SkillsDir`、`SkillsOffDir`、`TemplatesDir`、`StorePath`、`AssetsDir`，外加 `Dev` 布尔标记开发态。
开发态数据目录带 `-dev` 后缀。`CacheDir` 目前没有消费者——目录不会被创建，也不写东西。

---

## 1. 会话 JSONL

一行一条 JSON，只追加不回填。首行是会话头，之后全是条目。

```json
{"type":"session","version":1,"id":"SES_01J...","timestamp":"2026-01-01T00:00:00.000Z","cwd":"D:\\proj","title":"可选"}
{"type":"message","id":"SES_01J...","parentId":"","timestamp":"...","message":{"role":"user","content":[{"type":"text","text":"..."}]}}
{"type":"custom","id":"SES_...","parentId":"...","timestamp":"...","customType":"plan-mode","data":{"active":true}}
{"type":"compaction","id":"SES_...","parentId":"...","timestamp":"...","summary":"...","tokensBefore":41233}
```

**为什么只追加**：断电最多丢最后一行；整份重写在写到一半时会把唯一真相写坏。
**为什么头只写一次**：标题因此由条目派生（改名条目优先，否则首条用户消息），不必重写文件。

### 1.1 会话头（`domain.SessionHeaderDO`）

`type`（恒 `session`）、`version`（格式版本）、`id`、`timestamp`（ISO 串 UTC 毫秒）、`cwd`（绑定的工作目录）、`title`（可选）。

### 1.2 条目类型全表（`domain/session_file.go`）

条目公共字段：`type,id,parentId,timestamp`。`parentId` 串起分支，回放按活动分支走。

| `type` | 专属字段 | 写入方 | 消费者 / 语义 |
|---|---|---|---|
| `message` | `message`（`SessionMessageDO`） | `Session.AppendMessage` | 引擎上下文、历史回放、摘要、导出、详情 |
| `session_info` | `name` | `Session.Rename` | `parser.Summarize` 取标题；`parser.BuildHistory` 跳过（元信息不进对话流） |
| `model_change` | `provider,modelId` | 只有类型定义与解析，没有写入方 | `parser.Summarize` 取 `session_model`；历史回放跳过 |
| `thinking_level_change` | `thinkingLevel` | 同上 | 历史回放跳过 |
| `usage` | `usage`（`SessionUsageDO`） | `Session.AppendUsage`（当前无调用方） | `parser.Summarize` 累加 `total_tokens`/`cost_usd` |
| `compaction` | `summary,tokensBefore` | `Session.AppendCompaction`（`Chat.compactNow` 调用） | `Runner.history()` 以最后一条带 `summary` 的为准折摘要；`Session.ContextUsage` 遇压缩条目清零读数；`parser.BuildHistory` 渲染成 `engine-mark` 分隔 |
| `custom` | `customType,data` | `Session.AppendCustom` | `customType="plan-mode"` 的 `data.active` 由 `parser.PlanModeFromEntries` 派生计划模式；其它 `custom` 在历史里渲染成 `engine-mark` |

`SessionMessageDO`：`role(user|assistant|toolResult)、content(字符串或内容块数组)、model、provider、usage、stopReason、errorMessage、toolCallId、toolName、isError、details`。
`details` 是工具结果的附加信息（diff 文本、待办 `tasks`、`tidied`/`original_chars` 折叠标记）。
`SessionUsageDO`：`input,output,cacheRead,cacheWrite,totalTokens,cost{total}`。
`SessionBlockDO`：`type(text|thinking|toolCall|image),text,thinking,id,name,arguments,mime,data`。

### 1.3 读取路径

- `agent.Store.Open` → 引擎当前会话；`Session.Entries()` 返回条目副本；`ContextUsage()` 优先用活动分支上最后一条带 `usage` 的助手消息，无则按字符数 /4 估算。
- `parser.ReadEntries` + `parser.ActivePath`：从叶子沿 `parentId` 回溯活动分支。
- `parser.GuardSession`：解析出的路径必须落在 `SessionsDir` 内，越界给 5001/5003。
- 隐藏轮不改文件：`chat.hide:<会话路径>` 键存条目 ID 数组，回放时剔除。

---

## 2. 设置台账 `workbaby.json`

单个 JSON，启动整读进内存，变更原子整写（临时文件 + 换名），进程内互斥锁串行化。
解析失败时挪到 `workbaby.json.bad-<ts>` 留证并从空白开始——半截 JSON 多半来自断电。

```json
{"settings":{"ui.theme":"dark"},"skill_toggles":{"pdf":false}}
```

`skill_toggles` 按技能目录名记开关结果；**开关的真相是目录在哪个区**，这份台账只做记账与核对，
写入失败只 Warn，不回滚目录搬移。

### 2.1 设置键全表（`service/settings.go` 常量）

| 键 | 值类型 | 缺省 | 消费方 |
|---|---|---|---|
| `workspace.current` | 绝对路径 | 无 | `Settings.Workspace`、files/git/searcher/chat 的守卫、`agent.NewStore` 的 cwd 取值 |
| `workspace.history` | JSON 数组（`WorkspaceRowVO`，最新在前，≤24） | `[]` | `Settings.Workspaces`（工作区页、托盘菜单） |
| `prefs.last_session_file` | 会话文件路径 | 无 | `Prefs` 回显；前端 `restoreLastSession`；`Chat.BindSession` 写 |
| `prefs.restore_session` | `'1'` / `'0'` | 未写视为关 | `Settings.Prefs` → 前端决定是否恢复 |
| `prefs.close_action` | `tray` / `quit` | `tray` | `shell/window.go` 的关窗决策、`Prefs` |
| `prefs.tray_hint_shown` | `'1'` | 未写视为未提示 | `App.TrayHintPending`（首次关窗询问） |
| `chat.hide:<会话文件路径>` | JSON 数组（条目 ID） | 无 | `Sessions.hiddenSet` / `History` |
| `model.active_provider` | 服务商名 | 无 | `Chat.resolveModel`、`ChatStateRESP.model` |
| `model.active_model` | 模型 ID | 无 | 同上 |
| `model.thinking_level` | `off`/`minimal`/`low`/`medium`/`high` | `medium`（`NewChat` 兜底） | `Chat.level` → `Runner.Thinking` → 上游 `reasoning_effort` |
| `model.auto_compaction` | `'0'` / `'1'` | 未写视为开 | `Chat.autoCompactionDue`、`ChatStateRESP.auto_compaction` |
| `model.default_provider` | 服务商名 | 无 | 仅 `/models/defaults` 读写与 `DropActiveIf` 清理 |
| `model.default_model` | 模型 ID | 无 | 同上 |
| `model.default_thinking_level` | 思考档 | 无 | 同上 |
| `web.search_kind` | `duckduckgo`/`brave`/`exa`/`tavily` | `duckduckgo` | `Web.config`（含 `Status`/`Search`） |
| `web.search_key` | 密钥原文 | 无 | 同上 |
| `ui.locale` | `zh-CN` / `en` | `zh-CN`（`EnsureDefaults` 写入） | 前端 `applyLocale`、Element Plus 语言包 |
| `ui.font_scale` | 数值字符串 | `1`（`EnsureDefaults`） | 前端 `applyFontScale`（夹在 0.8~1.35） |
| `ui.reduce_motion` | `'0'` / `'1'` | `0`（`EnsureDefaults`） | 前端 `applyMotion` |
| `ui.mention_sessions` | 整数 `1`~`20` | `5`（`EnsureDefaults`） | 前端 `MentionBrowser`（条数）、`AppearancePanel`（同一项）；`Settings.normalize` 在保存入口收敛 |

`EnsureDefaults` 只在缺键时写 `ui.*` 四项；用户改过的值一个字不动。
`SaveSettings`（供各模块调用）直接落库，不走 `normalize`；只有 `Settings.Save`（`/settings/save`）收敛 `ui.mention_sessions`。

**仅前端维护的键**：`ui.theme`（`light`/`dark`）由前端 `stores/app.ts`、`CommandPalette`、`AppearancePanel` 直接读写，
后端没有对应常量——`/settings/save` 把它当普通不透明字符串原样存下，不认识它的语义。

---

## 3. SQLite `workbaby.db`

`repo.OpenDB`：纯 Go 驱动 `glebarez/sqlite`（无 cgo），`_pragma=foreign_keys(1)&journal_mode(WAL)&busy_timeout(5000)`，
GORM 日志丢弃（发布版没有控制台）。`AutoMigrate` 只增不改，删列属破坏性变更要走手写迁移。

### 3.1 活表

| 表 | 主键 | 字段 | 用途 |
|---|---|---|---|
| `provider` | `provider_name` | `base_url,api_protocol,auth_kind,api_key_secret,provider_sort,created_at_ms,updated_at_ms` | 服务商定义与凭据。`api_key_secret` 的 json tag 是 `-`，不出接口 |
| `provider_model` | `provider_name` + `model_id` | `model_name,api_protocol,context_window,max_tokens,supports_reasoning,input_kinds（逗号分隔）,model_sort,created_at_ms` | 每个服务商下的模型清单，对话页下拉与 `resolveModel` 的数据源 |
| `search_doc` | `doc_id` | `workspace_path,doc_path,doc_kind,doc_symbol,doc_symbol_kind,doc_line,doc_body,doc_size,doc_mtime_ms,doc_hit_count,doc_last_hit_ms` | 预索引的行映射 |

### 3.2 `search_doc` 的真实状态

`service/searcher.go` 只对它调三个方法：`ReplaceWorkspace`（重建时整工作区替换）、`BumpHit`（命中后累加计数）、`Count`（状态卡读数）。
**没有任何读回路径**：`Search` 用进程内 `docs` 切片，`EnsureIndexed` 判据是 `indexedAt > 0 && workspace 与当前一致`。
所以进程重启后内存为空，第一次检索会触发一次惰性重建——索引与 frecency 不跨重启生效，
`doc_hit_count`/`doc_last_hit_ms` 写进去也没有读者。`SearchRepo.Search`/`Clear` 无调用方。

### 3.3 数据库只有三张活表

`repo/db.go` 的 `migrate` 只登记上面三张表。会话索引、用量账、工具调用账、计划状态都没有对应的表：
会话正文的真相在 JSONL，用量的真相在每条消息的 `usage` 上，计划模式的真相在会话文件的 `custom` 条目里。
`AutoMigrate` 只增不改，所以老库里若残留了同名的历史表，它们不再被读写，也不影响功能。

---

## 4. MCP 清单

`data\mcp.json`（用户级）与 `<工作区>\.pi\mcp.json`（项目级），形状：

```json
{"mcpServers":{"server-name":{"command":"npx","args":["-y","@x/y"],"cwd":"D:\\proj","env":{"KEY":"v"},"timeout":30,"enabled":true,"description":"一句话"}}}
```

- 条目字段：`command`、`args[]`、`cwd`、`env{}`、`timeout`（秒）、`url`、`enabled`、`description`，以及用户手配的其它键。
- 合并语义：项目级同名覆盖用户级；`enabled` 缺省视为启用；`List` 的 `read_errors` 收集坏文件原因，单侧损坏不影响另一侧。
- 写入：`Mcp.Save` 先整份继承旧条目再覆盖表单管得到的字段（手配 headers/oauth 不丢），服务名只允许字母数字 `_` `-`。
- 引擎装载：`mcp_runtime.enabledEntries` 只取有 `command` 的条目（stdio），`url` 型不装载；
  每次 `Rebuild` 先收掉全部旧进程再按清单重装，单个服务失败只 Warn。

---

## 5. 技能与模板目录

| 目录 | 语义 | 可写 | 谁扫描 |
|---|---|---|---|
| `data\skills\<dir>\SKILL.md` | 启用态（引擎看得到） | 是（安装/导入/新建/编辑） | `skills_scan.scanDir(..., true)` |
| `data\skills_disabled\<dir>\SKILL.md` | 停用态（搬过来即关闭） | 是（开关/重置） | `skills_scan.scanDir(..., false)` |
| `<工作区>\.pi\skills\<dir>\SKILL.md` | 项目技能（属于那个仓库） | 否（开关与删除都拒） | `skills_scan` 的 `projectDir` |
| `<assets>\skills\<dir>` | 随包原件（10 条推荐技能的来源） | 否（只读） | `Skills.builtinSource`、`Install`/`Reset` |
| `data\templates\<name>.md` | 提示词模板，YAML 头 `name/description/args` | 是 | `Templates.List` |

识别口径：技能的身份是**目录名**（`skill_dir_name`），列表按 `skill_group` 再按 `skill_name` 排序，
同名技能只留启用区那份；模板的身份是文件名清洗后的 `template_name`（小写字母/数字/连字符），坏文件跳过。

---

## 6. 落盘纪律

- 所有写入走 `pkg.WriteFileAtomic`（临时文件 + 换名）或 O_APPEND 追加；不原地截断重写。
- 技能目录复制走 `copyTreeAtomic`：先落 `<target>.copying` 再换名，中途被杀不留半个技能。
- 会话条目只追加；引擎持有 `Session.mu` 串行化写入（并发追加会交出半行）。
- 台账与 MCP 清单的读-改-写各自持有进程内互斥锁（`Store.mu`、`Mcp.mu`）；两者不共锁，因为它们不共享文件。
