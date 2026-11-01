# 前端架构 · FRONTEND

Vue 3 + TypeScript 的单窗口应用，通过 HTTP 与 SSE 与本地服务对话。
前端的全部状态来自后端——它不另存一份真相。

## 分层

| 层 | 目录 | 职责 |
|---|---|---|
| api | `src/api` | 唯一请求出口；路径与后端端点一一对应 |
| stores | `src/stores` | chat（run 增量）/ runClock（本轮秒表）/ session（列表与当前）/ settings（清单）/ toast |
| composables | `src/composables` | chatBridge（SSE 事件→store 变更）、useSse（SSE 连接单例）、useTheme / useAppearance（主题切换与外观持久化）、filePeek（消息里文件路径的就地预览）、shellIntent（壳命令意图分发）、useSessionPanel（会话左栏的分组与选中状态）、useViewportScale（窗口宽→`--wb-vw-scale` 全局缩放） |
| components | `src/components` | 按域分组：chat / settings / workspace / layout / dashboard / common / features / skills / doc |
| views | `src/views` | 路由级页面 |
| util | `src/utils` | md（唯一渲染出口）、errors（业务码 → 界面文案）、echarts（按需注册）、num / text / time 格式化 |
| locales | `src/locales` | zh-CN（默认）+ en |

### api 层

`api/http.ts` 是唯一发起请求的地方：

- baseURL 由壳启动后注入（`127.0.0.1:{port}/api/v1`）
- 统一解包 `Resp { code, message, data }`，`code !== "0"` 抛 `ApiError`（带 code）
- 浏览器层失败翻成人话（断连、超时分别给不同提示）

`api/shell.ts` 是系统能力的唯一出口：端口、文件对话框、窗控、自启、退出、托盘事件。
内部走 Tauri invoke + listen，开发模式（浏览器直开）回落 localStorage。
**组件不得直接 import `@tauri-apps/*`**——统一收口才能让开发模式与生产模式行为一致。

### SSE 连接

`composables/useSse.ts` 持有唯一连接；`composables/chatBridge.ts` 把事件翻译成 store 变更。

- 挂在 App 壳级，不随视图卸载（切页面不该断事件流）
- 断线自动重连，重连时以 `last_event_id` 查询参数声明断点，服务端据此重放补齐遗漏事件
- 握手晚于挂载时也能建起来——端口是响应式的，拿普通变量 watch 等不到变化

## 路由与页面

hash 模式（WebView 的 file 协议不接受 history 模式的路径）。`/` 重定向到 `/overview`；
`/features`、`/dashboard`、`/files` 是旧入口别名，分别重定向到 `/plugins`、`/usage`、`/workspace`。

| 路径 | 视图 | 说明 |
|---|---|---|
| `/overview` | OverviewView | 能力总览四卡 + 最近会话（落地页） |
| `/chat` | ChatView | 对话主视图 |
| `/workspace` | WorkspaceView | 文件树 + 改动清单 + diff + 提交 / 历史 / 分支 |
| `/sessions` | SessionsView | 全部会话与概要 |
| `/agents` | AgentsView | 子代理观测：活跃清单 + 流式过程 + 结论 |
| `/apps` | AppsView | 外部微前端应用登记与隔离运行（qiankun） |
| `/models` | ModelsView | 模型服务与工具配置 |
| `/skills` | SkillsView | 技能清单台 |
| `/plugins` | FeaturesView | 12 项内置功能开关台（别名 /features） |
| `/mcp` | McpView | MCP 服务器配置与在线状态 |
| `/usage` | DashboardView | 用量仪表盘（别名 /dashboard） |
| `/help/:name?` | HelpView | 内置文档阅读 |
| `/settings` | SettingsView | 单页滚动：外观 / 行为 / 内省（旧板块路径一律落这里） |

**独立页与设置页共用组件**：`features/FeatureList` 与 `skills/SkillList` 各被两处引用——
两处各写一份必然漂移。

## ChatView 组件

| 组件 | 职责 |
|---|---|
| SessionPanel / SessionList | 左栏：新对话、时间分组、搜索、重命名/导出/删除 |
| MessageList | 消息流：历史回合 + 进行中回合 + 提问卡 |
| ChatMessage | 单条消息的容器：角色布局、复制、usage 徽标 |
| AssistantTurn | 一条助手消息：按原始顺序渲染思考/正文/工具卡 |
| SessionChips | 输入区上方的模型 / 思考档 / 工作区 chip 行 |
| ComposerMenu | 模型选择面板：左服务商右模型，顶部跨服务商搜索，↑↓/回车/Tab 键盘导航 |
| ThinkingBlock | 思考段折叠（默认收起） |
| ToolCard | 工具执行卡：一行摘要，展开参数/输出 |
| AskCard | 内核提问卡：confirm/select/input/editor + 结构化问卷 |
| ChatInput | 输入区：多行 + 附件 + @ 引用 + / 命令菜单；底栏在 ComposerBar，菜单在 ComposerSlashMenu / ComposerMentionMenu |
| ComposerBar | 输入区底栏：附件按钮 + 工作区和会话 chip + 水位 + 插话/排队 + 发送/停止 |
| ComposerAttach | 待发送附件列表（区外文件标红并阻断发送） |
| TurnChanges | 一轮回答改了哪些文件（实时轮才有增量 diff） |
| TurnFoot | 回合脚注：复制 + 用量读数 + 时间 |
| ContextMeter | 上下文水位环 + 读数，点击展开占用分解 |
| EngineStatusBar | 状态条 + 小部件行（功能 setStatus/setWidget 桥） |
| StatusHud | 底部常驻：模型/思考档/水位/耗时 |
| EngineCard | 左栏底部内核状态卡，显示就绪/阻塞原因 |
| ExampleCards | 首屏例句 |
| FilePeek | 就地看文件改动 |
| WorkspaceChip | 显示当前工作区；点击可改选目录，改选重启生效（设置页行为面板同样可改选） |

## 独立页组件

| 组件 | 归属 | 职责 |
|---|---|---|
| features/FeatureList | 概览 / 功能页 / 设置页 | 12 项功能清单与开关 |
| skills/SkillList | 技能页 / 设置页 | 技能清单、开关、正文抽屉、导入与新建 |
| doc/DocPreview | 工作区文件树 | PDF / Word / Excel / 文本预览 |
| doc/DocPreviewDrawer | 工作区文件树 | 预览抽屉 |
| workspace/FileTree | WorkspaceView | 工作区目录树 + 点文件就地预览（只读） |
| agents/AgentList | 子代理页 | 活跃清单：状态灯、角色、任务摘要、用时 |
| agents/AgentDetail | 子代理页 | 过程详情：任务 / 思考 / 工具卡 / 正文 / 结论 |
| apps/MicroAppHost | 外部应用页 | qiankun 容器：一个应用一个挂载点，切换即卸载重建 |

骨架组件（多个视图共用）：settings/SettingsSection（设置面板分节容器，八个面板共用）、
layout/TitleBar（自绘标题栏与窗控三键）、layout/NavRail（一级导航）、
layout/SidePanel（当前区左栏，188px 窄档）、layout/PageHeader（页头标题与动作）、
common/AppIcon（内联 SVG 图标）、common/CodeEditor（CodeMirror 封装）、
common/ResizableDrawer（可拖宽抽屉）、common/ToastHost（轻提示宿主）。

其余页面的组成：workspace（FileTree、DiffView、GitStateBanner、GitCommitPanel、GitBranchPanel、
GitHistoryPanel）、dashboard（KpiCard、UsageTrend、ModelShare）、
overview（能力四卡 + 最近会话）、sessions（时间分组清单 + 会话概要）、
help（文档目录 + Markdown 阅读）。

## 重依赖的运行时按需加载

三个功能区的引擎代码只在真正用到时才下载，首屏包不含它们：

| 页面 | 动态导入的依赖 | 分块 |
|---|---|---|
| /apps | qiankun + single-spa + import-html-entry | `vendor-micro` |
| 文档预览 | xlsx / docx-preview / pdfjs-dist | `vendor-xlsx` / `vendor-docx` / `vendor-pdf` |
| 对话正文 | marked / dompurify / highlight.js | `vendor-markdown` |

**子代理事件分流**：`agent:*` 五类事件与 `chat:*` 共用同一条会话 SSE 通道，
事件桥按前缀先分流到 agents store，再进对话状态机——一条连接、两套状态机。

## SettingsView 分组

设置是**单页滚动**，一节一件事；没有左侧目录也不再分二级 tab。

| 顺序 | 面板 | 管什么 |
|---|---|---|
| 1 | AppearancePanel | 主题、字体、字号、自定义背景 |
| 2 | BehaviorPanel | 默认工作区、界面语言、发送键、托盘与自启 |
| 3 | InspectPanel | 当前装配快照（工具 / 技能 / 命令 / 功能 / 系统提示词） |

模型、工具、MCP、技能、功能都有自己的**一级页**，它们的开关不再在设置里列一遍——
两处都能改就会出现「各自认为是权威」。分节骨架由 settings/SettingsSection 提供。

## 界面骨架

```
┌────────────────────────────────────────────────────────┐
│ 顶栏   ▢ WorkBaby │ 当前区标题…              ● ─ □ ✕ │
├───┬──────────┬─────────────────────────────────────────┤
│rail│ 当前区   │ 页头（标题 · 副标题 · 动作）             │
│62px│ 左栏     ├─────────────────────────────────────────┤
│   │ 188px    │ 主区                                    │
└───┴──────────┴─────────────────────────────────────────┘
```

一级导航在左 rail（62px，图标 + 短标签竖排），帮助与设置压在 rail 底部——
它们是旁路，但仍是 rail 里的一等入口。文件浏览不占 rail：它已并入工作区左栏。

## 样式文件分层

| 文件 | 只管 | 纪律 |
|---|---|---|
| `scale.css` | 尺寸 / 字体 / 动效刻度 | **不含色值**，两套主题共用 |
| `themes.css` | 色与两套主题（`--saas_dashboard-color-1..5` 是唯一色源） | 尺寸只引用 scale.css 的刻度 |
| `primitives.css` | 语义别名与面基元（`.wb-glass` / `.wb-code-inline` / 用户背景） | 不含任何色值，只引用 `--wb-*` |
| `buttons.css` | 五态形变基类（`.wb-btn` / `.wb-press` / `.wb-row`）与过渡 | 缩放节奏只在这里定义 |
| `ep-theme.css` | Element Plus 变量 → `--wb-*` 的**唯一一处**映射 | 第二份映射会静默覆盖第一份 |

拆开的原因是「改色」与「改刻度」是两件事：混在一个 500 行的文件里，
改任何一边都要通读全文，而通读之后的改动最容易误伤另一边。

## 设计令牌

**色值只在 `themes.css` 定义**，组件只允许 `var(--wb-*)`。
全站唯一色源是五档 `--saas_dashboard-color-1..5`；`--wb-brand-1..5` 只是它的别名，
不存新色值——存两处就一定会有人改了一处、漏了另一处。

| 档 | 浅色 | 暗色 | 用在哪 |
|---|---|---|---|
| 1 | `#6366f1` | `#818cf8` | 动作：主按钮、选中、强调文字、图表主序列 |
| 2 | `#818cf8` | `#a5b4fc` | 活动：流式光标、运行灯、思考中、图表次序列 |
| 3 | `#e0e7ff` | `#3730a3` | 淡强调：hover 淡底、标签底、用户气泡 |
| 4 | `#f8fafc` | `#0f172a` | 页底：窗口与工作区底色 |
| 5 | `#1e293b` | `#e2e8f0` | 正文与标题、实底反色按钮 |

1 档给「可点的东西」，2 档给「正在发生的事」——两个色的分工是这个体系的骨架。
暗色不是把浅色反过来，而是把五档重新排明度：相邻两档始终递亮，
不会出现深靛压在深底上读不出的序列。

尺寸走刻度（`--wb-sp-*` / `--wb-fs-*` / `--wb-radius-*` / `--wb-dur-*`），
UnoCSS 的刻度已接到令牌上，布局用原子类不绕开令牌。
全部尺寸乘 `--wb-vw-scale`（窗口宽 ÷ 1440），不同分辨率上整套刻度等比跟随。

视觉细节见 `DESIGN.md`。

## 按钮五态

所有可点元素给全五个状态，缺一个就是半成品。

| 状态 | 表现 |
|---|---|
| 默认 | 形状 × 权重 × 语义的静态样式 |
| 悬停 | 放大 1.06 + 抬阴影 |
| 按下 | 缩小 0.92 + 阴影收窄（实体按压感） |
| 禁用 | 降透明 0.45、不参与形变、指针禁用 |
| 加载 | 锁定尺寸（不参与缩放）+ 转圈指示器 |

刻度令牌只在 `themes.css` 定义一处：`--wb-btn-scale-hover`（1.06）/ `--wb-btn-scale-press`（0.92）。

| 控件类别 | 悬停 / 按下 | 基类 |
|---|---|---|
| 实心 / 描边按钮（含 EP） | **1.06 / 0.92** | `.wb-btn` · `.wb-press` · `.el-button` |
| 文字 / 链接按钮 | **不缩放**（缩放带动整行文字抖），只换色 | `.wb-btn--text` |
| 整行元素（会话行 / 技能行 / 文件行 / rail 项） | **不放大**（行宽 100%，放大会溢出被裁）/ 0.985 | `.wb-row` |

**Element Plus 按钮与自绘按钮共用同一套刻度令牌**——同一个操作条里两种按钮并排时，
点击手感必须一致。加载与禁用两态一律 `transform: none`，否则连点会让按钮持续抖动。
组件不得自己写 `scale(0.92)`：散落的值改不到、也查不到。

## 过渡动画

`buttons.css` 定义四组过渡：fade（淡入淡出）、rise（列表项淡入上移）、
pop（卡片与菜单缩放弹出）、skeleton（骨架屏微光）。
尊重系统的「减少动态效果」偏好——该偏好下所有过渡降到 1ms。

## 重型依赖的分块

PDF、表格、文档、微前端、Markdown 五个依赖体积可观，一律**按需导入**并单独分块：

| 分块 | 内容 |
|---|---|
| `vendor-pdf` | pdfjs-dist + vue-pdf-embed |
| `vendor-xlsx` | XLSX |
| `vendor-docx` | docx-preview |
| `vendor-micro` | qiankun + single-spa + import-html-entry |
| `vendor-markdown` | marked + dompurify + highlight.js |

另有 `vendor-echarts`、`vendor-cm`（CodeMirror）、`vendor-ep`（@vueuse）三块，
以及承载 Vue / Pinia / Router / Axios 的 `vendor` 主块。

不进对应页面就不下载它们的代码。Element Plus 不整体归堆——
按需导入的组件归进单个 chunk 会把内部依赖一起拉回首屏，
让未使用组件的 tree-shaking 与各视图的懒加载同时失效。

## 国际化

Vue I18n，**首次启动按系统语言**选 zh-CN / en（`navigator.language`），之后以设置表为准；
回退链兜底中文，英文缺 key 时不会出现空白。

- 所有界面文案走 `t()`，模板里不允许硬编码用户可见字符串
- 中英两套语言包**键集必须完全相等**：缺一个就是半边中文半边英文
- 业务错误的界面文案按错误码查（`utils/errors.ts` → `errCode.*`），
  **不把后端返回的中文原串直接当文案**：语言由前端当前 locale 决定，
  后端原文只作为「具体卡在哪」的细节拼在后面
- 时间与读数的单位（毫秒 / 秒 / 分钟前）也走语言包（`time.*` / `num.*`），
  中文跟单位、英文跟缩写是两种习惯
- Element Plus 的内置文案由根组件的 `ElConfigProvider` 消费当前语言

数据目录、文件路径、模型名等专有名词不翻译；语言名用各自语言写（`中文` / `English`）。

## 状态管理

| 模块 | 职责 |
|---|---|
| chat | 一次 run 的增量落地：15 个 `chat:*` 事件处理器、发送 / 停止 / 插话 / 应答 |
| session | 会话列表与当前会话；adopt（绑定到后端新建的会话）/ draft（未落盘的新对话）/ echo（本地回显） |
| settings | 全局设置与 warmup 清单（providers / skills / tools / features） |
| agents | 子代理观测：活跃清单 + 每条的流式过程（5 个 `agent:*` 事件处理器） |
| apps | 外部微前端应用册：登记项持久化在设置 KV `microapps`，运行态只在内存 |
| toast | 轻提示（悬停停表、最多 4 条） |

**界面不另存真相**：会话内容来自 session store，模型与思考档来自后端会话详情，
功能与工具的开关状态来自 warmup 清单。改开关后必须重拉清单，不能在前端乐观更新。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| hash 路由 | WebView 的 file 协议不接受 history 路径 | URL 带 `#`，不能直接分享深层链接到外部站点 |
| 一级导航放左 rail | 顶栏横向空间随窗口变化，图标入口在窄屏最先被挤掉 | 占 62px 固定宽度 |
| 组件按需引入 | 首屏体积 | 新组件要记得在模板里自然引用 |
| 重型依赖动态导入 | 不进页面就不下载 | 首次打开有加载延迟 |
| 前端不另存状态 | 避免前后端状态漂移 | 每次切换都要拉数据 |
| 子代理过程只走 SSE | 观测不污染上下文，父会话只拿结论 | 断线期间的中间过程要靠 /subagents/transcript 对账 |
| 微前端登记走设置 KV | 一份真相，不新增一张表 | 整册序列化成一个字符串，规模大了要拆表 |
