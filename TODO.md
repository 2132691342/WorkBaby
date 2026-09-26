# TODO

开发计划与当前进度。已完成的能力见 [`README.md`](README.md) 与 [`CHANGELOG.md`](CHANGELOG.md)。

## 进行中

| 项 | 说明 | 状态 |
|---|---|---|
| GUI 交互细验 | 自动化冒烟已过（窗口创建且响应 / HTTP 健康 / 首启种子 / 运行时解压）；剪贴板写入、JSON 折叠手感两项纯交互待随手验一次 | 可选人工 |

## 已完成

| 项 | 说明 |
|---|---|
| 面向新手收敛 | 设置中心 18→12 section（3 组：模型 / 能力 / 系统），移除 命令 / 钩子 / 文件 / 仪表盘 / 运行历史 入口，仓库导读（Wiki）整体下线（含 Go 侧与 spec 21），exec 白名单编辑器移除；工具页补 `functools` 组使 50 个工具真实可见（此前列表只画 20 行）；手册 14→12 页；i18n 清 100 死键；门禁与前后端测试全绿 |
| 文档-代码对齐第三轮 | 调研 5 个参考项目（pi / go-micro / ERP-AGENT / PandaX / gotool），取舍结论归档 `docs/REFERENCE-PROJECTS.md`；修复 6 处规格漂移（yolo 对 destructive 改回 ask、步骤记忆真正接线 `RepeatGuard(3, loop.StepsStore())`、能力段显式 `agent.Order*`、主循环伪码 / 队列接口 / pwsh 表述）；`API-CONTRACT.md` 补 2 事件 + details 字段 + /files 根路径 + app:ready 单通道；错误码 9105→8105；清理 pet 死订阅与约 90 条 i18n 死键；`TestRepeatGuardStepReuse` 锁定复用语义 |
| 首发构建 + 自动化冒烟 | `wails build -nsis -ldflags "-s -w" -trimpath` 产出 `build/bin/WorkBaby.exe`（68M）+ `WorkBaby-amd64-installer.exe`（67M），python 运行时随包；隔离 `WORKBABY_HOME` 首启冒烟通过：窗口创建且响应、健康 / 版本接口 OK、14 页手册在列、50 工具启停键种子落库、python 3.12.13 解压 `ready=true`；首启日志降噪（`IgnoreRecordNotFoundError`） |
| 用户手册补全 | `assets/docs` 7 → 14 页：新增 记忆中心 / 子智能体与任务委派 / 用户钩子 / 仓库导读 / 文件与工作区 / 仪表盘·运行历史·后台任务 / 目标·计划·待办；`GET /api/v1/docs` 实测 14 页全部在列 |
| 文档对齐扫描 | 全库文档 ↔ 代码一致性核查：清理桌宠 / 工作流残留与失效文件引用（chat_prepare / chat_finalize / routes_<域> / core/ 测试路径），统一工具数（20 业务 + 30 纯函数 = 50）、设置中心 18 section、AgentEvent 14 类口径；`internal/resource` 落地状态回写；删除无主遗产 `assets/docs/workflow.md` 与前端 Workflow 死类型 |
| PI 形态重构 Phase 3 | 7 项 PI 差距已对齐：2 层循环 / 三回调（TransformContext / ConvertToLlm / PrepareNextTurn）/ QueueMode 队列 / GetAPIKey / 工具 ExecutionMode / EventQueueDrained。`chat_*.go` 6 文件合并为 `chat_run.go`；`tool/functools/tools.go` 拆为 3 主题文件。`go build` / `go vet` / `go test` / `check-boundaries.ps1` / `check-contract.ps1` 全部通过。详见 `AGENTS.md §9.7` |
| PI 形态重构 Phase 2 | `internal/core/` → `internal/agent/` 重命名（16 文件 + 34 调用点）；`service/chat_task.go` 变量名 `agent` → `agentName`（避包名冲突）。详见 `AGENTS.md §9.5` |
| PI 形态重构 Phase 1 | 删除桌宠整包 + 14 个孤立前端目录；合并 `internal/server/routes_*.go` 11→1、`internal/api/api_*.go` 24→3、`internal/tool/functools/` 12→2、`service/chat_*.go` 12→10、`domain/` 5→3；抽取 `internal/llm/providerbase.go`。`go build` / `go vet` / `go test` / `check-boundaries.ps1` / `check-contract.ps1` 全部通过。详见 `AGENTS.md §9` |
| 文档体系重构 | 迁移到 `AGENTS.md` + `docs/` + `specs/features/` 结构；旧 `doc/` 与 `CLAUDE.md` 已移除 |
| 测试整理 | 删除弱断言与重复覆盖（26→24 文件）；补齐文件头导航注释；全量 5 秒内 |
| 注释精简 | 连续注释块全部收敛到 ≤3 行（≥6 行 3 处、4-5 行 105 处清零）；清除改动过程叙事与外部项目名 |

## 待办

### 功能
| 项 | 说明 | 优先级 |
|---|---|---|
| ~~用户手册补全~~ | ~~`assets/docs/` 7 → 14 页，覆盖全部设置 section 对应功能~~ | ✅ 完成 |
| ~~首次发行包~~ | ~~NSIS 安装包 + 内置运行时（python）归档随包校验~~ | ✅ 完成 |
| 过程卡交互增强 | 大 JSON 折叠层级、工具输出复制（已完成）；候选：输出内搜索、逐行折叠 | 低 |
| ~~事件载荷强类型化~~ | ~~剩余 6 类已强类型化（Phase 5）~~ | ✅ 完成 |

### 工程质量
| 项 | 说明 | 优先级 |
|---|---|---|
| PI 形态收尾：新建 session / settings 包 | 见 `AGENTS.md §9.6`；`internal/resource/` 已落地，session / settings 待需求驱动 | 中 |
| i18n 死键全量清扫 | `i18n-sync.mjs dead` 现报约 778 个死键（多轮重构累积）；清扫前需先排除动态构造键（如 `` t(`tool.verb.${kind}`) ``），按前缀分批清理 | 低 |
| 迁移到 golang-migrate | 当前为 GORM AutoMigrate（v1 策略）；出现破坏性变更前完成 | 低 |
| ~~前端 chunk 拆分~~ | ~~Phase 5 完成：主 chunk 1.3M → 272KB；mermaid / cytoscape / katex / echarts / markdown 各自成块按需加载~~ | ✅ 完成 |

### 已知限制（非缺陷，设计取舍）
| 限制 | 说明 |
|---|---|
| 单会话串行 | 同一会话内不能并行两个 run（可插话纠偏）；跨会话天然并行 |
| SQLite 单连接 | 写吞吐受限、长查询阻塞写；换桌面单用户场景的写锁确定性 |
| 无向量检索 | 记忆与知识库靠 FTS5 trigram + 短查询子串兜底，不做语义相似度 |
| 确定性压缩 | 零成本零延迟，但摘要质量低于 LLM 摘要 |
| 检查点仅最新轮 | 存储恒定，无法回放到更早轮次 |
| 仅 Windows | 托盘 / 单实例为 Win32 实现（桌宠已删除） |
