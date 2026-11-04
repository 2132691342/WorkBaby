# 数据模型 · DATA-MODEL

三处存储，分工明确：**会话 JSONL 是对话真相，SQLite 是壳的索引与偏好，
JSON 配置是引擎的输入**。壳从不重写引擎的真相，只在其上做台账。

## 1. SQLite（workbaby.db）

better-sqlite3，WAL + 忙等 5s，进程内单连接。迁移手写，
`PRAGMA user_version` 步进，每步幂等（当前 v1）。

| 表 | 列 | 用途 |
|---|---|---|
| `settings_kv` | setting_key PK, setting_value | 壳的全部键值（见 §2） |
| `skill_toggle` | skill_name PK, skill_enabled | 技能开关意图（实际禁用靠搬目录，此表保证界面状态持久） |
| `plugin_toggle` | plugin_name PK, plugin_enabled | 插件开关意图（同上，权威在注册表 JSON 与目录实况） |
| `usage_daily` | usage_day, provider_name, model_id, session_id PK | 用量日账（tokens / cost），供用量页聚合 |

## 2. 设置键（settings_kv）

| 键 | 值 | 消费方 |
|---|---|---|
| `ui.theme` / `ui.reduce_motion` / `ui.font_scale` | 界面偏好 | 前端 |
| `prefs.restore_session` | `'1'/'0'` | 启动恢复上次会话 |
| `prefs.last_session_file` / `prefs.last_engine_kind` | 会话路径 / 引擎名 | 恢复目标 |
| `shell.close_action` | `'tray'/'quit'` | 关窗行为 |
| `engine.detected_at` | 毫秒时间戳 | 前端变化检测 |
| `workspace.current` / `workspace.history` | 路径 / JSON 数组（≤20 条） | 工作区状态 |
| `chat.hide:<会话路径>` | JSON hid 数组 | 单轮隐藏台账 |

## 3. 会话 JSONL（真相源）

位置：`pi/sessions/--<工作区编码>--/<时间戳>_<会话id>.jsonl`。
工作区编码规则：剥一个头部分隔符，`/ \ :` 换 `-`，两端包 `--`。
文件首行是会话头（`{type:'session', version:3, id, cwd}`），其后每行一个条目，
条目以 `id` / `parentId` 构成**树**（分支编辑不换文件）。

回放规则（`service/parser` + `session_service`）：

1. **活动分支**：从最后一个条目沿 `parentId` 走到根，反转成时间序——
   分叉时天然取叶子所在分支。
2. **轮次分组**：`message` 条目按角色分组——user 开新轮；assistant 成一轮
   （内容块：text → 文本块，thinking → 思考块，toolCall → 工具块）；
   `toolResult` 按 `toolCallId` 挂回最近的工具块（填 output / is_error /
   差异文本），挂不上则落成独立工具轮；system 与 usage 不进历史视图。
3. **特殊条目**：`compaction` 转成引擎标记轮（显示压缩前 token 数）；
   `session_info` 的 name 作为会话标题；`custom_message`(display) 转标记轮。
4. **隐藏台账**：`hid`（条目 id，稳定）存在 SQLite；回放时剔除隐藏轮，
   会话文件本身不动。
5. **用量累计**：assistant 的 usage、`usage` 条目、压缩 usage 全部计入
   会话行的 tokens / cost——会话清单与用量页都从这里来。

## 4. 引擎 JSON 配置（pi 的家）

| 文件 | 写入方 | 内容 |
|---|---|---|
| `settings.json` | 引擎自身与技能/扩展开关 | 默认模型 / 工具集 / 资源路径 |
| `models.json` | 模型页 | `{providers: {<id>: {baseUrl, api, models[]}}}` |
| `auth.json` | 模型页 | `{<provider>: {type:'api_key', key}}`，0600 |
| `mcp.json` | MCP 页 | `{mcpServers: {<名>: {command,args,env} | {url}}}`，`enabled:false` 即关 |
| 项目级 `.pi/mcp.json`、`.pi/skills/` | MCP 页 / 技能页 | 工作区作用域；项目条目同名覆盖用户条目 |

## 5. 壳的台账

| 文件 | 内容 |
|---|---|
| `pi/settings.json` 的 `packages[]` | 插件声明（唯一权威）：裸串 = 启用，`{source, autoload:false}` = 停用 |
| `pi/settings.json` 的 `npmCommand` | 内置 npm 的调用形状（`[node.exe, npm-cli.js]`） |
| `pi/npm/node_modules/` | 已装插件的真实目录，插件身份以包名为准 |
| `pi/skills_disabled/<name>` | 技能禁用区（目录整体搬入/搬出） |
| `skill_toggle` 表 | 技能开关意图（目录是事实，表保证界面状态持久） |

**插件不再有自定义注册表**：pi 的 `settings.json` 已经是唯一登记处，
再加一份 JSON 就一定会出现两份不同步。包是否“真的装上”直接看
`pi/npm/node_modules/<name>/package.json` 在不在。
