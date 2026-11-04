# 数据模型

三处存储，分工明确：**会话 JSONL 是对话真相，SQLite 是壳的偏好与台账，
JSON 配置是引擎的输入**。壳从不重写引擎的真相，只在其上做台账。

## 1. SQLite（`%LOCALAPPDATA%\WorkBaby\workbaby.db`）

gorm + `glebarez/sqlite`（纯 Go，无 cgo），WAL + `busy_timeout=5000` +
`foreign_keys=1`，进程内单连接。表结构由 gorm 迁移，当前两张表：

| 表 | 列 | 用途 |
|---|---|---|
| `setting` | `setting_key` PK, `setting_value`, `updated_at` | 壳的全部键值（见 §2） |
| `skill_toggle` | `skill_dir_name` PK, `skill_enabled`, `updated_at` | 技能开关台账 |

**为什么只有两张表**：其余对象的真相都不在壳里。会话是 JSONL 文件，
插件是 `settings.json` 的 `packages[]`，MCP 是 `mcp.json`，模型是
`models.json`。把这些再抄进 SQLite 就多出第二份真相，而第二份一定会不同步。

时间戳一律毫秒 int64（`pkg.NowMs()`），不存 ISO 串。

## 2. 设置键（`setting` 表）

| 键 | 值 | 消费方 |
|---|---|---|
| `ui.theme` / `ui.reduce_motion` / `ui.font_scale` | 界面偏好 | 前端 |
| `prefs.restore_session` | `'1'` / `'0'` | 启动恢复上次会话 |
| `prefs.last_session_file` | 会话文件路径 | 恢复目标 |
| `prefs.close_action` | `'tray'` / `'quit'` | 关窗行为 |
| `prefs.tray_hint_shown` | `'1'` / `'0'` | 「收进托盘」提示是否已解释过 |
| `workspace.current` / `workspace.history` | 路径 / JSON 数组（≤ 20 条） | 工作区状态 |
| `chat.hide:<会话路径>` | JSON 数组（条目 id） | 单轮隐藏台账 |

键值一律字符串，复杂结构自己编码 JSON——KV 表不做类型推断，
加一列不如加一个键。

## 3. 会话 JSONL（对话真相）

位置：`pi\sessions\--<工作区编码>--\<时间戳>_<会话id>.jsonl`

工作区编码规则：剥掉一个头部分隔符，把 `/` `\` `:` 换成 `-`，两端包 `--`。
首行是会话头（`{type:"session", version, id, cwd}`），其后每行一个条目，
条目以 `id` / `parentId` 构成**树**——分支编辑不换文件。

回放规则（`service/parser`）：

1. **活动分支**：从最后一个条目沿 `parentId` 走到根，反转成时间序。
   分叉时天然取叶子所在分支。
2. **轮次分组**：`message` 条目按角色分组——user 开新轮；assistant 成一轮
   （内容块：`text` → 文本块，`thinking` → 思考块，`toolCall` → 工具块）；
   `toolResult` 按 `toolCallId` 挂回最近的工具块（填 output / is_error /
   差异文本），挂不上则落成独立工具轮；system 与 usage 不进历史视图。
3. **特殊条目**：`compaction` 转成引擎标记轮（显示压缩前 token 数）；
   `session_info` 的 `name` 作为会话标题；`custom_message`(display) 转标记轮。
4. **隐藏台账**：`hid`（条目 id，稳定）存 SQLite；回放时剔除隐藏轮，
   会话文件本身不动。
5. **用量累计**：assistant 的 `usage`、`usage` 条目、压缩 `usage` 全部计入
   会话行的 tokens / cost——会话清单与用量页都从这里来。

## 4. 引擎 JSON 配置（pi 的家）

| 文件 | 写入方 | 内容 |
|---|---|---|
| `settings.json` | 壳 + 引擎自身 | 默认模型 / `packages[]` / `npmCommand` |
| `models.json` | 模型页 | `{providers: {<id>: {baseUrl, api, models[]}}}` |
| `auth.json` | 模型页 | `{<provider>: {type:"api_key", key}}`，权限收紧 |
| `mcp.json` | MCP 页 | `{mcpServers: {<名>: {command,args,env} 或 {url}}}`，`enabled:false` 即关 |
| 工作区 `.pi\mcp.json`、`.pi\skills\` | MCP 页 / 技能页 | 工作区作用域；同名时项目条目覆盖用户条目 |

**读取一律走 `map[string]any`**：用户会手改这些文件，保留未知键比强类型更要紧。
读坏等同于「还没配过」，让界面重写一份比报错有用。

## 5. 壳的台账与事实源

| 文件 / 目录 | 地位 |
|---|---|
| `pi\settings.json` 的 `packages[]` | **插件的唯一权威**：裸串 = 启用，`{source, autoload:false}` = 停用 |
| `pi\settings.json` 的 `npmCommand` | 内置 npm 的调用形状（`[node.exe, npm-cli.js]`），必须指向沙箱 |
| `pi\npm\node_modules\` | 已装插件的真实目录；是否真装上以 `<包名>\package.json` 在不在为准 |
| `pi\skills\` / `pi\skills_disabled\` | 技能启用 / 停用（目录整体搬移，pi 只扫前者） |
| `skill_toggle` 表 | 记「用户改过默认值没有」，重启后按它还原 |

**插件不另建注册表**：pi 的 `settings.json` 已经是唯一登记处，
再加一份 JSON 就一定会出现两份不同步。同理，`plugin_catalog.go` 里的
12 条内置清单只是「随包分发哪些、默认开哪些」的策展表，不是运行时状态。

## 6. 沙箱目录一览

```
%LOCALAPPDATA%\WorkBaby\          （开发态 -dev 后缀）
├── workbaby.db                   # 上面 §1 两张表
├── cache\{npm,npm-global}\       # npm 缓存与全局前缀
├── logs\                         # 按天滚动
├── webview\                      # WebView2 用户数据
├── runtime\{node,PortableGit,python}\
├── pi-app\node_modules\          # pi 本体
└── pi\                           # PI_CODING_AGENT_DIR
    ├── settings.json models.json auth.json mcp.json
    ├── npm\node_modules\         # 插件
    ├── sessions\                 # 会话 JSONL
    ├── skills\ skills_disabled\
```
