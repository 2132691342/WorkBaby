# CHANGELOG

版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。本项目尚未正式发行，`0.x` 期间接口可能变动。

## [Unreleased]

### 变更
- **面向新手收敛：设置中心 18→12 section + 工具数真实可见**：
  - 工具页修复：列表分组漏掉后端真实存在的第 6 组 `functools`——30 个纯函数工具整组不可见，用户数出来只有 20 行（hero 总数却显示 50）；补 `tools.group.functools` 组后 50 个工具全部可见，数字与文档口径一致
  - 设置中心 4 组 18 section → 3 组 12 section（模型 / 能力 / 系统）：移除 自定义命令 / 用户钩子 / 文件 / 仪表盘 / 运行历史 / 仓库导读 六个开发者向入口；exec 二进制白名单编辑器移除（后端默认白名单继续生效）；高级页保留 托盘 / 记忆开关 / 免审授权撤销
  - 仓库导读（Wiki）整体下线：删除 Go 侧 routes / handler / service / domain 与 spec 21，`API-CONTRACT.md` 移除 2 端点与 8800 段，`client.ts` KNOWN_PREFIXES 同步；命令面板导航 12→8 条与旧路由重定向清理；删除 5 个视图组件 + dashboard 4 组件 + `stores/dashboard.ts` + `RunRecord` 类型；i18n 清理 100 死键（1554→1454）
  - 用户手册 14→12 页（删 wiki / hooks 两页）；PAGE-STRUCTURE / DESIGN / PROJECT-SPEC / README 同步收敛说明
- **文档-代码对齐第三轮（参考项目调研收口）**：
  - 调研 5 个参考项目并归档取舍结论至 `docs/REFERENCE-PROJECTS.md`：pi（内核契约基线）、go-micro（错误分类 / 重试 / 步骤复用 / 确定性压缩）、ERP-AGENT（Python 项目，仅取设计思路与反面教训）、PandaX（工程实践对照）、gotool（评估不引入，理由记录于 `AGENTS.md §1.1`）
  - `yolo` 权限模式对 `destructive` 风险改回 `ask`（不可逆操作永不免审，与规格及 `Mode.Decide` 注释一致）
  - 步骤记忆真正接线：`RepeatGuard(3, loop.StepsStore())` 消费检查点恢复的步骤——续跑命中直接复用结果、不重放副作用；本 run 内新记仅随检查点持久化、不在本 run 内复用（防「写后重读拿到旧值」）
  - 能力段拼接序显式化：各能力 Section 声明 `agent.Order*`（Persona10 / Env20 / Workspace30 / Memory40 / Knowledge50 / Skill60），删除 capability 本地 order 常量与零使用的 `OrderTodo`
  - 错误码 9105（技能脚本）归段 8000 → 8105；SSE hub 移除已删功能 `pet:*` 死订阅；i18n 移除桌宠死键
  - 文档：`API-CONTRACT.md` 补 `chat:tool-start` / `chat:queue-drained` 事件与 `details` 字段名、`/files` 根路径备注、`app:ready` 改单通道；spec 01 主循环伪码与 `MessageQueue` 接口对齐代码、spec 02 yolo 行、spec 03 拼接序与装配现状、spec 04 步骤复用语义、spec 08 移除 pwsh
- **文档-代码对齐第二轮（扫描 + 修复）**：
  - ExecutionMode 真正接线：`runTools` 弃用 `AllReadOnly` 只读启发式，改按工具声明裁决——批内任一 `Sequential` 整批串行，全部 Parallel（未声明视同）且 >1 走信号量并发；`file_write` / `file_edit` / `memory_write` 补声明 `Sequential`；`TestExecutionMode` 补强为阻塞探针（真正区分串行 / 并行）
  - 修 3 个 functools 参数名与结构体 tag 不匹配（schema camelCase `hasHeader` / `dropEmpty` / `groupBy` → snake_case，模型传参此前被静默丢弃）；`delegate_task` 的 schema 与描述改为真实内置 Agent 名 `default/explore`
  - 文档：`API-CONTRACT.md` 补 4 条漏列端点（settings 写入 ×3、`GET /kdocs/:id/file`）；`05-tool-system.md` 补 30 个 functools 目录 + 启停细则；新增 spec 20 子智能体委派 / 21 仓库导读 / 22 内置用户手册；DESIGN 设置子项 19→18；两份过程文档（ANALYSIS-PI-ALIGNMENT / REFACTOR-NOTES）归档至 `docs/archive/` 并加横幅
- **首发构建与自动化冒烟**：`wails build -nsis -ldflags "-s -w" -trimpath` 产出 `WorkBaby.exe` + `WorkBaby-amd64-installer.exe`，python 运行时随包；隔离 `WORKBABY_HOME` 首启冒烟通过（窗口创建且响应 / 健康与版本接口 / 14 页手册分发 / 50 工具启停键种子 / python 运行时解压 `ready`）；首启日志降噪（GORM `ErrRecordNotFound` 不再刷 error）
- **用户手册补全**：`assets/docs` 7 → 14 页（新增 记忆中心 / 子智能体与任务委派 / 用户钩子 / 仓库导读 / 文件与工作区 / 仪表盘·运行历史·后台任务 / 目标·计划·待办），随 `GET /api/v1/docs` 内嵌分发
- **文档对齐 + 前端残留清理**：全库文档与代码一致性核查修正（工具数统一为 20 业务 + 30 纯函数 = 50、设置中心 18 section、`AgentEvent` 14 类、9000 错误码段改「保留」）；删除桌宠 / 工作流文档残留与失效文件引用；`assets/docs/workflow.md` 与前端 Workflow 死类型 / 死键 / 死链移除；`stores/dashboard.ts` 的 `Stats` 契约对齐后端 `DashboardStatsRESP`；`KNOWN_PREFIXES` 补回 `/api/v1/folders`（修复 ChatInput 兜底目录树的 4040 运行时错误）；修复 en 字典 7 行缩进导致的 i18n-sync 误报
- **PI Phase 5 收尾**：
  - 事件载荷强类型化：`chat:retry` / `chat:error` / `chat:subagent-start` / `chat:subagent-done` / `chat:subagent-error` / `chat:queue-drained` 6 类事件改为 `domain.ChatRetryEvent` 等强类型载荷（前后端同形状）
  - 前端 chunk 拆分：`vite.config.ts` 加 `manualChunks`，主 chunk 1.3M → 272KB（5× 首屏加速）；mermaid / cytoscape / katex / echarts / markdown 各自成块按需加载
  - `go build` / `go vet` / `go test ./internal/...` / `npm run typecheck` / `npm test` / `check-boundaries.ps1` / `check-contract.ps1` 全部通过
- **PI 形态重构 Phase 3**：对齐 `pi-agent-core` 的 7 项差距（详见 `AGENTS.md §9.7`）。
  - 2 层循环：外层 wait follow-up / 内层 tool+steering（`agent/loop.go`）
  - 三回调分离：`TransformContext` / `ConvertToLlm` / `PrepareNextTurn`（旧 `BeforeTurn` 字段保留为兼容入口）
  - 队列：`SteeringQueue` / `FollowUpQueue` + `QueueMode`（`one-at-a-time` / `all`）
  - `GetAPIKey` 回调支持过期 key 刷新
  - 工具 `ExecutionMode`（`sequential` / `parallel`）替代 `Parallel + AllReadOnly` 启发式
  - 新增 `EventQueueDrained` 事件
  - service 简化：`chat_*.go` 6 文件（2062 LOC）合并为 `chat_run.go`；`tool/functools/tools.go` 拆为 `tools_data.go` / `tools_text.go` / `tools_io.go` 3 主题文件
  - 公开 API / HTTP 路由 / SSE 事件 / 数据库 schema / 前端契约 **零变化**
  - `go build` / `go vet` / `go test ./internal/...` / `check-boundaries.ps1` / `check-contract.ps1` 全部通过
- **PI 形态重构 Phase 2**：`internal/core/` 重命名为 `internal/agent/`（PI `pi-agent-core` 等价物）；34 个调用点同步更新；`scripts/check-boundaries.ps1` 的 `core-no-upward` / `core-no-http` 改名为 `agent-no-upward` / `agent-no-http`。`go build` / `go vet` / `go test` / `check-boundaries.ps1` / `check-contract.ps1` 全部通过。
- **PI 形态重构**（参见 `AGENTS.md §9`）：删除桌宠与孤立组件、合并 routes/api/functools fan-out、抽取 providerbase。`go build`、`go vet`、`go test ./internal/...`、`check-boundaries.ps1`、`check-contract.ps1` 全部通过。

### 计划
- 首次公开分发（安装包已可产出；剩余：代码签名与分发渠道）
- i18n 死键全量清扫（见 `TODO.md`）

## [0.1.0] — 当前

首个可用版本：Windows 桌面个人 AI 助手，具备完整的 Agent 对话与本地干活能力。

### 能力
- **Agent 内核**：显式 ReAct 主循环 + 可插拔护栏中间件链 + 确定性上下文压缩 + 检查点续跑 + 事件流
- **工具系统**：50 个内置工具（20 业务 + 30 纯函数）覆盖命令执行 / 文件读写 / 联网检索 / 文档解析 / 知识库 / 记忆 / 计划 / 委派 / 技能脚本
- **会话**：多会话、工作区绑定、权限四档、审批与补问可跨重启恢复、插话纠偏、目标模式自动续跑、后台任务
- **能力扩展**：Skill（三级来源）、MCP（stdio 客户端）、子智能体、自定义斜杠命令、用户生命周期钩子
- **本地数据**：SQLite（WAL + FTS5 trigram）落库、长期记忆、知识库 RAG、文件变更快照与回滚
- **桌面集成**：系统托盘、单实例保护、关闭到托盘、文件关联打开
- **可观测**：SSE 断线重放、token 用量逐调用落库、分段耗时归因、运行历史、仪表盘
- **界面**：晴空 / 紫夜双主题、设计令牌体系、18 页设置中心、40 屏可交互原型

### 已知边界
- 仅支持 Windows（托盘与单实例为 Win32 实现，非 Windows 为空实现）
- 单用户本机使用（HTTP 仅绑定回环 + 随机端口）
- 无向量语义检索（记忆与知识库依赖 FTS + 短查询子串兜底）
- 压缩为确定性折叠，摘要质量低于 LLM 摘要
