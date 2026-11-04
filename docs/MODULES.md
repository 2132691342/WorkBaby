# 模块 · MODULES

主进程按「叶子 → 类型 → 存储 → 业务 → 传输」分层，依赖只向下。
每个模块一节：职责 / 关键实现 / 为什么这么设计。

## pkg/（叶子工具，零业务依赖）

| 模块 | 职责与实现 |
|---|---|
| `apperror` | 业务错误：code 为数字字符串 + 用户可读 message + details 原话。边界上统一转信封 |
| `resp` | HTTP 信封与路由包装：恒 200，异常统一入信封；未知错误归 1000 并落日志 |
| `ulid` | 带前缀 ULID（RUN/ASK/EVT/JOB），时间序即排序序；白名单校验 |
| `fsutil` | 原子写（临时文件 + rename）、JSON 读写、目录保障、毫秒时间戳 |
| `log` | 按天滚动文件 + 控制台镜像；业务层只写 info/warn/error |
| `proc` | 唯一子进程出口：argv 数组直传、windowsHide、超时杀、逐行回调 |
| `env` | 子进程环境：PATH 内置运行时在前，HOME 锁数据目录 |
| `cmdline` | 命令行拆 argv（引号内不拆），给 MCP 页的 command_text 用 |
| `frontmatter` | markdown 头部字段读取，给 SKILL.md 用 |

## domain/（纯类型）

三个文件按域分：`types_core`（引擎/应用/偏好/引导总包）、`types_chat`
（块/历史轮/提问板/聊天状态）、`types_data`（会话/文件/git/模型/技能/插件/MCP）。
字段全部带业务前缀，与前端 `types/api.ts` 逐字段对齐——**接口契约唯一落点**。
另含 `errors.ts` 码表（与 docs/ERROR-CODES.md 一一对应）。

## repo/（SQLite）

`db.ts`：连接（WAL + busy_timeout）+ 手写迁移（user_version 步进，每步幂等）。
`settings_repo.ts`：KV 读写（事务批量）+ 工作区历史（置顶去重、遗忘联动当前）。
**为什么不用 ORM**：四张表 + 一条步进迁移，引框架是负资产。

## sse/（广播中心）

seq 单调发号，保留最近 1000 帧供 `since` 补帧，25s 心跳防断。
**为什么不用 WebSocket**：前端只需单向推送；EventSource + 查询参数令牌 +
自动补帧已覆盖全部场景，协议复杂度低一个量级。

## api/（传输薄壳）

`server.ts` 装配：json body(64mb，容纳图片 base64) → 令牌鉴权 →
`/api/v1` 挂五组路由 + `/events` → 404 兜底 → 异常兜底。
路由文件只做「解析请求 → 调 service → 包信封」，不写业务。

## service/（业务编排）

| 模块 | 职责与关键实现 |
|---|---|
| `bootstrap_service` | 引导总包 / 引擎状态 / 健康面板的视图组装（聚合计数与运行实况） |
| `settings_service` | 设置 KV、偏好（自启走系统登录项）、工作区、配置原文白名单读取 |
| `engine/runtime` | 运行时解压（zip 单根目录识别 + 改名落位）、node/npm/git/python/pi 路径与版本探测、npm 执行 |
| `app_service` | 应用行（pi + 三个运行时）与 pi 的安装/升级/卸载（内置 npm，进度广播） |
| `chat/` | `rpc_client`（JSONL 帧、请求按 id 关联、超时兜底）+ `chat_frames`（引擎事件 → SSE 词汇，实时块表）+ `chat_service`（绑定/驱动/状态组装/提问板） |
| `parser/` + `session_service` | 会话 JSONL → 条目树 → 活动分支 → 历史轮；清单/详情/改名/删除/隐藏台账/HTML 导出 |
| `skill_service` | 内置技能落位（已存在不覆盖，保住用户改动）、扫描、开关（搬目录）、增删改、项目技能复制 |
| `plugin_service` | 插件安装（npm --prefix 进沙箱）→ 找真实包 → 解析入口 → 写桥接 → 记注册表；开关 = 桥接目录改名 |
| `mcp_service` | 用户级/项目级 mcp.json 读写；command_text 拆 argv；开关改条目字段 |
| `model_service` | models.json / auth.json 读写 + 三种协议的远端模型探测（openai/anthropic/google） |
| `git_service` | 全套 git 面板操作，全部经内置 git.exe；porcelain v2 解析、numstat 合并、仓库状态检测 |
| `file_service` | 工作区文件浏览（路径越界校验、按名搜索、二进制识别） |
| `docs_service` | 内置帮助文档（文件名白名单） |

**为什么开关用「搬目录」而不是模式匹配**：引擎的资源发现规则以目录存在为准，
搬目录对任何引擎版本都成立，不依赖通配语义；禁用区就在旁边，用户肉眼可查。

## shell/（Electron 面）

`window`（无边框 1440×900，产物态加载前端 dist）、`bridge`（preload 合同 +
端点回填 + 关窗偏好）、`tray`（收托盘后的唯一入口，内联图标避免资源时序问题）。

## 前端（frontend/src）

| 模块 | 职责 |
|---|---|
| `api/` | axios 实例（信封拦截 → ApiError）+ 全部端点封装 + SSE 地址拼接 |
| `stores/` | app（引导/SSE 对账/会话/引擎）、chat（聊天状态与操作）、events（SSE 事件分发表）、prefs、agents |
| `composables/` | 输入草稿、附件、消息流渲染、运行状态、git 动作、模型表单等 |
| `utils/md.ts` | 流式 markdown 渲染（块级缓存，唯一出口） |
| `themes.css` 等 | 设计令牌唯一落点（见 DESIGN.md） |

## 依赖清单与取舍

| 依赖 | 为什么 | 替代方案与代价 |
|---|---|---|
| express 5 | 成熟、路由/中间件够用，无多余抽象 | 裸 http 手写路由表——省一个依赖但失去中间件生态，得不偿失 |
| better-sqlite3 | 同步 API 与单进程模型天然契合；预编译包免编译 | sql.js（纯 wasm，无文件锁语义）；node:sqlite（版本受限） |
| adm-zip | 纯 JS 解压三个运行时 zip，无原生依赖 | yauzl（API 更碎）；调 PowerShell Expand-Archive（闪窗风险） |
| ulid | 时间有序 ID，字典序即创建序 | uuid（无序，索引局部性差）；自造计数器（重启即乱） |
| esbuild | 主进程打包单文件，毫秒级构建 | tsc 直出（产物散、启动慢）；webpack（重） |
| vitest | 与 vite 共享转换管线，零配置跑 TS | jest（配置成本高、慢） |
| electron-builder | NSIS 打包 + extraResources 分发运行时 zip | 手写安装器脚本——不可维护 |

**明确不引入**：ORM / 迁移框架 / 状态机库 / 工作流引擎——规模撑不起，
规则写在 AGENTS.md 比藏在库里更直白。
