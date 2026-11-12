# WorkBaby

Windows 单机 AI 工作台：把你的 pi 装进一个 GUI 里。

不重写 pi，不代装环境，不复制一份配置。壳跑的就是你在终端里装好的那个 `pi`，
读写的就是你 `~/.pi/agent` 里的那套技能、插件、MCP、模型与会话。命令行里怎么改，
界面里就怎么变；界面里怎么改，终端里也立刻认。

## 为什么需要它

命令行 AI 助手对多数人有一个门槛：**操作**。模型、插件、技能、MCP 要靠改 JSON 和
记命令，看不清当前状态，也不知道哪一步失败了。

WorkBaby 把这件事收进一个 GUI：引擎、插件、技能、MCP、模型、会话、工作区全部在界面上
增删改查。**它不替你装任何东西**——没有 pi 就给一句「该去装什么」，而不是悄悄
往你机器上下载 110MB 运行时。

## 边界：壳做什么、不做什么

| 壳做 | 壳不做 |
|---|---|
| 找到你已装的 `pi`（手动路径 → PATH → npm 全局根） | 帮你下载 / 安装 pi |
| 以 RPC 模式常驻子进程，管它的生命周期 | 重写或内嵌引擎 |
| 读写 `~/.pi/agent` 里的配置与会话 | 复制一份隔离的配置目录 |
| 经 `pi install/remove/update` 管插件 | 自己造包注册表 |
| 探测系统 node / git / python | 随包分发运行时 |
| 模型服务商一律 API Key | OAuth / 登录体系 |

没有 pi 时界面是一页可读的诊断 + 安装指引；装好后点「重新检测」立刻可用。

## 能力

**引擎由壳全权管理**。`pi --mode rpc` 常驻子进程，会话、模型、思考档、上下文压缩、
插话与排队全部由 GUI 驱动。进程意外退出按指数退避自动拉起，连崩 5 次停手交给用户。

**打开即用，不等引擎**。装配收尾就后台预热引擎；对话页的历史与待办直接从会话文件回放，
引擎就绪后自动切到实况——打开应用立刻能看到上次的对话并开始打字。

**推荐 6 个插件**，一键装齐，可启停 / 升级 / 卸载：

| 插件 | 用途 |
|---|---|
| `pi-web-access` | 联网搜索、抓取 URL、克隆仓库、PDF 与视频理解 |
| `pi-lens` | LSP、lint、类型检查与符号结构化搜索 |
| `pi-plan-mode` | 只读探索 + 计划文件编辑 |
| `@juicesharp/rpiv-todo` | 实时待办清单，跨压缩存活 |
| `@juicesharp/rpiv-ask-user-question` | 结构化问卷，模型拿不准时先问你 |
| `pi-tidy` | 工具调用输出折叠 |

口径是「能力不重叠」：联网 / 代码体检 / 计划 / 待办 / 提问 / 输出折叠各占一格。
**推荐不等于预置**——壳不会往你的 `settings.json` 里写任何东西，你点了才装。
想装别的插件，插件页照样能装。

**10 个推荐技能**，一键装齐，可开关 / 编辑 / 重置 / 新建 / 导入 / 删除。
它们同样不会被自动落位到你的技能目录。

**对话即工作**。流式输出、思考折叠、工具卡片（含差异视图）、插话 / 排队 / 停止、
上下文水位、一键压缩、待办面板。引擎提问（确认 / 选择 / 输入 / 编辑器）直接弹卡可答。

**会话可回放**。pi 的 JSONL 会话文件是唯一真相；列表、改名、删除、隐藏单轮、
导出 HTML 都落在会话文件本身或壳的台账里，重启不丢。

**工作区**。文件树与预览、git 状态 / 差异 / 历史 / 分支 / 提交 / stash / 推拉。
.git 走系统装的 git，凭证走 Windows 凭据管理器。

**MCP**。stdio / HTTP 服务增删改查与开关，落在 `mcp.json`（用户级 + 工作区项目级），
一条「测试连接」完成 MCP 握手。

**模型**。服务商与模型增删改查，写 `models.json` + `auth.json`，密钥读出来恒为掩码。

## 目录

壳只写自己那一份数据目录，其余全是你的：

```
%LOCALAPPDATA%\WorkBaby\          （开发态加 -dev 后缀）
├── workbaby.json                 # 壳的偏好与台账（单文件 JSON）
├── logs\                         # 按天滚动
├── webview\                      # WebView2 用户数据
└── cache\                        # 壳自己的缓存

%USERPROFILE%\.pi\agent\          # 你的 pi 家，壳只读写、不接管
├── settings.json                 # 模型 + packages[]
├── models.json auth.json mcp.json
├── npm\node_modules\             # 插件
├── sessions\                     # 会话 JSONL（唯一持久真相）
└── skills\ skills_disabled\      # 技能启用 / 停用区
```

`PI_CODING_AGENT_DIR` 若被你自己设过，壳按它走。卸载程序会先问一句
**是否连数据目录一起删**，默认保留。

## 技术栈

| 层 | 选型 |
|---|---|
| 桌面壳 | Wails v3（无边框窗口，前端自绘标题栏，WebView2） |
| 语言 | Go 1.26（无 cgo） |
| 业务服务 | gin（业务 API + SSE 全走 HTTP，127.0.0.1 随机端口） |
| 持久化 | 标准库 JSON 文件（原子整写 + 互斥锁） |
| ID | ULID，带业务前缀 |
| 引擎 | 用户已装的 pi，RPC 模式子进程 |
| 前端 | Vue 3 + TypeScript + Vite + Element Plus + Pinia + UnoCSS |

## 开发

```bash
go build ./...                                        # 全包可编译
go vet ./...                                          # 静态检查
cd frontend && npx vue-tsc --noEmit -p tsconfig.json  # 前端类型检查
go test ./tests -count=1                              # 场景测试
```

`task build` 产出 `bin\WorkBaby.exe`，`task package` 出 NSIS 安装包。
工程规范见 `AGENTS.md`，架构与模块见 `docs/`。
