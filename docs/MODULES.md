# 模块设计

这一篇逐个模块讲清**它是什么、怎么实现、为什么、优势与代价**。
包结构与依赖方向的硬规则在 `AGENTS.md`，启动与请求时序在 `ARCHITECTURE.md`；
这里是每个模块自己的设计账。

## 模块与规格索引

| 模块 | 负责什么 | 规格 |
|---|---|---|
| `backend/server` | 路由表、SSE hub、统一信封、方法白名单 | [API-CONTRACT](API-CONTRACT.md) |
| `backend/api` | 薄 handler + 系统能力绑定 + 组合根 | [API-CONTRACT](API-CONTRACT.md) |
| `backend/service` | 全部业务编排与事件翻译 | 各规格 |
| `backend/pi` | PI 子进程监督、RPC 编解码、事件归一、会话文件索引 | [10](../specs/10-pi-supervisor.md) |
| `backend/pi/env` | 运行时复用、PI 安装与探测、命令执行环境、终端入口、配置互通 | [11](../specs/11-pi-bootstrap.md) |
| `backend/skill` | 技能解析与注册表（目录即 PI 的技能目录） | [03](../specs/03-skill.md) |
| `backend/git` | 工作区版本视图（读全开放 + 写白名单动作） | [08](../specs/08-workspace-git.md) |
| `backend/runtime` | 路径解析 + 内置 Node / Python 运行时 | [01](../specs/01-runtime.md) |
| `backend/repo` · `domain` · `pkg` | 设置 KV / 域模型 / 叶子工具包 | [DATA-MODEL](DATA-MODEL.md) |
| `frontend/src` | 六个路由视图 + 一套设计令牌 | [PAGE-STRUCTURE](PAGE-STRUCTURE.md) · [DESIGN](../DESIGN.md) |
| 壳层（`app.go`）· `tray` · `singleinstance` | Wails 生命周期、托盘、单实例、退出流程 | [04](../specs/04-desktop-shell.md) |

目录顺序即依赖顺序（上层在先、叶子在后）。

## server — HTTP 边界

**是什么**：gin 路由表、SSE hub、统一响应信封、方法白名单。它只认识 `api` 与 `domain`，
不知道任何业务编排。

**怎么实现**：

- `routes.go` 一张表收口全部路由：`/api/v1/{resource}/{action}/:id`，动作段在 id 之前，
  删除统一走 `POST /{resource}/delete/:id`，只允许 GET 与 POST
- `server.go` 三件事：CORS（只放行本机来源）、方法白名单（PUT/DELETE 直接 1002）、
  路径参数穿越检查（`:id` 里的 `..`、`%2F` 在门口拒绝）
- `sse.go` 是事件 hub：每个会话一条缓冲队列，支持 `Last-Event-ID` 重放；
  缓冲溢出先丢 delta 保关键事件，再溢出就断开并让前端拉快照对账（`chat:gap`）
- 响应信封在 `api/api_biz.go`：`ok()` 对切片 / map 做 nil 兜底（Go 的 nil slice 编码成
  `null`，前端按数组读就是运行时错误）

**为什么**：边界集中在一处，才能保证「所有出参永远是同一个形状、所有入口过同一组闸」。
分散到各 handler 的写法在第十个接口之后必然漂移。

**优势**：前端可以假设契约永远成立（数组不是 null、错误必带 code）；加接口只改一处表。
**代价**：每加一种动作要同时动 routes 与 api 两个文件。

---

## api — 薄 handler 与组合根

**是什么**：HTTP 入口到业务编排之间的翻译层，同时是**唯一的装配点**（组合根）。

**怎么实现**：

- `Handler.Startup` 是唯一装配入口：解析路径 → 打开 DB → 装配 repo / config / 各服务容器 /
  PI 监督器 / 技能注册表。装配结果挂在 `Handler` 与 `service.Env` 上
- 每个 handler 方法只做三件事：解析入参 → 调 service → `ok()/fail()` 包信封；
  唯一例外是查询型系统能力（运行时状态检测、对话框、自启），它们与业务编排无关
- 系统能力绑定（剪贴板、窗口、对话框、自启）分 Windows 与其它平台两个文件（`//go:build`），
  不写 `runtime.GOOS` 大分支

**为什么**：装配点只有一个，依赖图才可审计（`tools/check-boundaries` 按这个点放行）；
handler 薄，业务逻辑才有且只有一处（service）。

**优势**：测试可以直接起 `Startup` 拿到完整装配（api/api_test.go 就这么干）；
handler 不含逻辑，改业务永远不用碰它。
**代价**：`Handler` 结构体比较大（它就是依赖图本身）；新增依赖要过组合根这一关。

---

## service — 业务编排

**是什么**：全部业务流程的家：会话索引、对话运行、模型服务、技能、插件、工具、MCP、
统计、设置、工作区文件、git 编排、事件翻译。一个关注点一个文件。

**怎么实现**：

- `Env` 是服务间的共享依赖容器（repo、配置、PI 环境句柄、Emitter）；`New(env)` 装配全部服务
- `ChatService` 是最复杂的一个：登记 runState（runID、busy 标记、提问表）、
  管理插话 / 排队 / 停止队列、把 PI 事件翻译成 `chat:*` SSE 事件
- `Emitter` 是唯一事件出口：注入会话归属、分配单调 seq、按会话分道串行化——
  「分配 seq」与「投递」是一次原子步，否则前端按 seq 去重会丢事件
- `translate.go` 把 `pi.Event` 翻成界面词汇表：这一层是「协议 → 界面」的唯一翻译点，
  ANSI 色码在这里剥（GUI 没有终端语义）
- `engine.go` 是引擎页的数据出口：汇总 PI / 运行时 / 插件包的版本与运行状态，
  管住「安装 / 升级只有一个在跑」的闸门，升级完重启全部进程让新内核生效

**为什么**：编排层是事务边界与词汇表边界。能力域（pi/skill/git）只交出原始事实，
把事实翻成界面能直接渲染的东西是 service 的职责——这样能力域才能脱离界面独立测试与演进。

**优势**：每个关注点独立成文件，改统计不用碰对话；事件翻译只有一处，词汇表不会漂。
**代价**：`ChatService` 承担了运行态管理的复杂度（runState / 队列 / 提问表），是全仓最大的一个类型。

---

## backend/pi — PI 能力域

**是什么**：PI 子进程的监督者：拉起、守护、RPC 编解码、事件归一、会话文件索引。
它不实现 Agent——它运行并观测 Agent。

**怎么实现**：

- `client.go`：spawn `node <pi>/dist/bundle/cli.js --mode rpc`（npm shim 先还原成 node + 脚本，
  经 cmd.exe 起进程会闪控制台窗口；全部子进程 `CREATE_NO_WINDOW`）。
  stdout 按 LF 分帧（U+2028/U+2029 在 JSON 字符串里合法，按 Unicode 行分隔切会截断记录），
  每条记录按 `type` 分类：`response` 按 id 唤醒等待者、已知事件进翻译、
  `extension_ui_request` 分流、未知记录 debug 后丢弃（装了扩展的环境首帧就是它）
- `supervisor.go`：一个工作区一个进程（`cwd` 是进程级属性）。同工作区多条会话靠
  `switch_session` 切换；换工作区必然换进程
- `session.go`：读 PI 的会话 JSONL——列表按目录扫描（size+mtime 缓存）、
  详情按条目链投影、删除就是删文件。**只读不写**：JSONL 是 PI 的私有契约
- `pitest/` + `piptest/`：假 PI 进程夹具（Go 写的 RPC 替身，走真实管道也落会话文件），
  按夹具源码内容寻址全局只编译一次

**为什么**：PI 是外部进程，协议、进程生命周期、会话文件格式这三件事自成一个世界。
把它们关在一个包里（只依赖 `pkg` 与 `runtime`，不依赖 `domain`），
这个包就能被三个包的测试直接驱动，也能在 PI 升级协议时单独适配。

**优势**：内核升级只改这一个包；「谁在跑、跑在哪」有唯一答案。
**代价**：协议字段必须跟 PI 对齐（`protocol.go` 的 camelCase 是 PI 的，不是我们的）；
PI 改协议这里最先碎。

---

## backend/pi/env — 环境引导与配置互通

**是什么**：PI 的发现与安装、环境闸门判定、命令执行环境与终端入口的准备、
`models.json` / `settings.json` / `mcp.json` 的读写。

**怎么实现**：

- 发现：PATH → npm 前缀（内置 npm-global）逐级找 `pi.cmd`；找到后把 shim 还原成
  `node + cli.js`。**node 永远是绝对路径**：`exec.Command` 只按本进程的 PATH 找可执行文件
- 安装 / 升级：固定用**内置** `node + npm-cli.js`，前缀显式给出；官方源与国内镜像并联探测，
  一趟失败换另一个源整跑一遍（完整流程与原因见 [specs/11](../specs/11-pi-bootstrap.md）)
- 版本对账：本地版本读包目录的 `package.json`，最新版本问 npm 注册表的
  `dist-tags.latest`（abbreviated 元数据 + 5 分钟缓存 + 12 秒超时 + 换源重试）
- 命令执行环境（`shell.go`）：找真 bash（环境变量 / 注册表 / PATH 上的 `git.exe` 反推安装根）、
  按路径剔除 WSL 启动器、真跑一次验证；有 bash 写 `shellPath`，没有就把 `defaultTools`
  增量换成 `+powershell` / `-bash`。自己写过的值记在 `runtime/shell.json` 才敢撤回
- 终端入口（`terminal.go`）：写 `<data>/bin/pi.cmd`（钉死内置 node）+ 前置到 HKCU
  `Environment\Path`，写后广播 `WM_SETTINGCHANGE`
- 闸门 `Detect`：按 node → pi → model 顺序报**第一个**不满足项并给原因
- `WriteModels` / `WriteSettings`：camelCase 形状、未知字段原样保留、只改掌管键、字段级合并；
  `mcp.json` 只做原文往返；全部写入走 `WriteFileAtomic`

**为什么**：agent 目录是用户的（`PI_CODING_AGENT_DIR` 只继承不覆盖），WorkBaby 是
「共享租户」不是「所有者」——写配置的每一条纪律都从这来。

**优势**：界面改的配置 `pi` 命令立刻能用（同一份文件）；用户手写内容永远不会被抹掉。
**代价**：字段级合并比整体重写复杂；PI 加新字段时要确认「是不是掌管键」。

---

## backend/git — 工作区版本视图

**是什么**：git 的受控封装。读全开放（status / diff / 历史 / 分支），
写限白名单动作，全部走系统 git 命令。

**怎么实现**：

- `scope` 是核心抽象：一次操作先 `rev-parse --show-toplevel --show-prefix` 拿到
  仓库根与工作区前缀，之后**所有命令都在仓库根执行、路径参数都是仓库根相对**。
  界面用工作区相对路径，换算只在 scope 内发生——status 给出的路径可以原样喂回
  Stage / Diff / Discard，不必自己拼前缀
- `parsePorcelainZ` 用 `status --porcelain -z`：NUL 分隔不做路径转义，
  中文与空格路径不需要反解八进制；改名记录占两条 NUL 段要跳过第二条
- `parseDiff` 把统一差异折成行结构（file / hunk / add / del / ctx + 双侧行号），
  文件头路径经 scope 折回工作区相对；未跟踪文件 git diff 给不出内容，逐个补读正文
  （8MB 上限，目录跳过）
- 写入：每个动作一个导出函数，参数逐个校验（分支名 / 远端名 / 地址白名单，
  以 `-` 开头一律拒——那是 git 的选项面）。失败归因**合并 stdout 与 stderr**：
  git 把 `nothing to commit`、`no upstream branch` 写在 stdout，只看 stderr 只会报
  `exit status 1`。归因按稳定关键词分流成 1017/1020/1021/1024/1027 这些可执行的错误码
- 冲突不是失败：merge / rebase / pull 产生冲突时留下未完成的合并，状态里带
  `state`（merge / rebase / cherry-pick / revert）与逐文件 `conflict` 标记；
  继续（`commit --no-edit` / `rebase --continue`，配 `GIT_EDITOR=:`）、中止（`--abort`）各自一个动作
- 推送分级：`none`（默认）→ `lease`（`--force-with-lease`，远端变了就拒）→
  `force`（必须输入分支名确认）。**带租约的推送故意不前置 fetch**：
  租约比的是本地远端跟踪引用，先 fetch 会把「我上次同步后没人推过」这个前提抹掉，
  安全档就变成了无条件覆盖
- 推送 / 拉取全程 `GIT_TERMINAL_PROMPT=0`：GUI 里没有终端，等凭据输入的结局是卡到超时

**为什么**：写版本历史不可逆，白名单 + 显式校验是唯一的闸；路径换算收敛到 scope
是因为「两套相对路径」是这类封装最容易错的地方（实测在子目录工作区里必然碎）。

**优势**：git 语义永远与 Git 一致（零解析 `.git`）；读链路能被单测直接驱动；
冲突处置留在界面里完成，用户不必回命令行。
**代价**：机器必须装 git；每条链路都是真实子进程（测试约 30 秒，-short 跳过）；
`cherry-pick` / `stash` / `reset --soft` 未开放——它们的中间状态界面没有对应引导。

---

## backend/skill — 技能注册表

**是什么**：扫描、解析、启停 PI 的技能目录。目录即 PI 的技能目录，不另立存储。

**怎么实现**：

- `LoadDir` 扫 frontmatter（标量 / 列表 / 行内数组都认），坏文件只跳自己不中断扫描——
  解析失败是静默的，用户视角是「这个技能没装上」
- 三个来源：全局（`~/.pi/agent/skills/`）、工作区（`<ws>/.pi/skills/`）、包内
  （`npm/node_modules/*/skills/`）。包内技能只读：启停 = 停用整个包
- 启停 = 移目录：停用的移到 `skills.disabled/`（PI 就不扫了），启用移回来。
  不写任何标记文件——目录位置就是唯一状态

**为什么**：技能是 PI 按目录约定加载的，WorkBaby 自己存一份「启用状态」必然与 PI 的
实际行为分叉；移目录让两边的真相永远一致。

**优势**：界面看到的清单就是 PI 真正会加载的；命令行装的技能自动出现在界面。
**代价**：启停涉及文件移动（目录被占用时报错）；「停用」不能用于包内技能。

---

## backend/runtime — 路径解析与内置运行时

**是什么**：两件事：数据目录 / agent 目录 / 各文件路径的唯一解析点；
内置 Node 24 / Python 3.12 的 embed 与解压。

**怎么实现**：

- `Resolve` 按优先级定目录：`WORKBABY_HOME` → `%APPDATA%\WorkBaby`；
  agent 目录跟随 `PI_CODING_AGENT_DIR`，**只继承不注入**
- 归档经 `go:embed` 进 exe（Git LFS 托管源文件），启动装配期解压到数据目录，
  `.version` 标记命中版本跳过；解压前逐条目路径检查（拒绝绝对路径与 `..`）、512MB 上限；
  嵌进去的是 LFS 指针文本时按「包不可用」报错
- 内置运行时是**兜底**：系统已有合规版本（Node ≥ 22.19.0、Python 3.x）直接复用；
  Node 官方 zip 多一层顶层目录，解压时剥掉平铺（否则 `npm.cmd` 找错 node）
- PowerShell 从不内置，走系统自带的 5.1

**为什么**：目标是「用户机器上什么都没装也能起来」；但内置只是兜底——
用户自己装的高版本运行时体验更好，能复用就复用。

**优势**：分发只有一个 exe；不联网也能完成环境引导。
**代价**：exe 约 90MB；构建期强依赖 LFS 拉取；首启多等一次解压（约 3 秒）。

---

## backend/repo · db · domain — 持久层与域模型

**是什么**：GORM 持久化、SQLite 打开与迁移、域模型。

**怎么实现**：

- SQLite（WAL，pure-Go 无 CGO）只有一张 `settings` 表（KV）。迁移只增不删，
  旧版本残留的表不主动 DROP
- domain 一个聚合根一个文件：DO（数据库）/ DTO（能力域间）/ REQ（入参）/
  VO（视图）/ RESP（出参）同居一处；包级错误变量集中在文件顶部
- `pkg.FillNilSlices` 在 HTTP 出口统一兜底 nil 切片；构造处也一律初始化——
  双保险，兜底只救漏掉的那一次

**为什么**：会话、消息、用量都是 PI 的文件，再存一份就会分叉（见 [ARCHITECTURE](ARCHITECTURE.md)
「两个真相源」）；SQLite 只存界面偏好，让这类分叉在结构上不可能发生。

**优势**：没有需要迁移的业务表，卸载即净；域模型与数据库形态一处可查。
**代价**：所有跨层数据都要显式定义类型（不能就地声明结构体），类型文件较多。

---

## backend/pkg — 叶子工具包

**是什么**：无业务语义的工具：AppError、ID、日志、路径安全、HTTP 规则、文件辅助。

**怎么实现**：

- `AppError`：`code`（数字字符串）+ `message`（给用户）+ `details`（定位）+ `Err`（可 Unwrap）。
  错误码分段（详见 `docs/ERROR-CODES.md`），段位表在 AGENTS.md §2.4
- `SafeJoin`：拼路径 + 拒绝绝对路径与 `..`；`RealPath`：EvalSymlinks + Windows 短名展开。
  两者配套使用：短名（`RUNNER~1`）与长名字面不等，直接 `filepath.Rel` 会算出 `../..`
- `httprules`：GET/POST 白名单、路径参数穿越检查——server 的中间件只是调它

**为什么**：叶子工具包被全部层依赖，它自己必须不依赖任何业务包——
否则依赖方向整个作废。业务词汇（session / provider / chat）不允许出现在这里。

**优势**：路径安全与错误形态全仓只有一套实现。
**代价**：每加一个工具要想清楚它是不是真的无业务语义。

---

## backend/tray · singleinstance — 系统能力

**是什么**：托盘与单实例保护。

**怎么实现**：

- 托盘：关闭到托盘（可关），托盘菜单提供「打开 / 退出」。平台特定代码分文件（`//go:build windows`）
- 单实例：数据目录文件锁 + 本地 TCP IPC。二次启动把文件路径转交给已有实例后退出，
  已有实例唤起窗口并打开该文件

**为什么**：托盘与锁都是纯系统能力，与业务无关；独立成包让它们可以被单独替换或删除。

**优势**：主程序不感知实现细节。
**代价**：多两个小包；IPC 协议要自己维护。

---

## frontend/src — 界面

**是什么**：Vue 3 + TypeScript 单页应用，六个路由视图 + 一套设计令牌。

**怎么实现**：

- 目录一层：`api`（后端接口收敛）/ `components`（按域分：layout / chat / dashboard /
  settings / workspace / common）/ `composables` / `locales` / `stores`（Pinia Setup Store）/
  `types` / `utils` / `views` / `router`（hash 模式）
- 状态归属：`session`（权威快照）/ `chat`（当前 run 的临时态）/ `settings`（配置）/
  `toast`。**SSE 只推进行中的临时态**，任何 done / 重连都拉一次权威快照，前端不做增量合并。
  事件消费在 `composables/chatBridge.ts`（应用壳装一次），不随对话页卸载：
  离开对话页时 run 还在跑，事件必须继续落进 store
- 四个 CSS 分工：`themes.css`（色值与尺寸刻度唯一来源，`--wb-*` 令牌 + 两套主题）/
  `ep-theme.css`（Element Plus 变量接到 `--wb-*`）/ `base.css`（重置与基元）/
  `content.css`（EP 没有对应物的自绘区：消息流 / markdown / 工具卡 / diff）
- Element Plus 按需引入（`unplugin-vue-components`），禁止全量注册；
  图表 ECharts 按需注册，色值取 `--wb-ch-*`
- 消息流按内容块顺序渲染（`MessageVO.blocks`），长会话给非末尾条目加
  `content-visibility: auto`；贴底滚动由 JS 管（关掉浏览器的滚动锚定）
- 尺寸刻度乘 `--wb-vw-scale`（窗口宽 ÷ 1440，0.8~1.35）：不同分辨率整套刻度等比跟随

**为什么**：界面是「看见并控制」的落点。临时态与权威态分开是因为增量合并是
「两个会话叠在一起」这类 bug 的温床；设计令牌集中是因为视觉漂移只能靠单点定义防住。

**优势**：主题、缩放、EP 与自绘的视觉一致性都靠令牌保证；接口层收敛后后端改路径只改一处。
**代价**：令牌纪律要求所有组件不写裸色值 / 裸像素，写新组件时多一道约束。

---

## tools/check-boundaries — 依赖门禁

**是什么**：读编译器视角的真实 import 关系（`go list -json`），逐层比对允许表。

**怎么实现**：按包枚举 import，对照 AGENTS.md §2.2 的允许 / 禁止表；
额外检查能力域不回引上层。不做文本 grep——注释与字符串里的 import 字样会让 grep 得出错误结论。

**优势**：架构规则可机械验证，`check.ps1 -Full` 里跑。
**代价**：改包结构或调整依赖时要同步改它的规则表。

---

## scripts/check.ps1 — 验证入口

**是什么**：唯一验证入口，分档收口：gofmt → 注释规范 → 前端构建（dist 缺失或 `-Full` 时）→
编译 → vet 与依赖边界（仅 `-Full`）→ 测试。

**怎么实现**：

- 档位：无参快门（全部短测试，约 12s）；`-Pkg` 单包（约 5s，**不建前端**）；
  `-Run <关键字>` 定点（`-run` 拼空前缀让子测试名不必带入口名，且只编译含测试的包，约 12s）；
  `-Full` 全量门禁（约 50s）；`-List` 打印入口与子测试索引
- 前端只在要编译 main 包时才建：`frontend/dist` 只是 `main.go` 的 `go:embed` 硬依赖，
  其余包一概不依赖它——否则验一个后端包要白等十几秒的前端构建
- 测试一律跑 `./...`（`-Run` 除外：按磁盘上的 `*_test.go` 现算包清单）
- 失败时从日志摘摘要（`--- FAIL` / `panic` / `文件:行号`）打印，CI 里追加进 job summary
- `comments` 步骤把 §2.6 的「连续注释 ≤ 2 行」变成门禁（`gofmt` 给 `//go:` 指令补的分隔行豁免）
- `-short` 跳过三条慢链路（真 git 子进程、归档真解压、本机真 PI 回合），只在 `-Full` 跑到

**为什么**：验证命令散在文档 / CI / 本地各写一套必然漂移；一个脚本一个入口，
改验证流程只改一处。

**优势**：改一个小功能时单包 5 秒、定点 12 秒；失败摘要直接给到行号。
**代价**：脚本本身有档位逻辑（约 180 行），改它要理解各档的关系。
