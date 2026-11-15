# 视觉语言与交互原则

WorkBaby 的设计对象是「第一次用 AI 的办公人员」。视觉只服务两件事：
**看清助手在干什么**、**知道下一步点哪**。宁可朴素，不做需要解释的效果。

## 两条主线

1. **内容为王**：界面只有四层——顶栏、一级导航 rail、当前区左栏、主区；对话页多一根右侧工作台。别的都退后
2. **分层靠圆角 + 描边 + 一层极轻阴影**：卡片壳统一是 `.wb-card`（`--wb-r-xl` 圆角 +
   `--wb-line` 1px 描边 + `--wb-card-shadow`）。渐变只用于图表面积
   （`--wb-area-from` / `--wb-area-to`）

## 布局架构

```
┌─────────────────────────────────────────────────────────────┐
│ 顶栏 48px    ▢ WorkBaby │ 当前区标题…             ● ─ □ ✕  │
├───┬─────────────┬────────────────────────────┬──────────────┤
│ 对 │ 当前区左栏   │ 页头（标题 · 副标题 · 动作） │ 右侧工作台    │
│ 话 │ 232px       ├────────────────────────────┤ （仅对话页，  │
│ ── │ 窄档 188px  │ 主区：消息流 + 输入区        │  有标签才出现）│
│ 工 │ （会话列表） │      + 底部栈（待办/状态条） │              │
└───┴─────────────┴────────────────────────────┴──────────────┘
```

| 层 | 实现 | 规则 |
|---|---|---|
| 顶栏 | `TitleBar`，高 `--wb-topbar`（48px） | 品牌瓦片 · 当前区标题 · 连接状态 · 窗控；只回答「我在哪、连上没有」，不放导航 |
| 一级 rail | `AppRail`，宽 `--wb-rail`（62px） | 十项主入口（概览 / 对话 / 工作区 / 会话 / 模型 / 技能 / MCP / 联网 / 代码 / 用量），图标 + 一行 `--wb-fs-3xs` 短标签；应用与设置在 rail 底部 |
| 当前区左栏 | 对话页 = `SessionList`，宽 `--wb-side`（232px）；容器查询 ≤960px 收窄为 `--wb-side-narrow`（188px） | 会话列表常驻、位置固定不重排 |
| 页头 | 各视图 `.page-head`：标题 + 副标题 + 右侧动作 | 下缘 1px 分隔线，高度随内容不写死 |
| 主区 | 消息流 / 卡片栅格 / 表格 | 底色走表面阶梯（见下） |

**为什么一级导航放左 rail**：顶栏横向空间随窗口变化，图标入口在窄屏下最先被挤压；
导航竖排在左栏边缘，宽度恒定、位置恒定。应用与设置放进 rail 底部而不是顶栏角落的
纯图标——入口靠猜等于没有入口。

**rail 选中态**（`AppRail.vue`）：淡底 `--wb-selected` + 主色图标与文字 + 左缘 2px
主色竖条。会话行、设置项复用同一语言（`.wb-row-click` 的 `is-active`）。

## 表面阶梯与控件令牌

中性面不是「一个灰」，是六级阶梯，全部在 `themes.css` 定义：

| 令牌 | 浅白 · 石板 | 夜石板 | 用在哪 |
|---|---|---|---|
| `--wb-window` / `--wb-work` | `#f8fafc` | `#0b1220` / `#0f172a` | 窗底 · rail / 区底 |
| `--wb-surface` | `#ffffff` | `#131c31` | 表单行 · 列表底 |
| `--wb-raised` | `#ffffff` | `#1b2540` | 卡片 · 面板 |
| `--wb-hover` | `#f1f5f9` | `#263449` | 悬停 |
| `--wb-selected` | `#eef2ff` | `#23305a` | 选中（独有淡靛，回答「我在哪」） |
| `--wb-sunken` | `#eef2f7` | `#0b1220` | 凹陷（代码块底） |

相邻两级只差 2%~5% 亮度——读起来是一整片连续的工作区，而不是卡片叠卡片。
中性叠加另有 `--wb-tint` / `--wb-tint-lg` 两档：悬停底色用它，避免一屏几十个彩色块。

表单控件另走 `--wb-control-*`（bg / bg-hover / bg-disabled / border / border-hover /
shadow / shadow-focus）：控件要有「可以点进去」的浮凸感，与普通面的描边平铺区分开。

## 主题

设计语言「石板 & 靛蓝」：中性全走石板阶，强调只有一个靛蓝色相——明度分层是唯一的强弱手段。

**切换**：`html[data-theme]`。`stores/app.ts` 把 `ui.theme` 设置写进
`documentElement.dataset.theme`（默认浅色，`index.html` 预置 `data-theme="light"`）；
深浅两套块在 `themes.css` 结构对位，只换值。

**全站色源是五档 `--saas_dashboard-color-1..5`**：

| 档 | 语义 | 浅白 | 夜石板 |
|---|---|---|---|
| 1 | 动作（可点的东西） | `#6366f1` | `#818cf8` |
| 2 | 活动（正在发生的事） | `#818cf8` | `#a5b4fc` |
| 3 | 淡强调（标签底 · 气泡 · hover 淡底） | `#e0e7ff` | `#3730a3` |
| 4 | 页底 | `#f8fafc` | `#0f172a` |
| 5 | 正文与标题 | `#1e293b` | `#e2e8f0` |

组件不直接用五档，只认语义层：浅色下 `--wb-brand` / `--wb-brand-2..4` 是五档的别名；
深色下个别档为对比度换成独立值（如 `--wb-brand` 指向亮靛 `#a5b4fc`、
`--wb-brand-deep` 存 `#6366f1` 供实底按钮白字达标）。同一色值存两份，
就一定会有人改一处漏一处，所以组件层只认语义名。

- 1 档与 2 档的分工是「动作」与「活动」：给可点的用 1，给正在发生的用 2（`--wb-live`）
- 主按钮是全站唯一的大面积强调色：实底靛蓝 + 白字。其余按钮一律中性描边 + 中性文字
- 图表五序列 `--wb-ch-1..5` 同一色相明度分档，深的给主序列——灰会让占比环看起来像「没有数据」
- 状态色只留给「通过 / 出错 / 注意」：`--wb-ok` / `--wb-warn` / `--wb-bad`（+ `-soft` 淡底），降饱和不与主色抢
- 正文与次级文字按 WCAG AA 校准（正文 ≥ 4.5:1）；层次交给字号与字重，不靠灰度差区分

## 令牌纪律

- **色值只在 `themes.css` 定义**：组件只允许 `var(--wb-*)`；`.el-*` 选择器只允许出现在 `ep-theme.css`
- **尺寸走刻度**，全部随缩放轴联动：

| 组 | 令牌 | 档位 |
|---|---|---|
| 间距 | `--wb-sp-1..6,8,10,12,16` | 4px 基准十档 |
| 字号 | `--wb-fs-3xs..2xl` + `--wb-fs-display` + `--wb-fs-ctl/label/hint` | 10~34px 十档 + 控件文字三档 |
| 圆角 | `--wb-r-xs..2xl` + `--wb-r-full` | 4/6/7/8/12/14/16px + 胶囊 999px |
| 控件高 | `--wb-ctl-sm/ctl/ctl-lg/ctl-xl/ctl-icon` | 26/34/38/44/32px；开关 `--wb-switch-w/h`（44×24） |
| 图标瓦片 | `--wb-tile-sm/tile/tile-lg` | 30/40/52px |
| 动效 | `--wb-dur-fast/dur/slow/slower` + `--wb-ease` / `--wb-ease-out` | 110/165/240/320ms |
| 按压 | `--wb-press-hover/active/row-active` | 1.06 / 0.92 / 0.985 |
| 层级 | `--wb-z-panel/pop/modal/toast` | 20/60/80/100 |
| 布局 | `--wb-topbar/rail/side/side-narrow/panel/col/w-desc/diff-ln/avatar-size/w-input-sm` | 见布局架构与消息流 |

- **两根缩放轴**：`--wb-fs-scale`（外观设置的字号档，唯一缩放点在 `html { font-size }`）、
  `--wb-vw-scale`（`useViewportScale` 按「窗口宽 ÷ 1440」写入，上下限锁死 0.75~1.35）。
  分辨率不同的机器上整套刻度等比跟随，不为每个断点写媒体查询
- **胶囊只留给真正的圆形**（头像 / 状态点 / 水位环 / 滚动条 / `--wb-r-full`）；
  chip、输入框、图标钮走 `--wb-r` 一档。小控件套满圆角就是一根胶囊，看不出「可以点进去」的边界
- **UnoCSS 刻度已接令牌**（`uno.config.ts`）：spacing / borderRadius / fontSize / colors
  全部映射到 `--wb-*`——`gap-4` = `var(--wb-sp-4)`、`text-sm` = `var(--wb-fs-sm)`、
  `bg-raised` = `var(--wb-raised)`。布局与间距用原子类写，不会绕开令牌；
  快捷类 `card` / `card-pad` / `plate` / `row` / `readout` 也收口在这里
- **Element Plus 只是基元供给方**：`ep-theme.css` 把 EP 的 CSS 变量全量接到 `--wb-*`
  （`--el-color-primary` = `--wb-brand`、`--el-border-radius-base` = `--wb-r`、
  过渡时长接 `--wb-dur*`），按控件族拆七份，必须排在 `element-plus/dist/index.css`
  之后（`main.ts`）——同样的选择器后加载的赢。EP 在 `main.ts` 整体注册
  （`app.use(ElementPlus)`），模板可直接写 `<el-input>`、`<el-table>`、`<el-drawer>` 等；
  `ElMessageBox` 这类命令式 API 不是模板组件，用到它的文件仍要显式
  `import { ElMessageBox } from 'element-plus'`。`.el-button` 在模板里不直接用
  （按钮一律 `.wb-btn`），桥接只为 ElMessageBox 内部按钮与自绘手感对齐
- 字体四个语义变量：`--wb-font-sans`（正文）/ `--wb-font-ui`（界面正文，默认取 sans，
  外观设置「字体」只改这一个值）/ `--wb-font-display`（标题与读数）/ `--wb-font-mono`（路径与代码）
- 其余成组令牌：阴影 `--wb-shadow-1..3` / `--wb-card-shadow(-hover)` / `--wb-btn-shadow` /
  `--wb-btn-sheen`；代码块配色 `--wb-code-*`（底 / 边 / 注释 / 关键字 / 字符串等十二个）；
  行高三档 `--wb-lh-tight/base/loose` 与铭牌字距 `--wb-ls-plate`；文本选区 `--wb-sel`
- **引用不存在的变量会静默失效**——不是「样式不好看」，是静默丢样式，最危险

## 按钮 · 五态规格

每个可点元素必须给全五个状态，缺一个就是半成品。

**缩放节奏只在 `themes.css` 定义一处**（`--wb-press-hover` 1.06 / `--wb-press-active` 0.92 /
`--wb-press-row-active` 0.985），按形态分五档落点：

| 形态 | 悬停 / 按下 | 实现 |
|---|---|---|
| 按钮 · 图标钮 · chip（自绘与 EP 统一） | 1.06 / 0.92 | `.wb-btn`（`base-buttons.css`）、`.el-button`（`ep-buttons.css`） |
| text / link 文字按钮 | 不缩放，只换色 | `.wb-btn.text`、`.wb-pressable-text`——缩放带动整行文字抖动 |
| 整行元素（会话行 / 技能行 / 文件行 / rail 项） | 不放大 / 0.985 | `.wb-pressable-row`、`.wb-row-click`（`base-press.css`）——行宽 100% 放大会溢出，在 `overflow: hidden` 侧栏里被裁掉边缘 |
| 卡片 | 抬升 1px + 阴影加深 | `.wb-card--hover` |
| 分段控件 | 0.92 | `.seg-btn`（`base-press.css`）、`WbSeg` |

组件复用令牌，**不复制粘贴 `scale(0.92)`**——复制十几份的结果是有人改了十一处、
漏了一处，那一处就成了「点了没反应」。

| 状态 | 视觉 | 实现 |
|---|---|---|
| 默认 | 形状 × 权重 × 语义的静态样式 | `.wb-btn` 用 `--btn-*` 局部变量换肤；变体 `primary` / `ghost` / `danger` / `text`，尺寸 `sm` / `lg` / `icon` |
| 悬停 | 放大 + 变色 | `:hover:not(:disabled):not(.is-loading)` |
| 按下 | 缩小 + 变色，60ms 弹回 | `:active:…`，`transition-duration: 60ms` |
| 禁用 | 变淡 0.45 + not-allowed，不缩放 | `:disabled` / `.is-disabled` |
| 加载 | 转圈 + 文字保留 + 尺寸锁定 | `.is-loading`：`::before` 画圈 + `pointer-events: none`——转圈时再点会发出第二个请求，两次比一次更慢 |

- 危险按钮 = 中性描边 + 危险色文字（`.wb-btn.danger`）：彩色描边违反分层纪律，语义交给文字与底色
- EP 按钮经 `ep-buttons.css` 接同一组刻度令牌：同一个操作条里两种按钮并排时手感一致
- 异步动作必须接 loading（在 await 前挂上、`finally` 摘），只换文案不转圈不算数

## 消息流的视觉语言

对话页三栏：会话列表（左）+ 消息流（中）+ 右侧工作台（有标签才出现）。
消息列 `.msg-col` 收在 `--wb-col`（`min(880px × 视口比例, 100%)`）居中，
左侧 1px 脊线（`.msg-col::before`）串起整段对话——身份线由它承担，助手正文保持平面。

- **一轮 = 角色徽章 + 内容**：徽章是全大写微字胶囊（`.msg-badge`），用户侧染主色、
  助手侧弱化灰；助手轮在脊线上钉一枚 7px 节点，流式中转 `--wb-live` 并呼吸
- **用户消息右对齐气泡**（`--wb-brand-soft` 底、`--wb-r-lg`、右上角 `--wb-r-xs` 收尖，
  宽度上限 `min(78%, 640px)`），与助手左对齐平面正文形成左右分栏；用户轮不参与脊线
- **一个回合只有一个徽章、一份内容面板**：思考段、正文与工具卡按引擎吐出的原始顺序
  内联排（`TurnBlocks`，顺序永不重排——重排会把工具卡顶到正文前面，读不出先干了什么）；
  回合内块间 `--wb-sp-2`（8px）、回合间 `--wb-sp-3`（12px）
- 接着上一轮往下说的轮次不出头不钉节点（`continued`）；一轮里连续的活动段折成一行摘要
  （`ActivityGroup`，仅非流式时折叠——直播中折叠会遮住正在生长的段落）
- 轮尾读数（输入 / 输出 / 缓存 / 耗时）只在最后一轮显示，复制与删除摆在读数旁，不藏菜单

**改动可视化的三个入口**（都汇到右侧工作台）：

- 工具卡头部行内的「看改动」（`ToolCallCard`）：有 `file_path` 的工具块点它开工作台标签；
  没有文件路径的 diff 只能就地铺开（`DiffRows`）。「看改动」占头部一行不另起行——
  每个调用都占两行的话，一轮十几个调用就多出一屏空白
- 轮尾变更芯片（`TurnChanges`）：同一文件多块并成一行、增删行数累加，点芯片开标签——
  它只是补充，完整答案在 git 页
- 右侧工作台（`WorkPanel` + `useWorkPanel.ts`）：多标签常驻面板，标签按文件路径去重，
  重复点同一文件只激活已有标签；每个标签内 `FileInspect` 提供「差异 / 文件」双标签
  （`WbSeg` 切换，mode 记在标签上）——差异问 git 工作区真实那份（引擎给的可能是
  它当时的改动，用户又改过），文件问 `files/read`，路径变化时两份都预取。
  面板不是抽屉：不遮消息、不丢滚动位置，宽度可拖（300~720px，持久化 `localStorage`）

**输入区与状态**：底部栈（`TodoPanel` + `ChatComposer` + `ChatNotices`）由 `ChatView`
实测高度写入 `--wb-bottom-h`，供「回到最新」按钮与提示条定位。上下文水位
（`ContextMeter`）是 conic-gradient 环 + 详情弹层（剩余 / 缓存命中率 / 速率 / 花费），
75% 转 `--wb-warn`、90% 转 `--wb-bad`；没数据也渲染成半透明「– / –」——空着会让用户
以为图标坏了。队列 / 停止 / 发送三键常驻（动作被顶出去就是按钮消失）；
发送键在发送期间保持可点，点它等于喊停。

**长会话的渲染纪律**：

1. DOM 规模由分页约束：`HistoryList` 首屏 40 轮，列首常驻「加载更早的 {n} 轮」
   文字按钮向上补页——不藏在滚动事件里，用户往下滚之前也能看到
2. 流式 markdown 走块级增量渲染（`utils/md.ts`）：按顶层块切分缓存 HTML，每个节流
   tick 只重渲染变化中的尾块——全量重解析是 O(字数²)，分块后是 O(字数)。
   滚动容器 `overflow-anchor: none`，贴底由 `ChatView` 管
3. 思考可能几万字：流式期只渲染尾部 4000 字，正文一出现就自动收起
   （`ThinkingBlock`）——全量塞进 DOM 会把答案顶出屏

## 组件基元

| 场景 | 用什么 |
|---|---|
| 表单 / 表格 / 弹窗 / 抽屉 / 开关 / 选择 | Element Plus：`el-input` / `el-form` / `el-table` / `el-dialog` / `el-drawer` / `el-switch` / `el-select` / `el-radio-button`（经 `ep-theme.css` 接管，整体注册） |
| 布局 / 间距 / 栅格 | UnoCSS 原子类（刻度已接令牌） |
| 图表 | ECharts 按需注册（`UsageCharts.vue`），色值运行时读 `--wb-ch-*`；用量页监听 `ui.theme` 变化重绘 |
| 代码编辑 | `CodeEditor`：CodeMirror 6 只读 / 可编辑 + 按语言高亮。不用现成包装——主题、工具栏、状态栏都得听令牌的 |
| 消息流 / markdown / 工具卡 / 思考块 / 提问卡 | 自绘 `content-*.css`（EP 没有对应物，且是内容渲染不是表单基元） |
| 图标 | `AppIcon`：24 网格内联 SVG，笔画重量走 `--wb-icon-stroke`（暗色加粗一档抵消视觉误差）；禁止 Unicode / emoji 冒充图标 |
| 轻提示 | `ToastHost`：右下角堆叠、可点选复制、悬停暂停计时；默认 4.2s、错误 8s 自动收 |
| 浮层 | `WbPopover`：贴锚点弹、点外关、Esc 关、定位夹回视口、超高内部滚动（现用于上下文水位详情） |
| 分段切换 | `WbSeg`：互斥选项 + 方向键 / Home / End 键盘可达；五处手写 `.seg` 结构收成一份，五态与刻度共用 |
| 空态 | `WbEmpty`：图标瓦片 + 标题 + 提示 + 动作插槽；瓦片缺省中性色——空态不是错误 |
| 卡片壳 | 全局 `.wb-card` 类：`--wb-r-xl` 圆角 + `--wb-line` 1px 描边 + `--wb-card-shadow`，定义在 `content-surfaces.css` 唯一处；uno 快捷 `card-pad` 补内边距 |
| 骨架屏 | `WbSkeleton`：列表行数 / 卡片轮廓两种形态，微光在 `--wb-skeleton-from/to`（取自表面阶梯）间扫 |
| 差异行表 | `DiffRows`：行号列宽 `--wb-diff-ln` 与着色只有一份，工具卡与工作区页共用 |
| 命令面板 | `CommandPalette`：一个输入框搜全部动作（跳页 / 全局动作 / 最近会话 / 技能命令），键盘优先 |
| 启动失败 | `StartupError`：真实原因 + 「看日志」「重试」两个动作 |

**自绘门槛**：同一个结构在三个以上页面各写一遍才收成组件；新增组件优先用 EP 现成的。
基元的 props 形状放 `types/ui.ts`（SFC 的 `export interface` 不能被 import）。
`ModelPicker` / `ThinkingChip` / `WorkspaceChip` 的浮层监听（document pointerdown + Esc）
各自实现，与 `WbPopover` 并存。

### 设置面板

设置页按域拆面板组件（外观 / 启动行为 / 引擎 / 工作区 / 配置原文 / 模板 / 诊断 / 关于），
每张卡用 uno 快捷 `card-pad` 外壳 + 分节标题，说明文字走 `set-hint`。行内布局是普通
flex：标签在左、控件紧随其后，不让控件贴右边缘——宽卡里控件与说明隔半个屏幕，
相邻两行的开关还会挤成同一列，误点一次就改错一项。

### 读数不等于控件

只读的东西（上下文水位、耗时、模型名）**不套描边胶囊、不做悬停缩放**：
有盒子又没动作，等于请用户去点一个点不动的东西。要给反馈就换字色，不换几何。
uno 快捷 `readout`（等宽 + tabular-nums）是读数的默认字形。

### 差异视图

差异行表（`DiffRows`）被工具卡与工作区页共用，着色细节只有一份：

- 行号列宽走令牌 `--wb-diff-ln`（46px × 视口比例），装得下 5 位行号；两处各写一个 44px
  的结果是并排与内联对不齐
- hunk 头（`@@ -a,b +c,d @@`）跨满整行、主色字 + 淡底：它是「这段改动从哪开始」的唯一路标
- 上下文折叠只保留 hunk 前后若干行；多个 hunk 时页头给「上一处 / 下一处」跳转，
  锚点用 hunk 的行文本而不是行下标——折叠会让下标错位，而 `@@` 文本在一份差异里唯一
- 折叠段里的目标行先展开再滚：直接 `scrollIntoView` 会滚到一片空气上

## 反馈与动效

| 时刻 | 反馈 | 实现 |
|---|---|---|
| 点击 | 按下缩小（见五态表） | `--wb-press-*` |
| 异步开始 | 按钮转圈 + 禁用，同批操作只有一枚在动 | `.is-loading` |
| 异步成功 | 右下角轻提示一句，4.2s 自动收（悬停暂停） | `ToastHost` / `useToasts` |
| 异步失败 | 轻提示给出原因原文，错误 8s，不让消息无声消失 | `useToasts` |
| 列表增删 | 淡出 + 缩放 0.96，不做瞬时消失 | `.wb-list-*`（`base-elements.css`） |
| 视图切换 | 淡入淡出 + 6px 位移 | `.wb-fade-*`；`prefers-reduced-motion` 与 `[data-motion='reduced']` 时一律 1ms 到位 |
| 流式输出 | 正文末尾闪烁光标；思考块淡描边转 `--wb-live-line`；脊线节点呼吸；工具卡运行灯转圈 | `content-markdown.css` / `content-thinking.css` / `content-tools.css` |
| 等待首字 | 状态灯呼吸 + 一句「正在做什么」+ 耗时读数同行（等宽字），冷启动十几秒也不像卡住 | `TurnBlocks` 的 `.waiting`（`.dot.busy` 走 `wb-breathe`） |
| 工具卡展开 | 高度过渡展开 | `content-tools.css` 的 `tool-unfold` |

## 文案

- 一律走 i18n：`locales/zh-CN` 与 `en` 同构，key 按域分层（chat / common / manage /
  workspace / settings / overview / index / web）；模板里禁止硬编码用户可见字符串
- 文案里出现 `@` 必须写成 intlify 字面量 `{'@'}`——裸 `@` 会被解析成 linked 消息语法
  并在渲染时抛「Invalid linked format」
- 说人话：不出现「实例 / 端点 / 会话上下文」这类词，说「这条对话 / 接口地址 / 还能聊多少」
- 交互反馈的底线：异步动作要有 loading；空态给引导（`WbEmpty` / 对话页空态给三条可点的
  开场建议）；后端 `message` 直显——错误必须带**下一步**，不是「失败了」，
  而是「连不通：401，去检查密钥」
- 没有原因不硬编一句「操作成功」；读数缺失显示「–」而不是 0——0 是测量结果，「–」才是没测过
