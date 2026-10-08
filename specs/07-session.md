# 07 · 会话模型

## 定位

SQLite 里只有两张核心表撑起全部状态：`sessions` + `entries`。
**消息链**：entries 用 `parent_id` 串成链，`leaf_entry_id` 指向当前末端。

## 表结构（要点）

| 表 | 关键列 |
|---|---|
| sessions | id, title, workspace, provider_id, model, permission, leaf_entry_id, message_count, total_tokens, created_at, updated_at |
| entries | id, session_id, parent_id, seq, role, type, payload_json, usage_json, created_at |
| approvals | id, session_id, tool_call_id, tool, label, args_json, risk, reason, status, created_at, decided_at |
| providers | id, name, api, base_url, api_key_enc, models(JSON), is_default, created_at, updated_at |
| model_configs | 模型级能力配置（thinking / 识图 / 窗口等），provider_id + model 定位 |
| settings | key(PK), value |
| knowledge_docs / knowledge_chunks | 文档与分块，FTS5 由触发器同步 |
| token_usages | 每次 LLM 调用一行 |

## 条目类型

| type | role | payload | 说明 |
|---|---|---|---|
| message | user / assistant / tool | 见下 | 唯一的消息形态 |
| approval | — | 见 approval 门 | 审批决策卡挂在消息流里 |

| role | payload |
|---|---|
| user | `{content}` |
| assistant | `{content, thinking, tool_calls[], stop_reason, latency_ms}` |
| tool | `{content, tool_call_id, tool_name, is_error, latency_ms}` |

`payload_json` 是一段 JSON 而不是多列：消息形态会演进，
换 schema 时只动序列化层，迁移成本最低。

## 链还原

包级函数 `buildChain(entries, leafID)`：从 leaf 沿 `parent_id`
回溯到根，反转即当前上下文。放在 service 层而不是 repo 层——
repo 只管存取，「怎么拼成上下文」是业务语义。

**分支 = 只移动 `leaf_entry_id`**，旧分支数据仍在树上，
天然支持「回到某条重问一遍」。

数据库永远存完整历史：压缩只发生在发给模型的那一刻（见 spec 02），
不落库 compaction 条目。会话详情因此永远是完整的。

## 落库契约（事件 → 条目）

对话事件落库时守住两条与协议配对相关的硬约束：

- **声明在第一个结果到达前合并落库**：同轮多个 `tool_start` 若逐条落库，链上会留下
  `A(c1) → A(c2) → T(c1) → T(c2)`——工具结果与它的声明之间隔着另一条 assistant，
  历史回放时上游以 `tool result's tool id not found` 直接 400。正确做法是收集到的
  声明在第一个结果到来时合成一条 assistant 先落，再落结果。
- **ULID 熵源并发安全**：`ulid.Monotonic` 不是并发安全的，并行工具与并发请求下
  裸用会生成重复主键，落库整条丢失。`pkg.NewID` 用互斥锁串行化熵源。

## 生命周期

- 删除会话：级联删 entries / approvals / token_usages（单事务内）
- 自动命名：标题仍是默认值时，用首条用户消息前 24 字改标题
- 排序：按 `updated_at` 倒序，前端分组为今天 / 本周 / 更早

## Run 生命周期

```
send → runID → 内核循环 → done
  ├─ stop：ctx 取消，落已产出内容
  ├─ error：错误事件 + 已落内容保留
  └─ 审批挂起：run 阻塞在闸门等决策，超时 30 分钟按拒绝
```

一个会话同时只允许一个 run（`ErrSessionBusy`）。

## 测试

| 文件 | 覆盖的链路 |
|---|---|
| `service/chain_test.go`（`TestServiceRunChain`） | 建会话 → 发送 → 落库；审批闭环；错误轮半成品落库；插话按注入链序落库；声明与结果紧邻配对（跨轮 / 同轮并发）；默认模型继承与回填 |
| `api/api_test.go`（`TestHTTPChain`） | 真实 HTTP 栈上的 SSE 送达与 `Last-Event-ID` 重连补帧；启动装配与跨源预检 |
| `singleinstance/singleinstance_test.go` | 二次启动把文件路径（或唤起请求）转交主实例 |

## 取舍

消息树存 parent_id 而不是线性列表：`/clear` 与「回到开头」只是把叶子指针
挪回第一条，历史零拷贝保留。代价是每次追加都要读一次 leaf 指针，
量级是单行查询，可忽略。
