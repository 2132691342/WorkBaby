# docs/API-CONTRACT.md · 接口契约总表

业务接口走 HTTP（gin），统一前缀 `/api/v1`，**仅使用 GET / POST**。
系统能力（文件对话框 / 剪贴板 / 窗口 / 托盘）走 Wails 绑定，不在本表。

## 1. 统一约定

- 响应包裹：成功 `{code:0, message:"ok", data:...}`；失败 `{code:<错误码>, message:<文案>, details?}`
- 请求体统一 JSON 绑定；分页 / 条数由各接口的 `limit` / `k` 查询参数控制
- 契约版本：`GET /api/v1/meta/contract`；前端启动比对，不一致显式报错
- 字段一律 **snake_case**
- 路径风格：`/api/v1/{resource}/:id/{action}`；变量参数放路径末尾；删除走 `POST .../delete`

**错误码段位**：1000 通用/文件/路径 · 2000 配置/持久化 · 3000 LLM/Provider · 4000 工具/命令审批 · 5000 Agent · 6000 Memory · 7000 Knowledge/RAG · 8000 Skill/MCP · 9000 保留。
扩展段位：8200 AgentProfile · 8300 UserCommand · 8600 UserHook。

## 2. 端点

### 2.1 元信息与运维

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /meta/version | 版本 |
| GET | /meta/health | 健康检查 |
| GET | /meta/contract | 契约版本 |
| GET | /meta/runtime | 内置运行时状态（排障「命令找不到」） |
| GET | /admin/overview | 后台概览（各域计数） |
| POST | /admin/cleanup-token-usages | 清理上游误报的缓存 token 明细 |
| GET | /docs · /docs/:name | 文档列表 / 内容 |
| GET | /dashboard/stats · /trend · /token-trend | 仪表盘统计 / 趋势 / token 三线趋势 |
| GET | /events | **SSE 事件流**（见 §3） |
| GET | /files/*filepath | 受管文件静态服务（**注册在根路径，无 `/api/v1` 前缀**） |

### 2.2 会话与消息

| 方法 | 路径 | 说明 |
|---|---|---|
| GET / POST | /chat/sessions | 分页列表 / 新建 |
| GET | /chat/sessions/:id | 详情 |
| POST | /chat/sessions/:id/rename · /delete · /clear | 重命名 / 删除 / 清空消息 |
| POST | /chat/sessions/delete-batch | 批量删除 |
| GET | /chat/sessions/search | 跨会话检索 |
| GET | /chat/sessions/:id/params | 生效参数与来源 |
| POST | /chat/sessions/:id/model · /permission · /agent | 切换模型 / 权限模式 / 会话 Agent |
| POST | /chat/sessions/:id/workspace | 绑定工作区 |
| GET / POST | /chat/sessions/:id/goal | 目标模式：读取 / 设置（set / pause / resume / clear） |
| GET | /chat/sessions/:id/messages | 消息分页 |
| POST | /chat/messages/:sid/delete/:id · /truncate · /fork | 删单条 / 截断 / 分叉 |
| GET | /chat/sessions/:id/todos | 待办状态 |
| POST | /chat/sessions/:id/todos/:itemID/toggle | 勾选待办 |
| POST | /chat/sessions/:id/compact | 压缩归档历史 |
| GET | /chat/sessions/:id/usage/context | 上下文占用分段 |
| GET | /chat/commands?session_id= | 可用斜杠命令（内置 + 设置页 + `{home}/commands/*.md` + `<ws>/.workbaby/commands/*.md`；同名时工作区 > 用户级 > 设置页） |
| GET / POST | /chat/commands/custom | 自定义命令列表 / 按 name upsert |
| POST | /chat/commands/custom/:name/delete | 删除自定义命令 |
| POST | /chat/sessions/:id/pin · /archive | 置顶 / 归档（归档自动取消置顶） |
| GET / POST | /chat/sessions/:id/side | 辅助对话（GET 返回已有或 null；POST 幂等确保） |

### 2.3 流式运行

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /chat/stream | 建 run 并开始流式生成 |
| POST | /chat/sessions/:id/cancel | 中断当前 run（`:id` = sessionID） |
| POST | /chat/runs/:id/resume | 从检查点续跑（`:id` = runID） |
| POST | /chat/sessions/:id/steer | run 中插入指令 |
| GET | /chat/runs · /chat/runs/:id/events | 运行历史 / 某次运行事件明细 |

**cancel 与 resume 必须分路径**：前者的 id 是 sessionID，后者是 runID——由 `check-contract.ps1` 强制。

### 2.4 后台任务

提交即返回（pending），worker 池后台执行（复用聊天护栏链，审批门为 grantApprover——只吃免审授权、不弹窗）。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET / POST | /tasks | 列表（?limit=50）/ 提交（session_id / agent / prompt） |
| POST | /tasks/:id/cancel | 取消（pending 直接取消；running 触发 ctx 取消；幂等） |

状态机 `pending → running → completed / failed / cancelled`；重启时未终态任务统一标记失败。

### 2.5 审批与免审授权

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /chat/approval/:id/decide · /answer · /skip | 决策 / 补充输入答复 / 跳过 |
| GET | /chat/approvals/pending | 未决审批（刷新/重启恢复） |
| GET | /chat/approval-grants | 免审授权列表 |
| POST | /chat/approval-grants/:id/delete | 撤销免审授权 |

### 2.6 工作区、变更与工件

| 方法 | 路径 | 说明 |
|---|---|---|
| GET / POST | /trust | 信任列表 / 登记 |
| GET | /trust/resolve · /trust/roots | 解析某路径信任态 / 恒信任根 |
| POST | /trust/revoke | 撤销信任 |
| GET | /chat/workspace/:id/files · /ls · /file | 文件树 / 目录列举 / 读文件 |
| POST | /chat/workspace/:id/rename · /copy · /remove | 重命名 / 复制 / 删除 |
| GET | /chat/sessions/:id/changes | 文件变更列表（含 diff） |
| GET | /chat/changes/:cid | 单条变更详情 |
| POST | /chat/changes/:cid/rollback | 回滚变更 |
| GET | /chat/sessions/:id/artifacts | 产物列表 |
| POST | /chat/artifacts/:aid/delete | 删除产物 |

### 2.7 模型 Provider 与设置

| 方法 | 路径 | 说明 |
|---|---|---|
| GET / POST | /ai-provider | 列表 / 新建 |
| GET | /ai-provider/available · /kinds · /tiers · /presets | 可用模型 / 协议类型 / 档位 / 预设 |
| GET | /ai-provider/:id | 详情 |
| POST | /ai-provider/:id/update · /delete · /test | 更新 / 删除 / 连通性测试 |
| POST | /ai-provider/reload | 从 model.json 热重载 |
| GET | /ai-provider/circuit-status | 就绪 / 熔断状态 |
| POST | /ai-provider/:id/reset-circuit | 重置并重建 |
| GET | /settings · /settings/general · /settings/websearch · /settings/exec/agent | 设置读取 |
| POST | /settings/general · /settings/websearch · /settings/exec/agent | 设置写入 |
| GET / POST | /kv/:key | 任意 KV 读写 |

### 2.8 技能与 MCP

| 方法 | 路径 | 说明 |
|---|---|---|
| GET / POST | /skills | 列表 / 新建（粘贴 SKILL.md） |
| POST | /skills/import-zip | zip 批量导入（回执含失败清单） |
| POST | /skills/:name/enabled · /update · /delete | 启停 / 更新 / 删除（内置只读） |
| GET | /mcp/servers | 列表（env 掩码） |
| GET / POST | /mcp/servers/raw | 读取 / 写入 mcp.json |
| POST | /mcp/servers · /:name/enabled · /:name/delete · /reload · /reveal | 新增 / 启停 / 删除 / 重载 / 定位配置 |

### 2.9 工具

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /tools | 列表（含启用状态与 schema） |
| POST | /tools/:name/enabled | 启停工具 |

### 2.10 知识库与记忆

| 方法 | 路径 | 说明 |
|---|---|---|
| GET / POST | /kdocs | 文档列表 / 新增 |
| POST | /kdocs/import-file | 导入本地文件 |
| GET | /kdocs/search · /groups · /group/:group | 检索 / 分组 / 按分组列文档 |
| GET | /kdocs/:id | 详情 |
| GET | /kdocs/:id/file | 原文文件（下载 / 前端预览） |
| POST | /kdocs/:id/update · /delete · /reindex | 更新 / 删除 / 重建索引 |
| GET | /memory · /list · /search · /text | 概览 / 条目列表 / 检索 / 全文 |
| POST | /memory/append · /delete · /replace | 追加一条 / 删除 / 整篇覆盖 |

长期记忆是单一 MEMORY.md（不分类、无候选收件箱），检索走 FTS5 派生索引。

### 2.11 子智能体 · 钩子

| 方法 | 路径 | 说明 |
|---|---|---|
| GET / POST | /agent-profiles | 列表 / 按 name upsert（人设 / 工具策略 / 预算 / 模型） |
| POST | /agent-profiles/:name/enabled · /:name/delete | 启停（停用即从注册表摘除）/ 删除 |
| GET / POST | /hooks | 列表（event 已归一为 6 类事件名）/ 创建更新（后端能力，无内置界面） |
| POST | /hooks/:id/delete · /:id/test | 删除 / 试跑（样例载荷 → 决策 / 理由 / 注入上下文 / 耗时） |

### 2.12 文件夹与文件

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /folders/tree · /folders | 文件夹树 / 按父级列 |
| POST | /folders · /:id/update · /:id/delete | 新建 / 更新 / 删除 |
| GET | /files/search · /files | 搜索 / 列表 |
| POST | /files/upload · /upload-data | 登记本地文件 / 上传内存字节 |
| POST | /files/:id/delete | 删除 |
| GET | /files/:id/preview-url | 预览地址 |

## 3. SSE 事件

订阅：`GET /api/v1/events?scope=<前缀>&session_id=<会话>&run_id=<运行>&last_event_id=<seq>`

- 过滤规则：scope 前缀 → 会话 → run。**`run_id` / `session_id` 为空的事件（目标状态、用户主动触发的回滚）不属于任何 run，不会因订阅带了 run 而被丢弃**
- **订阅键是 `session_id`**：会话内全部 run 与无归属事件都送达；`run_id` 仅用于断线重放定位（目标模式的自动续跑会起新 run）
- 帧格式：`id: <seq>` + `event: <名称>` + `data: <JSON>`；seq 由 `service.Emitter` 统一分配
- 重放窗口不足时下发 `chat:gap`，前端转全量快照

### 3.1 chat:*（scope=chat）

| 事件 | 载荷要点 |
|---|---|
| `chat:stream.start` | 流开始（`model`、`resumed`） |
| `chat:stream` / `chat:thinking` | 正文 / 推理增量（`delta`，高频 map 直传） |
| `chat:turn-start` | 第 N 轮开始（`turn`） |
| `chat:checkpoint` | 检查点已写入（`turn`） |
| `chat:stats` | 本轮用量（强类型 `ChatStatsEvent`：turn / input_tokens / output_tokens / cache_read_tokens / cache_creation_tokens / total_tokens / latency_ms） |
| `chat:tool` | 工具调用开始（强类型 `ChatToolCallEvent`：id / name / arguments / agent / activity） |
| `chat:tool-start` | 工具真正开始执行（`id` / `agent` / `turn`；护栏放行后与 `chat:tool`「调用开始」分两拍） |
| `chat:queue-drained` | 插话 / 续接队列消费（强类型 `ChatQueueDrainedEvent`：queue / count / mode / turn） |
| `chat:tool-result` | 工具结果（强类型 `ChatToolResultEvent`：id / name / content / error / duration_ms / agent / ui_hint / data / meta / refused / refused_reason）。`meta` 含 `cwd`（exec 实际目录）、`same_failure_count`（同工具连续失败次数）、`adaptive_hint=1`（已注入改道提示）、`truncated_bytes` |
| `chat:approval` | 审批 / 补问（强类型 `ChatApprovalEvent`：id / command / reason / risk / can_remember；不可逆与补问 `can_remember` 恒 false） |
| `chat:approval-decided` | 审批决策（`id`、`decision`） |
| `chat:skill` | 命中技能（名称 / 来源 / 版本 / 工具白名单 / 注入字数） |
| `chat:todo` · `chat:goal` | 待办状态 / 会话目标（后者无 run 归属，按会话送达） |
| `chat:file-change` · `chat:artifact` | 文件变更（含 diff）/ 产物登记 |
| `chat:warn` | 告警（强类型 `ChatWarnEvent`，三种 kind 共用形状：`unbacked_claim` / `file_out_of_sandbox` / `agent_model_override`，扩展字段按 kind 生效） |
| `chat:compressed` | 自动压缩（强类型 `ChatCompressedEvent`：removed_messages / summary / truncated / filter_key / cutoff_at） |
| `chat:context-trimmed` | system 段被预算裁剪（`dropped_segments`、`budget_runes`） |
| `chat:retry` | 建流瞬时错误自动重试（`attempt`、`delay_ms`） |
| `chat:subagent-start` / `-done` / `-error` | 子 Agent 生命周期（`sub_run_id`、`agent`） |
| `chat:steer` | 插话已落库并入队（前端以本地回执提示） |
| `chat:error` | 错误（`code`、`message`） |
| `chat:done` | 终态（强类型 `ChatDoneEvent`：status / reason / stop_reason / message_id / usage） |
| `chat:gap` | 重放窗口失效 → 前端拉全量快照 |

**终态模型**：`chat:done` 之后 SSE 连接**保持打开**（目标模式的自动续跑要先跑一次校验 LLM 调用，可能数十秒后才起新 run）。

### 3.2 其他 scope

| 事件 | 说明 |
|---|---|
| `task:created` / `task:started` / `task:done` | 后台任务生命周期（`scope=task` 独立订阅） |
| `app:ready` | 应用就绪（携带 `server_port`、数据根、版本；**仅 Wails 事件通道**，HTTP 侧由前端轮询绑定兜底） |

## 4. Wails 绑定（非 HTTP）

Go 绑定（前端经 `@/wailsjs/go/main/App` 调用）：
`OpenFileDialog` / `OpenDirectoryDialog` 选择对话框、`OpenExternal` 打开外链、
`GetServerPort` 引导端口、`SettingValue` 读取设置。

生命周期事件：`app:ready` / `app:open-file`。
