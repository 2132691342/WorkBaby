# AGENTS.md · WorkBaby 工程规范

> 本文件是代码风格 / 分包 / 依赖方向 / 工作流的唯一权威。
> 架构与模块说明见 `docs/`；冲突时以本文件为准。

---

## 1. 技术栈（强制锁定）

| 层 | 选型 | 版本 |
|---|---|---|
| 语言 | Rust（edition 2021，stable） | 1.85+ |
| 桌面壳 | Tauri | 2.x（Frameless，前端自绘标题栏；Windows 用 WebView2） |
| HTTP 服务 | axum | 0.8.x（业务 API + SSE 全部走 HTTP，端口 127.0.0.1 随机） |
| 异步运行时 | tokio + tokio-util（CancellationToken） | 1.x |
| 序列化 | serde / serde_json | 1.x |
| HTTP 客户端 | reqwest（rustls，关默认特性）+ eventsource-stream | 0.12.x |
| 数据库 | rusqlite（bundled，含 FTS5；无系统依赖） | 0.32.x |
| ID | ULID（ulid crate），带业务前缀 | 见 §2.7 |
| 日志 | tracing + tracing-subscriber + tracing-appender | 分文件落盘 |
| 错误 | thiserror + 自研 AppError（code 为字符串） | 见 §2.4 |
| MCP 客户端 | rmcp（官方 Rust SDK，stdio + streamable HTTP） | 0.x |
| 模糊匹配 | nucleo | 0.5.x |
| HTTP 中间件 | tower-http（CORS，仅放行本机来源与 Tauri webview） | 0.6.x |
| 内置资源打包 | include_dir | 0.7.x |
| 前端 | Vue 3.5 + TypeScript 5.6 + Vite 6 | — |
| 状态管理 | Pinia ^2.3（Setup Store） | — |
| 路由 | Vue Router ^4.5（hash 模式） | — |
| HTTP 客户端 | Axios ^1.7 | — |
| UI 组件库 | Element Plus ^2.14（按需引入；文案跟随 i18n） | — |
| 原子 CSS | UnoCSS ^66（刻度全接 `--wb-*` 令牌） | — |
| 图表 | ECharts ^6（按需引入；色值走 `--wb-ch-*`） | — |
| 代码编辑器 | CodeMirror 6（markdown / json） | — |
| 国际化 | Vue I18n ^9.14（zh-CN 默认 + en） | — |
| 渲染 | marked · dompurify · highlight.js | markdown 唯一出口 `utils/md.ts` |
| 文档预览 | vue-pdf-embed · docx-preview · XLSX | 动态导入，按需加载 |
| 图可视化 | AntV X6 ^2.18 · BPMN-JS ^16.4 | 动态导入，按需加载 |
| 富文本 / 微前端 | WangEditor ^5.1 · Qiankun ^2.10 | 依赖已就位，按需接线 |
| 平台 | Windows | 只考虑 Windows；平台差异用 `#[cfg(windows)]`，不写运行时大分支 |

### 1.1 依赖原则

标准库 → 生态成熟 crate。新增依赖必须说明理由与替代方案，写入 `docs/MODULES.md` 对应小节。

**明确不引入**：工作流引擎、ORM、迁移框架、全局可变状态。
SQLite 手写迁移 SQL（`PRAGMA user_version` 步进）。

重型前端依赖（PDF、表格、文档、图谱）一律**动态导入**并单独分块，
不进首屏包——不进对应页面就不该下载它的代码。

### 1.2 内核归本仓实现

Agent 内核是本仓 Rust 实现（会话循环 / 工具执行 / 上下文压缩 / 重试 / 子代理），
进程内运行，不依赖任何外部 agent 进程，也不需要 Node 运行时。

扩展能力不设运行时插件市场，改为 **12 项原生化内置功能**（见 `docs/MODULES.md`
的 wb-features 小节），每项独立开关；**22 个内置技能**作为技能包打进二进制。

| 概念 | 落地形态 |
|---|---|
| Agent 内核 | `wb-kernel` crate（进程内） |
| 扩展能力 | 内置功能（同一组 `chat:*` SSE 词汇做 UI 桥） |
| 会话持久化 | 本仓自有 JSONL 会话文件（树形条目，可分支编辑） |
| 运行时扩展 | 不存在——功能开关即装配边界 |

---

## 2. 强制编码规范

### 2.1 包结构（Cargo workspace）

```
WorkBaby/
├── AGENTS.md / README.md / DESIGN.md
├── docs/                          # 架构 / 模块 / 数据模型 / API / 页面 / 错误码 / 开发 / 部署
├── frontend/                      # Vue 3 前端（唯一源码目录 frontend/src）
│   └── src/{api,components,composables,locales,stores,types,utils,views,router}
├── src-tauri/                     # Tauri 壳：入口、托盘、单实例、自启、对话框、窗控
│   ├── tauri.conf.json
│   └── src/{main.rs, lib.rs, cmds.rs}
├── assets/                        # 编译期打包进二进制的资源
│   ├── docs/                      # 内置帮助文档
│   ├── runtime/                   # 内置 PortableGit 与 Python 归档（zip）
│   └── skills/                    # 22 个内置技能包
└── crates/
    ├── wb-pkg/                    # 叶子工具：AppError、ulid、fsutil、realpath、ansi、httprules
    ├── wb-domain/                 # 纯类型：DO/DTO/REQ/RESP/VO + 错误码常量（只依赖 wb-pkg）
    ├── wb-repo/                   # SQLite：连接与迁移、设置 KV、技能/功能开关、会话索引、统计
    ├── wb-provider/               # 模型接入：wire API（openai-completions）、SSE 解析、usage、凭据
    ├── wb-tools/                  # Tool trait + 注册表 + 内置工具（read/bash/edit/write/grep/find/ls）
    ├── wb-kernel/                 # Agent 内核：会话树、循环、事件、压缩、重试、子代理
    ├── wb-skills/                 # SKILL.md 解析、三区发现注册表、内置技能包
    ├── wb-features/               # 12 项内置功能（todo/ask_user/memory/web/context_mode/
    │                              #   fff/lsp/plan/goal/loadout/inspect/context_view）
    ├── wb-git/                    # 工作区版本视图（读 status/diff/历史；写限白名单动作）
    ├── wb-mcp/                    # MCP 客户端编排（rmcp）：配置、连接池、工具并入
    ├── wb-service/                # 业务编排：对话 run、事件翻译、提示词组装、目录与装配
    └── wb-api/                    # axum 路由 + SSE hub + CORS + 组合根 Startup
```

**开包门槛**：有自己的一组概念 + 独立生命周期才开 crate，否则做 `wb-service` 里的一个模块。
每个 crate 内文件 ≤ 500 行（强制），超了就拆模块文件。

### 2.2 依赖方向（强制，由 Cargo 编译期保证）

```text
src-tauri ──► wb-api ──► wb-service ──► {wb-kernel, wb-features, wb-skills, wb-git, wb-mcp, wb-repo, wb-provider, wb-tools}
                                            │
              wb-kernel ──► {wb-tools, wb-provider, wb-domain, wb-pkg}
              wb-features ──► {wb-tools, wb-domain, wb-pkg}
              wb-skills / wb-git / wb-mcp ──► {wb-domain, wb-pkg}
                                            │
                              wb-provider ──► {wb-domain, wb-pkg}
                                            │
                                wb-domain ──► wb-pkg（叶子）
```

| crate | 允许依赖 | 禁止依赖 |
|---|---|---|
| wb-pkg | 标准库 / 第三方纯工具 | 任何其他 wb-* crate |
| wb-domain | wb-pkg | 其他 wb-* crate |
| wb-repo | wb-domain / wb-pkg / rusqlite | 服务层及以上、能力域 |
| wb-provider | wb-domain / wb-pkg | 内核 / 服务层 / 能力域 |
| wb-tools | wb-domain / wb-pkg | 内核 / 服务层 |
| wb-kernel | wb-tools / wb-provider / wb-domain / wb-pkg | 服务层 / api / 能力域 |
| wb-features | wb-tools / wb-domain / wb-pkg | 服务层 / api / wb-kernel |
| wb-skills · wb-git · wb-mcp | wb-domain / wb-pkg（wb-mcp 另加 rmcp） | 服务层 / api / 互相 |
| wb-service | 全部能力域 + wb-domain + wb-pkg | api / tauri / axum |
| wb-api | wb-service / wb-repo / wb-kernel / wb-domain / wb-pkg / axum / tokio | 越过 service 直摸 provider / tools / skills / features / git / mcp |
| src-tauri | wb-api | wb-service / wb-repo / 能力域 |

**组合根例外**：api 层直接开库（`Repo::open`）与操作会话文件（`SessionFile` / `Entry`）
是 api 的本职，不是越界；除此之外，能力域一律经 service。

**为什么这么切**：`wb-domain` 是叶子，类型定义不被任何实现污染；
`wb-service` 是唯一的编排层，能力域之间互不相识（features 不知道 kernel 的存在）；
`wb-api` 是唯一知道 HTTP 的地方。这三条让「换掉传输层」或「加一个新能力域」都只动一个 crate。

### 2.3 domain 单文件多形态

一个聚合根一个文件，DO / DTO / REQ / RESP / VO 同居，字段一律带业务前缀：

```rust
/// 会话详情响应。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SessionDetailRESP {
    /// 会话本体
    pub detail_session: SessionVO,
    /// 消息回放
    pub detail_messages: Vec<MessageVO>,
}
```

| 后缀 | 用途 | 落点 |
|---|---|---|
| `DO` | 持久化对象（库表一行的映射） | wb-repo |
| `REQ` | 入参（请求体 / 查询参数） | wb-domain |
| `RESP` | 出参（响应体） | wb-domain |
| `DTO` | 跨层转换的中间结构 | wb-service |
| `VO` | 给界面看的视图对象 | wb-domain |

**禁止**：字段名不带前缀（`name` / `code` 这类裸名一律拒绝）。
前缀必须表达归属：`session_title` / `feature_code` / `git_branch`。

### 2.4 错误处理（强制 AppError，code 为字符串）

```rust
pub struct AppError {
    /// 业务码（数字字符串）
    pub code: String,
    /// 人话描述
    pub message: String,
    /// 补充上下文（stderr / 路径 / 上游响应）
    pub details: Option<String>,
}
impl AppError {
    pub fn new(code: &str, message: impl Into<String>) -> AppError;
    pub fn wrap(code: &str, message: impl Into<String>, err: impl std::error::Error) -> AppError;
}
```

- **码值是数字字符串**（`"1001"`），不是数字：HTTP 状态码是 int，业务码是 string，类型上分开
- 各 crate 用 `thiserror` 定义内部错误，边界上（api / service 出口）统一转 `AppError`
- HTTP 出口统一信封 `Resp { code: String, message: String, data: T }`，`code != "0"` 即业务失败
- 错误码常量集中在 `wb-domain/src/error.rs`，新增码先改 `docs/ERROR-CODES.md` 再写代码

**错误码段位**：

| 段位 | 域 |
|---|---|
| 1000–1999 | 通用 / 文件 / 路径 |
| 2000–2999 | 配置 / 持久化 / DB |
| 3000–3999 | Provider / 模型配置 |
| 5000–5999 | 会话运行（忙 / 队列 / 收尾 / 内核提问） |
| 7000–7999 | 工具执行与外部进程（bash / LSP / 沙箱） |
| 7200–7299 | 沙箱 |
| 7300–7399 | 知识库与记忆 |
| 8000–8999 | Skill |
| 9100–9199 | MCP |
| 9700–9799 | 内核（agent 循环 / 压缩 / 子代理） |

### 2.5 命名规范

| 类别 | 风格 | 示例 |
|---|---|---|
| Rust 文件 | snake_case.rs | session_store.rs |
| crate / 模块 | 全小写 | wb-kernel / compaction |
| 类型 / trait | PascalCase，不加 I 前缀 | SessionStore / Tool |
| 函数 / 方法 | snake_case | list_sessions / begin_run |
| 常量 | SCREAMING_SNAKE_CASE | MAX_TOOL_OUTPUT_BYTES |
| JSON 字段 | snake_case（serde rename_all） | session_title |
| ID 前缀 | SCREAMING_SNAKE_CASE | SESS / ENT / RUN / MSG / TC / ASK |

**禁止**：接口加 I 前缀；`xxxId` 与 `xxx_id` 混排；前端出现与后端 json 字段名不一致的字段。

### 2.6 注释规范（强制精简）

注释的读者是「半年后要改这段代码的人」。

| 位置 | 要求 |
|---|---|
| 文件头 | `//!` 1 行说清职责（最多 2 行） |
| 类型 / trait | 1 行 `///` 说它是什么；契约类型可 2 行说边界 |
| 方法 | 1 行签名级说明（干什么 / 返回什么），不写实现步骤 |
| 字段 | **每个 struct 字段必须有注释**；枚举与哨兵值必须写清取值含义 |
| 行内 | 只写「为什么」，1 行 |

- **连续注释（含空注释行）≤ 3 行**；doc-comment 只写结论与边界，不写论证
- 每个文件 ≤ 500 行（含空行）；超限先拆模块
- **禁止**：过程性叙述、改动历史、外部项目名与「参考 / 借鉴 / 类似 X」字样

### 2.6.1 文档规范

文档只回答三件事：**这个项目是什么、每个模块怎么设计实现、为什么这么选型**。

- 根级：`README.md`（入口与能力一览 + 优势局限）/ `AGENTS.md`（本文件，唯一权威）/ `DESIGN.md`（视觉语言）
- `docs/`：架构 / 模块 / 数据模型 / API / 页面 / 错误码 / 开发 / 部署
- **文档只写最终现状**：不记录改动过程、不写「本版新增」「旧版如何」
- 不写外部项目名与「参考 / 借鉴 / 类似 X」——只讲自己怎么设计、为什么这么设计、代价是什么
- 文档与代码冲突时，先判断代码是不是错了；两边都要改到一致

### 2.7 ID：ULID + 业务前缀

```
{PREFIX}_{26 位 ULID}     例：SESS_01JQ8K2M3N4P5Q6R7S8T9V0WXY
```

| 前缀 | 实体 | 前缀 | 实体 |
|---|---|---|---|
| SESS | 会话 | MSG | 助手消息 |
| ENT | 会话条目（JSONL 行） | TC | 工具调用 |
| RUN | 一轮运行 | ASK | 内核提问 |

- ULID 的时间序即排序序：`SESS_` 开头的字符串直接字典序就是创建顺序
- 新增实体类型必须同时扩 `wb-pkg::ulid::Prefix` 与 `is_valid` 的白名单
- 裸 ULID（不带前缀）只用于「业务前缀自拼」的场景，如 `MEM_` / `DOC_` / `GOAL_`

### 2.8 时间戳统一毫秒整数

一律 `i64` 毫秒 Unix 时间戳。禁止 `chrono::DateTime` 出现在 DTO / VO / DB 层——
序列化成字符串会让排序与比较都要写解析。格式化只在界面层做。

### 2.9 平台特定代码

只支持 Windows。路径处理、子进程启动、UI 缩放都做了平台假设：
用 `#[cfg(windows)]` / `#[cfg(not(windows))]` 编译期分支，不写运行时大分支。

### 2.10 横切关注点

| 关注点 | 落点 | 纪律 |
|---|---|---|
| 命令执行 | `wb-tools::exec`（模型工具）+ git / MCP stdio / LSP / github_clone 各自出口 | argv 数组直传永不进 shell；CREATE_NO_WINDOW；超时杀进程——全仓纪律，新增出口自觉对齐 |
| 日志 | `tracing` | 业务层只 `warn!` / `error!`；`debug!` 只在调试分支 |
| 时间 | `SystemTime::now()` | 禁止 `Local::now()`（会话索引要可跨时区比较） |
| ID 生成 | `wb_pkg::ulid` | 禁止 `uuid` / 自造计数器 |
| JSON | serde | 禁用手写 JSON 字符串拼接 |
| 文件写 | `wb_pkg::fsutil::write_atomic` | 临时文件 + rename，避免半截文件 |

### 2.10.1 前端设计令牌（强制）

- **色值只在 `themes.css` 定义**：组件只允许 `var(--wb-*)`，禁止写死色值
- **尺寸走刻度**：间距 `--wb-sp-*`、字号 `--wb-fs-*`、圆角 `--wb-radius-*`、动效 `--wb-dur-*`
- UnoCSS 的刻度已接到令牌上（`gap-4` = `var(--wb-sp-4)`），布局用原子类不绕开令牌
- **Element Plus 是基元供给方**：`ep-theme.css` 把 EP 变量接到 `--wb-*`；
  EP 组件按需引入，未使用的组件连同体积一起摇掉
- **按钮五态齐全**：默认 / 悬停（放大 1.06）/ 按下（缩小 0.92）/ 禁用 / 加载。
  自绘按钮走 `.wb-btn`，EP 按钮由 `ep-theme.css` 覆写，两边共用刻度令牌
- 引用不存在的 CSS 变量会**静默失效**——那不是「样式不好看」，是静默丢样式

### 2.11 数据库迁移

`wb-repo` 手写迁移 SQL，用 `PRAGMA user_version` 步进：

```rust
// 每个迁移一步：版本号 +1，SQL 幂等
if version < 1 { conn.execute_batch(SCHEMA_V1)?; }
```

不引入迁移框架——三张表不值得背一套 DSL。

### 2.12 HTTP 与 API 路径约定（强制）

- **只允许 GET / POST**。没有 PUT / DELETE / PATCH——动词靠路径段表达：
  `POST /sessions/delete/{id}`、`POST /sessions/rename/{id}`
- 路径形状：`/{resource}/{action}/:id`，**id 永远在最后一段**
  - ✅ `/api/v1/sessions/delete/:id`
  - ❌ `/api/v1/sessions/:id/delete`
- 查询参数用 `?key=value`；写操作的主体走 POST + JSON body
- 前端路径前缀 `/api/v1` 由 `api/http.ts` 统一注入

### 2.13 内核不变量（强制）

| 不变量 | 含义 |
|---|---|
| 内核不感知界面 | `KernelEvent` 是内核唯一出参语言，不含任何界面词汇 |
| 内核不依赖上层 | `wb-kernel` 不知道 service / api / 界面的存在 |
| 工具契约唯一 | 所有工具（含功能工具与 MCP 工具）实现同一个 `Tool` trait |
| 事件顺序是契约 | 界面时序依赖事件顺序，改动顺序等于改界面行为 |
| 会话文件是真相 | 界面不另存状态，重启后从 JSONL 回放 |
| 功能开关即边界 | 关掉的功能不进工具表、不发事件；斜杠命令只来自启用的技能 |

---

## 3. 测试规范

测试的价值在于「改这里会坏那里」的地方。平凡断言不写。

**保留什么**

| 类别 | 例子 |
|---|---|
| 长链路 | 建会话 → 发送 → 工具 → 落盘 → 用量，全链路断言事件顺序与回放结果 |
| 状态机 | 待办依赖图、目标状态机、提问回答板的一次性投递 |
| 持久化往返 | 写入 → 重载 → 回放，数据一字不差 |
| 边界拒绝 | 路径逃逸、产物目录跳过、密钥扫描拦截 |
| 协议解析 | SSE 分片累积、HTML 提取、搜索结果解析、BPMN/DXF 文本编辑 |
| 真实外部依赖 | 真 git 仓库全链路（标 `#[ignore]`，慢速脚本显式跑） |

**不写什么**

- 只验证常量等于自身、只覆盖单一分支的断言
- 框架行为的复述（serde 的 rename、tokio 的 select）
- 每个函数一条的覆盖率驱动用例

**组织方式**

- 场景测试放 crate 的 `tests/` 目录（`cargo test -p wb-xxx` 精确跑一个子系统）
- 一个 crate 一个测试文件；文件内按「被测场景」分节，用 `// ---- 场景名 ----` 分隔
- 单元测试只在「同一文件内私有逻辑」且值得守时写
- 命名说清断言的是什么：`memory_roundtrip_searches_and_dedups` 而不是 `test_memory_1`
- 断言带失败信息（`"got {seen:?}"`），失败时能直接看出差在哪
- 测不了的路径要说清原因（如「需要真 git 仓库」），不留悬空的 `#[ignore]`

---

## 4. 架构门禁

提交前必须过下面五道，一条也不用少：

```bash
cargo fmt --all -- --check                          # 1 格式检查
cargo clippy --workspace --all-targets -- -D warnings   # 2 静态检查（全量拦截编译警告）
cargo test --workspace                              # 3 后端测试全绿
cd frontend && npm run typecheck                    # 4 前端类型检查
cd frontend && npm run build                        # 5 前端构建
```

与 CI（`.github/workflows/ci.yml`）跑同一套。
依赖真实外部环境的测试在 `cargo test --workspace -- --ignored` 里（标了 `#[ignore]`）。

缩小验证面：只改后端跑 1–3，只改前端跑 4–5；
只动一个 crate 时把第 1–3 条的 `--workspace` 换成 `-p wb-features`（crate 名即参数），改哪验哪。

**硬性约束**

| 约束 | 检查方式 |
|---|---|
| 文件 ≤ 500 行 | 提交前自检；超了先拆 |
| 依赖方向单向 | Cargo 编译期强制，不合规直接编译失败 |
| 只用 GET / POST | 路由表逐条核对 |
| 字段带业务前缀 | `wb-domain` 里每个 struct 逐个看 |
| 无编译警告 | `cargo clippy -D warnings` 全绿（覆盖 build 的全部警告） |
| 注释不含外部项目名 | 提交前搜一遍 |
| 注释不写过程痕迹 | 不写「v1 / 预留 / 规划内 / 旧版如何」 |

---

## 5. 工作流

1. **先读约束**：动 `wb-domain` 前想清楚字段前缀与错误码段位；动前端前想清楚令牌归属
2. **定位落点**：能放进现有 crate 就别开新 crate；能放进现有模块就别开新文件
3. **实现**：注释只写「为什么」，不留过程痕迹
4. **验证**：跑 §4 的五道门禁（`fmt` / `clippy -D warnings` / `cargo test` / 前端 `typecheck` / 前端 `build`）
5. **同步文档**：改了架构 / API / 错误码 / 页面结构，对应 `docs/` 一起改
6. **提交**：一次提交只做一件事，提交信息写清做了什么与为什么

**新增依赖的门禁**：写清「为什么需要」「替代方案是什么」「代价是什么」，
并同步到 `docs/MODULES.md` 与本文 §1 的技术栈表。
