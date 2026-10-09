# 11 · 桌面壳

## 定位

Wails 只负责「窗口 + 系统能力」，业务全部走内嵌 gin。
这个分工让前端在浏览器里也能开发调试。

## 窗口

- Frameless + 前端自绘标题栏（`--wails-draggable: drag` 声明拖拽区）
- 最小 960×640，默认 1280×800
- 关闭按钮（X / Alt+F4）走 `Quit → OnBeforeClose` 判定，去向由「关闭到托盘」设置决定

前端关闭按钮必须调 `WindowHide` 之外的 `Quit()`：`WindowHide` 只隐藏窗口，
**从不触发 `OnBeforeClose`**，等于把「关闭」一律变成收托盘，用户关掉驻留设置也退不掉。

### OnStartup 与 OnDomReady 没有顺序保证

Wails 把 `OnStartup` 放在**独立 goroutine** 里跑，`OnDomReady` 由 WebView2 的
导航回调触发。首次启动要解压内置 Python（约两秒），`domReady` 必然先到。

因此**不能**用「`Svc` 是不是 nil」判断启动失败——慢启动会被误判成失败。
正确做法是显式握手：

```
startup : defer close(startDone) → 成功置 startOK
domReady: <-startDone（上限 45s）→ 按 startOK 决定起服务还是报启动失败
```

### 关闭与退出的区别（必须显式区分）

Wails 的 `Quit()` 会**先调 `OnBeforeClose`**，返回 true 就直接 return。
所以「永远拦截」等于「退出按钮无效」：窗口没了、托盘没了，进程还活着，
还占着 exe，用户只能去任务管理器结束它。

| 入口 | 条件 | `OnBeforeClose` | 结果 |
|---|---|---|---|
| 窗口 X / Alt+F4 | 托盘驻留开、托盘已就绪、不在退出流程 | 提示后延迟隐藏并返回 true | 收进托盘 |
| 窗口 X / Alt+F4 | 其余情况（驻留关 / 托盘未就绪 / 正在退出） | 返回 false | 真正结束进程 |
| 托盘「退出」 | 先置 `quitting=true` | 返回 false | 真正结束进程 |

收进托盘前先推 `app:to-tray` 让界面提示一句，1.1 秒后再隐藏：
窗口「凭空消失」最让用户困惑。托盘没就绪时不拦截——藏进一个还不存在的托盘，
用户再也找不回来。

`shutdown` 清理完资源后直接 `os.Exit(0)`：WebView2 偶尔不返回 `Close`，
事件循环会永远等下去。另有 8 秒看门狗兜底这条路径不生效的情况。

## 生命周期

| 钩子 | 职责 |
|---|---|
| OnStartup | `api.Handler.Startup`：路径 → 日志 → 配置 → DB → 工具 → 技能 → 服务装配 |
| OnDomReady | 启动 gin（回环随机端口）→ 广播 `app:ready` → 拉起托盘 |
| OnBeforeClose | 按「关闭到托盘」设置收托盘或放行（见上） |
| OnShutdown | 停 HTTP → 退托盘并等它摘掉图标 → 关数据库 → 放实例锁 → 结束进程 |

**启动失败必须让用户看见**：`Startup` 失败时 `Svc` 为 nil，
`OnDomReady` 直接返回，界面空着且每个接口都 404，而真实原因只在日志里。
因此失败时记录原因并推 `app:startup-error` 给前端。

## Wails 绑定面（仅系统能力）

`ForceQuit` / `OpenFileDialog` / `OpenDirectoryDialog` / `AutoStartEnabled` / `SetAutoStart`。

- `App` 持有 `*api.Handler` 而不是嵌入它：嵌入会把几十个 gin handler 一起绑成
  JS 方法（签名里带 `gin.Context`，永远调不通），绑定面就废了
- 窗口控制与剪贴板由前端直接调 wails runtime 包（`wailsjs/runtime`），
  不进绑定面；复制按钮用 `navigator.clipboard`
- `backend/api` 是唯一允许 import wails 的包

## 托盘

`backend/tray` 独立成包（无窗口平台下是空实现）：打开窗口 / 新建对话 / 退出。

- 消息循环锁定 OS 线程：托盘窗口的创建与 `GetMessage` 必须在同一线程，
  否则菜单点击与退出回调都收不到消息
- 「打开窗口 / 新建对话」都 `WindowShow + WindowUnminimise`——
  隐藏与最小化是两种状态，只做一半就唤不回
- 菜单回调统一过 `safeCall`：托盘是进程级组件，回调里的一个 panic 会连同正在跑的
  对话一起带走整进程；兜住后只留下日志与堆栈
- 退出时 `Stop` 后 `WaitForExit(300ms)`：等消息循环把图标从通知区摘掉，
  进程抢跑会留下一个只能靠鼠标划过才消失的「幽灵图标」

## 开机自启

`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`，值是当前 exe 的绝对路径。
走 HKCU 而非 HKLM：不需要管理员权限，也不会在多用户机器上互相干扰。
设置页有开关；卸载器会清掉这个值，否则卸载后开机还会弹窗。

## 单实例

- 文件锁（`app.lock` 首字节独占）判断是否已有实例
- 主实例启动后监听回环随机端口，把端口写进 `ipc.port`
- 二次启动一律转交：读 `ipc.port` → TCP 发一行——带文件就是文件路径，
  不带就是空行（只想唤起窗口，否则双击图标像没反应）→ 主实例
  `WindowShow + WindowUnminimise`；带文件时再广播 `app:open-file` 给前端
- 转交到达时主实例可能还没装配完：在 `startDone` 上等（上限 45s）；
  装配没完成不碰 Wails 运行时——空 ctx 调它会直接终止进程
- **首次启动**（没有主实例可转交）时命令行里的文件路径由自己接手，
  等前端就绪后再发 `app:open-file`
- 主实例收不到握手（端口文件过期 / 进程已死）→ 当作首次启动正常继续
- 支持文件关联（md / txt 双击用 WorkBaby 打开）
- IPC 读取有 5 秒 deadline：对端发一半就卡住时，没有上限会把这条连接与 goroutine 永久挂着

## 桌面壳意图的落地

托盘「新建对话」与「外部打开文件」都先落到壳意图（`composables/shellIntent.ts`），
再切回对话页广播消费：用户当时可能在设置页，事件早于视图挂载就会丢。
对话页挂载时先消费一次存下的意图，之后靠事件即时响应；
挂进输入区走 `ChatInput.attachPath`，与选文件共用工作目录内外判定。

## 安装与覆盖

`build/windows/installer/project.nsi` 是**唯一可持久化定制**的安装器脚本；
同目录的 `wails_tools.nsh` 每次 `wails build` 都会从模板重新生成。

| 事项 | 做法 |
|---|---|
| 覆盖安装 | 写 exe 前先 `ExecWait 'taskkill /F /IM WorkBaby.exe /T' $0`——托盘常驻时 exe 仍被占用；必须用 `ExecWait`，`Exec` 不写退出码，判断等于没判；杀掉后等 1.5 秒释放文件句柄 |
| 旧数据 | 检测到 `%APPDATA%\WorkBaby\workbaby.db` 时问一句要不要在装前清空；默认保留，静默安装（`/S`）从不删 |
| 卸载数据 | 问一句是否删除本机数据（对话记录 / 配置含 API Key / 知识库 / 技能 / 日志 / 运行时）；默认保留，静默卸载同样不删 |
| 文件关联 | 模板的 `wails.associateFiles` 是空宏，直接在 `project.nsi` 里调 `APP_ASSOCIATE` |
| 脚本编码 | `project.nsi` 必须带 UTF-8 BOM，否则中文注释与字符串会被按系统代码页解析而报错 |
| 插件 | 该打包环境不带任何 NSIS 插件，只能用 `ExecWait` / `taskkill` 这类内置能力 |
| 本地自检 | 改完 `project.nsi` 先 `makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\WorkBaby.exe project.nsi` 编一遍再发 |

### WebView2 数据目录

`windows.Options.WebviewUserDataPath` 指向 `<数据目录>\webview`。Wails 默认把它放在
`%APPDATA%\<exe 名>`——一个名字像可执行文件、内容是几十 MB 浏览器缓存的目录：
用户会往杀软误报上想，卸载时也容易漏清。挪进数据目录后，卸载选「删数据」就是一次清干净。

## 事件注入

| Wails 事件 | 语义 |
|---|---|
| app:ready | `{server_port}`：前端据此设置 axios baseURL 与 SSE 地址 |
| app:startup-error | `{message}`：装配失败，前端显示错误屏 |
| app:to-tray | 正在收进托盘：前端先提示一句，1.1 秒后窗口才隐藏 |
| app:new-session | 托盘「新建对话」：切回对话页并新建 |
| app:open-file | 文件关联打开：切回对话页并把文件挂进输入区 |

## 取舍

- **绑定面只留 5 个系统方法**：业务接口全走 HTTP，前端在浏览器里就能跑通界面（开发不必开窗）。
  代价是启动顺序变成硬约束——端口没起来之前，界面所有请求都会 404，所以握手必须早于 `domReady`。
- **窗口控制不进绑定面**：前端直接调 wails runtime，少一层转发；
  代价是前端代码同时依赖 `wailsjs/runtime` 与 HTTP 两套通道，调试时要知道在哪一侧。
- **单实例靠文件锁 + 本地 TCP**：不引系统级互斥体，跨平台逻辑一致；代价是多一个端口文件的状态机
  （过期、对端已死、发一半卡住）都要自己兜。
- **托盘与主线程绑定**：菜单与退出必须走同一 OS 线程；代价是托盘相关代码不能随意丢进 goroutine。
