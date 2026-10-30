# 数据模型 · DATA-MODEL

三处存储：会话 JSONL（真相源）、SQLite（索引与偏好）、JSON 配置（模型/MCP）。

## 会话 JSONL

条目类型与回放算法见 [SESSIONS.md](SESSIONS.md)。

- 目录：`{数据目录}\sessions\{编码后工作目录}\`，文件名 `{起建毫秒时间}_{SESS_ULID}.jsonl`
- 编码规则：工作目录绝对路径中 `\` `/` `:` 替换为 `-`
- 首行 header：`{"type":"session","version":1,"session_id":"SESS_…","created_at":毫秒,"workspace":"D:\demo"}`
- 其后每行一条 entry：`{"type":…,"entry_id":"ENT_…","parent_entry_id":…|null,"timestamp":毫秒,…}`

| entry type | 专有字段 | 说明 |
|---|---|---|
| message | message（role/user/assistant/toolResult/system） | 唯一进上下文的类型 |
| model_change | provider_id, model_id | 模型切换 |
| thinking_change | thinking_level | 思考档位变更 |
| compaction | summary, first_kept_entry_id, tokens_before, usage | 压缩记录 |
| usage | kind, provider_id, model_id, usage | 不进上下文的用量（重试剔除等） |
| custom | custom_type, data | 内置功能状态持久化（不进上下文） |
| context_edit | target_entry_id, replacement | 追加式修改早前条目（null=剔除） |
| session_info | session_name? | 会话显示名 |

message.role = user / assistant / toolResult / system：

- user：`content`（text | image 块数组）
- assistant：`content`（text/thinking/toolCall 块有序数组）+ `provider_id/model_id/usage/stop_reason/duration_ms`
- toolResult：`tool_call_id/tool_name/content/is_error/duration_ms`
- 内容块：text / thinking / image(base64) / toolCall(`{tool_call_id, name, arguments}`)
- Usage：`{input, output, cache_read, cache_write, reasoning, total, cost{...}}`

不变量：只追加；上下文 = leaf→root 回放；toolResult 紧随其 toolCall。

## SQLite 表

```sql
-- v1 全量
CREATE TABLE settings_kv  (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER);
CREATE TABLE session_index (
  session_id TEXT PRIMARY KEY,           -- SESS_…
  session_name TEXT, workspace TEXT,     -- 显示名 / 工作目录
  provider_id TEXT, model_id TEXT,       -- 当前模型
  message_count INTEGER,                 -- 消息数
  last_active_at INTEGER,                -- 文件 mtime 毫秒
  summary TEXT                           -- 列表预览（最后一条正文前 80 字）
);
CREATE TABLE usage_daily (
  day TEXT, provider_id TEXT, model_id TEXT, session_id TEXT,
  input_tokens INTEGER, output_tokens INTEGER, cache_read INTEGER, cache_write INTEGER,
  cost_usd REAL, PRIMARY KEY (day, provider_id, model_id, session_id)
);
CREATE TABLE feature_toggle (feature_code TEXT PRIMARY KEY, enabled INTEGER, config TEXT);
CREATE TABLE skill_toggle   (skill_name TEXT PRIMARY KEY, enabled INTEGER);
CREATE TABLE tool_toggle    (tool_name TEXT PRIMARY KEY, enabled INTEGER);
```

迁移：`PRAGMA user_version` 从 0 步进；只增不删。

## 功能自有存储

各内置功能在自己的目录下开独立库，不进主库：

| 功能 | 文件 | 内容 |
|---|---|---|
| memory | `features\{编码工作区}\memory.db` | 记忆正文表 + FTS5 外部内容虚表 |
| fff | `features\{编码工作区}\frecency.db` | frecency 与查询历史 |
| context_mode | `features\{编码工作区}\knowledge.db` | 文档表 + chunk 表 + FTS5 虚表 |
| todo / plan / goal | `features\{编码工作区}\*.json` | JSON 文件 |
| web | `features\{编码工作区}\web\` | 抓取结果缓存 |

主库管设置与索引，功能库管自己的生命周期与清理策略——两者互不干扰。

## JSON 配置

| 文件 | 内容 |
|---|---|
| `models.json` | providers 键值：`{name?, baseUrl?, api?, apiKey?, models:[{id,name?,reasoning?,contextWindow?,maxTokens?,cost?}]}`（camelCase，对齐前端服务商面板契约） |
| `auth.json` | `{providerId: {type:"api_key", key} | {type:"oauth", refresh, access, expires}}` |
| `mcp.json` | `{serverName: {transport:"stdio"|"http", command?, args?, env?, url?, headers?, enabled?, exposure?}}` |

camelCase 只在这三个外部契约文件出现（前端面板直接读写）；
HTTP 出入参统一 snake_case。
