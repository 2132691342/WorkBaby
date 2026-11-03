# 前端架构 · FRONTEND

Vue 3 + TypeScript 的单窗口应用，通过 HTTP 与 SSE 与本地服务对话。
前端的全部状态来自后端——它不另存一份真相。

## 分层

| 层 | 目录 | 职责 |
|---|---|---|
| api | `src/api` | 唯一请求出口；路径与后端端点一一对应 |
| stores | `src/stores` | chat（run 增量）/ session（列表与当前）/ settings（清单与 KV）/ agents（子代理）/ apps（微前端册）/ runClock（本轮秒表）/ toast |
| composables | `src/composables` | chatBridge（SSE 事件 → store 变更）、useSse（SSE 连接单例）、useTheme / useAppearance（主题与外观持久化）、useViewportScale（窗口宽 → `--wb-vw-scale`）、useSessionPanel、useComposer*（草稿 / 载荷 / 命令 / 提及）、filePeek（消息里文件路径的就地预览）、shellIntent（壳命令意图分发） |
| components | `src/components` | 按域分组：chat / settings / workspace / layout / dashboard / common / features / skills / doc / agents / apps |
| views | `src/views` | 路由级页面（13 个） |
| utils | `src/utils` | md（唯一渲染出口）、errors（业务码 → 界面文案）、echarts（按需注册）、num / text / time / turns / composerMenu |
| locales | `src/locales` | zh-CN（默认）+ en，按域拆文件后汇总 |

### api 层

`api/http.ts` 是唯一发起请求的地方：

- baseURL 由壳启动后注入（`127.0.0.1:{port}/api/v1`）
- 统一解包 `Resp { code, message, data }`，`code !== "0"` 抛 `ApiError`（带 code）
- 浏览器层失败翻成人话（断连、超时分别给不同提示）

`api/shell.ts` 是系统能力的唯一出口：端口、文件对话框、窗控、自启、退出、托盘事件。
内部走 Tauri invoke + listen，开发模式（浏览器直开）回落 `localStorage.wb_dev_port`。
**组件不得直接 import `@tauri-apps/*`**——统一收口才能让开发模式与生产模式行为一致。

### SSE 连接

`composables/useSse.ts` 持有唯一连接；`composables/chatBridge.ts` 把事件翻译成 store 变更。

- 挂在 App 壳级，不随视图卸载（切页面不该断事件流）
- 断线自动重连，重连时以 `last_event_id` 查询参数声明断点，服务端据此重放补齐遗漏事件
- 握手晚于挂载时也能建起来——端口是响应式的，拿普通变量 watch 等不到变化
- `agent:*` 与 `chat:*` 共用同一条连接，事件桥按前缀先分流到 agents store

## 路由与页面

hash 模式（WebView 的 file 协议不接受 history 模式的路径）。`/` 重定向到 `/overview`；
`/features`、`/dashboard`、`/files` 是旧入口别名，分别重定向到 `/plugins`、`/usage`、`/workspace`。

| 路径 | 视图 | 说明 |
|---|---|---|
| `/overview` | `OverviewView` | 能力总览四卡 + 最近会话（落地页） |
| `/chat` | `ChatView` | 对话主视图 |
| `/workspace` | `WorkspaceView` | 文件树 + 改动清单 + diff + 提交 / 历史 / 分支 |
| `/sessions` | `SessionsView` | 全部会话与概要 |
| `/agents` | `AgentsView` | 子代理观测：活跃清单 + 流式过程 + 结论 |
| `/apps` | `AppsView` | 外部微前端应用登记与隔离运行（qiankun） |
| `/models` | `ModelsView` | 模型服务与工具配置 |
| `/skills` | `SkillsView` | 技能清单台 |
| `/plugins` | `FeaturesView` | 两段：12 项内置功能开关台 + 记忆库（别名 `/features`） |
| `/mcp` | `McpView` | MCP 服务器配置与在线状态 |
| `/usage` | `DashboardView` | 用量仪表盘（别名 `/dashboard`） |
| `/help/:name?` | `HelpView` | 内置文档阅读 |
| `/settings/:tab?` | `SettingsView` | 单页滚动：外观 / 行为 / 运行时 / 内省 |

**独立页与设置页共用组件**：`features/FeatureList` 与 `skills/SkillList` 各被两处引用——
两处各写一份必然漂移。

## 对话页组件

| 组件 | 职责 |
|---|---|
| `SessionPanel` / `SessionList` | 左栏：新对话、时间分组、搜索、重命名 / 导出 / 删除 |
| `MessageList` | 消息流：历史回合 + 进行中回合 + 提问卡 |
| `ChatMessage` | 单条消息容器：角色布局、复制、usage 徽标 |
| `AssistantTurn` | 一条助手消息：按原始顺序渲染思考 / 正文 / 工具卡 |
| `ThinkingBlock` | 思考段折叠（默认收起） |
| `ToolCard` | 工具执行卡：一行摘要，展开参数 / 输出；执行中自动展开并实时追加过程输出 |
| `AskCard` | 内核提问卡：confirm / select / input / editor + 结构化问卷 |
| `ChatInput` | 输入区：正文 + 技能芯片 + 待发送图片 + `@` 引用 + `/` 命令菜单 |
| `ComposerBar` | 底栏：引文件、图片、工作区与会话 chip、水位、插话 / 排队、发送 / 停止 |
| `ComposerMenu` | 模型选择面板：左服务商右模型，顶部跨服务商搜索，方向键 / 回车 / Tab 导航 |
| `ComposerSkill` / `ComposerImages` | 技能芯片（一条消息最多一个）/ 待发送图片条 |
| `ComposerSlashMenu` / `ComposerMentionMenu` | `/` 命令与 `@` 文件提及菜单 |
| `SessionChips` | 输入区上方的模型 / 思考档 / 工作区 chip 行 |
| `ContextMeter` | 上下文水位环 + 读数，点击展开占用分解 |
| `ActivityBar` | 输入框上方活动条：正在干什么 / 已输出多少字 / 耗时 / 安静过久提示 |
| `EngineStatusBar` / `StatusHud` / `EngineCard` | 状态条 / 底部常驻读数 / 左栏内核就绪卡 |
| `TurnChanges` / `TurnFoot` | 一轮改了哪些文件 / 回合脚注（复制 + 用量 + 时间） |
| `ChatHeader` | 页头：会话名 + 模型行 + 分支树抽屉开关 + 压缩上下文按钮 |
| `SessionTree` · `SessionTreeNode` | 会话分支树：主干高亮 + 分支侧枝，节点可点重开 / 切回旧支（递归渲染，深度不限） |
| `OnboardingWizard` | 首启引导：选服务商 → 填密钥 → 测连通，三步在对话页内做完 |
| `FilePeek` / `WorkspaceChip` | 就地看文件 / 显示并可改选工作区（热生效） |
| `features/FeatureDock` | 待办 / 目标 / 计划三个 panel 的 tab 抽屉；有内容才露出 |
| `ExampleCards` | 首屏例句 |

## 其余组件

| 组件 | 归属 | 职责 |
|---|---|---|
| `layout/TitleBar` · `NavRail` · `SidePanel` · `PageHeader` | 骨架 | 自绘标题栏与窗控三键 / 一级导航 rail / 当前区左栏 / 页头 |
| `common/AppIcon` · `CodeEditor` · `ResizableDrawer` · `ToastHost` · `EmptyState` | 骨架 | 内联 SVG 图标（24 网格）/ CodeMirror 封装 / 可拖宽抽屉 / 轻提示宿主 / 空态 |
| `settings/SettingsSection` | 骨架 | 设置分节容器（四个面板共用） |
| `settings/ProviderPresetPicker` | 骨架 | 内置服务商预设一键添加（密钥可选） |
| `features/FeatureList` · `FeatureConfigPop` · `TodoPanel` · `GoalPanel` · `PlanPanel` · `PlanNoteEditor` · `MemoryPanel` | 功能 | 功能清单与开关 / 按字段表渲染的配置抽屉 / 三块活状态面板 / 计划驳回与标注编辑区 / 记忆库 |
| `skills/SkillList` | 技能 | 清单、开关、正文抽屉、导入与新建 |
| `workspace/FileTree` · `DiffView` · `GitChangesPane` · `GitCommitPanel` · `GitHistoryPanel` · `GitBranchPanel` · `GitStateBanner` | 工作区 | 目录树 / 差异 / 改动清单 / 提交 / 历史 / 分支 / 冲突横幅 |
| `agents/AgentList` · `AgentDetail` | 子代理 | 活跃清单 / 过程详情 |
| `apps/MicroAppHost` | 外部应用 | qiankun 容器：一个应用一个挂载点，切换即卸载重建 |
| `dashboard/KpiCard` · `UsageTrend` · `ModelShare` | 用量 | 指标卡 / 趋势 / 模型占比 |
| `doc/DocPreview` · `DocPreviewDrawer` | 文档预览 | PDF / Word / Excel / 文本预览 |
| `settings/AppearancePanel` · `BehaviorPanel` · `RuntimePanel` · `InspectPanel` · `ProviderPanel` · `ToolPanel` · `MCPPanel` | 设置 | 外观 / 行为 / 内置运行时状态 / 装配快照 / 模型 / 工具 / MCP |

## SettingsView 分组

设置是**单页滚动**，一节一件事；没有左侧目录也不再分二级 tab。

| 顺序 | 面板 | 管什么 |
|---|---|---|
| 1 | `AppearancePanel` | 主题、字体、字号、自定义背景 |
| 2 | `BehaviorPanel` | 默认工作区、界面语言、发送键、安全闸门模式、项目约定开关、托盘与自启 |
| 3 | `RuntimePanel` | 内置运行时：bash / git / python 是否就位、命中路径、解压失败原因 |
| 4 | `InspectPanel` | 当前装配快照（工具 / 技能 / 命令 / 功能 / 系统提示词） |

模型、工具、MCP、技能、功能都有自己的**一级页**，它们的开关不再在设置里列一遍——
两处都能改就会出现「各自认为是权威」。

## 界面骨架

```
┌────────────────────────────────────────────────────────┐
│ 顶栏 48px  ▢ WorkBaby │ 当前区标题…            ● ─ □ ✕ │
├───┬──────────┬─────────────────────────────────────────┤
│rail│ 当前区   │ 页头（标题 · 副标题 · 动作）             │
│62px│ 左栏     ├─────────────────────────────────────────┤
│   │ 188px    │ 主区（消息流 / 卡片栅格 / 表格）         │
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
| `base.css` | 布局骨架与设置行等通用形态 | 不写色值 |
| `content.css` | 消息流 / markdown / 代码块 / 工具卡 / 思考块 | 只放原子类拼不出来、EP 也没有的东西 |
| `messages.css` / `composer-pop.css` | 对话页与输入区弹层的局部样式 | 跟随对应组件 |
| `ep-theme.css` / `ep-widgets.css` | Element Plus 变量 → `--wb-*` 的**唯一一处**映射与 EP 控件微调 | 第二份映射会静默覆盖第一份 |

拆开的原因是「改色」与「改刻度」是两件事：混在一个文件里，改任何一边都要通读全文，
而通读之后的改动最容易误伤另一边。

## 设计令牌

**色值只在 `themes.css` 定义**，组件只允许 `var(--wb-*)`。
全站唯一色源是五档 `--saas_dashboard-color-1..5`；`--wb-brand-1..5` 只是它的别名，
不存新色值——存两处就一定会有人改了一处、漏了另一处。
视觉细节见 [DESIGN.md](../DESIGN.md)。

尺寸走刻度（`--wb-sp-*` / `--wb-fs-*` / `--wb-radius-*` / `--wb-dur-*`），
UnoCSS 的刻度已接到令牌上，布局用原子类不绕开令牌。
全部尺寸乘 `--wb-vw-scale`（窗口宽 ÷ 1440），不同分辨率上整套刻度等比跟随。

## 按钮五态

所有可点元素给全五个状态：默认 / 悬停（放大 1.06）/ 按下（缩小 0.92）/ 禁用 / 加载。
刻度令牌只在 `themes.css` 定义一处：`--wb-btn-scale-hover` / `--wb-btn-scale-press`。

| 控件类别 | 悬停 / 按下 | 基类 |
|---|---|---|
| 实心 / 描边按钮（含 EP） | **1.06 / 0.92** | `.wb-btn` · `.wb-press` · `.el-button` |
| 文字 / 链接按钮 | **不缩放**（缩放带动整行文字抖），只换色 | `.wb-btn--text` |
| 整行元素（会话行 / 技能行 / 文件行 / rail 项） | **不放大**（行宽 100%，放大会溢出被裁）/ 0.985 | `.wb-row` |

加载与禁用两态一律 `transform: none`，否则连点会让按钮持续抖动。
组件不得自己写 `scale(0.92)`：散落的值改不到、也查不到。

## 重型依赖的运行时按需加载

五个依赖体积可观，一律**按需导入**并单独分块，不进对应页面就不下载：

| 分块 | 内容 | 用在哪 |
|---|---|---|
| `vendor-micro` | qiankun + single-spa + import-html-entry | `/apps` |
| `vendor-xlsx` | XLSX | 文档预览 |
| `vendor-docx` | docx-preview | 文档预览 |
| `vendor-pdf` | pdfjs-dist + vue-pdf-embed | 文档预览 |
| `vendor-markdown` | marked + dompurify + highlight.js | 对话正文 |

另有 `vendor-echarts`、`vendor-cm`（CodeMirror）、`vendor-ep` 三块，
以及承载 Vue / Pinia / Router / Axios 的 `vendor` 主块。
Element Plus 不整体归堆——按需导入的组件归进单个 chunk 会把内部依赖一起拉回首屏，
让未使用组件的 tree-shaking 与各视图的懒加载同时失效。

## 国际化

Vue I18n，**首次启动按系统语言**选 zh-CN / en（`navigator.language`），之后以设置表为准；
回退链兜底中文，英文缺 key 时不会出现空白。

- 所有界面文案走 `t()`，模板里不允许硬编码用户可见字符串
- 中英两套语言包**键集必须完全相等**：缺一个就是半边中文半边英文
- 业务错误的界面文案按错误码查（`utils/errors.ts` → `errCode.*`），
  **不把后端返回的中文原串直接当文案**：语言由前端当前 locale 决定，
  后端原文只作为「具体卡在哪」的细节拼在后面
- 时间与读数的单位（毫秒 / 秒 / 分钟前）也走语言包（`time.*` / `num.*`）
- Element Plus 的内置文案由根组件的 `ElConfigProvider` 消费当前语言
- 文案里出现 `@` 必须写成 intlify 字面量 `{'@'}`——裸 `@` 会被解析成 linked 消息语法

## 状态管理

| store | 职责 |
|---|---|
| `chat` | 一次 run 的增量落地：`chat:*` 事件处理器、发送 / 停止 / 插话 / 应答 |
| `session` | 会话列表与当前会话；adopt（绑定后端新建的会话）/ draft（未落盘的新对话）/ echo（本地回显） |
| `settings` | 全局设置与 warmup 清单（providers / skills / tools / features） |
| `agents` | 子代理观测：活跃清单 + 每条的流式过程（`agent:*` 事件处理器） |
| `apps` | 外部微前端应用册：登记项持久化在设置 KV `microapps`，运行态只在内存 |
| `runClock` / `toast` | 本轮秒表 / 轻提示（悬停停表、最多 4 条） |

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
| 子代理过程只走 SSE | 观测不污染上下文，父会话只拿结论 | 断线期间的中间过程要靠 `/subagents/transcript` 对账 |
| 微前端登记走设置 KV | 一份真相，不新增一张表 | 整册序列化成一个字符串，规模大了要拆表 |
| 消息列跟窗口走（`min(1240px, 100%)`） | 宽窗口里两侧的大片空白没人受益 | 极宽屏下行长需靠排版自行约束 |
| 窄窗口按容器查询收窄侧栏 | 判断的是内容区宽度而不是窗口宽度 | 与媒体查询两套阈值并存时要各自维护 |
