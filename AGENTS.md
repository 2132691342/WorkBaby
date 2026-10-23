# AGENTS.md · WorkBaby 工程规范

> 本文件是代码风格 / 分包 / 依赖方向 / 工作流的唯一权威。
> 设计说明见 `docs/` 与 `specs/`；冲突时以本文件为准。

---

## 1. 技术栈（强制锁定）

| 层 | 选型 | 版本 |
|---|---|---|
| 语言 | Go | 1.24.x（单 module `WorkBaby`） |
| 桌面壳 | Wails | v2.12.x（Frameless，前端自绘标题栏） |
| HTTP 服务 | gin | ^1.10.x（业务 API + SSE 全部走 HTTP） |
| 前端 | Vue 3 + TypeScript + Vite | 3.5 / 5.6 / 6 |
| 状态管理 | Pinia | ^2.3.x（Setup Store） |
| 路由 | Vue Router | ^4.5.x（hash 模式） |
| HTTP 客户端 | Axios | ^1.7.x |
| UI 组件库 | Element Plus | ^2.14.x（弹窗 / 表单 / 下拉 / 开关 / 确认气泡；文案跟随 i18n） |
| 原子 CSS | UnoCSS | ^66.x（布局与间距；色值仍走 --wb-* 令牌） |
| 图表 | ECharts | ^6.x（按需引入：Line / Pie + Grid / Tooltip；主题色接 --wb-ch-*） |
| 代码编辑器 | CodeMirror 6 | markdown / json 高亮；技能正文与 MCP 配置编辑 |
| 国际化 | Vue I18n | ^9.14.x（zh-CN 默认 + en；设置页可切换，Element Plus 文案同步） |
| 样式 | 原生 CSS 设计令牌（`themes.css` + `ep-theme.css` + `base.css` + `content.css`） | 色值与尺寸只在 themes.css 定义；EP 变量经 ep-theme.css 接到 --wb-* 令牌 |
| 渲染 | marked · dompurify · highlight.js | markdown 唯一出口 `utils/md.ts` |
| ORM | GORM + glebarez/sqlite | ^1.30.x（pure-Go，无 CGO） |
| 数据库 | SQLite | WAL；只存界面设置 KV——会话、消息与用量都在 PI 的会话文件里 |
| 配置 | Viper | ^1.19.x（YAML + ENV；运行时配置走 KV 表） |
| 日志 | log/slog（标准库） | info/warn/error 分文件落盘 |
| 实时通信 | SSE | 业务实时通信全部走 SSE（`/api/v1/events`），不引入 WebSocket |
| 测试 | testing（标准库） | 一包一入口、一个子测试一种坏法；`scripts/check.ps1` 收口全部验证（快门 / `-Pkg` / `-Run` / `-List` / `-Full`），耗时以编译为主 |
| ID | 沿用 PI 的 id；本轮 runID 用 google/uuid | 不自造业务 id，见 §2.7 |
| 托盘 | getlantern/systray | 关闭到托盘 |
| 内置运行时 | Node + Git Bash（MinGit） | `backend/runtime/bundled/`（go:embed 进 exe，归档经 LFS 托管），其余一律不内置 |
| 平台 | Windows | 本期只考虑 Windows；平台特定代码按 `//go:build windows` 分文件，不写 `runtime.GOOS` 大分支 |

### 1.1 依赖原则

标准库 → `golang.org/x/...` → 生态库。新增依赖必须说明理由与替代方案。

**明确不引入**：工作流引擎、微前端、通用工具库——当前功能面用不到，引入只增维护成本。
富文本编辑器（WangEditor 等）按需再评估；当前 CodeMirror 6 覆盖技能与配置编辑场景。

**视觉分层纪律不变**：Element Plus 只是基元供给方，视觉语言仍由 `themes.css` 的
靛蓝五档令牌说了算——EP 的 CSS 变量经 `ep-theme.css` 全部接到 `--wb-*`，
组件里禁止写 EP 主题色之外的色值；图表色值同理走 `--wb-ch-*`。
新增组件优先用 EP 现成的，不回退到自绘弹层。

### 1.2 内核归 PI，界面归 WorkBaby

Agent 内核是外部 PI 进程（`pi --mode rpc`），本项目**不重写内核**：工具、压缩、
重试、缓存、编辑、MCP、扩展、子代理全由 PI 演进，本机只做发现 / 安装 / 拉起 / 守护 /
协议编解码 / 事件翻译六件事。想加一个 agent 能力时先确认 PI 是否已有：已有就在界面上
暴露它，不自研第二份。

PI 不强制用户预先安装：内置 Node 与内置 npm 前缀可代装；用户机器上已有可用 PI 时
直接复用（specs/11）。

---

## 2. 强制编码规范

### 2.1 包结构

```
WorkBaby/
├── main.go                       # 入口：embed + wails.Run + 单实例守卫
├── app.go                        # App 结构体（持有 *api.Handler，生命周期委托给它；不嵌入，见 specs/04）
│
├── backend/
│   ├── server/                   # ① HTTP 层（gin）：路由 / SSE hub / 统一响应 / 方法白名单
│   ├── api/                      # ② 业务 handler（薄）+ 系统能力绑定（唯一允许 import wails 的包）
│   ├── pi/                       # ③ 能力域：PI 子进程监督（进程 / RPC 编解码 / 事件归一 / 会话文件索引）
│   │   ├── env/                  #    运行时复用、PI 安装与探测、命令执行环境、终端入口、配置互通
│   │   ├── pitest/               #    测试夹具：假 PI 进程（按脚本回 RPC，也落会话文件）
│   │   └── piptest/              #    测试夹具：编译假 PI、定位仓库根（只被测试引用）
│   ├── service/                  # ④ 业务编排层（事务边界；不写 SQL；不引 gin/Wails）
│   ├── repo/                     # ⑤ 持久层（GORM；不引上层）
│   ├── domain/                   # ⑥ 域模型（一个聚合根一个文件，DO/DTO/REQ/VO/RESP 同居一处）
│   ├── skill/                    # ⑦ Skill（parser/registry/loader，目录即 PI 的技能目录）
│   ├── git/                      # ⑧ 工作区版本视图（读 status/diff/历史，写限白名单动作，走系统 git）
│   ├── runtime/                  # ⑨ 路径解析 + 内置 Node / Git Bash 运行时（bundled/ 归档 go:embed + 解压）
│   │   └── runtimetest/          #    测试夹具：预置运行时标记，装配类测试跳过解压（只被测试引用）
│   ├── config/                   # ⑩ 配置（Viper YAML：工作目录与固定项）
│   ├── db/                       # ⑪ SQLite 打开 + 迁移
│   ├── pkg/                      # ⑫ 叶子工具包（AppError/ID/日志/httprules/fsutil）
│   ├── tray/                     # ⑬ 系统托盘
│   └── singleinstance/           # ⑭ 单实例保护（文件锁 + 本地 TCP IPC）
├── assets/                       # 应用内帮助文档（embed；Skill 管理见 specs/03）
├── build/                        # 平台资源与产物
├── docs/                         # 项目级文档：架构 / 模块 / 数据模型 / API 契约 / 页面 / 错误码 / 开发 / 部署
├── specs/                        # 子系统规格（01-13，一个子系统一篇）
└── tools/                        # Go 写的独立门禁工具（check-boundaries）
```

不单独设包的能力一律留在 `service/` + `domain/`：模型服务、引擎、技能、插件、工具、MCP、统计、
设置各自一个文件，它们只是「某个文件类型 / 某份配置」的编排，不值得开目录。
开包的门槛是「它有自己的一组概念 + 独立生命周期」，`pi` 与 `git` 满足，其余不满足。

**`backend/pi` 是能力域，不是胶水层**：它拥有“子进程 + 协议 + 事件归一”这三个自洽概念，
有独立的生命周期（spawn / 守护 / 重启 / 退出）、独立的错误段（9700-9799）与独立的测试入口。
把它的事件翻成界面词汇表在 `service/translate.go`——`pi` 不依赖 `domain`，
翻不回上层才能单独测试与单独复用。
`env/` 是它的子包：环境引导与协议无关，但只有它需要。

**禁止**：

- 在 `service / repo / api` 包内定义实体 / DTO / VO / 枚举（必须放 `domain/`）
- 错误码 / 工具类分散到各业务包（统一 `backend/pkg/`）
- 在 `main` 包内写业务代码
- `backend/pkg/` 内出现业务词汇（session / provider / chat 等）
- `backend/pkg/` 依赖任何其他 `backend/` 业务包（叶子工具包铁律）
- `backend/pi` import wails / api / server / gin / service，或让上层直接复制一份子进程与协议
- 任何层向 PI 的 JSONL 写入（只有 PI 能写它）
- 用字符串拼 git 写命令：`backend/git` 的写入走显式白名单（stage / unstage / resolve / discard /
  commit（含 --amend）/ 新建分支 / 切分支 / 删已合并分支 / merge / rebase / pull / fetch / push /
  init / remote add / 冲突继续与中止），`reset --hard` / `clean -fd` / `--no-verify` /
  `branch -D` / `-C <dir>` 一律拒绝；强制推送只走分级（`--force-with-lease` 或经分支名确认的 `--force`）
  （specs/08）
### 2.2 依赖方向（强制）

```
backend/server ──► backend/api ──► backend/service ──► 能力域（pi/skill/git）
                                          │
                                          ▼
                                     backend/repo ──► backend/domain
                                          │
                                          ▼
                              backend/pkg（叶子工具包：各层可依赖，自身不依赖任何 backend 业务包）
```

| 层 | 允许依赖 | 禁止依赖 |
|---|---|---|
| server | api / domain / pkg / gin | service / repo / 能力域 |
| api | service / domain / pkg / gin / wails；**装配期**持有 repo / config / db / 能力域句柄（组合根） | handler 方法绕过 service 直写业务（查询型系统能力除外） |
| service | repo / domain / pkg / config / db / runtime / skill / git / pi | api / server / gin / wails |
| repo | domain / pkg / gorm | api / service / 能力域 |
| domain | pkg | 任何上层 |
| **backend/pkg** | 标准库 / golang.org/x / 第三方工具 | **任何其他 backend 业务包** |
| pi | pkg / runtime | api / service / server / repo / **domain** / gin / wails |
| pi/env | pkg / runtime | 上层任何包 |
| skill · git | domain / pkg | api / service / server / 其他能力域 |
| runtime | pkg | 其他任何 backend 包 |
| config · db · repo | domain / pkg | 上层与能力域 |
| tray · singleinstance | domain / pkg | api / service / 能力域 |

**能力域之间不互引**：`pi` / `skill` / `git` / `runtime` 各自只往上交给 `service` 接。
它们要合作就在 `service` 里编排，不在能力域里互相 import——一互引，独立测试与独立演进同时作废。
`pi` 不依赖 `domain` 是这一组里最紧的一条：它只认 `pkg` 与 `runtime`，
把协议记录归一成自己的 `Event` 就上交，翻成界面词汇表是 `service` 的事。

**api = 组合根**：`Handler.Startup` 是唯一装配入口，持有 Repo / Cfg / 子进程句柄等
是它的职责（`tools/check-boundaries` 按此放行）；业务 handler 方法必须经
service 走业务流程，唯一例外是查询型系统能力（`/runtime` 状态检测）——
它们与对话框 / 自启同属「系统能力绑定」，不是业务编排。

**双主机**：业务 API 走 gin HTTP，Wails 绑定只保留系统能力（对话框/剪贴板/窗口）。
前端事件走 SSE。端口注入：`app:ready` 携带 `server_port`。

**子进程只许出现在 `backend/pi`**：`service` 通过它的接口调 PI，不自己 `os/exec`。
多一处 spawn 就是多一份“忘了收 stderr 会阻塞”的隐患。

### 2.3 聚合根文件组织（domain 单文件多形态）

一个聚合根一个文件，该实体所有形态同居一处：

| 后缀 | 语义 | 标配 |
|---|---|---|
| DO | Domain Object，映射数据库表 | 必有（必须显式 `TableName()`，防 GORM 把 DO 复数化成 `_dos`） |
| DTO | 能力域间传输载体 | 按需 |
| REQ | 前端 → 后端入参 | 按需 |
| VO | 组装后的视图对象 | 按需 |
| RESP | 后端 → 前端出参 | 按需 |

包级错误变量集中在文件顶部声明（`var ErrXxx = pkg.New(...)`）。

### 2.3.1 内联结构体边界（强制）

`server / api / service / repo` 四层禁止就地声明入参/出参/实体结构体，必须引用 `domain.XxxREQ/DTO/VO/RESP/DO`。

例外：`backend/tool/*` 内的快速参数解析允许内联（须加注释）；`backend/repo/*` 内 GORM 临时查询条件允许内联；
上游第三方接口的响应解析载体允许就地声明（与 §2.5.1 同理——它不是我们的入参 / 出参，
字段名还要忠实对方，目前只有 `service/provider.go` 解析 `/models` 列表那一处）。

### 2.3.2 空数组契约（强制）

Go 的 nil slice / nil map 编码成 `null`，前端按数组读就是一句
`Cannot read properties of null`——它崩在渲染里，用户看到的只有「界面出现异常」。

- 出参里的切片与 map 字段**构造时一律初始化**（`[]T{}` / `map[K]V{}`）：这是主修法
- HTTP 出口 `api.ok()` 走 `pkg.FillNilSlices` 兜底：契约说是数组就永远发 `[]`。
  兜底不替代构造处的初始化，它只是让漏掉的那一次不至于崩掉整个页面
- 前端接住数组字段时同样按「可能是 null」处理（`res.entries ?? []`）：
  后端修好了，缓存里的旧响应与假后端还在

### 2.3.3 前端目录（强制）

`frontend/src` 是唯一源码目录，只有一层：
`api / components / composables / locales / stores / types / utils / views / router`，
外加四个 CSS：

| 文件 | 职责 |
|---|---|
| `themes.css` | 色值与尺寸刻度的**唯一来源**（`--wb-*` 令牌 + 两套 `[data-theme]`） |
| `ep-theme.css` | Element Plus 变量 → `--wb-*` 的桥，外加按钮五态等细节对齐 |
| `base.css` | 重置、排版基线、滚动条、可复用的面与图标基元 |
| `content.css` | EP 没有对应物的自绘区：消息流 / markdown / 代码块 / 工具卡 / 思考块 / diff |

`components/` 下按域分子目录：`layout`（顶栏 / 页头 / 左栏外壳）、`chat`、`dashboard`、
`settings`、`workspace`、`common`（图标、代码编辑器、轻提示）。

**禁止**：新增第五个顶层 CSS；在组件里写死色值；把布局间距写成裸像素。
布局与间距用 UnoCSS 原子类（其刻度已接到 `--wb-*` 令牌）。

Wails 生成的绑定落在 `frontend/wailsjs`（`wails.json` 的 `wailsjsdir`）：
不进 `src`、不手改。绑定与源码分开，才不会出现 `src/src` 这种嵌套。

### 2.4 错误处理（强制 AppError）

```go
func New(code string, message, details string) *AppError
func Wrap(code string, message string, err error) *AppError
```

```go
// ✅ 业务错误
if path == "" { return nil, pkg.New("1001", "路径不能为空", "") }
// ✅ Wrap 底层错误
if err := os.WriteFile(p, data, 0o644); err != nil { return pkg.Wrap("1002", "写文件失败", err) }
// ❌ 禁止
return errors.New("provider not ready")   // 丢 code，前端无法分流
```

**码值是数字字符串**（`"1001"`），不是 int：HTTP 状态码与业务码同为 int 时前端只能靠字段名分辨，
字符串从类型上就把两者分开（`Resp` 的 `code` 是 `string`，HTTP 状态码是 int）。

**错误码段位**：

| 段位 | 域 |
|---|---|
| 1000–1999 | 通用 / 文件 / 路径 |
| 2000–2999 | 配置 / 持久化 / DB |
| 3000–3999 | Provider / 模型配置 |
| 4000–4999 | 已退役（原审批段）：工具执行与确认都在 PI 里，本段不再产生码 |
| 5000–5999 | 会话运行（忙 / 插话队列 / 收尾 / 引擎提问） |
| 7000–7999 | Runtime（内置运行时） |
| 8000–8999 | Skill |
| 9100–9199 | MCP（外部工具接入） |
| 9700–9799 | PI 子进程 / 协议 / 环境引导 |

6000 / 9200–9699 段位随一批已退役模块一起停用：这些码不再产生，文档里也不再保留。

每个码的完整含义与处置动作见 [`docs/ERROR-CODES.md`](docs/ERROR-CODES.md)。

### 2.5 命名规范

| 类别 | 风格 | 示例 |
|---|---|---|
| 文件名 | snake_case.go | ai_provider.go |
| 包名 | 全小写无下划线 | pi / env / skill |
| 类型 / 接口 | PascalCase（不加 I 前缀） | ProviderDO / Client |
| 函数 | 导出 PascalCase / 私有 camelCase | ListProviders / resolveOne |
| 变量 | camelCase | runID / compressRatio |
| 错误变量 | ErrXxx | ErrSessionNotFound |
| JSON tag | **snake_case** | json:"user_message_id" |
| GORM tag | snake_case | gorm:"primaryKey;size:64" |
| ID 前缀 | SCREAMING_SNAKE_CASE | PROVIDER / SESSION |

**禁止**：接口加 I 前缀；匈牙利命名；`xxxId` 与 `xxxID` 混排。

**JSON 契约铁律**：所有 HTTP 交互字段一律 snake_case。前端 `types/api.ts`
消费的字段名必须与后端 RESP 的 json tag 完全一致。

### 2.5.1 PI RPC DTO 例外

`backend/pi/protocol.go` 下的记录结构是 PI RPC 协议的反序列化载体，
json tag 必须忠实协议字段名（camelCase：`parentId` / `toolCallId` / `stopReason`）。
翻译层（`backend/service/translate.go`）产出的 WorkBaby 事件仍用 snake_case。

**与上游字段同名的本侧结构体，只在这一个文件里允许出现**。走出这个文件就转成 domain 类型。

### 2.6 注释规范（强制精简）

注释的读者是「半年后要改这段代码的人」。他要的是**这个东西是什么、边界在哪**，
不是作者的思考过程。

| 位置 | 要求 |
|---|---|
| 文件头 | 1 行说清这个文件的职责（最多 2 行） |
| 包 | 1 行 |
| 类型 | 1 行说它是什么；契约类类型可到 2 行说清边界 |
| 方法 | 1 行签名级说明（干什么 / 返回什么），不写实现步骤 |
| 字段 | 名字不自解释时写 1 行；枚举与哨兵值必须写清取值含义 |
| 行内 | 只写「为什么」，1 行 |

- **最低要求就这三样**：文件有头、导出方法有签名说明、字段有注释。够了就停——注释不是设计文档
- **连续注释块 ≤ 2 行**；方法注释的第二行只在「不这么做会出事」时才留。自查：在仓库根按
  「连续 3 行以上 `//` 的块」扫一遍源码，命中就压到 2 行（`go:build` / `go:embed` 指令前的分隔行除外）
- 注释只写**结论与边界**，不写论证：「为什么这么设计、当初踩过什么坑」属于 `docs/` 与 `specs/`
- 禁止：过程性叙述、TODO 历史、实现细节叙事、外部项目名与「参考 / 借鉴」字样

### 2.6.1 文档规范

文档只回答三件事：**这个项目是什么、每个模块怎么设计实现、为什么这么选型**。

- 根级：`README.md`（入口与能力一览）/ `AGENTS.md`（工程规范，唯一权威）/ `DESIGN.md`（视觉语言）
- `docs/`：项目级说明（架构 / 模块 / 数据模型 / API 契约 / 页面 / 错误码 / 开发 / 部署）
- `specs/`：子系统规格，编号 01-13，一个子系统一个文件
- 每篇只写现状：定位 → 设计 → 契约 → 关键流程 → 约束 → 取舍。**优势与代价必须写出来**
- 表格与短段落优先；能用表就不用列表
- **不写改动过程**：不建 CHANGELOG，不记录「本次改了什么」「之前坏在哪」，
  设计变更直接改进对应文档；历史由 Git 承载
- 不写外部项目名与「参考 / 借鉴 / 类似 X」——只讲自己怎么设计、为什么这么设计
- 文档必须与代码同步：改行为时同时改对着这篇行为的文档与注释，不同步视为未完成

### 2.7 ID：不造自己的业务 id

会话 id 与条目 id 都来自 PI 的会话文件，WorkBaby 只透传不生成：自己发一套 id
就会出现「同一个会话两个名字」，而界面与 `pi resume` 必须指向同一条会话。

本侧只造一个东西——**一轮运行的 runID**（`pkg.NewTraceID()`，UUID v4）：
它不进会话文件、不当主键，只用来把日志与本轮的事件流对齐同一次调用。

```go
func NewTraceID() string   // 只在 service.ChatService.beginRun 里调一次
```

没有用户表：单机桌面应用里本机用户就是唯一用户，凭据存在 PI 的 `auth.json` 里。

### 2.8 时间戳统一毫秒整数

跨边界的 struct 里时间一律 `int64` 毫秒，禁止 `time.Time` / `time.Duration`：
JSON 里的 RFC3339 字符串要前端再解一次，而 `Date.parse` 对不带时区的串会按本地时间猜。

```go
CreatedAt int64 `json:"created_at"`                    // 来自 PI：IsoMillis(entry.Timestamp)
UpdatedAt int64 `gorm:"autoUpdateTime:milli" json:"updated_at"`   // 本侧写的：数据库自动填
```

两个来源规则：消息上的时间优先取 `message.timestamp`，没有才解条目头的 ISO 串；
会话的「最后活动时间」取文件 mtime（PI 只会往会话文件追加，写完就是活动完）。

### 2.9 平台特定代码（build tag）

```go
//go:build windows
```

禁止一个文件里 `runtime.GOOS` 分支处理所有平台（托盘 / 剪贴板等系统能力必须分文件）。

### 2.10 横切关注点

| 关注点 | 方案 |
|---|---|
| 统一错误 | AppError + 错误码分段 |
| 日志 | slog 仅 info/warn/error；分文件落盘；ctx 注入 sessionID/runID |
| 实时通信 | SSE only（256 缓冲 / 慢客户端先挤 delta 保关键事件、回灌再丢关键事件就断开对账 / 重连带 `last_event_id` 重放并按 seq 去重，缓冲裁掉中段时主动发 `chat:gap`） |
| 压缩 | 由 PI 自己做（确定性清洗 + 按预算找切点）；WorkBaby 只展示 `compaction_end` 的结果 |
| 工具输出 | 由 PI 截断；WorkBaby 只做展示层截断（UI 不渲染万行） |
| PI 进程 | 一个工作区一个进程；归 `backend/pi` 独有，其他层不得 `os/exec` 拉起 PI |
| 事件出口 | `service.Emitter` 唯一出口（注入归属 + 分配 seq + sink 注入，按会话串行化） |
| Token 计量 | 用量读自 PI 的会话文件（assistant 消息上的 usage）；`GET /stats` 按天/模型/会话聚合 |
| 配置 | Viper（工作目录等固定项）+ settings KV 表（界面偏好） |
| 单实例 | 文件锁 + 本地 TCP IPC 转交文件路径 |
| 路径比较 | 同一份路径的短名（`RUNNER~1`）与长名、符号链接形式字面不等；取相对路径或判越界前先过 `pkg.RealPath` |

禁止引入：OpenTelemetry / Prometheus 客户端；AOP 框架；全局可变状态。

### 2.10.1 前端设计令牌（强制）

视觉语言见 `DESIGN.md`。核心约束：

- 色值只在 `themes.css` 定义（`--wb-*` 语义令牌 + `[data-theme]` 块）
- **表面走 `--wb-elev-*` 六级阶梯**（window / workspace / surface / raised / hover / selected）：
  相邻两级只差 2%~5% 亮度，读起来是一片连续工作区而不是卡片叠卡片。
  组件禁止自己选灰阶，一律引用阶梯；**表单控件另走 `--wb-control-*` 一组**
  （bg / bg-hover / bg-disabled / border / border-hover / shadow / shadow-focus），
  控件要有「可以点进去」的浮凸感，与普通面区分开
- **旧名 `--wb-surface*` / `--wb-side` / `--wb-panel` / `--wb-raise` / `--wb-sunken` 是别名**，
  在 `themes.css` 里指向阶梯对应级（两套主题各一份）。它们只是兼容既有引用的桥梁，
  **新代码一律写 `--wb-elev-*`，不得新增旧名引用**；别名不许单独赋色值，否则会盖掉阶梯
- 尺寸令牌全部乘 `--wb-vw-scale`（由 `useViewportScale` 按「窗口宽 ÷ 1440」写入，
上下限 0.8~1.35）：分辨率不同的机器上整套刻度等比跟随，不改代码；
组件层写死裸像素只允许在令牌定义处
- 强调色只有同一个靛蓝色相的五档，没有第二个色相：`#6366f1`（动作）· `#818cf8`（活动）·
  `#e0e7ff`（淡强调）· `#f8fafc`（更淡的底）· `#1e293b`（中性深）。
  数据可视化取 `--wb-ch-1..4`（靛蓝明度递减），其中 3/4 两档是两档淡靛，不是中性灰
- 组件禁止写死颜色：用 `var(--wb-*)`，不用 hex
- 分层靠「圆角 + 1px 中性描边 + 一层极轻阴影」（`--wb-card-shadow`）：禁彩色描边、禁 backdrop-blur 叠层。
  渐变只允许三处：图表面积（`--wb-area-*`）、欢迎页光晕、用户自选的背景图
- 布局与间距用 UnoCSS 原子类：`uno.config.ts` 已把 spacing / fontSize / borderRadius / colors
  全部接到 `--wb-*` 令牌，`gap-4` 就是 `var(--wb-sp-4)`。禁止在原子类里写裸像素或 hex
- 控件高度四档：`--wb-ctl-h-sm`(26px) / `--wb-ctl-h`(34px) / `--wb-ctl-h-lg`(38px) / `--wb-ctl-h-xl`(44px)，另有图标钮 `--wb-ctl-icon`(32px)；禁止裸像素高度
- 圆角只走七档：`xs`(4) / `sm`(6) / `radius`(8) / `lg`(12) / `xl`(14) / `2xl`(16) / `full`；
  `full` 只给真正的圆形（头像 / 状态点 / 水位环），chip 与小号输入框用 `sm`——胶囊会让小控件失去边界
- 装饰性图标瓦片用 `--wb-tile-sm`(30px) / `--wb-tile`(40px) / `--wb-tile-lg`(52px)：它们不是控件，但同一组尺寸反复出现，散成裸像素就会各自漂
- 按钮五态必须齐全（默认 / 悬停放大 / 按下缩小 / 禁用 / 加载）：每个可点元素都要给「点到了」的反馈
- **五态刻度全站统一 `1.06` / `0.92`**：EP 按钮在 `ep-theme.css` 里定，自绘按钮在各自组件里定，两边必须同一档——
  同一个操作条里 EP 按钮放大 1.03、旁边自绘图标钮放大 1.06，用户会感觉「有的按钮软、有的脆」。
  `text` / `link` 是文字级控件，缩放会带动整行文字抖动，只给颜色反馈；
  禁用与加载两态一律 `transform: none`，否则连点会让按钮持续抖
- 危险按钮用中性描边 + 危险色文字：彩色描边违反分层纪律，语义交给文字与底色
- 字号用刻度 `--wb-fs-*`；禁止任意像素值
- 字体只有三个语义变量：`--font-sans` / `--font-mono` / `--font-display`，全部在 `themes.css` 定义；`html, body` 必须显式声明 `font-family` 与 `font-size`
- 组件基元优先用 Element Plus（按钮 / 表单 / 开关 / 下拉 / 弹窗 / 气泡确认 / 标签 / 表格 / 抽屉 / 页签）；
  EP 变量经 `ep-theme.css` 接到 `--wb-*`，组件里禁止另写色值
- 图表一律 ECharts（`utils/echarts.ts` 唯一装配点，按需注册），色值取 `--wb-ch-*`；主题切换后重读 CSS 变量重绘
- **EP 只按需引入，禁止回到 `app.use(ElementPlus)` 全量注册**：由 `unplugin-vue-components` 的 `ElementPlusResolver`
  按模板标签导入实际用到的组件。配套约束——`manualChunks` 里不要把 `element-plus` 整体归堆，
  那会把未用组件（日历 / 穿梭框 / 走马灯等）连同内部依赖一起拉回首屏，让 tree-shaking 与懒加载同时失效
- 代码编辑一律 `CodeEditor`（CodeMirror 6）：markdown / json 高亮
- 文案一律走 i18n（`src/locales/`，key 按域分层）；用户可见的字符串禁止硬编码在模板里
- 消息流 / markdown 渲染 / 代码块 / 工具卡 / 思考块 / 提问卡保留自绘
  （它们是内容渲染不是表单基元，EP 没有对应物）；提问卡内部仍用 EP 的按钮与输入
- **一级导航只在左 rail**（`layout/NavRail.vue`，62px 图标 + 一行短标签竖排）：
  对话 / 工作区 / 技能 / 用量 / 帮助 / 设置全部平级——设置是 rail 底部的一等入口，
  不是顶栏角落的图标。顶栏只留品牌、当前区标题、连接灯与窗控；
  各视图不得自画返回或主页入口；左栏（会话列表 / 设置分组 / 文档目录）只放**当前区**的导航
- **有 rail 之后，二级目录必须用 `SidePanel width="sub"`**（`--wb-w-subside` 188px，浅一档底）：
  rail 已在左侧占位，二级目录再占 232px 就是两根侧栏并排，读作两套导航系统
- 消息流用**统一容器 + 角色徽章胶囊**（`.msg-badge`），不做「用户右气泡 / 助手左头像」——
  助手列的身份由正文面板的左侧 2px 脊线承担。**没有可读正文的助手消息不渲染外壳**
  （只有 toolCall 的回合就只出工具卡，否则从命令行跑出来的会话满屏都是空框）。
  长会话必须给 `.msg-col > *` 加
  `content-visibility: auto` + `contain-intrinsic-size: auto 120px`（末两条除外），
  并给 `.msg-scroll` 加 `overflow-anchor: none`，贴底由 JS 管——否则浏览器滚动锚定
  与 stick-to-bottom 打架，表现为「明明在底部却跳走了」
- 选中态 = 淡底 + 左侧 2px 竖条 + 主色（`.sess-row.is-on` / `.set-item.is-on` / `.help-item.is-on` 同一语言）
- 设置页的开关行一律用 `base.css` 的 `.wb-set-row`（说明在左、控件在右、整行宽度封在
  `--wb-w-desc`）：让控件贴右边缘时，中间空出半屏、相邻两行的开关挤成同一列，容易误点
- 只读读数（上下文水位 / 耗时 / 模型名）不套描边胶囊、不做悬停缩放：有盒子没动作等于骗点击，
  要反馈就换字色不换几何
- 图标统一走 `AppIcon` 内联 SVG（24 网格，`--wb-icon-stroke`）；禁止用 Unicode 字符或 Emoji 冒充图标
- 事件 data 的字段名必须与后端 json tag 一致（snake_case）；禁止 `as never` 之类的强转绕过类型检查
- **SSE 事件消费挂在应用壳（`composables/chatBridge.ts`），不挂在对话页**：run 在切去
  设置页时还在跑，视图卸载就等于把 done 与正文全丢掉；重回对话页只补快照，不重置 store
- 思考段默认收起（用户自己展开）；流式期间也不自动撑开，否则每轮都在抢版面

### 2.11 数据库迁移

- GORM AutoMigrate：新增字段只增不删、零值兜底；废弃字段留在表里（改语义靠代码兼容）
- 旧版本留下的表（sessions / providers / runs / knowledge_docs 等）**不主动 DROP**：
  它们不再被读写，留着不影响任何路径；删除动作只在用户自己迁移数据时做

### 2.12 HTTP 与 API 路径约定（强制）

| 维度 | 规则 |
|---|---|
| HTTP 方法白名单 | 业务仅允许 GET 与 POST（`backend/pkg/httprules.go` 收口） |
| 路径参数位置 | 变量参数必须落在路径末尾：`/{resource}/{action}/:id` |
| 删除语义 | 删除走 `POST /{resource}/delete/:id` |
| 流式事件 | `GET /api/v1/events?session_id={sid}` SSE，Last-Event-ID 重放 |

**路径风格唯一标准**：`/api/v1/{resource}/{action}/:id`——动作段在 id 之前，id 永远是最后一段
（`POST /api/v1/sessions/delete/:id`、`GET /api/v1/skills/content/:id`）。没有 id 的动作直接
`/api/v1/{resource}/{action}`（`POST /api/v1/extensions/reload`）。
一个动作需要两个变量（如 `providers/test` 的 `id`）时，把第二个变量移进请求体，
不要拼成 `/:id/:action`。前端 API 层收敛在 `frontend/src/api/index.ts`，
路径与 `backend/server/routes.go` 一一对应。

**例外**：调 PI 的 RPC 走 stdio，不受此限。

### 2.13 PI 监督契约（强制）

`backend/pi` 是整个工程最核心的子系统：**它不实现 Agent，它运行、观测并控制 Agent。**

内核是外部 PI 进程（`pi --mode rpc`）。WorkBaby 负责发现/安装它、拉起并守护它、
编解码它的 JSONL 协议、把记录归一成有语义的事件；**把事件再翻成界面词汇表在 `service`**——
`pi` 不依赖 `domain`，它只认 `pkg` 与 `runtime`，靠接口把事件交给上层。

#### 2.13.1 进程模型

```text
一个工作区 = 一个 PI 进程
  ├─ spawn: node <pi>/dist/bundle/cli.js --mode rpc --append-system-prompt <persona.md>（cwd = 工作区；提交信息草稿等短调用用 --no-session）
  │        npm 的 pi.cmd / npm.cmd 先解析成 node + 脚本：经 cmd.exe 起进程会弹控制台窗口
  ├─ stdout: 按 LF 切分 → 记录分类 → 事件翻译
  ├─ stderr: 只进日志尾部缓冲，不解析为协议
  └─ 退出: 关 stdin → 等 agent_settled 或 5s → kill
```

`cwd` 是进程级属性，因此跨工作区的会话**不能**共用一个进程。
同一工作区的多个会话通过 `switch_session` 在同一个进程里切换（串行）。

#### 2.13.2 协议记录分类

```go
type Record struct {
    Type string `json:"type"`
    ID   string `json:"id"`
}
```

| 记录 | 判据 | 处置 |
|---|---|---|
| 命令响应 | `type == "response"` | 按 `id` 唤醒等待者 |
| 会话事件 | 已知事件名 | 进翻译层 |
| 扩展 UI | `type == "extension_ui_request"` | `confirm`/`select`/`input`/`editor` 转 `chat:ask`；`notify`/`setStatus`/`setWidget`/`setTitle` 转只读反馈事件；其余方法忽略 |
| 未知 | 其余 | debug 日志后丢弃 |

#### 2.13.3 事件翻译映射

| PI 事件 | WorkBaby SSE 事件 |
|---|---|
| `agent_start` | `chat:start` |
| `message_update`/`text_delta` | `chat:delta`（kind=text） |
| `message_update`/`thinking_delta` | `chat:delta`（kind=thinking） |
| `tool_execution_start` | `chat:tool_start` |
| `message_update`/`toolcall_end` | `chat:tool_start`（参数到这里才完整：`toolcall_delta` 只给半截 JSON） |
| `tool_execution_end` | `chat:tool_end` |
| `turn_end` | `chat:context` |
| `compaction_end` | `chat:compressed` |
| `auto_retry_start` | `chat:retry` |
| `agent_settled` | `chat:done` |
| `extension_ui_request`（`confirm`/`select`/`input`/`editor`） | `chat:ask` |
| `extension_ui_request`（`notify`） | `chat:notify`（info/warning/error，无需应答） |
| `extension_ui_request`（`setStatus`） | `chat:status`（状态条，空 text 即清除） |
| `extension_ui_request`（`setWidget`） | `chat:widget`（文本行小部件，空行即清除） |
| `extension_ui_request`（`setTitle`） | `chat:title` |

`chat:tool_start` 有两条来源：`tool_execution_start` 是常规路径（参数已完整），
`message_update` 里的 `toolcall_end` 是兵底（有些内核只在消息流里把参数补齐）。

#### 2.13.4 不变量

| 不变量 | 锁死原因 |
|---|---|
| stdout 分帧只按 LF | U+2028/U+2029 在 JSON 字符串里合法，按 Unicode 行分隔切分会截断记录 |
| 未知记录忽略，不报错 | 装了扩展的环境首帧就是 `extension_ui_request`，当错误处理则处处起不来 |
| 提问只靠 `extension_ui_request` 对话对 | PI 默认不问审批；闸门只能由扩展拦 `tool_call` 并走 `ctx.ui.confirm`，界面只负责显示与回传 |
| agent 目录不覆盖 | `PI_CODING_AGENT_DIR` 只能继承：换成私有目录，用户装好的技能 / 插件 / 模型 / 会话全部看不见 |
| 一个工作区一个当前会话 | 进程的会话指针只有一个：同目录另一条对话还在跑时，切会话前必须先等它停下 |
| 命令按 id 关联 | PI 异步处理，响应顺序与发送顺序无保证 |
| `chat:done` 只在 `agent_settled` | 自动重试/压缩/插话都挂在 `agent_end` 之后，提前完成会让界面自相矛盾 |
| 一个工作区一个进程 | `cwd` 是进程级属性，跨工作区共进程会让工具写到错的目录 |
| 停止必先 `clear_queue` | 不取回就 `abort`，用户排队的那句话凭空消失 |
| 进程死亡不自动重发本轮 | 工具可能已有副作用，重发等于执行两次 |
| stderr 不解析为协议 | PI 把诊断写 stderr，误解析会把日志当命令 |
| stdin 必须完整写入 | 半条 JSON 会让 PI 的解析器吞掉后续命令，表现为「命令全部超时」 |
| 入口不走 shell | npm 的 `pi.cmd` / `npm.cmd` 先起 cmd.exe 再拉 node，每次调用都闪一个控制台窗口；一律解析成 node + 入口脚本，并给全部子进程设 `CREATE_NO_WINDOW` |
| 界面文本剥掉 ANSI | 内核与扩展按终端习惯给状态文字上色，GUI 没有终端语义，原样显示就是 `[39m[2mCache 0.0%[22;39m` 这种乱码 |
| 事件出口仍按会话串行化 | 沿用 `service.Emitter`：并行工具的表现层仍在同一会话里竞争 seq |
| 绝不写 PI 的 JSONL | 格式是 PI 私有契约，写坏它 PI 自己都读不回来 |
| 会话运行中不删会话文件 | 不等 `agent_settled` 就删，PI 会往已删文件继续追加 |
| 写 `settings.json` 只改自己掌管的顶层键 | PI 与用户还在里面写 `packages` / `cacheWarming` / `codemode`，整体重写会把它们连同已装的包一起抹掉 |
| 子代理必须用独立 PI 进程 | 一个进程只有一个 streaming 状态，同进程起子会话会把父会话的流顶掉 |
| 内置 Node 解压必须平铺到根 | 多一层目录会让 `npm.cmd` 找错 node，表现为「npm 装了但 PI 起不来」；MinGit 同为平铺，且只带 `sh.exe`，解压后复制一份命名 `bash.exe` 才有 bash 可用 |
| PATH 注入必须前置拼接，且只注入当前生效来源的目录 | 后置拼接会被用户 PATH 上的旧 node 抢先，内置 node 等于没生效；用户 node / PI 生效时把自家数据目录也塞进去，会在 PI 子进程里遮住用户正在用的工具 |
| npm 装包跟随当前生效 node | 用户 node 合规（≥下界）就用它的 npm-cli.js，不无故解压内置；npm 与 node 必须同源，装出来的包要在这份解释器上跑 |
| 终端入口目录必须前置到用户 PATH，并扫出抢先的旧 pi | 两份 pi 共存时，界面与终端跑在不同内核上；系统级 PATH 无法修改时只能把冲突显式说出来 |
| 用户本机已有 pi 优先，自装的只在缺装时兜底 | 用户已有 PI 就复用它，绝不装出第二份内核；PATH 扫描按目录顺序进行且跳过自建的终端转发目录 |
| 终端入口钉当前生效那份 PI 与 node，漂移即刷新 | 入口（`pi.cmd` + 给 Git Bash 的 `pi`）指向的不是当前生效份时，界面与终端就是两套内核；启动与升级后都要重刷 |
| 更新是否成功看版本，不看命令退出码 | `pi update --extensions` 会跳过固定版本并成功返回，权限与网络故障也可能成功返回 |
| 交给 exec 的 node 必须是绝对路径 | `exec.Command` 只按本进程的 PATH 找可执行文件；内置 node 不在 PATH 上时，字面量 `node` 就是一句 9705 |
| Node 下界是 22.19.0 | PI 1.4 的 engines 要求；只比主版本会把 22.15 放进来，装包与运行一路 EBADENGINE |
| 升级前先停进程 | Windows 会锁住已加载的 `.node` 原生模块，覆写失败表现为 npm 的 EPERM |
| 环境 `Ready` 判定在 spawn 之前 | 没配模型就 spawn，失败点会推到第一次对话，错误难以归因 |
| 会话/消息/提问等界面词表不变 | 前端只认自己的 `chat:*`；换词表等于重写整个前端 |

详见 `specs/10-pi-supervisor.md` / `11-pi-bootstrap.md`。

---

## 3. 测试规范

**原则**：一次失败必须能指认出一件真实坏事。达不到这个标准的断言不写。

### 3.1 保留判据

写之前先问：**这条测试失败时，用户界面上会出现什么坏事？**
说不出那句坏事，它就不该存在。

| 保留 | 删除 |
|---|---|
| 跨模块 / 跨轮次 / 跨进程的长链路（建会话→发送→落盘→算用量） | 单个纯函数的输入输出 |
| 协议硬约束（分帧、按 id 关联、done 只挂最后、空数组永不 null） | 简单 CRUD、字段映射、枚举转换 |
| 安全护栏（路径穿越、受控 git 写入、命令注入白名单） | 幂等 setter、构造函数冒烟 |
| 顺序语义（内容块顺序、事件顺序、分支历史隔离） | 只验「不 panic」「非 nil」的弱断言 |
| 跨边界的端到端（HTTP / SSE / 子进程 stdio） | 同一行为在不同文件里的重复断言 |

两个硬规则：

1. **一个子测试只讲一种坏法**。子测试是定位与 `-Run` 的把手，失败报文必须能直接指到坏的是什么；
   同一条链路里紧密相关的几个坏法（如事件顺序与字段还原）合并进一个子测试，共享昂贵 setup。
2. **同一个行为只断言一次**。能力域自己的语义归能力域的链，编排层的语义归 service 链，
   不在两处各断一遍。
3. **场景尽量共用仓库与进程**。一份仓库 / 一个假进程是几十次子进程调用；
   能在同一份 setup 上推进场景就不要另起一份——日常验证的耗时大头在这里。

### 3.2 规模与速度约束

- **一个包一个测试入口**：入口统一叫 `Test<Domain>Chain`，文件名 `<subject>_test.go`。
  同包内其余函数一律是 `check*` 辅助（不是 `Test*`），由入口用 `t.Run` 调起。
- **子测试名写成一句「坏的现象」**：它就是定位的把手，也是 `-Run` 的入参。
  查找靠命名，不靠搜代码。
- **昂贵的 setup 只在入口准备一次**：起 HTTP 服务、装配服务容器都属于这一类。
  每个子测试各起一套，加一条分支就在给总时长做乘法。
- **假 PI 夹具全局只编译一次**：`pi/piptest.Build` 按夹具源码内容寻址产物路径并带跨进程锁，
  三个包共用同一份 exe。自己再写一遍 `go build` 会让总时长做三次乘法。
- **不重复起版本探测进程**：`node -v` / `pi --version` 都是真实进程启动
  （约 0.25s），`pi/env` 按可执行文件路径缓存。闸门被反复问同一个问题是常态。
- **装配类测试不真解压归档**：`runtime/runtimetest.SeedMarkers` 预置「已解压 + 版本标记」后，
  装配只跑配置 / DB / 服务 / 工具注册的真实链路。真解压只在 `runtime` 包做一次。
- **慢链路用标准 `-short` 隔离**：只有三处——真 `git` 子进程链路、内置运行时归档真解压、
  本机真 PI 的真实回合。三处都有 `testing.Short()` 守卫，日常 `-short` 直接跳过。
  不要另造自定义 tag 或环境变量开关。
- 环境依赖（系统 shell、真实网络、运行时归档、本机 PI）用 `exec.LookPath` / `t.Skip` 守卫后跳过。
- 每个测试文件**首行必须有导航注释**：写清楚覆盖哪条链路、坏了的表现是什么。
- 多轮对话一律用假 PI 进程：`pi/pitest` 提供一个可编程的 `pi.exe` 替身（Go 写的），
  它走真实 stdin/stdout 与 LF 分帧，也像真 PI 一样把会话写进 JSONL。

### 3.3 验证入口

一个脚本收口全部验证动作，不在文档 / CI / 本地各写一套命令。分档按「改了多大」选：

| 命令 | 跑什么 | 什么时候用 |
|---|---|---|
| `pwsh scripts/check.ps1` | 格式 → 注释规范 → 编译 → 全部短测试（dist 缺失先建前端） | 日常快门 |
| `pwsh scripts/check.ps1 -Pkg backend/pi/env` | 格式 → 编译该包 → 测该包（短），**不碰前端** | 明确只碰了一个包 |
| `pwsh scripts/check.ps1 -Run 冲突` | 格式 → 编译 → 只编译含测试的包，只跑名字命中的子测试 | 改了某一条行为，要一眼看到它 |
| `pwsh scripts/check.ps1 -List` | 列出全部入口与其下子测试名（一棵可 grep 的索引） | 不知道去哪找时先拿索引 |
| `pwsh scripts/check.ps1 -Full` | 前端构建、vet、依赖边界、全量测试 | 改了前端 / 发版前；CI 跑这一档 |

- **默认安静**：只有 `-Run` 档带 `-v`（那时量小）。全量 `-v` 是几百行刷屏，对定位没有帮助；
  失败时脚本自己从日志里摘出摘要（子测试名 + `.go:行号`）。
- **`-Run` 可以只给子测试名片段**：`-run` 的参数按 `/` 分段匹配名字的每一层，脚本给它拼
  一个空前缀（`/冲突`），空段匹配任意入口名——于是不必知道它挂在哪个 `Chain` 下。
- **`-Run` 只编译含测试的包**：按磁盘上的 `*_test.go` 现算包清单，不跑测试的包不参与编译。
- **前端只在要编译 main 包时才建**：`frontend/dist` 只是 `main.go` 的 `go:embed` 硬依赖，
  `-Pkg backend/...` 一概不碰它——否则验一个后端包要白等十几秒的前端构建。
- **全量测试一律 `./...`**：不在那里维护「有测试的包」清单，那份清单会漂。

短测试跳过真 git 链路、归档真解压与本机 PI 真实回合；这三条只在 `-Full` 跑到，
CI 跑这一档，所以本地不跑不等于没人跑。
失败摘要收在脚本内，不另起一个脚本：两个文件就要维护两处。
`-race` 需要 cgo（本项目默认关 CGO 以保持 SQLite 纯 Go），要跑得先装 MinGW 再
`CGO_ENABLED=1 go test -race ./backend/...`；没装 C 编译器时它直接 `build failed`，那不是回归。

实测耗时（本机）：`-Pkg` 约 5s（不含前端）、`-Run` 约 12s、快门约 12s、`-Full` 约 50s。
耗时大头是编译与链接测试二进制、以及真 git 子进程链路；断言本体多在亚秒级。

### 3.4 测试索引

6 个入口 / 6 个文件（一包一入口），共 24 条子测试，每条只讲一种坏法。
没列进来的包（server / config / db / domain / pkg / repo / skill / tray / singleinstance，以及壳层）
不单独设测试：它们的正确性由流经它的链路断言。

| 入口 | 覆盖的链路 |
|---|---|
| `pi/pi_test.go` · TestPiChain | 五条链：**协议**（分帧、未知记录、按 id 关联与四种 fire-and-forget 扩展请求字段）；**事件与翻译**（顺序 agent_end → 重试 → settled、流式增量两类、工具结果与压缩）；**提问桥**（请求字段解析、响应写回同一 id）；**生命周期与进程归属**（有序退出、崩溃报错、死进程命令立刻失败、一个工作区一个进程）；**本机 PI 真实回合**（`-short` 跳过） |
| `pi/env/env_test.go` · TestPiEnvChain | 五条链：**环境闸门与 agent 目录**（node → pi → model 报首个阻塞项并给原因；agent 目录跟随 `PI_CODING_AGENT_DIR` 且不被进程环境覆盖）；**配置写入**（models.json 的 camelCase 形状与未知字段保留，settings.json 只改掌管键且 compaction 字段级合并、packages 两种写法、原子替换，mcp.json 原文往返）；**入口与终端位置**（shim 还原成 node + 脚本且绝不吐字面量 node；入口目录被前置且能认出抢先的旧 pi；入口是 pi.cmd + pi 双份、sh 里只有正斜杠路径）；**用户 pi 优先**（PATH 上那份抢先于自装前缀，自生成的终端转发不算数）；**命令执行环境**（WSL 启动器被剔除；shellPath 引导、更好的 bash 出现时撤换、自己写过的才撤回；defaultTools 永不被碰） |
| `api/api_test.go` · TestHTTPChain | 真实 HTTP 栈两条链：**闸门与契约**（预检放行 + 方法白名单 + 路径穿越门口拒绝 + 空数组永不 null + 引导字段支撑环境闸门）；**SSE 全链**（新对话现场创建 → 子进程 → 翻译 → Hub → 帧，seq 递增、`data` 回带 `event`、用量落袋、上一轮收尾后再发下一轮）与 Last-Event-ID 重连。服务只启一次，子测试共用 |
| `service/chain_test.go` · TestServiceChain | 六条链：**会话索引**（列表 / 详情 / 内容块顺序 / 用量口径 / 工具清单 / 删除删的是文件）；**一轮对话与用量**（新会话现场创建、done 最后、重试事件、落盘后能算出用量与按模型归因）；**停止与空闲排队**（先取回排队文本再中止、abort 落内核标记、空闲 follow_up 等价完整一轮、ANSI 码进界面前被剥）；**技能与插件**（技能三区启停靠移目录、包内技能不可搬、坏技能文件只跳自己、插件登记项与包的来源种类、停用扩展摘登记项）；**模型服务与 MCP**（models.json 密钥掩码与默认模型、服务商缺少名字 / 地址 / 模型 ID 分别报 3109 / 3113 / 3111、坏 MCP JSON 不落盘、删除连带清默认、留空的模型能力按默认值兜底）；**跨工作区并行**（各自一个进程，不串会话） |
| `git/git_test.go` · TestGitChain | 真仓库端到端（唯一用真 git 子进程的链，约 30 秒），按坏法分五条：**读**（路径换算与状态归类 / 中文带空格路径还原 / 差异行号与文件头 / 未跟踪整篇新增 / 越界拒绝 / 非仓库给人话）；**提交**（勾选入栈 → 空信息被拒 → 提交带 hash → 历史与提交详情可读 → 出栈只动索引 → 丢弃与 amend 的护栏 → 失败归因报 1017/1020 而不是 exit status 1）；**推送**（首次推送自动建上游 / 远端领先报 1024 / 带租约强制推送报 1027 / 确认后的纯强制推送能过）；**合并冲突**（冲突落地为可继续状态 / 未解决就提交或继续报 1021 / 逐文件标记后继续 / 中止回原状）；**分支**（注入面白名单 → 新建切换 → 合并 → 历史隔离 → 删已合并）。场景共用仓库；`-short` 与没装 git 都跳过 |
| `runtime/runtime_test.go` · TestBundledRuntimeChain | 一条链：**归档完整性、解压安全与真实布局**（SHA 常量一致、LFS 指针当场识破、zip-slip 拒绝且正常路径不误伤；真实解压验布局——Node 剥掉顶层单目录、EnsureGitBash 全链产出 bash.exe 与二次命中标记；`-short` 跳过真解压） |

**改这里 → 只跑这条**（定位与验证都从这张表进；单包一律用脚本）：

| 改动位置 | 命令 |
|---|---|
| `pi/`（子进程、协议编解码、事件归一） | `pwsh scripts/check.ps1 -Pkg backend/pi` |
| `pi/env/`（Node 探测、PI 安装、配置互通） | `pwsh scripts/check.ps1 -Pkg backend/pi/env` |
| `service/`（会话、对话、落库、编排） | `pwsh scripts/check.ps1 -Pkg backend/service` |
| `api/` · `server/`（端到端 HTTP + SSE） | `pwsh scripts/check.ps1 -Pkg backend/api` |
| `skill/` | 由 `-Pkg backend/service` 覆盖（技能解析流经配置落盘链路） |
| `git/` | `pwsh scripts/check.ps1 -Pkg backend/git`（短档会跳过真 git 链，改这里要跑 `-Full`） |
| `runtime/` | `pwsh scripts/check.ps1 -Pkg backend/runtime` |
| 某一条子测试 | `pwsh scripts/check.ps1 -Run '提问'`（或 `-Run 'TestPiChain/提问'`） |
| 不知道入口叫什么 | `pwsh scripts/check.ps1 -List` |
| 拿不准 | `pwsh scripts/check.ps1` |
| 改了前端 | `cd frontend; npm run build`（或直接跑 `-Full`） |
| 提交前 / CI | `pwsh scripts/check.ps1 -Full` |


---

## 4. 架构门禁

```bash
pwsh scripts/check.ps1 -Full         # 格式 + 前端构建 + 编译 + vet + 依赖边界 + 全量测试
wails build                          # 产物 build/bin/WorkBaby.exe
```

脚本逐步展开就是下面这些命令；单独跑某一步时才手敲：

```bash
gofmt -l .                           # 必须无输出：Go 源码统一 LF（见 .gitattributes）
go build ./...                       # 编译比测试更早暴露类型错误，覆盖全部包
go vet ./...                         # 只在 -Full 跑
go run ./tools/check-boundaries      # 依赖方向门禁，只在 -Full 跑
go test -short -count=1 ./...        # 短测试：真 git 链路、归档真解压、本机 PI 回合都跳过
go test -count=1 ./...               # 全量（-Full）：上面三条慢链路只有这里跑到
cd frontend && npm run build         # vue-tsc -b && vite build：类型检查已串在构建里
```

测试一律喂 `./...`：不维护「有测试的包」清单——清单会漂，漂了就是测试悄悄不跑；
没有测试文件的包进来只是多一次编译命中缓存。

`gofmt -l .` 要能当门禁用，行尾就必须统一：`.gitattributes` 里 `*.go text eol=lf`
（Windows 上 `core.autocrlf=true` 会把检出内容写成 CRLF，而 gofmt 把 CRLF 文件一律
报成「未格式化」）。§2.6 的注释行数上限由 `check.ps1` 的 `comments` 步骤机械核对
（无 `rg` 依赖，快门与全量都跑）。

`tools/check-boundaries` 用 `go list -json` 读编译器视角的真实 import 关系，
逐层比对 §2.2 的允许表，并额外检查能力域不回引上层。它不做文本 grep——注释、
字符串字面量与构建标签分支都会让 grep 得出错误结论。改动包结构或调整依赖后必须跑通。

文档与实现不一致时，**以本文为准**；发现实现跑偏就改实现，不要改本文迁就代码。

---

## 5. 工作流

- 最小改动：外科手术原则；改动前后跑测试
- 验证优先：修 Bug 先写复现测试
- YAGNI：禁止过早抽象、单实现接口
- **内核归 PI，界面归 WorkBaby**：想加一个 agent 能力时，先确认 PI 是否已有；
  已有就在界面上暴露它，不自研第二份
- **文档与代码同步**：改行为时同时改对着这篇行为的文档与注释，不同步视为未完成
