# API 契约总表

业务接口走 HTTP（gin），统一前缀 `/api/v1`，仅使用 GET / POST。系统能力（文件对话框、剪贴板、窗口、托盘）走 Wails 绑定，不在此表。

## 统一约定

- 响应包裹：成功 `{code:0, data:...}`；失败 `{code:<错误码>, message:<文案>, detail?}`。
- 请求体统一 JSON 绑定（类型不符即返回参数错误）；分页/条数参数由各接口的 `limit` / `k` 查询参数控制。
- 契约版本：`GET /api/v1/meta/contract`；前端启动比对，不一致显式报错。
- 错误码按域分段（新增错误复用所属段位）：1000 通用/文件/路径、2000 配置/持久化、3000 LLM/Provider、4000 工具/命令审批、5000 Agent（含会话与消息编排）、6000 Memory、7000 Knowledge/RAG、8000 Skill/MCP、9000 Pet；扩展段位：8200 AgentProfile、8300 UserCommand、8600 UserHook、8800 Wiki。

## 端点

### 元信息与运维

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /meta/version | 版本 |
| GET | /meta/health | 健康检查 |
| GET | /meta/contract | 契约版本 |
| GET | /meta/runtime | 内置运行时状态（排障「命令找不到」） |
| GET | /admin/overview | 后台概览（各域计数） |
| POST | /admin/cleanup-token-usages | 清理上游误报的缓存 token 明细 |
| GET | /docs | 文档列表 |
| GET | /docs/:name | 文档内容 |
| GET | /dashboard/stats | 仪表盘统计 |
| GET | /dashboard/trend | 会话/消息/token 趋势 |
| GET | /dashboard/token-trend | token 三线趋势（today/week/month/custom） |
| GET | /events | **SSE 事件流**（`scope`、`session_id`、`run_id`、`last_event_id`；会话级订阅，run 供重放定位） |
| GET | /files/*filepath | 受管文件静态服务 |

### 会话与消息

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /chat/sessions | 会话分页列表 |
| POST | /chat/sessions | 新建会话 |
| GET | /chat/sessions/:id | 会话详情 |
| POST | /chat/sessions/:id/rename | 重命名 |
| POST | /chat/sessions/:id/delete | 删除 |
| POST | /chat/sessions/delete-batch | 批量删除 |
| POST | /chat/sessions/:id/clear | 清空消息 |
| GET | /chat/sessions/search | 跨会话检索 |
| GET | /chat/sessions/:id/params | 生效参数与来源 |
| POST | /chat/sessions/:id/model | 切换模型 |
| POST | /chat/sessions/:id/permission | 切换权限模式 |
| POST | /chat/sessions/:id/agent | 切换会话 Agent（写元数据，下一轮生效） |
| POST | /chat/sessions/:id/workspace | 绑定工作区 |
| GET/POST | /chat/sessions/:id/goal | 目标模式：读取 / 设置（set / pause / resume / clear） |
| GET | /chat/sessions/:id/messages | 消息分页 |
| POST | /chat/messages/:sid/delete/:id | 删除单条 |
| POST | /chat/messages/:sid/truncate | 截断到指定消息 |
| POST | /chat/messages/:sid/fork | 从指定消息分叉 |
| GET | /chat/sessions/:id/todos | 待办状态 |
| POST | /chat/sessions/:id/todos/:itemID/toggle | 勾选待办 |
| POST | /chat/sessions/:id/compact | 压缩归档历史 |
| GET | /chat/sessions/:id/usage/context | 上下文占用分段 |
| GET | /chat/commands?session_id= | 可用斜杠命令（内置 + 设置页自定义 + `{home}/commands/*.md` + `<ws>/.workbaby/commands/*.md` 合并；custom 项带 prompt 模板与 source 来源标记，同名时工作区文件 > 用户级文件 > 设置页记录） |
| GET/POST | /chat/commands/custom | 自定义命令列表 / 按 name upsert |
| POST | /chat/commands/custom/:name/delete | 删除自定义命令 |
| POST | /chat/sessions/:id/pin | 置顶/取消置顶 |
| POST | /chat/sessions/:id/archive | 归档/取消归档（status 切换；归档自动取消置顶） |
| GET/POST | /chat/sessions/:id/side | 辅助对话（GET 返回已有或 null；POST 幂等确保） |

### 流式运行

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /chat/stream | 建 run 并开始流式生成 |
| POST | /chat/sessions/:id/cancel | 中断当前 run（`:id` = sessionID） |
| POST | /chat/runs/:id/resume | 从检查点续跑（`:id` = runID） |
| POST | /chat/sessions/:id/steer | run 中插入指令 |
| GET | /chat/runs | 运行历史列表 |
| GET | /chat/runs/:id/events | 某次运行的事件明细 |

### 后台任务

持久化异步 Agent 作业：提交即返回（pending），worker 池后台执行（复用聊天护栏链，
审批门为 grantApprover——只吃免审授权、不弹窗），实时变化经 `task:*` SSE（scope=task）。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /tasks | 任务列表（?limit=50，最新在前；items + total） |
| POST | /tasks | 提交任务（body: session_id / agent / prompt；返回 pending 任务） |
| POST | /tasks/:id/cancel | 取消任务（pending 直接取消；running 触发 ctx 取消；幂等） |

状态机：`pending → running → completed / failed / cancelled`；应用重启时未终态任务统一标记失败。

### Wiki 仓库导读

确定性扫描生成（不依赖 LLM）；`session_id` 定位工作区，跳过依赖/构建目录。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /wiki/overview | 画像（语言统计/入口文件/目录树，每文件带一句话摘要） |
| GET | /wiki/page | 单页：目录页子项清单 / 文件页正文（上限 512KB） |

### 用户钩子

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /hooks | 钩子列表（event 已归一为现行七类事件名） |
| POST | /hooks | 创建/更新（按 id upsert，id 空=新建；matcher 写法与事件合法性在此校验） |
| POST | /hooks/:id/delete | 删除 |
| POST | /hooks/:id/test | 试跑（按事件生成的样例载荷，返回决策 / 理由 / 注入上下文 / 耗时） |

钩子子进程协议（输出解析、决策字段、事件名归一）见 doc/05「用户钩子」。

### 审批与免审授权

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /chat/approval/:id/decide | 审批决策（批准 / 本会话允许 / 拒绝） |
| POST | /chat/approval/:id/answer | 补充输入答复 |
| POST | /chat/approval/:id/skip | 跳过补充输入 |
| GET | /chat/approvals/pending | 未决审批（刷新/重启恢复） |
| GET | /chat/approval-grants | 免审授权列表（「本会话允许」持久化授权） |
| POST | /chat/approval-grants/:id/delete | 撤销免审授权 |

### 工作区、变更与工件

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /trust | 信任列表 |
| GET | /trust/resolve | 解析某路径信任态 |
| POST | /trust | 登记信任 |
| POST | /trust/revoke | 撤销信任 |
| GET | /trust/roots | 恒信任根列表 |
| GET | /chat/workspace/:id/files | 工作区文件树 |
| GET | /chat/workspace/:id/ls | 目录列举 |
| GET | /chat/workspace/:id/file | 读文件 |
| POST | /chat/workspace/:id/rename | 重命名 |
| POST | /chat/workspace/:id/copy | 复制 |
| POST | /chat/workspace/:id/remove | 删除 |
| GET | /chat/sessions/:id/changes | 文件变更列表（含 diff） |
| GET | /chat/changes/:cid | 单条变更详情 |
| POST | /chat/changes/:cid/rollback | 回滚变更 |
| GET | /chat/sessions/:id/artifacts | 产物列表 |
| POST | /chat/artifacts/:aid/delete | 删除产物 |

### 模型 Provider 与设置

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /ai-provider | Provider 列表 |
| POST | /ai-provider | 新建 |
| GET | /ai-provider/available | 可用模型（供会话选择） |
| GET | /ai-provider/kinds | 支持的协议类型 |
| GET | /ai-provider/tiers | 合法档位取值（primary/backup，仅排序） |
| GET | /ai-provider/presets | 内置模型服务预设（新增 provider 一键预填） |
| GET | /ai-provider/:id | 详情 |
| POST | /ai-provider/:id/update | 更新 |
| POST | /ai-provider/:id/delete | 删除 |
| POST | /ai-provider/:id/test | 连通性测试 |
| POST | /ai-provider/reload | 从 model.json 热重载 |
| GET | /ai-provider/circuit-status | 就绪 / 熔断状态 |
| POST | /ai-provider/:id/reset-circuit | 重置并重建 |
| GET | /settings | 全量设置 |
| GET/POST | /settings/general | 通用设置 |
| GET/POST | /settings/websearch | 搜索配置 |
| GET/POST | /settings/exec/agent | exec 命令白名单 |
| GET/POST | /kv/:key | 任意 KV 读写 |

### 技能与 MCP

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /skills | 技能列表 |
| POST | /skills | 新建/导入（粘贴 SKILL.md） |
| POST | /skills/import-zip | zip 批量导入（回执含失败清单） |
| POST | /skills/:name/enabled | 启停 |
| POST | /skills/:name/update | 更新（内置只读） |
| POST | /skills/:name/delete | 删除（内置只读） |
| GET | /mcp/servers | MCP server 列表（env 掩码） |
| GET/POST | /mcp/servers/raw | 读取/写入 mcp.json |
| POST | /mcp/servers | 新增 |
| POST | /mcp/servers/:name/enabled | 启停 |
| POST | /mcp/servers/:name/delete | 删除 |
| POST | /mcp/servers/reload | 重载 |
| POST | /mcp/servers/reveal | 定位配置文件 |

### 工具

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /tools | 工具列表（含启用状态与 schema） |
| POST | /tools/:name/enabled | 启停工具 |

### 知识库与记忆

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /kdocs | 文档列表 |
| POST | /kdocs | 新增文档 |
| POST | /kdocs/import-file | 导入本地文件 |
| GET | /kdocs/search | 检索 |
| GET | /kdocs/groups | 分组 |
| GET | /kdocs/group/:group | 按分组列文档 |
| GET | /kdocs/:id | 详情 |
| POST | /kdocs/:id/update | 更新 |
| POST | /kdocs/:id/delete | 删除 |
| POST | /kdocs/:id/reindex | 重建索引 |

长期记忆是单一 MEMORY.md（不分类、无候选收件箱），检索走 FTS5 派生索引。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /memory | 记忆概览 |
| GET | /memory/list | 条目列表 |
| GET | /memory/search | 记忆检索 |
| GET | /memory/text | 全文（Markdown 原文） |
| POST | /memory/append | 追加一条（指定分节） |
| POST | /memory/delete | 删除一条 |
| POST | /memory/replace | 整篇覆盖 |

### 子智能体

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /agent-profiles | 自定义子智能体列表 |
| POST | /agent-profiles | 按 name upsert（人设 / 工具策略 / 预算 / 模型） |
| POST | /agent-profiles/:name/enabled | 启停（停用即从注册表摘除） |
| POST | /agent-profiles/:name/delete | 删除 |

### 桌宠、文件夹与文件

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /pet/state | 桌宠状态 |
| GET | /pet/config | 桌宠配置 |
| POST | /pet/config/update | 更新配置 |
| GET | /pet/sprites | 形象列表 |
| POST | /pet/sprites | 新增形象 |
| POST | /pet/sprites/:id/delete | 删除形象 |
| POST | /pet/window/:mode | 切换形态（pet / main） |
| POST | /pet/window/click-through | 设置桌宠命中区域（body: enabled + rects[]，物理像素；空列表恢复整窗可交互） |
| GET | /folders/tree | 文件夹树 |
| GET | /folders | 按父级列文件夹 |
| POST | /folders | 新建文件夹 |
| POST | /folders/:id/update | 更新 |
| POST | /folders/:id/delete | 删除 |
| GET | /files/search | 搜索文件 |
| GET | /files | 文件列表 |
| POST | /files/upload | 上传（登记本地文件，需本地路径） |
| POST | /files/upload-data | 上传内存字节（粘贴/拖拽的图片无本地路径） |
| POST | /files/:id/delete | 删除 |
| GET | /files/:id/preview-url | 预览地址 |

## SSE 事件

订阅：`GET /api/v1/events?scope=<前缀>&session_id=<会话 id>&run_id=<运行 id>&last_event_id=<seq>`。

- 过滤规则：scope 前缀 → 会话 → run。**`run_id` / `session_id` 为空的事件（目标状态、用户主动触发的回滚）不属于任何 run，不会因订阅带了 run 而被丢弃**；
- 订阅键是 `session_id`：会话内全部 run 与无归属事件都送达；`run_id` 仅用于断线重放定位（目标模式的自动续跑会起新 run）；
- 帧格式：`id: <seq>`、`event: <名称>`、`data: <JSON>`；`seq` 由服务端按 run 单调分配（`service.Emitter` 统一入账），支持断线重放；重放窗口不足时下发 `chat:gap` 转全量快照。

| 事件 | 载荷要点 |
|---|---|
| `sse-ready` | 连接就绪（重放完成） |
| `ping` | 心跳（前端 watchdog 续期） |
| `chat:stream.start` | 流开始（携带模型名；前端由建流响应覆盖，不单独消费） |
| `chat:stream` | 正文增量 |
| `chat:thinking` | 思维链增量 |
| `chat:tool-start` | 工具开始执行（前端由 `chat:tool` 落地 running 态，不单独消费） |
| `chat:steer` | 插话已落库并入队（前端以本地回执提示） |
| `chat:stats` | 本轮用量（input/output/cache/total/latency） |
| `chat:turn-start` | 第 N 轮开始（多轮任务轮次推进可见） |
| `chat:checkpoint` | 检查点已写入（续跑位点轮次） |
| `chat:skill` | 命中技能（名称/来源/版本/工具白名单/注入字数） |
| `chat:tool` | 工具调用开始（id/name/arguments/agent/activity） |
| `chat:tool-result` | 工具结果（id/name/content/error/duration_ms/agent/ui_hint） |
| `chat:approval` | 审批请求（id/command/reason/risk/can_remember） |
| `chat:approval-decided` | 审批决策 |
| `chat:subagent-start` / `-done` / `-error` | 子 Agent 生命周期 |
| `chat:todo` | 待办状态变化 |
| `chat:goal` | 会话目标状态（set/pause/resume 与自动续跑校验后共用；无 run 归属，按会话送达） |
| `chat:file-change` | 文件变更（含 diff 信息） |
| `chat:artifact` | 产物登记 |
| `chat:warn` | 告警（越界写入；反幻觉核验 `kind=unbacked_claim`） |
| `chat:compressed` | 自动压缩发生（移除轮次、恢复引用） |
| `chat:context-trimmed` | system 段被预算裁剪（被丢段、预算） |
| `chat:retry` | 建流瞬时错误自动重试（`{attempt, delay_ms}`，前端 `streamingRetry` 横幅据此显示「正在重试 N/3」） |
| `chat:error` | 错误（code/message） |
| `chat:done` | 终态（status/reason/stop_reason/message_id/usage） |
| `chat:gap` | 重放窗口失效 → 前端拉全量快照 |
| `task:created` / `task:started` / `task:done` | 后台任务生命周期（`scope=task` 独立订阅） |
| `pet:state` | 桌宠状态机变化（idle / happy / working / sleeping） |
| `pet:show` / `pet:hide` | 桌宠形态切换 |
| `app:ready` | 应用就绪（携带 `server_port`、数据根、版本；Wails 与 HTTP 双通道） |

## Wails 绑定（非 HTTP）

- **Go 绑定**（`api.Handler` 方法，前端经 `@/wailsjs/go/main/App` 调用）：`OpenFileDialog` / `OpenDirectoryDialog` 选择对话框、`OpenExternal` 打开外链、`ReadLocalImage` / `UploadPetSprite` / `SavePetSpriteImage` 形象资产读写、`PetToggleMode` / `PetMove` 桌宠窗口、`GetServerPort` 引导端口、`SettingValue` 读取设置；生命周期事件 `app:ready` / `app:open-file`。
- **Wails JS runtime**（前端直接调用 `@/wailsjs/runtime/runtime`）：窗口最小化/最大化/关闭、事件订阅，不经 Go 绑定。
