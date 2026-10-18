# WorkBaby 界面与体验整备计划

> 目标：在不重写 PI、不换内核架构的前提下，把 8 个问题逐条闭环。
> 原则：内核归 PI、界面归 WorkBaby（AGENTS.md §1.2）；改动前后跑 `scripts/check.ps1`。

## 〇、三项决策记录（已确认）

1. **并行会话**：按「两个 CLI 窗口 = 两个工作区」的场景理解——这正是现有架构（一个工作区一个进程）已经支持的形态。**不改进程模型**，只把「多会话同时跑」在界面上做可见、可切换。
2. **提问多选**：核对过 PI 源码（`rpc-extension-ui.md` + `rpc-mode.ts`），RPC 协议的 `select` 回传的是**单个 value**；CLI 里见到的多选来自 TUI 的 `custom()` 组件，RPC 模式明确不支持（`custom() returns undefined`）。因此 WorkBaby 把 confirm / select / input / editor 四种问答做到精致（长选项列表带过滤、editor 用多行文本域），并在计划里如实标注这一协议边界；未来 PI 若给 select 加多选协议，翻译层一处即可跟上。
3. **前端技术栈**：不整体引入 Element Plus——现有 `themes.css + wb-ui.css` 自绘体系就是按你给的靛蓝五档配色案例建的，引组件库反而偏离该视觉语言。仅在确有收益处按需引轻量库（当前评估无需）。

## 一、问题根因定位（已核实到文件）

| # | 问题 | 根因 |
|---|---|---|
| 1 | 设置页内容挤在半边 | `SettingsView.vue` 的 `.wrap-md` 上限 1080px 且面板内部多用单列；宽屏下留白过大 |
| 2 | 整体 UI 待打磨 | 页头/卡片/列表形态在各视图各自实现，未统一成案例中的「图标瓦片页头 + 读数卡 + 悬停反馈」语言 |
| 3 | 加载页不好看 | `App.vue` 的 `.boot` 只有一个色块 + 一行字，无阶段感与动效 |
| 4 | skill/插件/tool/MCP 首屏不展示 | 各面板 `onMounted` 才各自拉取；工具清单依赖「最近会话的工具声明」快照（没聊过就是空）；MCP 在线状态要等 PI 进程起来；store 无预热，点刷新也只刷单面板 |
| 5 | 会话历史要求 | `SessionList.vue` 全量分组展示，无条数上限与「更多」 |
| 6 | 并行会话 / 提问 | 并行已支持但界面不可见；AskCard 形态偏简陋；PI 的 fire-and-forget 事件（notify/setStatus/setWidget）在 `translate.go` 里被静默丢弃 |
| 7 | 输入与流式 | 基础已扎实；存在一处真 bug（`MessageItem.vue` 思考正文渲染了两遍）；运行读数（耗时/token）未展示 |
| 8 | 全链路闭环 | 随各条修复合并验证；文档与测试同步收口 |

## 二、实施方案（按阶段）

### 阶段 1 · 设置页布局重排（问题 1 + 问题 2 的设置部分）

- `SettingsView.vue`：`.wrap-md` 放宽到 `min(100%, 1280px)`；页头改为「图标瓦片 + 标题 + 副题 + 右侧动作区」的统一形态。
- 各 Panel（`ProviderPanel / SkillPanel / ExtensionPanel / ToolPanel / MCPPanel / AppearancePanel / BehaviorPanel`）：
  - 列表类内容改为**响应式卡片网格**（`repeat(auto-fill, minmax(320px, 1fr))`），宽屏两列、窄屏一列，不再一列到底；
  - 表单类（Provider 编辑器、MCP 保存表单）用 `grid2` 已有基元，但补齐窄屏折叠；
  - 统一 `.pn-head`（标题 + 说明 + 动作区）为共享样式，收进 `wb-ui.css`。
- 全部按钮沿用 `.btn` 五态基元（默认/悬停放大 1.03/按下 0.96/禁用/加载），不新造。

### 阶段 2 · 启动加载页重做（问题 3）

- `App.vue` 的 `.boot` 重做：
  - 靛蓝双光晕背景（纯 CSS `blur` 圆，对应案例的 aura 手法；浅暗两主题各一套令牌）；
  - WB 标识 tile + 呼吸动效；下方分阶段状态行：`握手本地服务 → 读取引导数据 → 检查助手环境`（阶段由 `bootstrap.ts` 的 status + `loadBoot` 进度驱动）；
  - 失败态保留现有重试/退出入口，视觉同一语言。
- 不引任何依赖，纯 CSS 动画（`--wb-dur-*` / `--wb-ease` 刻度内）。

### 阶段 3 · 首屏数据预热与「真实清单」（问题 4）

后端：
- `service` 新增一个轻量聚合探测 `POST /env/probe`（或挂 `/bootstrap` 扩展字段）：后台并行拉 skills / providers / tools 快照 / extensions / mcp 配置清单，一次回传；PI 进程**不因进设置页而常驻**。
- 工具清单：快照为 static 时如实标注「按内置表推断，聊过一轮后显示真实声明」（现有文案保留），并在**任一会话跑完一轮后**由翻译层在 `agent_settled` 时让快照缓存失效，前端下次进面板即见真实清单。
- MCP 在线状态：进 MCP 面板时若进程未起，显示「配置了但未连接（对话开始后接入）」而非空白，消除「点了也不出来」的错觉。

前端：
- `stores/settings.ts` 增加 `warmup()`：App `loadBoot()` 成功后并行预拉 skills/providers/tools/extensions/mcp 到 store；
- 各 Panel 改为「store 有缓存直接渲染 + 静默刷新」，删除「必须手点刷新/重连才出数据」的路径；
- 列表加载态统一用 `PageState`/骨架，杜绝白屏。

### 阶段 4 · 会话列表「最近 10 条 + 更多」（问题 5）

- `SessionList.vue`：默认只渲染最新 10 条（跨分组按更新时间取前 10，分组标题保留）；
- 列表底部加「更多」按钮：每次追加 20 条，直到全部展示；按钮带已收起条数提示（`还有 N 条`）；
- 展开状态在会话面板会话期内保持，不随重渲染丢失。

### 阶段 5 · 并行会话可见性 + 提问卡强化（问题 6）

并行会话（不改架构）：
- `SessionList.vue`：运行中的会话行加活动呼吸点（`--wb-live` 色）与「运行中」小标；
- `WorkspaceChip` / 会话面板：显示当前工作区是否有会话在跑，切到运行中的工作区时明确提示「这个工作区有会话在跑」；
- `SessionVO.Running` 已由后端给出，前端只补 UI；打开其他工作区会话不受当前运行影响（后端 `Send` 只锁同工作区，已满足）。

提问与引擎反馈：
- `AskCard.vue` 重做：confirm 双按钮、select 带选项列表（长列表可滚动 + 键盘 ↑↓ + 回车）、input 单行、editor 多行文本域（自动撑高）；全部按钮走五态基元；
- `translate.go` + `protocol.go`：把 fire-and-forget 事件接进来——
  - `notify` → 新 SSE 事件 `chat:notify`（info/warning/error），前端 ToastHost 弹出；
  - `setStatus` → 新 SSE 事件 `chat:status`，会话头部/输入区上方显示引擎状态条（可清除）；
  - `setWidget` → 折叠渲染在消息流末尾（插件输出的小部件，文本行）；
  - `setTitle` / `set_editor_text` 按语义落地（标题进标题栏、编辑器预填进输入框）。
- 错误码沿用 5xxx 段；`docs/ERROR-CODES.md` 如有新增码同步。

### 阶段 6 · 输入区与流式输出打磨（问题 7）

- 修 bug：`MessageItem.vue` 删除重复的第二个 `wb-think-body` 渲染块；
- 运行读数：run 期间在助手列底部显示「耗时 · 本轮 token」（数据已在 `runState`，经 `chat:context`/`chat:done` 补齐即可）；
- 输入区：
  - 聚焦态视觉强化（品牌色描边 + 光晕环）；
  - 草稿按会话保留（切会话不丢正在写的话，参照 pi-harness 的 composer-cache 思路，存前端 store 即可）；
  - 附件 chips、队列 dock、插话/排队 seg 的过渡动效统一走 `--wb-dur-*`；
- 流式：光标闪烁已有，补「新块入场」微动效与滚动贴底的平滑化（现有 rAF 机制上微调）；
- 思考块流式期间的展开/收起行为保持现状（正文出现自动收起）。

### 阶段 7 · 全链路检查、测试与文档同步（问题 8）

- 逐条业务闭环走查：新建会话→发送→插话/排队→提问应答→停止取回队列→删除会话；换工作区→技能重扫；模型服务增删→默认模型→测试连通；技能启停→斜杠菜单即时可见。
- 测试（按 AGENTS.md §3 判据，只加跨链路断言）：
  - `pi/pi_test.go`：fire-and-forget 事件（notify/setStatus）的解析与翻译各一条；
  - `service/chain_test.go`：会话列表运行态标记、探测聚合接口形状；
  - `api/api_test.go`：新端点契约（信封、空数组不 null）。
- 文档同步：`specs/17-pi-supervisor.md`（事件翻译表补 notify/setStatus/setWidget 行）、`docs/` 对应页面（若设置页行为变化）、`DESIGN.md` 不动（视觉语言未变）。
- 验证：`pwsh scripts/check.ps1`（快门）→ 涉及 pi/service/api 的 `-Pkg` 分包 → 最后 `-Full`。

## 三、明确不做

- 不重写 PI 内核、不引第二份 Agent 实现；
- 不改「一个工作区一个 PI 进程」的不变量（与 CLI 双窗口场景一致）；
- 不整体引入 Element Plus / UnoCSS / ECharts（现有自绘体系即目标视觉语言）;
- 不在设置页探测时常驻拉起 PI 进程；
- 不做国际化 / 微前端 / 富文本编辑器等当前用不到的能力。

## 四、验收口径

1. 宽屏（≥1600px）设置页内容铺满主区、网格自适应；窄窗（960px）不横向溢出。
2. 冷启动加载页有阶段反馈与动效，失败可重试。
3. 首次进入设置页，各面板无需手动刷新即有数据（或明确标注数据来源与时机）。
4. 会话列表默认 10 条 + 「更多」逐步展开。
5. 两个工作区同时跑会话时，两侧会话行均亮运行灯，可自由切换查看。
6. 引擎 notify/状态条/提问卡在界面上完整可见、可应答；confirm/select/input/editor 四态可用。
7. `scripts/check.ps1` 快门通过，`-Full` 通过。
