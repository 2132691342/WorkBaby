# CHANGELOG

版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。`0.x` 期间接口可能变动。

## [1.0.0]

首个正式版。

### 托盘

- **托盘图标**：Windows 托盘缺省没有图标（任务栏里只是透明占位，找不到应用）；
  现在注入与应用可执行文件一致的那枚图标，并补悬停提示。图标经 `go:embed`
  进二进制，托盘与窗口共用同一份资源

### 修复

- 打包版所有 POST 变 Network Error（CORS 预检被 methodGuard 拦下）——见 0.6.1
- 内置 Python 解压缺 stripComponents，链路从未可能就绪——见 0.6.1
- 全站缺 `box-sizing: border-box` 导致非全屏横向溢出——见 0.6.1

### 测试

- 新增真实 HTTP 栈全端点冒烟（含跨源预检回归）：模拟 WebView2 来源走一遍
  CORS，普通单测测不出的浏览器侧断裂从此有守门员

## [0.6.1]

对照 `AGENTS.md` 与参考实现（pi / pigo）做全面体检，全部是规范归位，无功能变化。

### 规范修复

- **错误处理全站收口 AppError**：`llm.ErrNoAPIKey` / `ErrBadStream`、retry 未配置、
  openai / anthropic / ollama 上游错误文本、单实例端口记录无效——原先 7 处
  `errors.New` 丢 code，前端无法按段位分流；现在分别落 3001/3002/3105/3106/2304
- **统一响应外壳进 domain**：新增 `domain.Resp`，`api.ok/fail` 与 SSE 端点
  建流前出参改引 domain，消除 server/api 层内联结构体（§2.3.1）
- **前端裸像素字号归位**：`App.vue` / `ToolRunRow.vue` 改用 `--wb-fs-xs`（§2.10.1）
- `main.go` 日志统一走 `pkg.Infof/Warnf/Errorf`，不再用标准库 log

### 打包版所有 POST 按钮报 Network Error（最严重）

- **真凶**：gin 中间件顺序——`methodGuard` 排在 `corsLocal` 前面，浏览器跨源 POST
  的 OPTIONS 预检被「只允许 GET 与 POST」拦下，响应不带任何 CORS 头，
  浏览器直接拒绝后续 POST。GET 不需要预检所以列表照常加载，唯独写操作全灭；
  开发态与 Go 测试都不走浏览器预检，因此一直没暴露。
  顺序对调 + 新增预检回归测试（OPTIONS 必须返回 204 + ACAO）
- axios 网络层错误翻译成人话：断连/被拦时提示「连不上本地服务（端口），
  检查残留进程或代理拦截」，不再是干巴巴的 Network Error

### 内置 Python 永远「未就绪」

- **两个断点**：① `build/bin` 旁边没有 runtimes 包（旧脚本找的是解压目录）；
  ② 压缩包带顶层 `python/` 目录而解压代码从没实现 manifest 声明的
  stripComponents——解压出 `runtime/python/python/python.exe`，代码找的却是
  `runtime/python/python.exe`，这条链路从未可能就绪
- `extractTarGz` 实现顶层剥离（含跳过纯目录条目）；`safeExtractPath` 补上
  对 `/abs` 带根路径的拒绝；新增真实归档解压测试与穿越拒绝测试
- `copy-runtimes.ps1` 口径对齐 tar.gz，产物旁与 NSIS 安装器都能拿到包

### 界面与交互整改（非全屏「啥都显示不完」的真凶）

- **全站缺 `box-sizing: border-box`**：`.wrap` 族是 `width:100% + 左右 padding`，
  内容盒恒比父级宽出约 40px——任何非全屏窗口都横向溢出，全屏才被富余掩盖。
  补标准重置后，960×640（应用最小窗）全站零溢出，逐一截图核验
- `html/body` 禁止滚动、滚动容器 `overflow-x: hidden`：
  个别子元素超宽时裁切而非整页横移
- `select.input` 显式锁高，与 input 同网格行不再上下错位
- **模型服务表单**：加「添加/编辑模型服务」标题；新服务不显示永远禁用的
  「拉取模型列表」（保存后再出现）；去掉表单下方突兀的
  「正在添加第 1 个模型服务」空态卡，改为保存按钮旁的一句提示
- **外观与行为**：网格重排——主题 | 开机自启 等高并排，执行方式三档横排一行，
  消除空格与参差；「自动」档更名「先问我」（它其实是最谨慎的一档）
- 仪表盘窄窗读数四格 2×2；残留裸像素字号（10/10.5/11/21px）全部归位令牌

### 界面修复（设置页按钮「看得见点不着」，上一轮）

- **技能 / 知识库工具栏整体失效**：工具栏 `class="bar"` 与 wb-ui.css 进度条
  组件 `.bar { height: 5px; overflow: hidden }` 撞名，按钮被裁成细条且点击失效。
  工具栏改名 `.toolbar`，进度条组件不受影响
- 列表行浏览器默认外边距撑爆（h5 上下 20px、p 上下 11px）：行内归零
- 长描述把开关挤到下一行：`.skill .main` flex-basis 改 0
- 列表动作组任何宽度都强制换行：`.rli > .grow` flex-basis 100% 改 0
- 技能面板错误提示用了不存在的 `alert-bad` 修饰类，改 `is-bad`

### 文档修订

- `AGENTS.md`：技术栈表移除 Element Plus（前端实际无 UI 组件库，全自绘），
  依赖黑名单补入 UI 组件库
- `docs/API-CONTRACT.md`：补技能三条路由（新建 / 导入 / 删除）
- `docs/PAGE-STRUCTURE.md`：移除不存在的 `AttachList.vue`（附件 chip 内联在 ChatInput）
- `docs/ARCHITECTURE.md`：规模数字更新（99 个 Go 文件 / 39 个前端源文件）
- `README.md`：规格数量统一为 01-14
- `docs/DEPLOYMENT.md` + `scripts/copy-runtimes.ps1`：运行时分发口径统一为
  tar.gz 包（与 `runtime.ArchivePath` 一致），修复脚本永远"找不到"的问题

## [0.6.0]

补齐「功能闭环」，并把界面从「能看」整改到「能用」。

### 技能闭环（之前只能开关，等于没法用）

- **新建技能**：设置页内置模板，填名字 + 说明 + 正文即可保存，写进数据目录并立刻注册
- **导入技能**：支持选 `SKILL.md`、选含它的文件夹、或选装满技能的一层目录；
  同名自动加序号，不静默覆盖
- **删除技能**：只删用户技能，内置技能拒绝并提示「可以先关掉」
- **修 `Registry.Get` 只认名字**：前端拿到的 id 是 `name@source` 形式，
  按 id 删除永远返回「技能不存在」
- **删除目录按 Location 反推**：导入重名加过序号时，重算 `skillsDir/name` 会指错地方

> 内置的 `create-skill` 只是提示词，而技能目录在工作目录之外，模型没理由写得进去。
> 「能不能自己写」必须有一条**不依赖模型**的路径。

### 界面

按真机逐页体检后整改，这一轮的改动几乎全是**布局崩坏**而不是配色：

- **修会话列表横排**（`.sess-group` 漏了 `flex-direction: column`）：
  分组标题「今天 / 本周 / 更早」竖成一列，会话名互相挤压成一条
- **补全站横向溢出防护**：`min-width: auto` 的 flex 子项会把页面撑破，
  表现为右侧内容被裁、底部多一条横向滚动条。窗口最窄 960px，必须兜住
- **仪表盘读数不再挤成竖排**：`flex: 1` 四等分在窄窗口把「3.9 秒」压成两行，
  改 `auto-fit + minmax(160px, 1fr)` 自行换行
- **列表行动作不再被挤出可视区**：标题独占整行、动作组右对齐换行，
  长路径按省略号收尾而不是硬裁
- **模型服务表单**：模板 chips 独占一行、字段栅格在窄屏降为单列
- **知识库改为一步添加**：原来「再选一个 → 添加这些」两步，
  新手会以为第一次点击没生效；现在选中即建索引
- **技能行恢复紧凑**：开关与删除回到标题同一行右侧
- 设置页补页面标题与一句话说明；仪表盘无数据时保留四个读数块与提示卡

### 工具链

- 加**开发态假后端**（`frontend/src/src/dev/`）：`npm run dev` 直接在浏览器里
  看到成品界面，改视觉不必每次 `wails build` 再开窗。
  生产构建下 `import.meta.env.DEV` 为 false，整段被摇掉，不进包

## [0.5.2]

### 修复

- **首次启动必然被误判为失败**（真凶）：Wails 把 `OnStartup` 放在**独立 goroutine**
  里跑（`frontend.go:222`），而 `OnDomReady` 由 WebView2 导航回调触发，
  **两者没有任何顺序保证**。原实现用「`Svc` 是不是 nil」判断启动失败，
  而首次启动要解压 46MB 内置 Python（约两秒），`domReady` 必然先到 →
  界面永远停在启动页。改为显式握手：`startup` 结束关闭 `startDone`，
  `domReady` 等它（上限 45 秒），不再靠猜

## [0.5.1]

修 0.5.0 引入的「永远停在启动中」并补上失败可见性。

### 修复

- **卡在「启动中…」永不进主界面**：`App.vue` 用 `wb:ready` 自定义事件判断就绪，
  而该事件在「监听器注册之前」派发同样会丢——和 Wails 事件是同一类竞态。
  改为**直接读模块级响应式状态**，组件挂载时读到的是当前值而不是「未来的事件」
- **启动失败时前端收不到任何提示**：`app:startup-error` 原先走 `service.Emitter`，
  而启动失败恰恰意味着 HTTP/SSE 还不存在——这条错误永远送不出去。
  改走 Wails 事件总线
- **数据库被占用时不说人话**：旧版退出失败留下的僵尸进程会锁住 `workbaby.db`，
  现在直接提示「另一个 WorkBaby 还在运行，请先在任务管理器里结束它」

### 新增

- **启动失败页**：显示原因 + 「重试」+「退出应用」两个动作，
  不再让用户对着一个永远转圈的窗口发呆
- **20 秒握手超时兜底**：端口一直没来就明确报错，而不是无限等待
- **`ForceQuit` 绑定**：前端在托盘异常 / 启动失败时也有一条确定走通的真退出路径

## [0.5.0]

桌面应用该有的东西补齐，同时挖出四个「静默失效」的严重缺陷。

### 修复

- **全新机器首次启动直接失败**（这才是全站 404 的真正根因）：
  `viper.SetConfigFile` 指定路径时返回的是 `*fs.PathError` 而非
  `ConfigFileNotFoundError`，`isNotExist` 漏判 → 启动即失败 → `Svc` 为 nil →
  `OnDomReady` 直接返回 → **界面空着、每个接口 404，而真实原因只在日志里**
- **「退出」按钮是空操作**：`OnBeforeClose` 无条件返回 true，
  而 Wails 的 `Quit()` 会先调它再决定是否关闭，于是托盘点退出被自己拦下。
  表现为托盘图标消失但进程还活着，并一直占着 exe（这也是覆盖安装失败的原因）。
  现用 `quitting` 标志位区分「关窗口」与「真退出」，`shutdown` 清理后直接结束进程，
  另有 8 秒看门狗兜底 WebView2 不返回的情况
- **内置技能一个都加载不到**：`//go:embed skills` 的 FS 根是 `skills/`，
  代码却传了仓库路径 `assets/skills`，遍历报错被忽略 → 静默返回空列表
- **含列表的 frontmatter 导致技能被跳过**：解析成 `map[string]string`，
  遇到 `when_to_use:` 这类 YAML 列表就整体失败。`office-docs` 一直没能加载
- **读写锁配对错误**：`skill.Registry` 的 `List()` / `Get()` 用了
  `RLock` + `defer Unlock`，一调用就 panic。而 `BuildSystem` 每轮对话都会
  调 `Render()` → `List()`，等于**第一次发消息就崩**
- **内置技能正文打不开**：embed 出来的 `Location` 是虚拟路径，
  模型的 `read` 工具读不到；现在正文随注册表存内存
- **SQLite 连接从不关闭**：退出后数据目录被占用，无法删除也无法备份
- **版本号三处不一致**：`main.go` 与 `wails.json` 还停在 0.2.0

### 新增

- **开机自启**：`HKCU\...\Run` 写入 exe 路径，设置页有开关，卸载器会清理
- **启动失败可见**：记录失败原因并推 `app:startup-error` 给前端，
  启动期关键节点全部落日志文件（原先只写 stderr，GUI 程序里没人看得见）
- **首次启动也认文件关联**：命令行里的 md/txt 由主实例自己接手

### 安装器

- 覆盖安装前先 `taskkill` 掉运行中的实例（托盘常驻时 exe 仍被占用）
- 文件关联从空宏改成真正注册 md / txt
- `project.nsi` 补 UTF-8 BOM：没有它，中文注释与字符串会被按系统代码页解析而报错
- 卸载时清掉开机自启项

### 文档

- `specs/11` 补「关闭与退出的区别」与安装器定制约束；
  `AGENTS.md` 补 4 条不变量与 3 个测试文件

## [0.4.0]

### 修复

- **所有接口 404**：`app:ready` 的监听器注册在 `onMounted` 的**动态 import** 回调里，
  而 Go 侧 `domReady` 早就把事件发出去了。Wails 事件不缓冲给后注册的监听器，
  端口永远注不进来，axios 拿空 baseURL 去打 Wails 静态服务器。现改为
  `main.ts` 顶层静态注册 + Go 侧启动后 6 秒内重复广播，前端幂等接收

### 新增

- **仪表盘**：`GET /stats` 汇总 token 用量，按天 / 按模型 / 按会话；
  柱状图与占比条全部用原生 CSS，不引图表库。天数可切 7 / 14 / 30
- **实时上下文水位**：内核每轮广播 `chat:context`（已用 / 窗口 / 百分比），
  输入框内一条细水位条，≥80% 转警告色
- **`@` 引用文件**：输入框里 `@` 选文件，附件正文由后端现读拼进消息
  （上限 5 个 / 每个 64KB），读不到会明说而不是静默跳过
- **斜杠命令**：`/new` `/clear` `/model` `/permission`

### 调整

- 「默认模型」与「动手前规矩」两个切换入口从 chat-head 移进**输入框内部**，
  与输入区同处一屏，不再让人在两个地方找同一个设置
- `specs/` 扩到 14 篇（新增「用量与上下文水位」）；`docs/API-CONTRACT.md`
  补 `/stats`、`chat:context`、附件入参与 SSE 断线对账

## [0.3.0]

把 0.2.0 声称完成但实际没落地的部分补齐，并按审计结果统一文档与实现。

### 内核

- **单层流式循环**：撤掉外层 follow-up 循环与三钩子结构。插话与排队合并为一个队列，
  轮间注入；审批收敛成唯一的 `Gate` 回调
- **压缩改为确定性裁剪**：移除 LLM 摘要路径。`agent.Compact(msgs, Budget)` 在每次
  发送前按预算找 user 边界切点，数据库永远存完整历史，压缩不落库
- **重复调用防护**：同一工具同一参数第 3 次出现直接判失败，结果写回让模型换路，
  比撞轮数上限更容易排查
- 修复 `ContextWind` 拼写与死钩子 `AfterToolCall`

### 修复

- **「本会话内都放行」失效**：审批决策只把布尔值推回等待通道，scope 丢失，
  导致该分支永远走不到。改为回传决策对象
- **前端字体变量未定义**：`--font-mono` / `--font-display` / `--font-sans`
  三个变量在代码里用了 68 处却从未定义，等宽体与标题字体全站失效；
  `html, body` 也没声明 `font-family` / `font-size`
- **标题栏蓝字蓝底**：「WB」前景色与背景色同值，肉眼不可见
- **工具耗时永不显示**：`ToolRunRow` 的 prop 名与 store 字段的 snake_case 不匹配
- **上下文压缩无反馈**：`chat:compressed` 前端没接 handler
- **知识库一次只能加一个文件**：`OpenFileDialog` 缺多选参数
- **会话标题不会自动命名**：首条用户消息不再覆盖默认标题

### 规范对齐

- 删除 gotool 依赖（`knowledge` / `pkg` / `tool/web` 三处 import + go.mod），
  补上标准库实现
- 工具注册期即编译 JSON Schema，坏 Schema 在启动期就炸
- 工具输出统一双上限截断（2000 行 / 50KB），超出落临时文件并把路径告诉模型
- `read` 截断时给出可执行的续读提示（`offset=2001`）而不是只说「已截断」
- 新增 Unicode 路径规整：把不换行空格等规整成普通空格、剥掉前导 `@`，
  解决「从微信/Word 复制的路径明明存在却读不到」
- 写工具结果区分「新建」与「覆盖」；`edit` 保持原文件行尾与 BOM
- 技能启停状态落 settings KV（此前只改内存，重启即丢）
- 删死代码：`EventChatWarn`、`usageOf`、`AutoRecall`、`SettingDisabledSkills`、
  `EntryTypeModelChange`、`CountEntries`

### 前端

- **移除 Element Plus**：全量引入近 1MB 却一个组件都没用到，改用自建基元
- 新增 `AppIcon.vue`：全站图标统一为内联 SVG，替换 14 处 Unicode 字符与 4 处 Emoji
- 消息列与输入框共用 `max-width`，左右边缘对齐
- 消息操作栏常驻（不再 hover 才出现），补 `:focus-visible` 与自定义滚动条
- 设置页四面板改 `v-if` 懒挂载，不再一进设置页就发三个请求
- 模型服务面板接 `/providers/models` 拉模型下拉，不再要求用户手打模型 ID
- 审批卡渲染到触发它的回合旁，不再堆在消息流末尾
- 标题栏补连接状态灯；`chat:compressed` 给出「已整理上下文」提示
- 清理：`vitest.config.ts`（vitest 已不在依赖里）、`@types/dompurify`、
  `index.html` 残留的 Tailwind 类、`wb-ui.css` 外层 `@layer` 包裹

### 文档

- 补 `specs/13-prompt-and-settings.md`（系统提示构建 + 设置键位表）
- `docs/API-CONTRACT.md` 补 SSE 断线对账三规则，修正权限档为 `ask/auto_edit/yolo`
- `docs/` 修正数据目录为 `%APPDATA%`、内置 Python 查找顺序
- specs 全部对齐真实契约：单层循环、`Budget`、`Input` 字段、
  超时 120s、输出上限、分块 600/1200、技能三级来源
- `AGENTS.md` 写明「为什么砍掉外层循环」与「为什么砍掉 PrepareNextTurn」，
  并新增工具输出、重复调用、字体变量、图标纪律等强制条

## [0.2.0]

围绕「简洁高效、面向新手」做了一次整体重构：后端与前端全部重写。

- 后端从 460 个文件缩到 88 个：撤掉仪表盘、运行历史、命令面板、MCP、
  用户钩子、长期记忆、子智能体、后台任务、检查点续跑
- 前端从 141 个文件缩到 31 个：两个路由、四个设置板块、组件基元收口 `wb-ui.css`
- 设计令牌收口 `themes.css`（晴空 / 紫夜两套主题）
- 业务 API 全面转向 gin HTTP + SSE；Wails 绑定只剩系统能力
- 会话改为 append-only 条目链，分支 = 移动 `leaf_entry_id`

## [0.1.0]

首个可用版本：Windows 桌面个人 AI 助手，具备完整的 Agent 对话与本地干活能力。
