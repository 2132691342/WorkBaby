# 09 · 会话与消息

## 1. 会话模型

`ChatSessionDO` 关键字段：

| 字段 | 说明 |
|---|---|
| `ProviderID` / `Model` | 本会话使用的模型服务与模型名 |
| `WorkspacePath` | 绑定的外部工作目录（绝对路径；空 = 默认工作区） |
| `PermissionMode` | 会话级权限模式（覆盖全局 `agent.session_mode`） |
| `Kind` | `normal` 主会话 / `side` 辅助会话（不出现在侧栏） |
| `ParentID` / `BranchPoint` | 会话树血缘：分叉出的会话记住来源与分叉点 seq |
| `Pinned` | 置顶（排序 `pinned DESC → last_message_at DESC`） |
| `MessageCount` / `LastMessageAt` | 列表页统计 |
| `MetadataJSON` | 会话级状态（目标模式、压缩边界、当前 Agent 等），避免为每种状态加列 |

### 辅助对话（side）

右栏的并行问答：继承主会话历史（有界截取 + 对齐 user 轮次）但不进主时间线。
主对话被审批卡住时仍可用。`Kind=side` 的会话被侧栏列表过滤。

### 会话工作区

| 情况 | 工作区根 |
|---|---|
| 绑定了 `WorkspacePath` | 该目录（沙箱 `.workbaby/` 在其下） |
| 未绑定 | `{home}/workspaces/{sessionID}` |

**单一数据源**：`service.SessionContext` 提供工作区根与过程数据目录，
工具沙箱 / 快照 / 文件面板 / chat 共用同一实例，禁止各装配点自行拼接。

## 2. 一次发送的完整链路

`POST /chat/stream` → `ChatService.SendStream(ctx, sessionID, content, fileIDs, params)`：

| # | 步骤 |
|---|---|
| 1 | 取会话级锁（关键区只覆盖「占位落库 + run 登记」，跨会话互不阻塞） |
| 2 | 补齐 Provider/Model 兜底、解析附件（图片转多模态 part） |
| 3 | `prepareRun`：取消该会话遗留挂起审批 → 分配序号（user + assistant 占位各一条）→ 生成 `runID` → 登记执行平面 → 落 user 消息 + assistant 占位（`streaming`） |
| 4 | 建可取消 ctx 并登记到 `runRegistry`，立刻返回 `{run_id, assistant_msg_id}` |
| 5 | 后台 goroutine `runLLM` 继续：墙钟上限（Agent 定义或 10 分钟） |

`runLLM → executeAgent` 的装配顺序：

```
技能同步 → Agent 模型/推理强度覆盖 → UserPromptSubmit 钩子（可阻断）
→ 取 Provider → 拉历史（含多模态）→ 辅助会话前缀 → 上下文装配（能力 Preload + 服务侧附加段 + 预算裁剪）
→ 工具暴露唯一入口（启用 → 技能白名单 → Agent 策略）→ 预算与采样计算
→ newCoreLoop（Sink=事件映射，Hooks=插话/续跑，注入 Delegator 与 InputRequester）
→ RebuildHistory 清洗后 Run（或 Resume）
```

收尾（`chat_run.go`）：按每次上游调用落 `token_usages` → 写运行记录 → assistant 落库
→ 失败或取消时回滚本 run 新增的免审授权 → 反幻觉核验 → 能力沉淀 → 目标模式判定
→ **发 `chat:done`（落库后才发）**。

**`chat:done` 为什么必须最后发**：前端收到它会立刻拉权威快照，
早于落库会让「空 content」覆盖已渲染内容，过程块一并丢失。

## 3. 消息与序号

消息角色：`user / assistant / tool / system`。

| 字段 | 说明 |
|---|---|
| `Seq` | 会话内单调。工具消息与注入消息**统一走同一分配器**（`allocSeq`），否则序号撞号会让增量分页漏消息 |
| `RunID` | 归属的 run（前端按 run 过滤事件） |
| `ToolCalls` | assistant 发起的工具调用（JSON 文本） |
| `ToolCallID` | tool 消息与调用的配对键 |
| `Status` | `pending / streaming / completed / failed / cancelled / archived` |
| `StopReason` | 归一化停止原因 |
| token 字段 / `LatencyMs` / `Cost` | 用量与成本 |

### 过程块（message_blocks）

消息正文之外，执行过程按块落库：`thinking / text / tool_call / tool_result / artifact / genui / skill`。

| 设计点 | 原因 |
|---|---|
| 块带 `Seq` | 块的顺序即真实输出顺序（叙述 → 工具 → 叙述），刷新后能完整复现 |
| 正文切片落块 | 正文在工具调用之前与轮次结束时落块，而非「过程全在前、正文全在后」 |
| 工具结果落库为 `role=tool` 消息 | 下次 run 重建上下文时模型能看到「上次为什么失败」 |

## 4. 插话（steer）

run 进行中插入用户消息：

| 步 | 行为 |
|---|---|
| 1 | 消息**立即落库**（前端可见）并分配 seq |
| 2 | 内容进注入队列 |
| 3 | 内核在 `Steering`（轮间：跑过工具后、下一轮之前）消费 |
| 4 | 若模型已准备收尾，则由 `FollowUp`（收尾缝）消费——保证插话不被丢弃 |

无活动 run 时 `QueueSteer` 返回 `ErrRunNotActive`；
run 结束（含取消/失败）后队列作废（注入消息只属于本次 run）。

## 5. 消息对账（前端）

`chat:done` 后前端拉权威快照，按三态判定本条回复的落位：

| 状态 | 行为 |
|---|---|
| `dedupe` | 权威已含 → 不本地再 push（避免同一回复两条） |
| `fill` | 权威是空占位 → 补正文 + 用本地块还原过程 |
| `missing` | 权威未含 → 本地兜底插入 |

并发拉取用 `loadSeq` 守卫：过期响应直接丢弃（防慢响应把新快照覆盖回旧快照）。

## 6. 会话操作

| 操作 | 语义 |
|---|---|
| 重命名 | 首条用户消息自动命名（可用附件名兜底） |
| 置顶 / 归档 | 归档自动取消置顶 |
| 清空 | 清空消息但保留会话 |
| 压缩归档 | `compact`：归档历史并写压缩边界（前端「查看折叠纪要」的数据源） |
| 分叉 | 从指定消息分叉出新会话（记住来源与分叉点） |
| 截断 | 截断到指定消息 |
| 删除 | 单条 / 批量 / 整会话 |

## 7. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 一次对话 = 一次 run（显式生命周期） | 中断、续跑、用量归属、审计都有明确边界 | 一次 run 内的并发只能靠工具层并行 |
| 会话级锁 + 插话 | 跨会话天然并行；同会话中途纠偏不打断执行 | 同会话无法真正并行两个 run |
| 会话级状态存 `MetadataJSON` | 不必为每种状态加列 | 无法用 SQL 直接过滤这些字段 |
| 序号统一分配器 | 增量分页不漏消息 | 进程内水位需与 DB 最大值同步（首次读最大值） |
| `chat:done` 落库后发 | 前端不会用空内容覆盖已渲染结果 | 终态到达稍晚（多一次写库耗时） |
