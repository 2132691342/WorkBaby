# WorkBaby

Windows 单机 AI 工作台：一个把智能体引擎装进 GUI 的桌面应用。

智能体循环、模型协议、工具执行、会话落盘全部是本仓库的 Go 代码，跑在同一个进程里。
配好一个服务商的 API Key 就能开始干活——不需要装引擎、不需要装 node、不需要插件生态。
机器上已有的 git / node / python 按需探测、直接调用。

## 能力

| 能力 | 具体做什么 | 落点 |
|---|---|---|
| 对话与工具循环 | 流式输出、思考折叠、工具卡片与差异视图、停止 / 插话 / 排队、上下文水位、手动与自动压缩 | `agent/loop.go` · `service/chat_*.go` · `views/ChatView.vue` |
| 计划模式 | 会话级只读闸门：写工具拦执行不拦声明，状态写进会话文件 | `agent/policy.go` · `service/plan_mode.go` |
| 提问卡 | 模型用 `ask` 工具发 confirm / select / input / editor 四种卡，等用户答完再继续 | `agent/tool_ask.go` · `POST /chat/answer` |
| 待办 | `todo` 工具维护任务清单，对话页的待办面板实时显示，内容由活动分支上最近一次调用派生 | `agent/tool_todo.go` · `service/parser/todos.go` · `components/chat/TodoPanel.vue` |
| 会话 | 列表与全文检索、改名、删除、隐藏单轮、复制、从某轮分叉、导出 Markdown / HTML | `service/sessions.go` · `session_html.go` · `views/SessionsView.vue` |
| 技能 | 10 个推荐技能一键装齐；启用 / 停用靠目录搬移，可编辑、新建、导入、重置、删除 | `service/skills*.go` · `views/SkillsView.vue` |
| MCP | 登记 stdio / HTTP 服务与开关，「测试连接」走真握手；引擎装载的工具只来自 stdio | `service/mcp*.go` · `agent/mcp_client.go` · `views/McpView.vue` |
| 工作区文件 | 文件树与搜索、读写 / 新建 / 重命名 / 删除，Markdown、代码、PDF、Word、Excel 预览 | `service/files.go` · `views/WorkspaceView.vue` |
| Git | 状态 / 差异 / 提交历史 / 暂存 / 提交 / 分支 / 合并 / 冲突处理 / stash / 初始化 / 远程推拉 | `service/git*.go` |
| 预索引 | 工作区文件与符号建索引，SQLite 持久化，模糊相关度叠 frecency 排序 | `backend/index/` · `service/searcher.go` · `views/IndexView.vue` |
| 联网 | DuckDuckGo / Brave / Exa / Tavily 四类搜索 + 正文抽取（HTML / PDF / JSON） | `backend/web/` · `service/web.go` · `views/WebView.vue` |
| 用量统计 | token 与成本聚合，按天趋势与按模型占比，数字只从会话文件读，不另存一份 | `views/UsageView.vue` |
| 提示词模板 | md + YAML 头的模板增删改，与内置命令一起进输入区的斜杠菜单 | `service/templates.go` · `composables/useBuiltinCommands.ts` |
| 服务商与模型 | 多服务商多模型的增删改查、连通性探测、思考档按会话切换、新会话默认模型 | `service/models.go` · `views/ModelsView.vue` |
| 桌面集成 | 系统托盘、关窗行为偏好、开机自启（HKCU Run）、最近工作目录、直达数据目录 | `shell/tray.go` · `service/platform/autostart_windows.go` |

## 设计要点

**引擎在进程内。** `backend/agent/` 是纯 Go 包，只依赖标准库与 `pkg`、`domain`、`web`，
不知道 HTTP、界面与上层编排存在。没有子进程要拉起、没有握手协议、没有崩溃重启、没有版本探测。

**壳与接口走本地 HTTP。** Wails3 无边框窗口（前端自绘标题栏）内嵌一个监听 `127.0.0.1:0` 的 gin 服务，
每次启动随机令牌（HTTP 走 `X-Desktop-Token`，SSE 走 `?token=`）；只允许 GET / POST，动词靠路径段表达。

**事件顺序是契约。** SSE 帧带单调 `seq`，断线按 `since` 补帧，心跳保活；补帧与实时帧在同一队列里定序。

**会话 JSONL 是唯一真相。** 每轮对话追加进 `sessions\<工作区编码>\SES_*.jsonl`，
文件头只在创建时写一次、之后永不重写：改名也是追加一条 `session_info` 条目。
标题、待办、计划模式与活动分支都由条目回放派生；唯一例外是隐藏轮的台账，它记在 `workbaby.json`。

**工具注册表决定能力边界。** 现役内置工具是 `read` `write` `edit` `ls` `grep` `find`
`bash` `todo` `ask`，索引与联网工具按配置条件装载，MCP 工具整体替换注册。
系统提示里的工具清单从注册表现读，两者不会不一致。

**计划模式是注册表上的一个闸门。** 只读态挂在 `ToolPolicy` 上，一次实现管住全部写工具；
判定规则「未登记按写处理」——漏登记写工具的代价远大于漏登记读工具。

**运行时不随包分发。** git / node / python 一律探测系统已装（`pkg.LookPath` 自己扫 PATH，
不起 `where` / `which`），缺了就在应用页标出来。子进程一律隐藏窗口、超时必杀。

**边界靠代码守。** 工具与文件接口按工作区根解析路径并拒绝越界；联网出口拦环回、私网与
链路本地地址；服务商密钥存真值、出接口一律换成不可复原的掩码，提交空串表示「保留原值」。

**存储各管一段。** `workbaby.json` 放设置、技能开关与隐藏轮台账（原子整写 + 互斥锁），
`workbaby.db` 放服务商、模型与检索文档（派生数据，WAL），会话在 JSONL，
MCP 清单在 `mcp.json`（用户级与工作区级，项目级同名覆盖）。没有第二份注册表。

**优势**：交付物是一个 exe 加一个 `assets` 目录，排障只看一个进程与一个数据目录，引擎能脱离界面单测。
**代价**：能力上限就是自己实现的工具集——模型协议只有 OpenAI 兼容 `/chat/completions`，第三方扩展只有 MCP 一条路。

## 目录

```
WorkBaby/
├── main.go              # 装配根：go:embed 前端产物 + 构建期注入版本号
├── Taskfile.yml         # 任务入口（build / dev / check / package / test / smoke / icons）
├── backend/
│   ├── domain/          # 纯类型：VO / REQ / RESP / DTO / DO
│   ├── pkg/             # 叶子工具：apperror / codes / ulid / fsutil / log / proc / paths
│   ├── repo/            # 持久化：JSON 设置台账 + SQLite（GORM）
│   ├── agent/           # 进程内引擎：轮次循环 / 模型适配 / 工具集 / 会话落盘 / 输出折叠
│   ├── web/             # 联网：搜索适配器 / 正文抽取 / PDF / 地址安全
│   ├── index/           # 预索引：文件与符号抽取 / 模糊检索 / frecency
│   ├── sse/             # 事件总线：seq 发号 / 断线补帧 / 心跳
│   ├── api/             # gin 薄壳：分组路由与统一信封
│   ├── service/         # 业务编排：parser/（会话解析）· platform/（Windows 动作）
│   └── shell/           # 窗口 / 托盘 / 端点装配
├── frontend/src/        # Vue 单页：api / components / composables / locales / stores / views
├── assets/skills/       # 随包分发的推荐技能包
├── bin/                 # 构建产物：WorkBaby.exe 与安装包
├── build/               # wails3 打包配置、图标、NSIS 脚本
├── scripts/             # 开发期 PowerShell 助手
├── tests/               # Go 场景测试，一文件一个主题
├── .github/workflows/   # CI 门禁与 tag 发布流水线
└── docs/                # ARCHITECTURE / MODULES / API / DATA-MODEL / DEV
```

运行数据只在 `%LOCALAPPDATA%\WorkBaby\`（开发态带 `-dev` 后缀）：

```
├── workbaby.json        # 设置、技能开关与隐藏轮台账
├── workbaby.db          # SQLite：服务商 / 模型 / 检索文档
├── mcp.json             # MCP 服务清单（工作区级另有同名文件）
├── logs/                # 按天滚动：wb-YYYY-MM-DD.log
├── webview/             # WebView2 用户数据
├── sessions/            # 会话 JSONL，按工作区编码分子目录
├── skills/  skills_disabled/   # 技能启用区 / 停用区
└── templates/           # 提示词模板（md + YAML 头）
```

安装态是 exe + `assets\`，前端产物已 `go:embed` 进 exe。卸载时先问是否连数据目录一起删，默认保留。

## 技术栈

| 层 | 选型 | 版本 |
|---|---|---|
| 桌面壳 | Wails v3（WebView2，无边框窗口） | `github.com/wailsapp/wails/v3` 3.1.0 |
| 语言 | Go，模块 `WorkBaby`，无 cgo | 1.26.0 |
| 业务服务 | gin | 1.12.0 |
| 持久化 | GORM + 纯 Go SQLite 驱动（`glebarez/sqlite`） | gorm 1.31.2 / sqlite 1.11.0 |
| ID | `oklog/ulid/v2`，带业务前缀 `RUN` `ASK` `EVT` `JOB` `SES` | 2.1.2 |
| 网页解析 | `golang.org/x/net/html` | 0.60.0 |
| 模型协议 | OpenAI 兼容 `/chat/completions` 流式 | — |
| 前端框架 | Vue + TypeScript + Vite + Vue Router + Pinia + Axios | 3.4.34 / 5.4.5 / 5.2.12 / 4.3.2 / 2.1.7 / 1.6.8 |
| 组件与样式 | Element Plus + UnoCSS | 2.9.0 / 0.58.6 |
| 编辑器与图表 | CodeMirror 6 + ECharts | codemirror 6.0.1 / echarts 5.5.0 |
| 国际化 | Vue I18n（zh-CN / en） | 9.10.2 |
| Markdown 渲染 | marked + DOMPurify | 12.0.2 / 3.1.6 |

## 开发

| 命令 | 作用 |
|---|---|
| `task build` | 出前端产物并编 `bin\WorkBaby.exe`（版本号取 `git describe`） |
| `task dev` | `wails3 dev`：后端热重载 + Vite 热更 |
| `task test` | `go test ./tests -count=1`，秒级场景测试 |
| `task smoke` | 起纯后端实例把免参数只读接口全打一遍，只验接线不验业务 |
| `task package` | `task build` 后打 NSIS 安装包，需要 `makensis` 在 PATH |
| `task icons` | 重生成全套图标（改主稿或字标后跑一次，产物一起提交） |

四道门禁 = 格式（`fmt:check`）/ 静态检查（`vet`）/ 场景测试（`test`）/ 前端（`ts:check` + `i18n`），
`task check` 按这个顺序跑完。**先 `task build` 才能 `task vet`**：`go vet ./...` 要编译 `package main`，`frontend\dist` 与生成绑定必须先就位。

`scripts/` 下三个 PowerShell 助手：`backend.ps1` 起一个不建窗的纯后端实例并报出随机端口
（被另两个点源引用），`dev.ps1` 起后端 + Vite 供浏览器直连前端，`smoke.ps1` 是 `task smoke` 的实现。

工程规范见 `AGENTS.md`，架构与模块细节见 `docs/`。CI 在 windows runner 上跑同一套门禁并构建，推 tag 出安装包。
