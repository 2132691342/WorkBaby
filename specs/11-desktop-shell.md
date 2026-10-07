# 11 · 桌面壳

## 定位

Wails 只负责「窗口 + 系统能力」，业务全部走内嵌 gin。
这个分工让前端在浏览器里也能开发调试。

## 窗口

- Frameless + 前端自绘标题栏（`--wails-draggable: drag` 声明拖拽区）
- 最小 960×640，默认 1280×800
- 关闭按钮 = 隐藏到托盘（`OnBeforeClose` 拦截），托盘菜单里才有「退出」

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

| 入口 | 标志位 | `OnBeforeClose` | 结果 |
|---|---|---|---|
| 窗口 X / Alt+F4 / 标题栏关闭 | `quitting=false` | 隐藏窗口并返回 true | 收进托盘 |
| 托盘「退出」 | 先置 `quitting=true` | 返回 false | 真正结束进程 |

`shutdown` 清理完资源后直接 `os.Exit(0)`：WebView2 偶尔不返回 `Close`，
事件循环会永远等下去。另有 8 秒看门狗兜底这条路径不生效的情况。

## 生命周期

| 钩子 | 职责 |
|---|---|
| OnStartup | `api.Handler.Startup`：路径 → 日志 → 配置 → DB → 工具 → 技能 → 服务装配 |
| OnDomReady | 启动 gin（回环随机端口）→ 广播 `app:ready` → 拉起托盘 |
| OnBeforeClose | 隐藏窗口（关闭到托盘）；退出流程中放行 |
| OnShutdown | 停 HTTP → 退托盘 → 关数据库 → 放实例锁 → 结束进程 |

**启动失败必须让用户看见**：`Startup` 失败时 `Svc` 为 nil，
`OnDomReady` 直接返回，界面空着且每个接口都 404，而真实原因只在日志里。
因此失败时记录原因并推 `app:startup-error` 给前端。

## Wails 绑定面（仅系统能力）

OpenFileDialog / OpenDirectoryDialog / ClipboardGetText / ClipboardSetText /
WindowMinimise / WindowToggleMaximise / WindowHide / WindowShow /
AutoStartEnabled / SetAutoStart

`internal/api` 是唯一允许 import wails 的包。

## 托盘

`getlantern/systray`：打开窗口 / 新建对话 / 退出。

## 开机自启

`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`，值是当前 exe 的绝对路径。
走 HKCU 而非 HKLM：不需要管理员权限，也不会在多用户机器上互相干扰。
设置页有开关；卸载器会清掉这个值，否则卸载后开机还会弹窗。

## 单实例

- 文件锁（`app.lock` 首字节独占）判断是否已有实例
- 主实例启动后监听回环随机端口，把端口写进 `ipc.port`
- 二次启动：读 `ipc.port` → TCP 发一行文件路径 → 主实例收下后唤起窗口，
  并广播 `app:open-file` 给前端
- **首次启动**（没有主实例可转交）时命令行里的文件路径由自己接手，
  等前端就绪后再发 `app:open-file`
- 主实例收不到握手（端口文件过期 / 进程已死）→ 当作首次启动正常继续
- 支持文件关联（md / txt 双击用 WorkBaby 打开）

## 安装与覆盖

`build/windows/installer/project.nsi` 是**唯一可持久化定制**的安装器脚本；
同目录的 `wails_tools.nsh` 每次 `wails build` 都会从模板重新生成。

| 事项 | 做法 |
|---|---|
| 覆盖安装 | 写 exe 前先 `taskkill /F /IM WorkBaby.exe /T`，杀到了才提示；托盘常驻时 exe 仍被占用 |
| 文件关联 | 模板的 `wails.associateFiles` 是空宏，直接在 `project.nsi` 里调 `APP_ASSOCIATE` |
| 脚本编码 | `project.nsi` 必须带 UTF-8 BOM，否则中文注释与字符串会被按系统代码页解析而报错 |
| 插件 | 该打包环境不带任何 NSIS 插件，只能用 `Exec` / `taskkill` 这类内置能力 |

## 事件注入

| Wails 事件 | 语义 |
|---|---|
| app:ready | `{server_port}`：前端据此设置 axios baseURL 与 SSE 地址 |
| app:new-session | 托盘「新建对话」 |
| app:open-file | 文件关联打开 |
