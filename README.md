# WorkBaby

Windows 桌面单机 AI 助手：Tauri 壳 + 内嵌 HTTP 服务 + 自研 Rust agent 内核。
12 项内置功能与 22 个内置技能全部原生实现，每项独立开关。

## 它是什么

- **对话即工作**：选好模型直接开聊，助手会读文件、跑命令、改代码、查资料、写文档。
  全过程流式可见——工具执行有卡片、思考有折叠块、上下文有水位环。
- **内核归本仓**：会话循环、工具批执行、上下文压缩、自动重试、子代理全部是 Rust 实现，
  进程内运行，不依赖外部 agent 进程，也不需要 Node 运行时。
- **能力可开关**：待办清单、结构化提问、持久记忆、联网访问、省上下文沙箱、模糊搜索、
  语言诊断、计划评审、目标追踪、装载配置、内省面板、上下文地图——12 项原生功能，
  关掉即从工具表与事件流中撤出。
- **子代理看得见**：模型派出去的每个子代理都有独立会话与实时过程，
  它在读什么、跑什么、说了什么全部可见，只有结论回灌主对话。
- **看得见也点得动**：待办勾选、目标达成 / 阻塞、计划批准 / 驳回 / 标注、记忆增删——
  面板不是只读快照，用户与模型改的是同一份状态、走同一套校验。
- **运行时零依赖**：内置 PortableGit（Git Bash / git / 358 个 GNU 工具）与 Python 3.13
  随安装包分发，首次启动后台解压到数据目录；子进程 PATH 排序为
  内置运行时 → 系统目录 → 用户 PATH，用户没装 Git、没装 Python 也能跑命令、
  写脚本、做文档转换，且不改动用户环境。Windows 原生操作（注册表 / 服务 / WMI）
  走 PowerShell——用户装了 pwsh 7 就用 7，否则用系统自带的 5.1，两者共用同一套执行纪律。
- **执行过程看得见**：长命令不等跑完才回显——输出边跑边追加进工具卡（节流到 120ms），
  子代理的命令同样可见。
- **工作区随手换**：换目录立即生效——会话、文件树、git 视图、技能与功能存储一起换，
  不用重启应用。
- **开箱能说话**：首启引导把「选服务商 → 填密钥 → 测连通」做在对话页里，
  九家常见服务商预设一键添加（含国内直连的），不必先手写 models.json。
- **三家模型协议都通**：OpenAI 兼容、Anthropic Messages、Gemini 三套 wire 原生实现，
  直连 Claude 与 Gemini 不用绕网关。
- **危险操作先问你**：递归删除、磁盘操作、全局安装、force push、改注册表等
  一眼能看出风险的动作，执行前会先问你一句；可切成「每步都问」或「全部放行」。
- **MCP 也能 OAuth 登录**：远端鉴权的 MCP server 走授权码 + PKCE，
  浏览器点一下「允许」就把凭据写进配置，不必手写 header。
- **项目约定自动读**：工作区及祖先目录里的 `AGENTS.md` / `CLAUDE.md` 自动注入上下文。
- **技能随应用分发**：前端与全栈开发、Office 文档、系统化调试与任务对齐等技能打包进二进制，
  也可导入用户自己的技能。

## 技术栈

| 层 | 选型 |
|---|---|
| 桌面壳 | Tauri 2（WebView2，Frameless 自绘标题栏，托盘常驻，单实例） |
| 业务服务 | 内嵌 axum HTTP（127.0.0.1 随机端口）+ SSE 实时事件流 |
| Agent 内核 | `wb-kernel`：会话树 / 工具批 / 上下文压缩 / 重试 / 子代理 |
| 模型接入 | OpenAI 兼容 wire API，`models.json` 自定义 Provider |
| 存储 | 会话 JSONL（树形条目，可分支编辑）+ SQLite（设置 / 索引 / 统计 / 记忆 FTS5） |
| 前端 | Vue 3.5 + TypeScript + Vite 6 + Element Plus（按需）+ UnoCSS + ECharts + Vue I18n |

## 架构分层

```
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

依赖方向由 Cargo 编译期强制：`wb-domain` 是叶子（只依赖 `wb-pkg`），
`wb-api` 是唯一知道 HTTP 的地方，`wb-service` 编排而不实现细节。

## 文档

| 文档 | 内容 |
|---|---|
| [AGENTS.md](AGENTS.md) | 工程规范唯一权威：分层 / 命名 / 错误码 / 注释 / 门禁 |
| [DESIGN.md](DESIGN.md) | 视觉语言：令牌 / 表面阶梯 / 按钮五态 / 布局 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 整体架构、进程模型、数据流、关键取舍 |
| [docs/MODULES.md](docs/MODULES.md) | 每个 crate 的职责 / 对外接口 / 边界 |
| [docs/KERNEL.md](docs/KERNEL.md) | Agent 内核：会话树、主循环、压缩、重试、子代理 |
| [docs/SESSIONS.md](docs/SESSIONS.md) | 会话存储：条目类型、回放算法、索引与统计 |
| [docs/TOOLS.md](docs/TOOLS.md) | 工具体系：Tool 契约、内置与功能工具、执行纪律 |
| [docs/PROVIDER.md](docs/PROVIDER.md) | 模型接入：wire API、流式解析、错误分类 |
| [docs/SKILLS.md](docs/SKILLS.md) | 技能体系：SKILL.md 格式、三区发现、注入 |
| [docs/FEATURES.md](docs/FEATURES.md) | 12 项内置功能的设计与实现 |
| [docs/WORKSPACE.md](docs/WORKSPACE.md) | 工作区语义与 Git 写白名单 |
| [docs/MCP.md](docs/MCP.md) | MCP 客户端与连接池 |
| [docs/FRONTEND.md](docs/FRONTEND.md) | 前端架构、页面与组件、令牌、构建分块 |
| [docs/SHELL.md](docs/SHELL.md) | 桌面壳、进程模型、数据目录 |
| [docs/DATA-MODEL.md](docs/DATA-MODEL.md) | 数据模型：会话树 / SQLite 表 / JSON 配置 |
| [docs/API.md](docs/API.md) | HTTP 契约：端点 / 信封 / SSE 事件 |
| [docs/ERROR-CODES.md](docs/ERROR-CODES.md) | 业务错误码全表与处置建议 |
| [docs/DEV.md](docs/DEV.md) | 开发环境、构建、测试、验证流程 |
| [docs/DEPLOY.md](docs/DEPLOY.md) | 打包内容、安装形态、发布流程 |

## 开发

```bash
# 后端
cargo check --workspace              # 编译检查
cargo test --workspace               # 测试（约 20 秒）
cargo test --workspace -- --ignored  # 需要真实外部环境的慢速测试

# 前端
cd frontend && npm install
npm run typecheck                    # 类型检查
npm run build                        # 构建到 frontend/dist
npm run dev                          # 前端开发服务器
npm run tauri dev                    # 整体开发模式（壳 + 热更）

# 五道门禁（与 CI 同一套）
cargo fmt --all -- --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
cd frontend && npm run typecheck && npm run build
```

`cargo xt` / `cargo xchk` 是 `.cargo/config.toml` 里的别名，分别等于上面前两条。

## 优势与局限

**优势**

- 内核在进程内：没有子进程通信的协议漂移，也没有 Node 运行时依赖，
  响应延迟与错误可诊断性都更好。
- 能力全部原生：12 项功能按开关装配，模型工具共用同一套 `Tool` 契约，
  不存在插件加载失败的可能。
- 单色相设计系统：强调色只有靛蓝五档，界面层级靠明度与圆角表达，一眼能看出哪里可点。
- 数据全在本地：会话是 JSONL、设置与索引是 SQLite，不依赖任何云端服务即可完整使用。

**局限**

- 仅支持 Windows：路径处理、进程启动与 UI 缩放都做了平台假设，不做跨平台抽象。
- 模型服务商需要用户自备密钥：不内置任何免费额度。
- 联网与语言诊断依赖外部条件：搜索需要可达的网络，语言服务器需要用户自行安装
  （bash / git / python 已由内置运行时兜底）。
- 审批闸门默认「危险才问」：日常命令自动放行，一眼能看出风险的动作才拦；
  可切成「每步都问」或「全部放行」。闸门不是文件系统沙箱，绝对路径写入与网络不拦截。
