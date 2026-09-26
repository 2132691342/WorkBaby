# ANALYSIS-PI-ALIGNMENT.md · 全面调研与对齐决策

> **已归档**：历史过程记录，不再维护。结论已合并至 [`AGENTS.md §9`](../../AGENTS.md) 与 [`ARCHITECTURE.md`](../ARCHITECTURE.md)；现状以那两处为准。

> 本文件是 2026-09-26 大重构的**调研结论与决策基线**。先于代码改动固化方向，避免边写边漂移。
> 完成本轮后归入 `docs/ARCHITECTURE.md` 与 `AGENTS.md`，本文件不再维护。

---

## 1. 调研范围

### 1.1 参考项目（全部到位）

| 项目 | 路径 | 形态 | 调研重点 |
|---|---|---|---|
| **PI** | `D:\GitFiles\pi` | TypeScript (Node) | `pi-agent-core` 的 harness 设计、Agent 状态机、2 层循环、生命周期事件 |
| **go-micro** | `D:\GoFiles\go-micro` | Go | `agent/` 与 `ai/` 包结构；Go 风格的多 Provider、工具注册、MCP 子进程模式 |
| **ERP-AGENT** | `D:\GitFiles\ERP-AGENT` | Python (LangGraph) | 阶段状态机 / RubricMiddleware / CompositeBackend / 子 Agent 委派 |
| **PandaX** | `D:\GoFiles\PandaX` | Go 企业级 web | 工程分层、Viper/ORM 装配、Web 路由规范 |
| **gotool** | `D:\GoFiles\gotool` | Go 工具库 | 叶子工具包惯例 |

### 1.2 WorkBaby 当前结构

| 维度 | 现状 |
|---|---|
| 后端 Go 代码 | **40,019 LOC**（含测试）|
| 后端包数 | **23 个** (`internal/*`) |
| 前端 .vue | **45 个** |
| 前端目录 | 8 大域 (`api / chat / components / composables / i18n / markdown / stores / types / utils`) |
| 文档 | 7 篇 (`README` `DESIGN` `AGENTS` `PROJECT-SPEC` `ARCHITECTURE` `API-CONTRACT` `COMPONENT-GUIDELINES` `PAGE-STRUCTURE` `DEVELOPMENT` `DEPLOYMENT` `REFACTOR-NOTES`) |
| 规格 | **19 个**（按 `agent/capability/chat/system/tools` 5 域分文件）|
| 内置运行时 | 已精简到 **python 单资产**（REFACTOR-NOTES 已记录）|
| 平台支持 | **仅 Windows**（托盘 / 单实例为 Win32 实现）|

### 1.3 包粒度（按 LOC 排序）

| 包 | 文件数 | LOC | 评价 |
|---|---|---|---|
| `service` | 50 | 13,382 | **过细**：chat 8 文件、approval 712、context 578 均可合并 |
| `tool` | 38 | ~5,800 | 子包按域拆分清晰；functools 1228 行单文件偏大 |
| `domain` | 34 | ~3,500 | 与 AGENTS.md §2.3 「单文件多形态」一致 |
| `repo` | 24 | ~2,500 | 表对应；可接受 |
| `llm` | 22 | ~2,200 | providerbase 已抽取；可压缩 |
| `agent` | 16 | ~2,000 | 已 well-shaped；**待 PI 风格二次对齐** |
| `pkg` | 10 | ~1,500 | 叶子铁律，可接受 |
| `capability` | 9 | ~700 | 三通道契约稳定 |
| `server` | 8 | ~1,000 | 已合并；可接受 |
| 其他 | 14 | ~3,500 | — |

---

## 2. 当前架构与 PI 的差距分析

### 2.1 WorkBaby **已对齐** PI 的部分（无需重做）

| PI 设计 | WorkBaby 对应 | 状态 |
|---|---|---|
| 单 for 循环主内核 | `internal/agent/loop.go` 显式 for | ✅ |
| `BeforeTurn` / `AfterToolCall` 钩子 | `Hooks{}` 结构 | ✅ |
| `getSteeringMessages` / `getFollowUpMessages` | `Steering` / `FollowUp` 钩子 | ✅ |
| `shouldStopAfterTurn` | `ShouldStop` 钩子 | ✅ |
| Checkpoint + Resume | `checkpoint.go` + `Loop.Resume()` | ✅ |
| 事件流 | `Sink.Emit()` + 12 类 EventKind | ✅ |
| 拒绝 = 回执（`Refused`） | `tool.ToolResult.Refused` + `Meta["refused_reason"]` | ✅ |
| 幂等步骤记忆 | `MapSteps` + `RepeatGuard` | ✅（PI 没有）|
| 子 Agent 委派 | `delegate_core.go` 四重隔离 | ✅（PI 没有）|

### 2.2 WorkBaby **未对齐** PI 的部分（重构目标）

| 差距 | 当前位置 | PI 风格 | 影响 |
|---|---|---|---|
| 单层循环 + 内部 follow-up 拼接 | `loop.go` 用 `for turn { ... if fu continue }` | **2 层循环**：外层 wait follow-up → 内层 tool+steering | 长任务中途插话更稳健 |
| `BeforeTurn` 同时承担「转换 LLM」与「上下文裁剪」 | `BeforeTurn(ctx, turn, msgs)` | `convertToLlm` + `transformContext` 两个独立回调 | 关注点分离；测试更纯 |
| 压缩耦合在 `Compressor` 接口内 | `compress.go` | `prepareNextTurn` 回调（可装压缩 + 改 model + 改 thinking） | 同一回调位可表达更多「下一轮准备」|
| Steering / FollowUp 直接返回消息 | `Hooks.Steering(ctx)` | 队列 + `QueueMode: one-at-a-time | all` | 多条插话可批量入队 |
| 工具执行模式判定靠 `Parallel + AllReadOnly` | `Loop.cfg.Parallel` | `toolExecution: "sequential" | "parallel"` 工具级 `executionMode` | 更细粒度 |
| API key 编译期持有 | `Loop.provider` 固定 | `getApiKey(provider)` 回调（支持过期刷新）| 与 Anthropic OAuth 兼容 |
| `Agent` 状态化包装（state + listeners + queues）| 仅有 `Loop` 单次调用 | 持久 state + listener 订阅 | UI / store 多订阅者解耦 |

### 2.3 当前设计的复杂度问题（用户重点关注）

| 问题 | 现状 | 根因 |
|---|---|---|
| **service 50 个文件** | chat 拆 8 文件（chat/agent/finalize/prepare/runs/sessions/stream/task）| AGENTS.md §9.1 Phase 1 已合并 gates/usage，本轮未继续 |
| **domain 34 个文件** | 多为单 DO 一文件 | 与 §2.3 规范一致；可接受，但仍有优化空间（合并关联 DO） |
| **tool/functools/tools.go 1228 行** | 26 个工具工厂集中 | Phase 1 已从 12→2 文件；可再分主题到 3-4 文件 |
| **routes_*.go 11 文件** | 已合并到 1 个 `routes.go`（963 LOC）| ✅ |
| **api_*.go 24 文件** | 已合并到 3 文件 | ✅ |
| **AGENTS.md §9.5 Phase 2** | `core → agent` 已重命名（16 文件 / 34 调用点）| ✅ |
| **Phase 2 未做的项** | session/resource/settings/tool/builtin 5 包 | §9.6 评估后认为零行为变化 / 反向耦合 → 延后 |

---

## 3. 文档完备性检查

### 3.1 文档矩阵（现状）

| 文档 | 字数 | 与代码对齐 | 评价 |
|---|---|---|---|
| `README.md` | 7.5K | ✅ | 完整 |
| `DESIGN.md` | 7.0K | ✅ | 完整 |
| `AGENTS.md` | 26.5K | ✅（含 §9 PI 形态重构记录）| 完整 |
| `CHANGELOG.md` | 2.4K | ✅ | 完整 |
| `TODO.md` | 3.2K | ✅ | 完整 |
| `docs/PROJECT-SPEC.md` | 6.8K | ✅ | 完整 |
| `docs/ARCHITECTURE.md` | 13.0K | ✅ | 完整 |
| `docs/API-CONTRACT.md` | 12.7K | ✅ | 完整 |
| `docs/COMPONENT-GUIDELINES.md` | 6.4K | ✅ | 完整 |
| `docs/PAGE-STRUCTURE.md` | 6.1K | ✅ | 完整 |
| `docs/DEVELOPMENT.md` | 5.9K | ✅ | 完整 |
| `docs/DEPLOYMENT.md` | 5.5K | ✅ | 完整 |
| `docs/REFACTOR-NOTES.md` | 5.0K | ✅ | 完整 |
| `specs/features/*` (19 文件) | 1.9K 平均 | ✅ | 完整 |

**结论**：**文档与代码 100% 对齐**，无重大 gap。**问题在于：**
1. 文档说的是「PI 形态重构 Phase 1 + 2 step 1」，但代码本身还**未达到完整 PI 对齐**（§2.2 差距）
2. `AGENTS.md §9.5` / §9.6 标记的「未做的项」需要在本文档**显式说明本轮计划补完**

### 3.2 本轮需补的文档

| 文档 | 改动 | 优先级 |
|---|---|---|
| `AGENTS.md §9.5` | 更新为「本轮目标」：在 §2.2 表中列出的 7 项差距已对齐 | 高 |
| `AGENTS.md §2.2` | 增补 PI 2 层循环、`prepareNextTurn`、`QueueMode`、`getApiKey` 等概念 | 高 |
| `docs/ANALYSIS-PI-ALIGNMENT.md` | 本文件 → 重命名为最终对齐决策，纳入版本控制 | 高 |
| `docs/ARCHITECTURE.md §1` | 包结构图更新（合并 chat_* 等） | 中 |
| `docs/ARCHITECTURE.md §6` | 数据流图加 2 层循环与 `prepareNextTurn` 节点 | 中 |
| `specs/features/agent/01-react-loop.md` | 重写：2 层循环、`convertToLlm` / `transformContext` / `prepareNextTurn` 三回调 | 高 |
| `specs/features/agent/02-guard-chain.md` | 增补：为何保留 middleware 链 vs PI 简单回调（safety 多维度） | 中 |
| `specs/features/agent/03-context.md` | 增补 `transformContext` 与 `convertToLlm` 的边界 | 中 |
| `CHANGELOG.md` | 新增「[Unreleased] PI 形态重构 Phase 3」条目 | 高 |
| `TODO.md` | 标记 Phase 3 项完成；新建 Phase 4 | 中 |

---

## 4. 重构方向决策

### 4.1 不做什么（明确否定）

| 选项 | 否决理由 |
|---|---|
| **从零重写** | 40K LOC + 12 月沉淀 + 测试体系会全部归零；用户其实只要「更 PI」 |
| **引入 PI 的 Session / Resource / Settings / Tool/builtin 包** | §9.6 已评估：「包装型重构，零行为变化」属下次迭代 |
| **替换 middleware 链为 PI 风格的单一 before/after 回调** | WorkBaby 的安全护栏是 6 维度（Expose/Schema/Policy/Approval/Hook/Adaptive/Repeat）；单一回调会让审批门、路径信任、用户钩子互相污染 |
| **全量用 LangGraph / DeepAgent** | 引入 Python 依赖违反「仅 Windows + 纯 Go」原则 |
| **删除 skill / mcp / 子 Agent / 用户钩子** | 用户明确「扩展 skill mcp」，且这些是产品价值核心 |

### 4.2 本轮要做什么（明确肯定）

| 项 | 目标 |
|---|---|
| **2 层循环** | 外层 wait follow-up → 内层 tool+steering；目标模式的自动续跑与检查点续跑统一进外层 |
| **`convertToLlm` / `transformContext` / `prepareNextTurn` 三回调分离** | `BeforeTurn` 拆为两个：`transformContext`（上下文裁剪）和 `convertToLlm`（协议归一）|
| **`prepareNextTurn` 回调** | 接管压缩 + 模型/思考档切换；与 `BeforeTurn` 并存不互斥 |
| **`QueueMode` 队列** | Steering / FollowUp 改为 `one-at-a-time | all` 队列；可批量 |
| **`getApiKey(provider)`** | 适配 Provider 可换 key 的场景 |
| **`toolExecution` 工具级模式** | 工具元数据 `ExecutionMode: "sequential | parallel"`，取代全局 `Parallel + AllReadOnly` 启发式 |
| **service 文件合并** | chat_* 8 文件合并为 3（chat.go / chat_stream.go / chat_recovery.go） |
| **functools 主题分文件** | `tools.go` 1228 行拆为 `tools_io.go` / `tools_text.go` / `tools_data.go` |
| **保留 middleware 链** | 6 维度安全护栏不动；理由写入 `02-guard-chain.md` |

### 4.3 范围与顺序

```
Phase 1（文档先行，半天）
  ├─ AGENTS.md 更新 PI 对齐清单
  ├─ specs/agent/01 重写为 2 层循环
  ├─ specs/agent/02 增补 middleware 链保留理由
  ├─ CHANGELOG / TODO 更新
  └─ ANALYSIS-PI-ALIGNMENT.md 落版

Phase 2（agent 包对齐，1-2 天）
  ├─ loop.go: 单层 → 2 层
  ├─ 增补 convertToLlm / transformContext / prepareNextTurn 回调
  ├─ 增补 QueueMode 队列
  ├─ Loop.Run 签名扩展（不破坏现有调用）
  └─ loop_test.go / helpers_test.go 同步

Phase 3（service 与 tool 简化，1-2 天）
  ├─ chat_* 8 → 3
  ├─ functools/tools.go → 3 主题文件
  ├─ approval/context/chat 等过 500 行的文件按域内聚合
  └─ 单文件测试函数数 ≤6 / 全量 <10s 不变

Phase 4（验证，半天）
  ├─ go build ./...
  ├─ go vet ./...
  ├─ go test ./internal/... -count=1 -timeout 60s
  ├─ powershell scripts/check-boundaries.ps1
  ├─ powershell scripts/check-contract.ps1
  └─ cd frontend && npm run typecheck && npm test
```

### 4.4 风险与回退

| 风险 | 缓解 |
|---|---|
| 2 层循环破坏现有 ReAct 测试 | loop_test.go 同步更新用例；保留单层调用兼容入口 `RunOnce(ctx, history)` |
| 回调拆分改变 Hooks 公开字段 | 拆为 Hooks + Callbacks 双结构；Hooks 保留旧字段标记 Deprecated，新代码走 Callbacks |
| service 文件合并破坏 import | 全量 grep `internal/service/chat_*` 调用方 → 同步迁移；保留包级别名兼容 |
| 测试时间变长 | 微压缩规则不变；functools 单测随文件拆分整体迁移 |

---

## 5. 决策矩阵（一页速览）

| 决策点 | 选择 | 理由 |
|---|---|---|
| 重写 vs 演进 | **演进** | 40K LOC 重写无收益 |
| PI 风格采纳范围 | **2 层循环 + 3 回调 + 队列**（7 项）| §2.2 表 |
| Middleware 链 | **保留** | 6 维度安全护栏不可妥协 |
| 引入新包（session/resource/settings）| **不做** | §9.6 评估后延后 |
| 前端重构 | **本轮不动** | 后端契约稳定；前端仅同步字段 |
| 内置运行时 | **仅 python**（已落地）| REFACTOR-NOTES |
| 平台支持 | **仅 Windows**（已落地）| AGENTS.md §9.6 |

---

## 6. 关联文档

- `AGENTS.md §9` PI 形态重构记录（本文件后纳入 §9.7 Phase 3）
- `docs/ARCHITECTURE.md` 包结构（本轮同步更新）
- `specs/features/agent/01-react-loop.md` 重写为 2 层循环
- `specs/features/agent/02-guard-chain.md` 增补 middleware 保留理由
- `CHANGELOG.md` 新增 Phase 3 条目
- `TODO.md` 进度滚动