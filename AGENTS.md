# AGENTS.md · WorkBaby 工程规范

> 本文件是代码风格 / 分包 / 依赖方向 / 工作流的唯一权威。
> 架构与模块说明见 `docs/`；冲突时以本文件为准。

---

## 1. 定位与技术栈（强制锁定）

WorkBaby 是一个**安装即用的 Windows 单机 AI 助手**：Electron 壳把
**内置 Node.js + pi 引擎 + 插件 + 技能 + MCP + Git/Python 运行时**装进一个
`%LOCALAPPDATA%\WorkBaby` 沙箱，由 GUI 统一调用与管理。外部装没装
node / pi 与本壳无关；壳内管理的 pi 也不支持绕过 GUI 直接使用。

| 层 | 选型 | 版本 |
|---|---|---|
| 桌面壳 | Electron（Frameless，前端自绘标题栏，Windows WebView 层） | 43.x |
| 语言 | TypeScript（strict，esbuild 打包主进程为 CJS 单文件） | 5.6 |
| 业务服务 | express（业务 API + SSE 全部走 HTTP，127.0.0.1 随机端口） | 5.x |
| 数据库 | better-sqlite3（WAL，手写迁移 `PRAGMA user_version` 步进） | 13.x |
| ID | ULID，带业务前缀 | — |
| Agent 引擎 | pi（`@earendil-works/pi-coding-agent`，RPC 模式子进程） | 随 npm 最新 |
| 内置运行时 | node v24.21.0 / PortableGit / Python 3.13（zip 随包分发，首启解压） | — |
| 前端 | Vue 3.5 + Vite 6 + Element Plus 2.14 + Pinia + Vue Router(hash) + Axios | — |
| 前端辅件 | UnoCSS（刻度接 `--wb-*`）· ECharts · CodeMirror 6 · Vue I18n | — |
| 测试 | vitest（仓库根 `tests/`） | — |

**平台**：只考虑 Windows；子进程一律 `windowsHide`（即 CREATE_NO_WINDOW）。

### 1.1 沙箱原则（本壳的存在意义）

- 引擎与工具链全部落在数据目录：`data\runtime\{node,PortableGit,python}`、
  `data\pi\{settings,models,mcp,auth,sessions,skills,extensions,plugins}`、`data\pi-app`（pi 本体）。
- 子进程 PATH **内置运行时在前、系统在后**（`pkg/env.ts::toolEnv`），HOME 指向数据目录；
  agent 的 bash / python 一律先命中内置副本，不碰用户环境。
- pi 的整个家目录由 `PI_CODING_AGENT_DIR` 重定向；`PI_PACKAGE_DIR` 指到沙箱内。
- 管理面（插件 / 技能 / MCP / 模型）只经 GUI 暴露，不对外提供 CLI。

---

## 2. 强制编码规范

### 2.1 目录结构

```
WorkBaby/
├── AGENTS.md / README.md / DESIGN.md
├── docs/                          # 架构 / API / 数据 / 部署等（与代码同步更新）
├── electron/
│   ├── main/
│   │   ├── index.ts               # 装配根：上下文→日志→DB→HTTP→托盘→窗口，重活后台
│   │   ├── context.ts             # 全局运行时：路径布局、鉴权令牌、端口（装配后只读）
│   │   ├── domain/                # 纯类型：VO/REQ/RESP + 错误码常量（不依赖实现）
│   │   ├── pkg/                   # 叶子工具：apperror/resp/ulid/fsutil/log/proc/env/cmdline/frontmatter
│   │   ├── repo/                  # SQLite：连接迁移、设置 KV
│   │   ├── sse/                   # SSE hub：seq 发号、断线补帧、心跳
│   │   ├── api/                   # express：server + 五组路由（薄壳，只解析与转发）
│   │   ├── service/               # 业务编排：settings/docs/file/git/session/skill/plugin/mcp/model/app/bootstrap
│   │   │   ├── engine/runtime.ts  # 运行时路径与版本探测、npm 执行
│   │   │   ├── parser/            # pi 会话 JSONL → 条目树/活动分支
│   │   │   └── chat/              # rpc_client / chat_frames / chat_service
│   │   └── shell/                 # 窗口 / preload 桥 / 托盘
│   └── preload/index.ts           # window.wb 桥（频道名是合同）
├── frontend/src/{api,components,composables,locales,stores,types,utils,views,router}
├── assets/{docs,runtime,skills}   # 编译期随包分发：帮助文档、运行时 zip、内置技能包
├── scripts/{dev.mjs,build-main.mjs}
└── tests/                         # vitest：场景测试，按被测对象一文件
```

**开目录门槛**：有一组独立概念 + 独立生命周期才开目录，否则做现有模块里的一个文件。
每个文件 ≤ 500 行（强制），超了就拆。

### 2.2 依赖方向（强制）

```text
index(装配) ──► shell ──► api ──► service ──► {repo, engine/runtime, chat, parser}
                                   │
                    service ──► pkg / domain（只读类型）
```

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| pkg | 标准库 / 第三方纯工具 | domain / repo / service / api / electron |
| domain | pkg | 任何实现层 |
| repo | pkg / domain | service / api / electron |
| service | repo / pkg / domain / electron(app 等) | api 的路由层 |
| api | service / domain / pkg | 越过 service 直摸 repo |
| shell | electron / service（仅设置与端口回填） | api |
| 前端 | 自身 + `window.wb` + `/api/v1` | 直摸文件系统 / 子进程 |

**组合根例外**：`index.ts` 直接开库与起服务是装配本职；`shell/bridge.ts` 读
settings_service 的关窗偏好同理。

### 2.3 domain 单文件多形态

一个域一个类型文件，VO / REQ / RESP 同居，字段一律带业务前缀：

```ts
/** 会话列表行。 */
export interface SessionRowVO {
  /** 会话 ID */
  session_id: string
  /** 标题（首条用户消息截断） */
  session_title: string
}
```

| 后缀 | 用途 |
|---|---|
| `VO` | 给界面看的视图对象（RESP 载荷） |
| `REQ` | 入参（POST body / query） |
| `DO` | 持久化行映射（repo 内） |

**禁止**：裸字段名（`name` / `code` 一律拒绝）；前端字段与后端 JSON 字段名不一致。

### 2.4 错误处理（强制 AppError，code 为字符串）

```ts
export class AppError extends Error {
  /** 业务码（数字字符串） */
  code: string
  /** 人话描述（直接给用户看） */
  message: string
  /** 补充上下文（stderr / 路径 / 上游响应原话） */
  details?: string
}
```

- 业务码是**数字字符串**（`"1001"`）；HTTP 状态码是 number，信封 `code` 是 string
- HTTP 出口统一信封 `{ code, message, detail?, data }`，恒回 200，`code !== "0"` 即业务失败
- 码值集中在 `domain/errors.ts`，与 `docs/ERROR-CODES.md` 一一对应；新增码先改文档再写代码

**错误码段位**：1000–1999 通用/文件/git；2000–2999 配置/DB；3000–3999 服务商/模型；
5000–5999 会话运行；7000–7999 外部进程；8000–8999 Skill；9100–9199 MCP；9700–9799 引擎 RPC。

### 2.5 命名 / 注释 / 行数

| 类别 | 规则 |
|---|---|
| 文件名 | 全小写 + 下划线（`rpc_client.ts`） |
| 类型 / 接口 | PascalCase，不加 I 前缀 |
| 函数 | camelCase；常量 SCREAMING_SNAKE_CASE |
| JSON 字段 | snake_case + 业务前缀 |
| ID | `{PREFIX}_{26位ULID}`；现役前缀 `RUN` / `ASK` / `EVT` / `JOB`，白名单在 `pkg/ulid.ts` |
| 时间戳 | 一律毫秒 number（`pkg/fsutil::nowMs`）；禁止 ISO 串进 DTO |
| 注释 | 文件头 1 行 `//!`；类型 / 方法 1 行 `///` 说结论；连续注释 ≤ 3 行；只写「为什么」 |
| 行数 | 每文件 ≤ 500 行 |
| **禁止** | 注释出现外部项目名 / 过程痕迹（「旧版」「v1」「预留」）；`uuid`；手拼 JSON；本地时区时间 |

### 2.6 子进程纪律

- argv 数组直传，永不进 shell；`windowsHide: true`（引 `pkg/proc`，不写十六进制）
- 超时必杀；stdout 严格按 LF 切帧（不用 readline——U+2028/2029 会误切）
- 探测程序路径用 `existsSync`，**禁止跑 `where` / `which`**（会闪控制台）
- 新增子进程出口自觉对齐 `pkg/proc::runProc`

### 2.7 HTTP 与 API 路径约定（强制）

- **只允许 GET / POST**；动词靠路径段表达：`POST /sessions/delete` + `{ path }`
- 路径形状 `/{resource}/{action}`；查询参数 `?key=value`；前端统一 `/api/v1` 前缀
- 鉴权：HTTP 走 `X-Desktop-Token` 头；SSE 走 `?token=`（EventSource 带不了自定义头）

### 2.8 服务不变量（强制）

| 不变量 | 含义 |
|---|---|
| 会话文件是真相 | pi 的 JSONL 会话文件是唯一持久真相；界面状态重启后由回放重建 |
| 事件顺序是契约 | SSE 帧 `{seq, event, data}`，seq 单调；改 `chat:*` 词汇等于改界面行为 |
| 一个工作区一个引擎 | pi RPC 子进程按当前工作区绑定；切工作区即重建 |
| 看得见就点得动 | 提问等面板可写；回答经 `extension_ui_response` 直达引擎 |
| 写入只有一套 | 插件 / 技能 / MCP 的落盘与校验只有 service 一套，不写第二套 |
| 启动不等重物 | 端口与界面先就绪；运行时解压、技能落位后台跑，进度经 `app:progress` 报 |
| 引擎不感知界面 | service/chat 只发 domain 词汇；翻译成 SSE 是 chat_frames 的职责 |
| 密钥不回显 | `api_key` 读出来恒为掩码；空串语义是「保留原值」 |

### 2.9 前端设计令牌（强制）

- **色值只在 `themes.css` 定义**：组件只允许 `var(--wb-*)`，禁止写死色值
- 尺寸走刻度：间距 `--wb-sp-*`、字号 `--wb-fs-*`、圆角 `--wb-radius-*`、动效 `--wb-dur-*`
- UnoCSS 刻度已接令牌（`gap-4` = `var(--wb-sp-4)`）；Element Plus 变量由 `ep-theme.css` 接桥
- **按钮五态齐全**：默认 / 悬停（放大 1.06）/ 按下（缩小 0.92）/ 禁用 / 加载；自绘走 `.wb-btn`
- 引用不存在的 CSS 变量会**静默失效**——那是丢样式，不是「不好看」
- 交互反馈：所有异步动作要有 loading / 过渡；空态给引导；错误给可读文案（后端 `message` 直显）

---

## 3. 测试规范

测试的价值在「改这里会坏那里」的地方；平凡断言不写。

**保留**：状态机与解析（会话树回放 / 分支 / 隐藏轮）、持久化往返（设置库写读）、
纯函数边界（命令行拆分 / frontmatter / 差异解析 / ULID 校验）。
**不写**：复述框架行为的用例、单分支覆盖率驱动用例。

- 位置：仓库根 `tests/`，一文件一个被测对象，`npx vitest run` 十几秒内跑完
- 命名说清断言的是什么；断言带失败信息
- electron 依赖模块不进测试；可测的纯逻辑抽到 `pkg/`
- 测不了的路径（需真实引擎 / 网络）不写悬空用例

---

## 4. 架构门禁

提交前必须过四道，一条不能少：

```bash
npm run typecheck        # 1 主进程 TS 严格检查
npm run build            # 2 主进程 esbuild + 前端构建（内含 vue-tsc）
npx vitest run           # 3 测试全绿
npm run dist             # 发布打包（electron-builder NSIS），发布前另跑
```

**硬性约束**：文件 ≤ 500 行；依赖方向单向；只用 GET / POST；字段带业务前缀；
信封恒 200；注释无外部项目名与过程痕迹；子进程 windowsHide + 超时杀。

---

## 5. 工作流

1. **先读约束**：动 domain 前想清楚字段前缀与错误码段位；动前端前想清楚令牌归属
2. **定位落点**：能进现有模块就别开新文件；纯逻辑进 `pkg/`（可测），编排进 `service/`
3. **实现**：注释只写「为什么」，不留过程痕迹
4. **验证**：跑 §4 的门禁；改哪个层跑哪道，但 typecheck + test 必跑
5. **同步文档**：改了架构 / API / 错误码 / 页面结构，对应 `docs/` 一起改
6. **提交**：一次提交只做一件事，提交信息写清做了什么与为什么

**新增依赖的门禁**：写清「为什么需要」「替代方案是什么」「代价是什么」，
并同步到 `docs/MODULES.md` 与本文 §1 的技术栈表。
