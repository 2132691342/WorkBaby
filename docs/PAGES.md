# 页面 · PAGES

路由（hash 模式）与组件清单。整体设计见 [FRONTEND.md](FRONTEND.md)。

## 路由

| 路径 | 视图 | 说明 |
|---|---|---|
| `/` | ChatView | 对话主视图 |
| `/overview` | OverviewView | 概览：能力总览四卡 + 最近会话 |
| `/workspace` | WorkspaceView | 文件树 + diff + Git |
| `/features` | FeaturesView | 内置功能开关台 |
| `/skills` | SkillsView | 技能清单台 |
| `/files` | FilesView | 文件浏览 + 文档就地预览 |
| `/sessions` | SessionsView | 全部会话与概要 |
| `/dashboard` | DashboardView | 用量仪表盘 |
| `/help/:name?` | HelpView | 内置文档阅读 |
| `/settings/:tab?` | SettingsView | 设置页，tab 落 URL |

## ChatView 组件

| 组件 | 职责 |
|---|---|
| SessionPanel / SessionList | 左栏：新对话、时间分组、搜索、重命名/导出/删除 |
| MessageList | 消息流：历史回合 + 进行中回合 + 提问卡 |
| AssistantTurn | 一条助手消息：按原始顺序渲染思考/正文/工具卡 |
| ThinkingBlock | 思考段折叠（默认收起） |
| ToolCard | 工具执行卡：一行摘要，展开参数/输出 |
| AskCard | 内核提问卡：confirm/select/input/editor + 结构化问卷 |
| ChatInput | 输入区：多行 + 附件 + @ 引用 + / 命令菜单 |
| ContextMeter | 上下文水位环 + 读数，点击展开占用分解 |
| EngineStatusBar | 状态条 + 小部件行（功能 setStatus/setWidget 桥） |
| StatusHud | 底部常驻：模型/思考档/水位/耗时 |
| EngineCard | 左栏底部内核状态卡，显示就绪/阻塞原因 |
| ExampleCards | 首屏例句 |
| FilePeek | 就地看文件改动 |
| WorkspaceChip | 切工作目录 |

## 独立页组件

| 组件 | 归属 | 职责 |
|---|---|---|
| features/FeatureList | 概览 / 功能页 / 设置页 | 12 项功能清单与开关 |
| skills/SkillList | 技能页 / 设置页 | 技能清单、开关、正文抽屉、导入与新建 |
| doc/DocPreview | 文件页 | PDF / Word / Excel / 文本预览 |
| doc/DocPreviewDrawer | 文件页 | 预览抽屉 |
| graph/RelationGraph | 关系图谱 | 能力关系可视化 |
| graph/BpmnDiagram | 工作流图 | 只读画布，缩放平移可用 |

**同一份逻辑只写一处**：功能页与设置页共用 `FeatureList`，技能页与设置页共用 `SkillList`。

## SettingsView 分组

| 组 | 项 |
|---|---|
| 通用 | 外观（主题/字体/字号/背景）、行为（语言/自启/关窗行为） |
| 模型 | 服务商（models.json 面板）、默认模型 |
| 能力 | 内置功能、技能、工具、MCP |
| 内核 | 状态与数据目录、帮助 |
| 引导 | 工作目录管理 |

对应面板：AppearancePanel / BehaviorPanel / ProviderPanel / FeaturePanel /
SkillPanel / ToolPanel / MCPPanel / InspectPanel。

## WorkspaceView / Dashboard / Help / Overview / Sessions / Files

- workspace：DiffView、GitStateBanner、GitCommitPanel、GitBranchPanel、GitHistoryPanel
- dashboard：KpiCard、UsageTrend（双面积线）、ModelShare（占比环）
- help：文档目录（GET /docs）+ Markdown 阅读
- overview：能力总览四卡（功能/技能/工具/MCP 计数）+ 最近会话列表
- sessions：左侧按时间分组的会话清单 + 右侧会话概要与操作
- files：面包屑导航 + 目录/文件列表 + 文档预览抽屉

## 状态与桥

| store | 职责 |
|---|---|
| chat | 一次 run 的增量落地：20 个 `chat:*` 处理器、发送/停止/插话/应答 |
| session | 会话列表与当前会话；adopt / draft / echo |
| settings | 全局设置与 warmup 清单（providers/skills/tools/features） |
| toast | 轻提示（悬停停表、最多 4 条） |

SSE 消费挂 `composables/chatBridge.ts`（应用壳级，不随视图卸载）；
连接单例在 `composables/useSse.ts`。
壳层调用收口在 `src/api/shell.ts`：端口/对话框/窗控/自启/退出/托盘事件，
内部走 Tauri invoke + listen，开发模式回落 localStorage，组件不得直接 import `@tauri-apps/*`。

## 交互基元

`buttons.css` 定义全站按钮五态与过渡动画（fade / rise / pop / skeleton）。
Element Plus 按钮与自绘 `.wb-btn` 共用刻度令牌
（`--wb-btn-scale-hover` 1.06 / `--wb-btn-scale-press` 0.92），
并尊重系统的「减少动态效果」偏好。
