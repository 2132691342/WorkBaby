# 06 · 内置运行时（Python / PowerShell）

## 定位

只内置 Python 与 PowerShell 两种运行时：Python 覆盖办公场景（表格 / 文档 / 图片 / 爬虫），
PowerShell 是 Windows 自动化的第一入口（文件批处理 / 系统设置 / 进程管理）。
两者的归档都**内嵌在 exe 里**（`go:embed`），首次启动解压到数据目录后优先于系统 PATH；
用户机器上不需要任何随附文件。其他语言一律走系统 PATH。

## 归档

| 项 | 位置 / 约定 |
|---|---|
| 归档目录 | `backend/runtime/bundled/`（与代码同包，`//go:embed all:bundled`） |
| Python | `python-3.12.8-embed-amd64.zip`：Windows embeddable 发行包，平铺布局，不剥层 |
| PowerShell | `PowerShell-7.4.2-win-x64.zip`：win-x64 发行 zip，本身平铺（pwsh.exe 在根），不剥层 |
| 版本托管 | Git LFS（`.gitattributes`），仓库里必须存真文件 |
| 一致性 | 版本常量（`pythonVersion` / `powershellVersion`）与 `*ArchiveSHA256` 常量必须与归档同步；`TestBundledRuntimeChain` 守护三者一致 |

只认 zip：Windows 上 zip 是原生格式，两个运行时的上游发行包也都只发 zip，
少一次格式转换就少一处失败点。embeddable 包默认关掉 `site`（`python312._pth` 里
`#import site` 是注释），解压后必须打开它，否则装不了用户包。

升级归档的显式步骤：从对应版本的上游发布页下载新 zip 替换 `bundled/` 里的同名文件 →
改版本常量 → 算出新归档的 SHA-256 填进 `*ArchiveSHA256` 常量 →
跑 `go test ./backend/runtime/`。漏改版本常量时 `versionMatches` 会拿旧标记跳过解压，
用户机器上永远停在旧运行时，SHA 守护就是拦这个。归档没放进 `bundled/` 时测试跳过——
归档是可选资源，缺它只是「内置运行时不可用」，不该让整套测试变红。

## 查找顺序

```go
runtime.PythonExe(paths) :=
  EnsurePython(paths)      // ① 内置：装配期自动解压到数据目录（幂等，只解析一次）
| lookPath("python.exe")   // ② 系统 PATH
| lookPath("python")       // ③ 无扩展名（兼容异常环境）

runtime.PowerShellExe(paths) :=
  EnsurePowerShell(paths)              // ① 内置 pwsh 7.4.2（解压失败则跳过，不算致命）
| lookPath("pwsh.exe" / "pwsh")        // ② 系统 PowerShell 7+
| lookPath("powershell.exe" / "powershell") // ③ 系统 Windows PowerShell 5.1 兜底
```

Python 返回空串时，python 工具执行报错（内置未就绪且系统也没有）；
PowerShell 返回空串的情形被 ③ 兜住，工具不会失败。

错误码（runtime 解压与探测层）：

| 码 | 含义 |
|---|---|
| 7001 | 运行时包不可用：未嵌入 / 解压失败 / 解压后找不到可执行文件 |
| 7002 | 内嵌的是 Git LFS 指针文本（构建机没执行 `git lfs pull`），或没有可用的 Python |
| 7004 | 归档条目含不安全路径（绝对路径 / `..`），拒绝解压 |

状态查询与重新检测（设置页「内置运行时」区）：

- `GET /api/v1/runtime`：返回两套运行时的 `*_exe / *_source / *_version / *_error`；
  `source` 取 `bundled`（内置就绪）/ `system`（用系统已装的那份）/ 空（未就绪）
- `POST /api/v1/runtime/redetect`：清探测缓存后重跑一遍，并**把新路径写回
  `Env.ToolDeps`**——工具依赖是启动期冻结的快照，只清缓存不改它，
  会出现「设置页显示已就绪、跑脚本却报没有可用的 Python」
- 工具路径的探测结果有缓存（一次进程生命周期内只解析一次），缓存可被重新检测清空；
  `Status` / `StatusPowerShell` 走的是 `Ensure*`：首次调用会真解压（装配期即由它完成首装），
  已解压则按 `.version` 直接命中
- **缓存与解压闸门分离**：解压可能耗时数十秒，放在缓存锁里会让缓存命中的读路径
  一起排队——而那是每次工具调用都要走的路径。缓存命中只碰微秒级的锁，
  未命中时在独立闸门下用 double-check 保证只有一次真解压

## 解压

1. 目标目录里没有 `.version` 或版本不一致 → 解压；版本不一致时**先整目录清掉**——
   3.11 的 DLL 留在 3.12 目录里，用户会以为还在用旧版本
2. 解压逐条目校验路径，拒绝绝对路径与 `..`（7004）
3. 总体积上限 512MB，防压缩包炸弹
4. 打开 `site`、写 `.version` 标记；命中版本就跳过解压，秒开
5. 内嵌字节是 LFS 指针文本时按「包不可用」报错（7002），不把它当压缩包喂给解压器

测试侧的对应机制：装配类测试（`api` / `service`）用
`backend/runtime/runtimetest.SeedMarkers` 预置 `.version` 与可执行文件占位，
让上述第 1~4 步被跳过——装配链路照常全跑，只有上百 MB 的归档解压不重复做。
真解压只在 `runtime` 包的 `TestBundledRuntimeChain` 里做一次（SHA 校验 + 平铺布局）。

## 目录布局

```
backend/runtime/bundled/python-3.12.8-embed-amd64.zip   # go:embed 进 exe
backend/runtime/bundled/PowerShell-7.4.2-win-x64.zip

%APPDATA%/WorkBaby/runtime/python/                 # 解压后
├── python.exe
├── python312.dll
├── python312.zip（标准库）
├── python312._pth（已打开 import site）
└── *.pyd
%APPDATA%/WorkBaby/runtime/powershell/
└── pwsh.exe
```

## 构建与发布

- 归档经 LFS 托管；构建机必须先 `git lfs pull`，release.yml 在构建前校验归档不是指针文本
- 归档只存在于 exe 内部：没有「安装器额外拷一份 runtimes 目录」这种失败模式
- exe 因此明显变大（含两份归档）；发布流程用「体积明显偏小视为未嵌入」兜底

## powershell 工具

| 参数 | 说明 |
|---|---|
| command | 要执行的 PowerShell 命令 |
| cwd | 工作目录，相对路径按工作区解析（SafeJoin 校验），缺省为工作区根 |
| timeout_ms | 超时毫秒，默认 120000，上限 1800000 |

- 内置 pwsh 优先（`tool.Deps.PowerShellExe` 注入），兜底系统 `powershell.exe`；
  `-NoProfile -NonInteractive` 对 pwsh 7 与 5.1 通用
- 子进程统一 `hideConsole`：GUI 进程拉起控制台程序时不设
  `CREATE_NO_WINDOW` 会每次闪黑框，工具调用频繁时不可接受
- 超时终止按进程树（taskkill /T），避免孙进程继续占用文件与端口
- `pip install` 不走 python 工具——需要装包时用本工具（需审批）

## 取舍

不做多版本管理、不做虚拟环境隔离：个人助手要的是「开箱能跑」，
确定性来自内置解释器本身，而不是环境管理。
embeddable 包不带 pip：装第三方库走 powershell 工具显式执行（需审批），
不藏着掖着——新手需要看见「装了什么」。
内嵌带来 exe 体积与构建期对 LFS 的依赖，
换来的是分发只有一个文件、用户无需安装任何运行时。
PowerShell 只内置 pwsh 7.4.2 单版本：系统自带的 5.1 仅作兜底，不承诺行为一致。
