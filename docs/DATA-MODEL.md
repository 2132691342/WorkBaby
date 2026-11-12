# 数据模型

三处存储，分工明确：**会话 JSONL 是对话真相，workbaby.json 是壳的偏好与台账，
JSON 配置是引擎的输入**。壳从不重写引擎的真相，只在其上做台账。

## 1. 壳存储（`%LOCALAPPDATA%\WorkBaby\workbaby.json`）

单个 JSON 文件，启动整读进内存，每次变更原子整写（临时文件 + 换名），
进程内互斥锁串行化。条目量级是几十条，不值得引入数据库：

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

**为什么只有两段**：其余对象的真相都不在壳里。会话是 JSONL 文件，
插件是 `settings.json` 的 `packages[]`，MCP 是 `mcp.json`，模型是
`models.json`。把这些再抄进壳存储就多出第二份真相，而第二份一定会不同步。

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
| `workspace.current` / `workspace.history` | 路径 / JSON 数组（≤ 24 条） | 工作区状态 |
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
4. **隐藏台账**：`hid`（条目 id，稳定）存壳存储；回放时剔除隐藏轮，
   会话文件本身不动。
5. **用量累计**：assistant 的 `usage`、`usage` 条目、压缩 `usage` 全部计入
   会话行的 tokens / cost——会话清单与用量页都从这里来。
6. **待办派生**：todo 插件的清单只在内存里，但它每次 `todo` 工具调用的结果
   `details.tasks` 都带全量表落进会话文件——活动分支上最近一次成功调用就是
   当前待办（聊天页的待办面板由此回放，与插件自己的 replay 语义一致）。

## 4. 引擎 JSON 配置（pi 的家）

| 文件 | 写入方 | 内容 |
|---|---|---|
| `settings.json` | 壳 + 引擎自身 | 默认模型（`defaultProvider` / `defaultModel` / `defaultThinkingLevel`）/ `packages[]`；壳只写前三个键（用户点了保存才写），`npmCommand` 由后台预热清掉 |
| `models.json` | 模型页 | `{providers: {<id>: {baseUrl, api, models[]}}}` |
| `auth.json` | 模型页 | `{<provider>: {type:"api_key", key}}`，权限收紧 |
| `mcp.json` | MCP 页 | `{mcpServers: {<名>: {command,args,env} 或 {url}}}`，`enabled:false` 即关 |
| 工作区 `.pi\mcp.json`、`.pi\skills\` | MCP 页 / 技能页 | 工作区作用域；同名时项目条目覆盖用户条目 |

`auth.json` 里一律 `type: "api_key"`：本项目只用 API Key，不走 OAuth。

**读取一律走 `map[string]any`**：用户会手改这些文件，保留未知键比强类型更要紧。
读坏等同于「还没配过」，让界面重写一份比报错有用。

## 5. 壳的台账与事实源

| 文件 / 目录 | 地位 |
|---|---|
| `~/.pi/agent/settings.json` 的 `packages[]` | **插件的唯一权威**：裸串 = 启用，`{source, autoload:false}` = 停用 |
| `~/.pi/agent/npm\node_modules\` | 已装插件的真实目录；是否真装上以 `<包名>\package.json` 在不在为准 |
| `~/.pi/agent/skills\` / `skills_disabled\` | 技能启用 / 停用（目录整体搬移，pi 只扫前者） |
| `skill_toggle` 表 | 记录用户显式改过的开关；真相是技能目录在哪个区，台账写失败只记日志不阻断 |

**插件不另建注册表**：pi 的 `settings.json` 已经是唯一登记处，
再加一份 JSON 就一定会出现两份不同步。同理，`plugin_catalog.go` 里的
推荐清单只是「推荐哪些插件」的策展表（当前 6 个），**不是预置清单**：
壳不会在首启时往用户的 `settings.json` 里写任何东西，用户点了才装。
技能的 `skill_catalog.go` 同理，推荐技能同样只展示、不自动落位。

## 6. 目录一览

```
%LOCALAPPDATA%\WorkBaby\          （开发态 -dev 后缀）
├── workbaby.json                 # 壳的偏好与台账
├── cache\                        # 壳自己的缓存
├── logs\                         # 按天滚动
└── webview\                      # WebView2 用户数据

%USERPROFILE%\.pi\agent\          # 用户的 pi 家（或 PI_CODING_AGENT_DIR）
├── settings.json models.json auth.json mcp.json
├── npm\node_modules\             # 插件
├── sessions\                     # 会话 JSONL
└── skills\ skills_disabled\
```
