# 前端架构 · FRONTEND

Vue 3 + TypeScript 的单窗口应用，通过 HTTP 与 SSE 与本地服务对话。
前端的全部状态来自后端——它不另存一份真相。

## 分层

| 层 | 目录 | 职责 |
|---|---|---|
| api | `src/api` | 唯一请求出口；路径与后端端点一一对应 |
| stores | `src/stores` | chat（run 增量）/ session（列表与当前）/ settings（清单）/ toast |
| composables | `src/composables` | chatBridge（事件→store）、useSse（连接单例）、useTheme、useAppearance |
| components | `src/components` | 按域分组：chat / settings / workspace / layout / dashboard / common / features / skills / doc / graph |
| views | `src/views` | 路由级页面 |
| utils | `src/utils` | md（唯一渲染出口）、echarts（按需注册）、num / text / time 格式化 |
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
- 断线自动重连，重连后带 `Last-Event-ID` 让服务端重放遗漏事件
- 握手晚于挂载时也能建起来——端口是响应式的，拿普通变量 watch 等不到变化

## 路由与页面

hash 模式（WebView 的 file 协议不接受 history 模式的路径）。

| 路径 | 视图 | 说明 |
|---|---|---|
| `/` | ChatView | 对话主视图 |
| `/overview` | OverviewView | 能力总览四卡 + 最近会话 |
| `/workspace` | WorkspaceView | 文件树 + diff + Git |
| `/features` | FeaturesView | 12 项内置功能开关台 |
| `/skills` | SkillsView | 技能清单台 |
| `/files` | FilesView | 文件浏览 + 文档就地预览 |
| `/sessions` | SessionsView | 全部会话与概要 |
| `/dashboard` | DashboardView | 用量仪表盘 |
| `/help/:name?` | HelpView | 内置文档阅读 |
| `/settings/:tab?` | SettingsView | tab 落 URL（刷新与前进后退都停在原处） |

**独立页与设置页共用组件**：`features/FeatureList` 与 `skills/SkillList` 各被两处引用——
两处各写一份必然漂移。

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

一级导航在左 rail（62px，图标 + 短标签竖排），设置与帮助压在 rail 底部——
它们是旁路，但仍是 rail 里的一等入口。

## 设计令牌

**色值只在 `themes.css` 定义**，组件只允许 `var(--wb-*)`。
强调色是单一靛蓝色相的五档：

| 档 | 色值 | 用在哪 |
|---|---|---|
| 靛蓝 | `#6366f1` | 动作：主按钮、选中、强调文字、图表主序列 |
| 淡靛 | `#818cf8` | 活动：流式光标、运行状态灯、思考中、图表次序列 |
| 淡紫底 | `#e0e7ff` | 淡强调：hover 淡底、标签底、图表第三序列 |
| 石板底 | `#f8fafc` | 页面底、选中淡底、提示条底 |
| 石板深 | `#1e293b` | 正文与标题、反色按钮、图表第四序列 |

靛蓝给「可点的东西」，淡靛给「正在发生的事」——两个色的分工是这个体系的骨架。

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
| 禁用 | 降透明、不参与形变、指针禁用 |
| 加载 | 锁定尺寸（不参与缩放）+ 转圈指示器 |

刻度令牌：`--wb-btn-scale-hover`（1.06）/ `--wb-btn-scale-press`（0.92）。

**Element Plus 按钮与自绘按钮共用同一套刻度令牌**
（`--wb-btn-scale-hover` / `--wb-btn-scale-press`）——同一个操作条里两种按钮并排时，
点击手感必须一致。加载与禁用两态一律 `transform: none`，否则连点会让按钮持续抖动。

text / link 类文字按钮不缩放（缩放带动整行文字抖动），只换色。

## 过渡动画

`buttons.css` 定义四组过渡：fade（淡入淡出）、rise（列表项淡入上移）、
pop（卡片与菜单缩放弹出）、skeleton（骨架屏微光）。
尊重系统的「减少动态效果」偏好——该偏好下所有过渡降到 1ms。

## 重型依赖的分块

PDF、表格、文档、图谱四个依赖体积可观，一律**动态导入**并单独分块：

| 分块 | 内容 |
|---|---|
| `vendor-pdf` | pdfjs-dist + vue-pdf-embed |
| `vendor-xlsx` | XLSX |
| `vendor-docx` | docx-preview |
| `vendor-graph` | AntV X6 + BPMN-JS |

另有 `vendor-markdown`（marked + dompurify + highlight.js）、`vendor-echarts`、
`vendor-cm`（CodeMirror）三块，以及承载 Vue / Pinia / Router / Axios 的 `vendor` 主块。

不进对应页面就不下载它们的代码。Element Plus 不整体归堆——
按需导入的组件归进单个 chunk 会把内部依赖一起拉回首屏，
让未使用组件的 tree-shaking 与各视图的懒加载同时失效。

## 国际化

Vue I18n，zh-CN 默认 + en。所有界面文案走 `t()`；
Element Plus 的内置文案由根组件的 `ElConfigProvider` 消费当前语言。

数据目录、文件路径、模型名等专有名词不翻译。

## 状态管理

| store | 职责 |
|---|---|
| chat | 一次 run 的增量落地：20 个 `chat:*` 事件处理器、发送 / 停止 / 插话 / 应答 |
| session | 会话列表与当前会话；adopt（绑定到后端新建的会话）/ draft（未落盘的新对话）/ echo（本地回显） |
| settings | 全局设置与 warmup 清单（providers / skills / tools / features） |
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
| 用 hash 而非 history | 单窗口应用无服务端路由配合 | 深链接体验略差 |
