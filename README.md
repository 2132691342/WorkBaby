# WorkBaby

Windows 桌面单机 AI 助手：Electron 壳 + 内嵌 HTTP 服务 + pi 引擎驱动。
把 Node.js、pi 编码智能体、插件、技能、MCP 与 Git / Python 运行时装进一个沙箱，
安装即用，不依赖、不污染用户环境。

## 它是什么

- **安装即用**：内置 Node.js 24.21 / PortableGit / Python 3.13 随安装包分发，
  首次启动后台解压到 `%LOCALAPPDATA%\WorkBaby`；用户没装 node、Git、Python 也能跑。
- **pi 由壳全权管理**：pi 编码智能体经内置 npm 装进沙箱（「应用」页一键安装 / 升级 / 卸载），
  以 RPC 模式常驻子进程，会话、模型、思考等级、压缩全部由 GUI 驱动。
- **插件生态兼容**：pi 的扩展包（联网访问、待办、提问、记忆、LSP、计划评审、目标追踪等）
  经内置 npm 安装到沙箱，可启停 / 升级 / 卸载；扩展对 pi 透明生效。
- **技能随包分发**：内置技能包首启落位沙箱，可开关 / 编辑 / 新建 / 删除；
  工作区里的项目技能（`.pi/skills`）同样可见可复制。
- **MCP 配置面**：stdio / HTTP 服务增删改查与开关，落在 pi 的 `mcp.json`（用户级 +
  工作区项目级），pi 自动加载。
- **对话即工作**：流式输出、思考折叠、工具卡片（含差异视图）、插话 / 排队 / 停止、
  上下文水位与统计、一键压缩——引擎侧事件全部实时可见。
- **会话树可回放**：pi 的 JSONL 会话文件是唯一真相；列表、改名、删除、隐藏单轮、
  导出 HTML 都落在会话文件本身或壳的台账里，重启不丢。
- **git 面板**：状态 / 差异 / 历史 / 分支 / 提交 / 推拉全部经内置 git，
  用户没装 git 也能用。
- **看得见也点得动**：引擎提问（确认 / 选择 / 输入 / 编辑器）直接弹卡回答，
  用户与模型改的是同一份状态。

## 沙箱布局

```
%LOCALAPPDATA%\WorkBaby\
├── workbaby.db            # 设置 KV、开关台账
├── logs\                  # 按天滚动日志
├── runtime\               # 内置 node / PortableGit / python（首启解压）
├── pi-app\                # pi 本体（内置 npm --prefix 安装）
└── pi\                    # PI_CODING_AGENT_DIR（pi 的家）
    ├── settings.json / models.json / mcp.json / auth.json
    ├── sessions\          # 会话 JSONL（按工作区编码分目录）
    ├── skills\            # 内置 + 新建技能（skills_disabled 为停用区）
    ├── extensions\        # 插件桥接目录（.disabled-* 为停用区）
    └── plugins\           # 插件实际安装目录（npm 产物）
```

子进程 PATH 内置运行时在前、系统在后；agent 的 bash / python 一律先命中内置副本。

## 开发

```bash
npm install && cd frontend && npm install && cd ..
npm run dev          # 构建主进程 → Vite 热更 → Electron
npm run typecheck    # 主进程 TS 检查
npm run build        # 主进程 + 前端产物
npx vitest run       # 测试
npm run dist         # electron-builder 打 NSIS 安装包
```

纯浏览器调试前端：`wb.dev.port` / `wb.dev.token` 写进 localStorage 即可直连后端。

## 文档

| 文档 | 内容 |
|---|---|
| `AGENTS.md` | 工程规范（分包 / 依赖方向 / 编码与测试纪律，唯一权威） |
| `DESIGN.md` | 视觉语言与设计令牌 |
| `docs/ARCHITECTURE.md` | 壳架构：装配、SSE、沙箱布局、引擎生命周期 |
| `docs/API.md` | `/api/v1` 端点契约（信封 / 鉴权 / SSE 帧） |
| `docs/ERROR-CODES.md` | 业务码段位与文案 |
| `docs/MODULES.md` | 依赖清单与取舍理由 |

## 优势与局限

**优势**：零环境依赖的Windows AI 工作台；pi 生态全兼容；GUI 完全接管引擎管理；
会话与配置都在一个可备份的目录里。

**局限**：只支持 Windows；引擎能力以 pi 为准（无子代理面板、OAuth 登录走 API Key）；
插件升级依赖 npm 源可达。
