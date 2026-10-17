# 域模型与持久化

状态落在**两个地方**，分工严格：

| 数据 | 位置 | 谁写 |
|---|---|---|
| 消息、工具结果、压缩摘要、分支树 | PI 的 JSONL（`{数据目录}/pi-agent/sessions/**`） | **只有 PI** |
| 其余一切（WorkBaby 的账） | `workbaby.db`（SQLite） | 只有 WorkBaby |

跨边界字段形态只有一种：主键是带前缀的 ULID 字符串，时间是毫秒整数。

**绝不写 PI 的 JSONL**：格式是 PI 的私有契约，写坏它 PI 自己都读不回来。

## 1. 约定

| 约定 | 内容 |
|---|---|
| 表与 DO | 一个聚合根一个 `domain/*.go`，DO 必须显式 `TableName()`——GORM 会把 `XxxDO` 复数化成 `xxx_dos` |
| 主键 | ULID + 领域前缀（`SESSION_` / `RUN_` 等），生成见 `domain/id.go` |
| 时间戳 | `CreatedAt` / `UpdatedAt` 一律毫秒整数（`autoCreateTime:milli`）；跨边界 struct 里不出现 `time.Time` |
| JSON tag | 一律 snake_case；前端 `types/api.ts` 的字段名必须与后端 json tag 完全一致 |
| 迁移 | GORM AutoMigrate：只增不删、零值兜底；FTS5 虚表与触发器用 raw SQL 逐条创建 |
| 连接 | `pool.SetMaxOpenConns(1)`，单连接 + WAL 让写行为完全可预测 |

连接期 PRAGMA：WAL（读写并发）、`busy_timeout=5000`（规避瞬时写锁）、
`synchronous=NORMAL`、`foreign_keys=ON`。

**WAL 还让第二个进程能读**：知识库的 MCP server（`--mcp-knowledge`）是独立进程，
它打开同一个库文件读 FTS5 索引。

## 2. 表一览

| 表 | DO | 用途 |
|---|---|---|
| `sessions` | `SessionDO` | 会话元数据 + 指向 PI 会话的指针 |
| `approvals` | `ApprovalDO` | 审批卡（跨进程重启仍有条） |
| `providers` | `ProviderDO` | 模型服务配置，密钥 AES-GCM 密文 |
| `model_configs` | `ModelConfigDO` | 按「服务 + 模型」的覆写配置 |
| `knowledge_docs` / `knowledge_chunks` | `KnowledgeDocDO` / `KnowledgeChunkDO` | 知识库文档与切片 |
| `knowledge_chunks_fts` | （FTS5 虚表） | 全文检索，靠触发器与切片同步 |
| `settings` | `SettingDO` | 键值设置全集 |
| `token_usages` | `TokenUsageDO` | 每次模型调用一行用量 |
| `runs` | `RunDO` | 一次 run 一行账 |
| `context_snapshots` | `ContextSnapshotDO` | 每轮一张水位快照 |
| `checkpoints` | `CheckpointDO` | 每次 run 起点打一个点 |
| `orchestra_evals` | `EvalDO` | 评估记录（specs/19） |
| `orchestra_baselines` | `BaselineDO` | 回归基线 |
| `memories` | `MemoryDO` | 长期记忆（specs/22） |

**已删除**：`entries`。消息真身在 PI 的 JSONL 里，WorkBaby 不再持有条目链。
只读投影见 §3。

MCP 配置不是表：它是 `<agent-dir>/mcp.json`（specs/16）。

### sessions

```go
ID            string // ULID，WorkBaby 自己的 id，界面与路由用它
PiSessionID   string // PI 的 uuid
PiSessionFile string // JSONL 绝对路径
Title         string
Workspace     string
Model         string // "provider/modelId"
ThinkingLevel string
Permission    string // auto / ask_write / readonly
AllowedTools  string // JSON 数组：本会话「总是允许」的工具（specs/27）
Hidden        bool   // 子代理的载体，不进列表
MessageCount  int    // 从 get_session_stats 同步
TotalTokens   int
CreatedAt     int64
UpdatedAt     int64
```

- `ID` 与 `PiSessionID` 分开：JSONL 文件名里的 uuid 要 `new_session` 之后才有，
  不能做提前标识
- `Permission` 是本会话的审批档，写进 `workbaby-policy.json` 供扩展读取（specs/27）
- `hidden=true` 是子代理的载体会话，不进列表但可被直接打开

### 为什么没有 entries 表

保留它会得到两份消息副本，而两份必然分叉：

| 问题 | 后果 |
|---|---|
| PI 压缩了上下文 | WorkBaby 那份没有压缩摘要，投影出的历史与模型实际看到的不一致 |
| PI 的 `context_edit` 剔除了一条 | WorkBaby 仍显示它生效 |
| 分支树 | PI 的 `id`/`parentId` 才是真树，复制一份要自己维护同一套树操作 |

所以 WorkBaby **只读投影**：打开会话时读 JSONL，投影成消息视图。
列表页不读 JSONL（进列表读一百个文件就是一百次 IO），只读 SQLite 元数据。

## 3. JSONL 投影

`GET /sessions/:id` 读文件、按行解析、投影：

| JSONL entry | 出参 |
|---|---|
| `session` | 不输出（头信息进 SessionVO） |
| `message`（user / assistant / toolResult） | 一条 MessageVO |
| `compaction` | 「已压缩」提示条 + summary 正文 |
| `branch_summary` | 「分支摘要」提示条 |
| `custom_message` | 一条 MessageVO（`display=false` 标记为隐藏） |
| `model_change` / `thinking_level_change` | 时间线锚点提示条 |
| `usage` / `custom` / `label` / `session_info` / `context_edit` | 不出消息 |

**不做上下文重建**：WorkBaby 只显示"文件里有什么"，不决定"模型看到什么"。
后者是 PI 的职责，重建错了会让界面与模型的实际输入不一致。

### 消息内容块的形态

`AssistantMessage.content` 是数组，元素有三种：

| 元素 | 映射 |
|---|---|
| `{type:"text", text}` | 正文 |
| `{type:"thinking", thinking}` | 思考（前端折叠展示） |
| `{type:"toolCall", id, name, arguments}` | 工具卡（参数） |

`ToolResultMessage` 的 `content` 同为数组（`text` / `image`），
`isError` 进卡片状态，`details` 保留原样（前端按工具名决定怎么渲染）。

### model_configs

复合主键 `(provider_id, model)`。目录认不出的私有模型靠它闭环。
这些值同时写进 PI 的 `models.json`（specs/13），**WorkBaby 是唯一写入者**：

| 列 | 语义 |
|---|---|
| `context_window` | 0 = 跟随内置目录；>0 即覆写 |
| `max_output` | 0 = 按窗口派生；>0 即覆写 |
| `tool_call` / `vision` | 布尔覆写 |
| `thinking` | 布尔覆写 |
| `temperature` / `top_p` | `SamplingUnset`（-1）表示未设置 |

### knowledge_chunks + FTS5

```sql
CREATE VIRTUAL TABLE knowledge_chunks_fts
USING fts5(content, doc_id UNINDEXED, chunk_id UNINDEXED, tokenize='trigram')
```

三个触发器（AFTER INSERT / DELETE / UPDATE）把 `knowledge_chunks.content` 同步进虚表。
`trigram` 是硬要求：默认 `unicode61` 不按字切分 CJK，整句中文会退化成一个 token。

### 观测域表（specs/23）

```
runs                id / session_id / provider_id / model / status / stop_reason / turns
                    / tokens / input / output / cached / cost / context / max_tokens
                    / compressed / started_at / finished_at
context_snapshots   id / run_id / session_id / turn / used / window / compressed / created_at
checkpoints         id / session_id / run_id / pi_entry_id / turn / prompt / created_at
```

- `runs` 不存正文：正文在 PI 的 JSONL 里，这张表只记账与位点
- `runs.status`（running / done / failed / aborted）与 `stop_reason`
  （completed / length / toolUse / aborted / error）分开：合并就分不清
  「被用户停掉」与「正常说完」
- 进程退出时残留的 `running` 行在启动时收口成 `aborted`（`reason=interrupted`）：
  执行句柄是内存态，重启后没人办得完
- `checkpoints.pi_entry_id` 是那一轮起点的 PI 条目 id。回到它是 `fork`（新建一条 JSONL），
  **不删任何内容**
- `context_snapshots` 每轮一张，回答「第几轮涨到多少、压过几次」
- 隐藏会话按 `orchestra_retention_days` 归档：删 WorkBaby 的会话行 + PI 的 JSONL 文件，
  **不删 `token_usages`**——用量账与评估指标跨会话存活

## 4. 形态命名

同一实体的不同形态同居一个文件，便于对照：

| 后缀 | 语义 | 出现位置 |
|---|---|---|
| DO | 数据库行形态 | 只在 repo 层进出 |
| REQ | 前端入参 | server → api → service |
| VO | 组装后的视图对象 | service 出口 |
| RESP | 接口出参 | api → server |

`server / api / service / repo` 四层禁止就地声明这些结构体。
DO 永远不直接出：`ProviderDO.APIKeyEnc` 的 tag 是 `json:"-"`，出参经 `ProviderVO` 换成 `has_key`。

## 5. 设置的双轨

静态默认（`domain.DefaultSettings`，首启写入）+ 运行时 KV（`settings` 表）。
区分依据：会不会被用户在设置页改。

**但要分清存放位置**（见 specs/13）：

| 设置 | 存在哪 |
|---|---|
| 主题、字号、背景图、关闭行为 | SQLite KV |
| 默认模型、思考档、`defaultTools`、压缩参数 | PI `settings.json` |
| Provider / 模型 / Key | PI `models.json` |
| MCP server | PI `mcp.json` |

握手时 `/bootstrap` 把设置全集一次性下发给前端，前端不在渲染时逐个拉单个键。

## 6. 已知代价

- 删会话要两步（SQLite 行 + JSONL 文件），任何一步失败都靠下次启动的对账扫尾
- 单连接写：SQLite 写串行，偶发长事务会挡住读；靠 `busy_timeout` 兜底
- AutoMigrate 只增不删：废弃的列（如 `entries` 之外的旧列）会留在表里，语义变化靠代码兼容
- 打开长会话要解析整份 JSONL：上千条的会话有一次线性开销
  （列表页不受影响，它只读元数据）
- PI 的 JSONL 格式是它的私有契约：它升级格式时投影层要跟
