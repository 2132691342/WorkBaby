# WorkBaby

一个能干活的个人 AI 助手——Windows 桌面客户端。Rust 内核 + Tauri 壳，
12 项内置 agent 能力与 22 个内置技能，全部可开关。

## 它是什么

- **对话即工作**：选好模型直接开聊，助手会读文件、跑命令、改代码、查资料、写文档；
  全过程流式可见——工具执行有卡片、思考有折叠块、上下文有水位环。
- **自带内核**：会话循环、工具执行、上下文压缩、自动重试、子代理全部是 Rust 实现，
  进程内运行，不依赖外部 agent 进程，也不需要 Node 运行时。
- **能力可开关**：待办清单、结构化提问、持久记忆、联网搜索、省上下文沙箱、模糊搜索、
  LSP 诊断、计划评审、目标追踪、装载配置、内省面板、上下文用量地图——12 项功能原生实现，
  一键启停，关掉即从装配中撤出。
- **子代理看得见**：模型派出去的每个子代理都有独立会话与实时过程——
  它在读什么、跑什么、说了什么全部可见，结论才回灌主对话。
- **可接外部应用**：自己带来的前端应用登记入口即可挂进来，qiankun 隔离运行，随时停用。
- **技能随应用分发**：前端与全栈开发、Office 文档、系统化调试与任务对齐等 22 个技能打包进二进制，
  也可导入用户自己的技能。
- **运行时零依赖**：内置 PortableGit（Git Bash / git / GNU 工具）与 Python 3.13
  随安装包分发，启动时解压到数据目录，子进程 PATH 优先指向内置运行时——
  用户没装 Git、没装 Python 也能跑命令、写脚本、做文档转换，且不动用户环境。

## 技术形态

| 层 | 选型 |
|---|---|
| 桌面壳 | Tauri 2（WebView2，Frameless 自绘标题栏，托盘常驻，单实例） |
| 业务服务 | 内嵌 axum HTTP（127.0.0.1 随机端口）+ SSE 实时事件流 |
| Agent 内核 | `wb-kernel`：会话树 / 工具批执行 / 上下文压缩 / 重试 / 子代理 |
| 模型接入 | OpenAI 兼容 wire API（覆盖 OpenAI / DeepSeek / Moonshot / OpenRouter 等网关），`models.json` 自定义 Provider |
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
                                wb-domain ──► wb-pkg
```

依赖方向由 Cargo 编译期强制：`wb-domain` 是叶子（只依赖 `wb-pkg`），
`wb-api` 是唯一知道 HTTP 的地方，`wb-service` 编排而不实现细节。

## 文档

| 文档 | 内容 |
|---|---|
| [AGENTS.md](AGENTS.md) | 工程规范唯一权威：分层 / 命名 / 错误码 / 注释 / 门禁 |
| [DESIGN.md](DESIGN.md) | 视觉语言：令牌 / 表面阶梯 / 按钮五态 / 布局 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 整体架构、进程模型、数据流、关键取舍 |
| [docs/KERNEL.md](docs/KERNEL.md) | Agent 内核：会话树、循环、压缩、重试 |
| [docs/TOOLS.md](docs/TOOLS.md) | 工具体系：Tool 契约、内置工具、执行纪律 |
| [docs/PROVIDER.md](docs/PROVIDER.md) | 模型接入：wire API、流式解析、错误分类 |
| [docs/SESSIONS.md](docs/SESSIONS.md) | 会话存储：条目类型、回放算法 |
| [docs/SKILLS.md](docs/SKILLS.md) | 技能体系：SKILL.md 格式、三区发现、注入 |
| [docs/FEATURES.md](docs/FEATURES.md) | 12 项内置功能 |
| [docs/WORKSPACE.md](docs/WORKSPACE.md) | 工作区与 Git 写白名单 |
| [docs/MCP.md](docs/MCP.md) | MCP 客户端与连接池 |
| [docs/FRONTEND.md](docs/FRONTEND.md) | 前端架构、页面与组件、令牌、构建分块 |
| [docs/SHELL.md](docs/SHELL.md) | 桌面壳、进程模型、数据目录 |
| [docs/MODULES.md](docs/MODULES.md) | 每个 crate 的职责 / 对外接口 / 边界 |
| [docs/DATA-MODEL.md](docs/DATA-MODEL.md) | 数据模型：会话树 / 设置 / 索引 / 统计 / 记忆 |
| [docs/API.md](docs/API.md) | HTTP 契约：端点 / 信封 / 事件流 |
| [docs/ERROR-CODES.md](docs/ERROR-CODES.md) | 业务错误码全表与处置建议 |
| [docs/DEV.md](docs/DEV.md) | 开发环境、构建、验证流程 |
| [docs/DEPLOY.md](docs/DEPLOY.md) | 打包与分发 |

## 开发

```bash
# 后端
cargo check --workspace          # 编译检查
cargo test --workspace           # 测试

# 前端
cd frontend && npm install
npm run typecheck                # 类型检查
npm run build                    # 构建到 frontend/dist
npm run dev                      # 前端开发服务器
npm run tauri dev                # 整体开发模式（壳 + 热更）

# 五道门禁（与 CI 同一套）
cargo fmt --all -- --check
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
cd frontend && npm run typecheck && npm run build

# 需要真实外部环境的慢速测试
cargo test --workspace -- --ignored
```

## 优势与局限

**优势**

- 内核在进程内：没有子进程通信的协议漂移，也没有 Node 运行时依赖，响应延迟与错误可诊断性都更好。
- 能力全部原生：12 项功能原生装配，模型工具共用同一套 `Tool` 契约，开关即装配，
  不存在插件加载失败的可能。
- 单色相设计系统：强调色只有靛蓝五档，界面层级靠明度与圆角表达，一眼能看出哪里可点。
- 数据全在本地：会话是 JSONL、设置与索引是 SQLite，不依赖任何云端服务即可完整使用。

**局限**

- 仅支持 Windows：路径处理、进程启动与 UI 缩放都做了平台假设，不做跨平台抽象。
- 模型服务商需要用户自备密钥：不内置任何免费额度。
- 联网与语言诊断依赖外部条件：搜索需要可达的网络，语言服务器需要用户自行安装
  （bash / git / python 已由内置运行时兜底）。
