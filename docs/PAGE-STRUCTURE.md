# docs/PAGE-STRUCTURE.md · 页面与视图结构

## 1. 路由表

Wails 单 HTML 资源下 history 模式无法 fallback，统一用 **hash 路由**；路由层不做鉴权守卫（鉴权由 Wails 绑定承载）。

信息架构：外壳左栏只保留任务主链路（聊天 / 设置），功能页全部收进设置中心。
旧路由（`/memory` `/skills` …）重定向到 `/settings?tab=x`，命令面板与历史书签的深链仍可达。

| 路径 | 组件 | 说明 |
|---|---|---|
| `/` | — | 重定向 `/chat`（主场景是聊天） |
| `/chat` | `chat/ChatView.vue` | 空态工作区 |
| `/chat/:id` | `chat/ChatView.vue` | 指定会话 |
| `/settings` | `settings/SettingsView.vue` | 设置中心（`?tab=` 切换 19 个 section） |
| `/pet/desktop` | `pet/PetDesktop.vue` | 桌宠独立窗口形态 |
| 其余 | — | 重定向 `/chat` |

## 2. 应用外壳

```
┌────────────────────────────────────────────────────────────┐
│ 标题栏：窗口控制 + 会话标题 + 模型 + 工作区 chip              │
├──────────┬──────────────────────────────┬──────────────────┤
│ 会话侧栏  │ 主区                          │ 右栏面板（可收起）│
│ 236px    │                              │ 336 / 420px      │
│          │  · 消息流（过程块按序渲染）    │                  │
│ · 搜索   │  · 停止原因横幅（可续跑时）    │ · 工作区          │
│ · 会话树 │  · 目标卡 / 待办卡             │ · 变更与产物      │
│ · 置顶   │  · 审批卡（内联）              │ · 后台任务        │
│ · 归档   │  · 输入区（模型/权限/引用/环） │ · 辅助对话        │
└──────────┴──────────────────────────────┴──────────────────┘
```

| 区域 | 组件 | 关键行为 |
|---|---|---|
| 标题栏 | `ChatView.vue` 头部 | 工作区 chip（点开 picker）；运行中常驻停止按钮 |
| 会话侧栏 | `chat/SessionSidebar.vue` | 置顶 / 归档 / 搜索 / 清空 / 压缩 / 批量管理 |
| 主区 | `chat/MessageList.vue` + `chat/MessageItem.vue` | 消息流 + 过程块 + 内联审批；切会话先清空列表显示骨架（避免闪假空态） |
| 输入区 | `chat/ChatInput.vue` + `composer/` | 一排放得下：模型 / 权限 / 参数 / 命令 / 引用 / 上下文环 / 发送；流式时变 `插入（steer） + 停止` |
| 右栏 | `chat/WorkspacePanel.vue` / `FileChangesPanel.vue` / `tasks/` / `chat/SideConversation.vue` | 同一位置、同一宽度、同一关闭方式 |

## 3. 聊天工作区（6 个状态）

| 状态 | 结构 |
|---|---|
| 空态 · 新任务 | 草稿态不落库；示例卡可直接起一个任务 |
| 会话进行中 | 思维链折叠 + 工具时间线 + 变更聚合 + 用量徽标（`UsageBadge.vue`） |
| 流式执行中 | `StreamingBubble.vue` 正文边出边渲染 + 工具运行中计时；可插话 / 排队 / 停止 |
| 审批暂停 | `ApprovalInline.vue`：命令 / 工作目录 / 风险 / 已等待 + `拒绝 / 本会话允许 / 仍要批准` |
| 目标模式 | `GoalCard.vue` + `PinnedPlan.vue`（待办推进 + 计划胶囊） |
| 中断与恢复 | `StopReasonBanner.vue`：停止原因 +「继续」入口（可续跑终态集合唯一在 `chat/models/blocks.ts`） |

### 3.1 过程块渲染

`chat/MessageBlocksRenderer.vue` 是过程渲染的唯一实现，接受两种输入（历史块序列 / 流式暂存序列），内部归一为 `RenderBlock` 后只写一遍渲染逻辑。

块类型：`thinking / text / tool_call / tool_result / artifact / skill / genui`。

| 渲染规则 | 说明 |
|---|---|
| 分组 | `groupToolRuns` 按**相邻性**切分：连续工具块归入一张过程卡，正文块自然切断分组——保留「叙述 → 工具 → 叙述」真实顺序 |
| 配对 | `tool_call` 与同 `tool_call_id` 的 `tool_result` 合并展示为一个工具单元 |
| 工具卡徽标 | 失败计数 `×N`（同工具连续失败）/「改道」（内核已注入替代路径）/ cwd（exec 实际工作目录） |
| 结果渲染 | 知识库命中 → 来源卡；diff → `DiffView`；多行 → 行列表；JSON → 缩进着色（超 14 行默认折叠）；其余原样 |
| 复制 | args / result 各一个复制入口（`useClipboard` legacy 降级 + ✓ 反馈） |

## 4. 设置中心（19 个 section）

`SettingsView.vue` + `?tab=` 切换；左导航一级化（19 子项直接列出，分组标题只做视觉断行）。

| 组 | section |
|---|---|
| 模型 | 模型服务（Provider / 预设 / 连通测试 / 熔断）、网络搜索 |
| 能力 | 工具、技能、MCP 服务、子智能体、自定义命令、用户钩子 |
| 数据 | 记忆中心、知识库、仓库导读（Wiki）、文件、仪表盘、运行历史 |
| 系统 | 外观、安全与高级（exec 白名单 / 免审授权）、桌宠、文档、关于 |

## 5. 独立窗口与系统集成

| 界面 | 组件 / 机制 |
|---|---|
| 桌宠窗口（260×300） | `pet/PetDesktop.vue`；Win32 `CreateRectRgn` + `CombineRgn` 设置命中区域实现点击穿透 |
| 托盘 | `internal/tray`（Win32 原生）；菜单：显示/隐藏、召唤·收起桌宠、退出 |
| 单实例 | `internal/singleinstance`（命名互斥 + 本地 TCP IPC）；第二次启动唤起已有实例并传文件路径 |
| 文件关联 | `.md / .txt / .pdf` 双击唤起 |
| 命令面板 | `common/CommandPalette.vue`（Ctrl+K） |
| 启动引导 | 无 Provider 时引导添加（设置 → 模型） |

## 6. 主窗口 / 桌宠态切换

两种形态共享同一前端上下文与状态（单窗口切换，非多窗口）：

| 形态 | 尺寸 | 特性 |
|---|---|---|
| 主窗口态 | ≥960×640 | 完整三栏布局 |
| 桌宠态 | 260×300 | 可置顶、可移动、非命中区域鼠标穿透 |

切换时记忆主窗口位置与尺寸并在切回时还原。
