# 架构 · ARCHITECTURE

WorkBaby 是 Windows 桌面单机 AI 助手。一句话讲清它的形态：
**Electron 壳把引擎、运行时、插件、技能全部装进 `%LOCALAPPDATA%\WorkBaby`
沙箱，渲染层只与壳内 HTTP 服务说话，引擎以 RPC 子进程常驻。**

## 1. 进程拓扑

```
┌────────────────────────────────────────────────────────────┐
│ 渲染进程  Vue 3 + Pinia（业务全部经 HTTP，不经 IPC 做业务）      │
│   window.wb 桥：端点分发 / 文件夹选择 / 窗控 / 外链              │
└───────────────▲────────────────────────────────────────────┘
                │ HTTP /api/v1（X-Desktop-Token）+ SSE /events
┌───────────────┴────────────────────────────────────────────┐
│ 主进程（装配根 index.ts）                                     │
│   express ──► api 路由（薄壳）──► service（业务编排）──► repo  │
│   sse hub：seq 单调发号、断线补帧、心跳                        │
│   shell：无边框窗口 / 托盘 / preload 桥                       │
└───────────────▲────────────────────────────────────────────┘
                │ JSONL over stdio（RPC 模式）
┌───────────────┴────────────────────────────────────────────┐
│ pi 子进程：node.exe pi-cli.js --mode rpc                     │
│   会话循环 / 工具执行 / 压缩 / 重试 —— 引擎自己的事，壳不重做     │
│   扩展（插件）与技能、MCP 由 pi 按沙箱内目录自动发现             │
└────────────────────────────────────────────────────────────┘
```

**为什么这样切**：

- 壳只做「管理面 + 传输面」，不做「智能面」。会话循环、工具执行、上下文压缩
  是引擎的本职，壳重做一遍只会做出第二个引擎——所以壳把 pi 当成一个受管的
  常驻服务，自己只负责装配、驱动、翻译事件、持久化台账。
- 渲染层与主进程之间只有一条业务通道（HTTP + SSE）。IPC 桥只管「壳能力」
  （窗口、对话框、端点分发），业务不进 IPC——换传输层或做远程化时只动一个层。
- 引擎与壳之间是 JSONL 文本协议，进程隔离：引擎崩溃不影响壳，壳崩溃不损坏会话。

## 2. 装配顺序（index.ts）

```
单实例锁 → initContext（路径/令牌）→ initLog → openDb（WAL + 迁移）
  → app.ready → registerBridge → 建窗（隐藏）→ 托盘
  → 起HTTP(随机端口) → setPort + 回填桥 → 广播就绪
  → 后台：解压运行时 zip → 落位内置技能 → 写 npmCommand → 声明内置插件
  → pi 已装则继续：装内置插件（失败留给界面重试）
```

- **启动不等重物**：端口与界面先就绪；三个运行时 zip（约 115MB）与技能包
  在后台解压，进度经 SSE `app:progress` 上报，完成后广播 `engine:status`。
- **端点回填**：preload 桥的 `wb:endpoint` 在 HTTP 起来之前返回空 `wb_base_url`，
  起来之后返回真实端口与令牌；渲染层首屏据此装配 axios 与 EventSource。
- **退出**：`before-quit` 顺序关引擎子进程 → 关库 → 关日志 → 销毁托盘。

## 3. 沙箱布局（数据目录）

```
%LOCALAPPDATA%\WorkBaby\            （开发态为 WorkBaby-dev）
├── workbaby.db                     设置 KV、开关台账（WAL）
├── cache\
│   ├── npm\                       npm 缓存（npm_config_cache）
│   └── npm-global\                npm 全局前缀（npm_config_prefix）
├── logs\wb-YYYY-MM-DD.log          按天滚动日志
├── runtime\
│   ├── node\                       内置 Node.js（node.exe + 完整 npm）
│   ├── PortableGit\                git + Git Bash + GNU 工具链
│   └── python\                     Python 3.13
├── pi-app\                         pi 本体（内置 npm --prefix 安装产物）
└── pi\                             PI_CODING_AGENT_DIR（引擎的家）
    ├── settings.json               默认模型 + packages 声明 + npmCommand
    ├── models.json                 自定义服务商与模型
    ├── auth.json                   API Key（0600）
    ├── mcp.json                    MCP 服务（用户级）
    ├── npm\                        插件安装根（node_modules 在这里）
    ├── sessions\--<工作区编码>--\   会话 JSONL（树形条目）
    ├── skills\  skills_disabled\   技能（禁用 = 搬进后者）
    └── prompts\ themes\            提示模板与主题
```

**隔离手段**：

- 子进程 PATH 把内置 node/git/python 排在最前，HOME/USERPROFILE 指向数据目录——
  agent 跑 bash、python、git 一律先命中内置副本，不写用户全局配置。
- npm 的 `cache` 与 `prefix` 也锁进数据目录，装包不落 `%APPDATA%\npm`。
- 引擎家目录由 `PI_CODING_AGENT_DIR` 一个环境变量整体重定向；
  `PI_SKIP_VERSION_CHECK=1` 与 `PI_TELEMETRY=0` 关掉引擎自带的远端探测与上报。
- **内置 node/git/python 取不到时直接报错，绝不回退系统同名命令**——
  那等于拿用户环境跑包，正是本壳要避免的事。
- 管理面（安装/卸载/升级/开关）只经 GUI 暴露；沙箱内的 pi 不对外提供 CLI 入口。

**不要设 `PI_PACKAGE_DIR`**：它改写的是 pi 自己找 `dist/` 与内置主题的根，
指错会让引擎根本起不来。插件的安装根是 `pi/npm`，由 pi 自己的包管理决定，与它无关。

## 3.1 内置插件与技能清单

首启后台跑两件事，互不阻塞界面：

1. **内置插件（12 个）**：`service/plugin_catalog.ts` 是唯一清单，版本 pin 在这里。
   `declareBuiltinPlugins()` 把声明写进 `settings.json` 的 `packages`，
   `provisionBuiltinPlugins()` 用内置 npm 逐个装进 `pi/npm`。单个失败不阻塞其余，
   界面每行有重试。
2. **内置技能（22 个）**：`service/skill_catalog.ts` 是唯一清单，带分组与依赖提示。
   默认启用的复制进 `pi/skills`，默认停用的直接进 `pi/skills_disabled`；
   已存在的目录永不覆盖（保住用户改动）。

**启停实现**：插件启停只改 `packages` 条目（裸串 = 启用，`{source, autoload:false}` = 停用），
秒级生效且磁盘不动；技能启停靠目录搬进/搬出禁用区，对任何引擎版本都成立。

## 4. 引擎生命周期（service/chat）

- **绑定**：一个工作区一个 pi RPC 子进程。`ensureRpc` 惰性拉起：
  当前无子进程或工作区已变 → 杀旧起新，`get_state` 拿会话文件、
  `get_commands` 拿斜杠命令、`get_available_thinking_levels` 拿思考档位。
- **驱动**：发消息 = `prompt`（附件：图片走 base64 `images` 字段，文件折进消息文本）；
  插话 = `steer`；追加 = `follow_up`；停止 = `abort`；压缩 = `compact`；
  换模型 / 思考档 / 自动压缩各有一条 RPC 命令。
- **事件翻译**：引擎的会话事件（消息增量、工具生命周期、队列、压缩、重试、
  UI 请求）由 `chat_frames` 翻译成壳的 SSE 词汇 `chat:*`。翻译器维护一个
  实时块表（文本/思考/工具），`message_end` 的完整内容作为权威快照覆盖增量。
- **提问板**：引擎侧对话框请求（select/confirm/input/editor）原样转成
  `chat:ask` 帧；用户回答经 `extension_ui_response` 直达引擎。
  通知 / 状态栏 / 小组件 / 编辑器预填是单向帧，无需应答。
- **恢复**：会话文件是唯一真相。重启后 `chat/state` 直接解析 JSONL 重建
  历史轮；正在跑的会话切走再切回 = `switch_session`。

## 5. 通信契约

- HTTP：恒 200 + 信封 `{ code, message, detail?, data }`，`code` 为数字字符串，
  `'0'` 即成功。鉴权：`X-Desktop-Token` 头；SSE 走 `?token=` 查询参数。
- SSE：`GET /api/v1/events?token=…&since=<seq>`。帧为默认 `message` 事件，
  `data` 是 `{ seq, event, data }`；hub 保留最近 1000 帧用于断线补帧，
  补不上的缺口由前端拿 `chat/state` 全量对账。
- 频道名单（preload 合同，不许改）：`wb:endpoint` / `wb:pick-files` /
  `wb:pick-folder` / `wb:ready` / `wb:open-external` / `wb:window-ctl` /
  `wb:quit` / `wb:close-decision`。

## 6. 优势与代价

**优势**：

- 安装即用：用户机器上什么都不用装，运行时随包分发、首启解压。
- 引擎可整体升级：pi 升级 = 内置 npm 重装一次；壳的翻译层不变，界面不动。
- 会话可携带：JSONL 即真相，拷目录即备份，回放逻辑在壳里只有一份。
- 沙箱可丢弃：删掉数据目录 = 干净卸载，不残留用户环境。

**代价与边界**：

- 引擎能力受 RPC 协议约束：子代理面板、OAuth 登录流等终端交互能力不暴露。
- 多了一层事件翻译：引擎事件与界面词汇的映射需要跟随引擎版本维护。
- 只支持 Windows：路径、进程、托盘都按 Windows 语义写（`windowsHide`、
  注册表自启等），跨平台要动 `pkg/env`、`shell/` 与打包配置。
