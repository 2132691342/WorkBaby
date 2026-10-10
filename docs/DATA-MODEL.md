# 域模型与持久化

全部状态落在一个 SQLite 文件里：`workbaby.db`（路径见 `docs/DEVELOPMENT.md` 的数据目录）。
持久化只有两层：`backend/repo`（GORM 查询）与 `backend/domain`（DO 定义）。
**跨边界的字段形态只有一种**：主键是带前缀的 ULID 字符串，时间是毫秒整数。

## 1. 约定

| 约定 | 内容 |
|---|---|
| 表与 DO | 一个聚合根一个 `domain/*.go`，DO 必须显式 `TableName()`——GORM 会把 `XxxDO` 复数化成 `xxx_dos` |
| 主键 | ULID + 领域前缀（`SESSION_` / `ENTRY_` 等），生成见 `domain/id.go` |
| 时间戳 | `CreatedAt` / `UpdatedAt` 一律毫秒整数（`autoCreateTime:milli`）；跨边界 struct 里不出现 `time.Time` |
| JSON tag | 一律 snake_case；前端 `types/api.ts` 的字段名必须与后端 json tag 完全一致 |
| 迁移 | GORM AutoMigrate：只增不删、零值兜底；FTS5 虚表与触发器用 raw SQL 逐条创建 |
| 连接 | `pool.SetMaxOpenConns(1)`，单连接 + WAL 让写行为完全可预测 |

连接期 PRAGMA：WAL（读写并发）、`busy_timeout=5000`（规避瞬时写锁）、`synchronous=NORMAL`、`foreign_keys=ON`。

## 2. 表一览

| 表 | DO | 用途 |
|---|---|---|
| `sessions` | `SessionDO` | 会话本体 + 分支指针 + 累计计数 |
| `entries` | `EntryDO` | 会话条目：append-only，树状链 |
| `approvals` | `ApprovalDO` | 审批卡（跨进程重启仍有条） |
| `providers` | `ProviderDO` | 模型服务配置，密钥 AES-GCM 密文 |
| `model_configs` | `ModelConfigDO` | 按「服务 + 模型」的覆写配置 |
| `knowledge_docs` / `knowledge_chunks` | `KnowledgeDocDO` / `KnowledgeChunkDO` | 知识库文档与切片 |
| `knowledge_chunks_fts` | （FTS5 虚表） | 全文检索，靠触发器与切片同步 |
| `settings` | `SettingDO` | 键值设置全集 |
| `token_usages` | `TokenUsageDO` | 每次 LLM 调用一行用量 |

### sessions

`leaf_entry_id` 是当前分支指针，`message_count` / `total_tokens` 是冗余计数（避免列表页回溯整棵树）。
`permission`（`ask` / `auto_edit` / `yolo`）与 `provider_id` + `model` 一并存会话：会话换了模型和规矩，不影响其他会话。

### entries —— 树状链

```
E1(user) → E2(assistant+tool_calls) → E3(tool) → E4(assistant)  ← leaf
                └→ E3'(tool, 另一分支) → E4'                      ← 分支
```

- `parent_id` 串成树，`seq` 是会话内的写入顺序；seq 分配、插入与 `leaf_entry_id`
  推进在同一个写事务里完成（读最大 seq 与插入绑成原子步），并发追加不会撞号或分叉
- append-only：不改不删，错误的分支靠移动 `leaf_entry_id` 放弃
- 为什么要树而不是线性表：`/clear`（回到新起点）与「从某条历史重开」是同一棵树的不同叶子，
  零成本实现；代价是「列全部消息」必须从 leaf 沿 `parent_id` 上溯
- `payload_json` 存 `MessagePayload`（ thinking / content / tool_calls / tool_call_id /
  tool_name / is_error / stop_reason / latency_ms / images ），一种 payload 覆盖所有消息形态
- `usage_json` 存这一轮的 `UsageVO`（token 数与上下文占用在此定稿）：append-only 表不回头补写，
  所以计量必须在写这条消息时就算准

### model_configs

复合主键 `(provider_id, model)`，`ContextWindow` 为 0 表示跟随内置目录（`domain/modelcap.go`）。
存在的意义是**让私有部署与改名模型能被正确计费与算水位**——目录认不出的模型靠它闭环。
`MaxOutput` 列已废弃：输出预算按「窗口 1/8」派生（`domain.ModelCapability.OutputBudget`，
窗口来自内置目录时再与厂商硬上限取小），不写死、不由用户设置，列仅为兼容既有表结构而保留。
`Temperature` / `TopP` 为 `SamplingUnset`（-1）时不向上游下发，0 是合法的确定性取值。

### knowledge_chunks + FTS5

```sql
CREATE VIRTUAL TABLE knowledge_chunks_fts
USING fts5(content, doc_id UNINDEXED, chunk_id UNINDEXED, tokenize='trigram')
```

三个触发器（AFTER INSERT / DELETE / UPDATE）把 `knowledge_chunks.content` 同步进虚表。
`trigram` 是硬要求：默认 `unicode61` 不按字切分 CJK，整句中文会退化成一个 token，检索零命中。
代价是 trigram 只认连续的三个字符，两三个字的短查询需要子串兜底（见 `specs/09`）。

## 3. 形态命名

同一实体的不同形态同居一个文件，便于对照：

| 后缀 | 语义 | 出现位置 |
|---|---|---|
| DO | 数据库行形态 | 只在 repo 层进出 |
| REQ | 前端入参 | server → api → service |
| VO | 组装后的视图对象 | service 出口 |
| RESP | 接口出参 | api → server |

`server / api / service / repo` 四层禁止就地声明这些结构体，必须引用 `domain` 里的定义。
DO 永远不直接出：`ProviderDO.APIKeyEnc` 的 tag 是 `json:"-"`，出参经 `ProviderVO` 换成 `has_key`。

## 4. 设置的双轨

静态默认（`domain.DefaultSettings`，首启写入）+ 运行时 KV（`settings` 表）。
区分依据：会不会被用户在设置页改。会改的进表，不会的留在代码里。

握手时 `/bootstrap` 把设置全集一次性下发给前端，前端不在渲染时逐个拉单个键。

## 5. 已知代价

- 完整历史（含消息的 base64 图片）都存在库里，体积随使用增长
- 单连接写：SQLite 写串行，偶发长事务会挡住读；靠 `busy_timeout` 兜底并对用户说人话
- AutoMigrate 只增不删：废弃字段只能留在表里（如 `model_configs.max_output`），改字段语义要靠代码兼容
