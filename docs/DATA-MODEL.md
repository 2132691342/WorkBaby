# 数据模型 · DATA-MODEL

三处存储：会话 JSONL（真相源）、SQLite（索引与偏好）、JSON 配置（模型 / MCP）。
各自的分工在 [SESSIONS.md](SESSIONS.md) 与 [FEATURES.md](FEATURES.md) 有实现细节，
本文只列结构与不变量。

## 会话 JSONL

- 目录：`{数据目录}\sessions\{编码后工作目录}\`
- 文件名：`{起建毫秒时间}_{SESS_ULID}.jsonl`
- **叶指针**：`{同上}.jsonl.leaf`（当前分支终点，属于旁路而不属于树）
- 编码规则：工作目录绝对路径中 `\` `/` `:` 替换为 `-`
- 首行 header：`{"type":"session","version":1,"session_id":"SESS_…","created_at":毫秒,"workspace":"D:\demo"}`
- 其后每行一条 entry：`type` / `entry_id` / `parent_entry_id` / `timestamp` 公共字段，
  类型专有字段经 serde flatten 直接铺在行顶层：

```json
{"type":"message","entry_id":"ENT_…","parent_entry_id":…|null,"timestamp":毫秒,"message":{…}}
```

| entry type | 专有字段 | 进上下文 |
|---|---|---|
| `message` | message（role=user / assistant / tool_result） | 是 |
| `model_change` | provider_id, model_id | 否 |
| `thinking_change` | thinking_level | 否 |
| `compaction` | summary, first_kept_entry_id, tokens_before | 是（摘要形态） |
| `usage` | kind, usage | 否 |
| `custom` | custom_type, data | 否 |
| `context_edit` | target_entry_id, replacement | 间接（仅 null 形态） |
| `session_info` | session_name? | 否 |

`message` 三形态：

- **user**：`content`(文本) + `timestamp` + 可选 `skill` / `images[{mime, base64}]`
- **assistant**：`content`（text / thinking / tool_call 块有序数组）+
  `provider_id` / `model_id` / `message_id` / `usage` / `stop_reason` / `timestamp` / `duration_ms`
- **tool_result**：`tool_call_id` / `tool_name` / `content` / `is_error` / `timestamp` /
  `duration_ms` / 可选 `file_path` / `diff`

`Usage`：`{input, output, cache_read, cache_write, reasoning}`。

不变量：只追加；上下文 = leaf→root 回放；工具结果紧随其工具调用（缺失时回放补齐）。

## SQLite 主库

`{数据目录}\settings.db`，迁移用 `PRAGMA user_version` 从 0 步进到 1。

```sql
CREATE TABLE settings_kv (
  key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE session_index (
  session_id          TEXT PRIMARY KEY,
  session_name        TEXT NOT NULL DEFAULT '',
  session_workspace   TEXT NOT NULL DEFAULT '',
  session_provider    TEXT NOT NULL DEFAULT '',
  session_model       TEXT NOT NULL DEFAULT '',
  session_message_count INTEGER NOT NULL DEFAULT 0,
  session_last_active_at INTEGER NOT NULL DEFAULT 0,
  session_summary     TEXT NOT NULL DEFAULT ''
);
CREATE TABLE usage_daily (
  day TEXT NOT NULL, provider_id TEXT NOT NULL, model_id TEXT NOT NULL, session_id TEXT NOT NULL,
  input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
  cache_read   INTEGER NOT NULL DEFAULT 0, cache_write  INTEGER NOT NULL DEFAULT 0,
  cost_usd     REAL    NOT NULL DEFAULT 0,
  PRIMARY KEY (day, provider_id, model_id, session_id)
);
CREATE TABLE feature_toggle (feature_code TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 0, config TEXT NOT NULL DEFAULT '{}');
CREATE TABLE skill_toggle   (skill_name   TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 1);
CREATE TABLE tool_toggle    (tool_name    TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 1);
```

连接参数：WAL 日志模式 + `busy_timeout` 5 秒。

**开关表的「未记录」语义是设计出来的**：功能开关未记录时由调用方给默认值——
功能是装配边界，多一个默认开的功能就多一份常驻工具与命令，该显式选择；
技能与工具开关未记录即开——它们是能力清单，缺省可用才符合「装好就能干活」的预期，
关掉才需要一条记录。

`settings_kv` 里的键（全集）：

| 键 | 含义 |
|---|---|
| `workspace` | 当前工作区绝对路径 |
| `default_provider` / `default_model` | 默认模型（新建会话与启动引导都用它） |
| `close_action` | 关窗策略（`tray` 收托盘 / `quit` 直接退出） |
| `background_file` / `background_opacity` / `background_blur` | 自选背景图与它的两个参数 |
| `microapps` | 外部微前端应用册（整册序列化成一个字符串） |

## 功能自有存储

各内置功能在 `features\{编码工作区}\` 下开独立库，不进主库：

| 功能 | 文件 | 结构 |
|---|---|---|
| memory | `memory.db` | `memory` 表 + `memory_fts`（FTS5 外部内容虚表）+ 三条同步触发器（增 / 删 / 改）；会话检索用一次性 `line_fts` 表 |
| fff | `fff.db` | frecency 与查询历史 |
| context_mode | `knowledge.db` | 文档表 + chunk 表 + FTS5 虚表 |
| todo / plan / goal | `todo.json` / `plan.json` / `goal.json` | 单文件 JSON |
| 待答提问 | `pending_asks.json` | 待答卡片数组（带创建时间，超期不再弹） |

主库管设置与索引，功能库管自己的生命周期与清理策略——两者互不干扰。

## JSON 配置

| 文件 | 位置 | 内容 |
|---|---|---|
| `models.json` | 数据目录 | providers 键值：`{name?, baseUrl?, api?, apiKey?, headers?, models:[{id,name?,reasoning?,vision?,contextWindow?,maxTokens?}]}`（**camelCase**） |
| `auth.json` | 数据目录 | `{"providers": {providerId: {type:"api_key", key} \| {type:"o_auth", refresh, access, expires}}}` |
| `mcp.json` | 数据目录全局 + `{工作区}\.workbaby\mcp.json` 覆盖 | `{serverName: {transport:"stdio"\|"http", command?, args?, env?, url?, headers?, enabled?, exposure?, timeout?}}`，接受平铺与 `{"mcpServers": {...}}` 包裹两种形态 |

camelCase 规则只对 `models.json` 生效（前端面板直接读写）；`auth.json` 由后端经
`/providers/upsert`、`/providers/reveal` 读写；`mcp.json` 由面板提交原文、后端校验落盘。
HTTP 出入参统一 snake_case。

`auth.json` 的凭据带 `type` 判别标签，OAuth 变体的字面量是 `o_auth`（serde snake_case）。
手写成 `oauth` 会让整个文件解析失败（2001）——判别标签不认识就整份拒收，不做部分降级。

**密钥级联顺序**：取某个 provider 的实际密钥时，`auth.json` 里的 ApiKey 优先，
为空才展开 `models.json` 的 `$ENV` 字面量（取同名环境变量）。
这样 `models.json` 可以拷贝分享而不带密钥——密钥是本机隐私，配置是可迁移资产。

## 版本一致

版本号三处必须一致：根 `Cargo.toml` 的 `workspace.package.version`、
`src-tauri/Cargo.toml` 的 `version`、`src-tauri/tauri.conf.json` 的 `version`。
发布流水线会校验 tag 与 `tauri.conf.json` 的 version 一致（见 [DEPLOY.md](DEPLOY.md)）。
