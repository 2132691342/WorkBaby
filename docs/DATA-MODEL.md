# 数据模型 · DATA-MODEL

三处存储：会话 JSONL（真相源）、SQLite（索引与偏好）、JSON 配置（模型/MCP）。

## 会话 JSONL

条目类型与回放算法见 [SESSIONS.md](SESSIONS.md)。

- 目录：`{数据目录}\sessions\{编码后工作目录}\`，文件名 `{起建毫秒时间}_{SESS_ULID}.jsonl`
- 编码规则：工作目录绝对路径中 `\` `/` `:` 替换为 `-`
- 首行 header：`{"type":"session","version":1,"session_id":"SESS_…","created_at":毫秒,"workspace":"D:\demo"}`
- 其后每行一条 entry：
  `{"type":…,"entry_id":"ENT_…","parent_entry_id":…|null,"timestamp":毫秒,"payload":{…}}`

| entry type | 专有字段 | 说明 |
|---|---|---|
| message | message（role=user/assistant/toolResult） | 唯一进上下文的类型 |
| model_change | provider_id, model_id | 模型切换 |
| thinking_change | thinking_level | 思考档位变更 |
| compaction | summary, first_kept_entry_id, tokens_before | 压缩记录 |
| usage | kind, usage | 不进上下文的用量 |
| custom | custom_type, data | 内置功能状态持久化（不进上下文） |
| context_edit | target_entry_id, replacement | 追加式修改早前条目（null=剔除） |
| session_info | session_name? | 会话显示名 |

message.role = user / assistant / toolResult：

- user：`content`（文本）+ `timestamp`
- assistant：`content`（text/thinking/toolCall 块有序数组）+ `provider_id/model_id/message_id/usage/stop_reason/timestamp/duration_ms`
- toolResult：`tool_call_id/tool_name/content/is_error/timestamp/duration_ms`
- 内容块：text / thinking / toolCall(`{tool_call_id, name, arguments}`)
- Usage：`{input, output, cache_read, cache_write, reasoning}`

不变量：只追加；上下文 = leaf→root 回放；toolResult 紧随其 toolCall。

## SQLite 主库

`{数据目录}\settings.db`，迁移用 `PRAGMA user_version` 从 0 步进到 1。

```sql
CREATE TABLE settings_kv (
  key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE session_index (
  session_id      TEXT PRIMARY KEY,
  session_name    TEXT NOT NULL DEFAULT '',
  session_workspace TEXT NOT NULL DEFAULT '',
  session_provider TEXT NOT NULL DEFAULT '',
  session_model   TEXT NOT NULL DEFAULT '',
  session_message_count INTEGER NOT NULL DEFAULT 0,
  session_last_active_at INTEGER NOT NULL DEFAULT 0,
  session_summary TEXT NOT NULL DEFAULT ''
);
CREATE TABLE usage_daily (
  day TEXT NOT NULL, provider_id TEXT NOT NULL, model_id TEXT NOT NULL, session_id TEXT NOT NULL,
  input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
  cache_read INTEGER NOT NULL DEFAULT 0, cache_write INTEGER NOT NULL DEFAULT 0,
  cost_usd REAL NOT NULL DEFAULT 0,
  PRIMARY KEY (day, provider_id, model_id, session_id)
);
CREATE TABLE feature_toggle (feature_code TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 0, config TEXT NOT NULL DEFAULT '{}');
CREATE TABLE skill_toggle   (skill_name   TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 1);
CREATE TABLE tool_toggle    (tool_name    TEXT PRIMARY KEY, enabled INTEGER NOT NULL DEFAULT 1);
```

连接参数：WAL 日志模式 + `busy_timeout` 5 秒。
开关表的「未记录」语义各不相同：功能开关未记录时由调用方给默认值，
技能与工具开关未记录即开。

## 功能自有存储

各内置功能在自己的目录下开独立库，不进主库：

| 功能 | 文件 | 内容 |
|---|---|---|
| memory | `features\{编码工作区}\memory.db` | 记忆正文表 + FTS5 外部内容虚表 + 同步触发器 |
| fff | `features\{编码工作区}\frecency.db` | frecency 与查询历史 |
| context_mode | `features\{编码工作区}\knowledge.db` | 文档表 + chunk 表 + FTS5 虚表 |
| todo / plan / goal | `features\{编码工作区}\*.json` | JSON 文件 |
| web | `features\{编码工作区}\web\` | 抓取结果缓存 |

主库管设置与索引，功能库管自己的生命周期与清理策略——两者互不干扰。

## JSON 配置

| 文件 | 内容 |
|---|---|
| `models.json` | providers 键值：`{name?, baseUrl?, api?, apiKey?, headers?, models:[{id,name?,reasoning?,vision?,contextWindow?,maxTokens?}]}`（camelCase） |
| `auth.json` | `{providerId: {type:"api_key", key} \| {type:"oauth", refresh, access, expires}}` |
| `mcp.json` | `{serverName: {transport:"stdio"\|"http", command?, args?, env?, url?, headers?, enabled?, exposure?, timeout?}}` |

camelCase 只在这三个外部契约文件出现（前端面板直接读写）；
HTTP 出入参统一 snake_case。
