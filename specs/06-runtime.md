# 06 · 内置运行时（Python / PowerShell）

## 定位

只内置 Python 与 PowerShell 两种运行时：Python 覆盖办公场景（表格 / 文档 / 图片 / 爬虫），
PowerShell 是 Windows 自动化的第一入口（文件批处理 / 系统设置 / 进程管理）。
两者都以归档随产物分发在 `runtimes/`（Git LFS），解压到数据目录后优先于系统 PATH。
其他语言一律走系统 PATH。

## 查找顺序

```go
runtime.PythonExe(paths) :=
  EnsurePython(paths)      // ① 内置：装配期自动解压到数据目录（sync.Once 幂等）
| lookPath("python.exe")   // ② 系统 PATH
| lookPath("python")       // ③ 无扩展名（兼容异常环境）

runtime.PowerShellExe(paths) :=
  EnsurePowerShell(paths)              // ① 内置 pwsh 7.4.2（解压失败则跳过，不算致命）
| lookPath("pwsh.exe" / "pwsh")        // ② 系统 PowerShell 7+
| lookPath("powershell.exe" / "powershell") // ③ 系统 Windows PowerShell 5.1 兜底
```

Python 返回空串时，python 工具执行报 8002（内置未解压且系统也没有）；
PowerShell 返回空串的情形被 ③ 兜住，工具不会失败。
8011 / 8012 是 runtime 解压与探测层的错误码（解压失败 / 包缺失或 LFS 指针），
`/bootstrap` 的 `python_ready` 让前端能提前在设置页显示状态灯。

## 解压

内置运行时以 `python-<版本>-win-x64.tar.gz` 与 `PowerShell-<版本>-win-x64.zip`
随产物分发在 exe 同级的 `runtimes/`：

1. 目标目录里没有 `.version` 或版本不一致 → 解压
2. 解压逐条目校验路径，拒绝绝对路径与 `..`（8004）
3. 总体积上限 512MB，防压缩包炸弹
4. 写 `.version` 标记；命中版本就跳过解压，秒开
5. 归档若是 Git LFS 指针文本（未拉取二进制），不把它当包解开，按包缺失报错
   （Python 8002 / PowerShell 8012）

Python tar.gz 剥离顶层目录；PowerShell 官方 zip 本身平铺（pwsh.exe 在根），不剥层。

## 目录布局

```
<exeDir>/runtimes/python-3.12.13-win-x64.tar.gz   # 随产物分发
<exeDir>/runtimes/PowerShell-7.4.2-win-x64.zip
%APPDATA%/WorkBaby/runtime/python/                 # 解压后
├── python.exe
├── python312._pth
└── Lib/site-packages/
%APPDATA%/WorkBaby/runtime/powershell/
└── pwsh.exe
```

归档定位探测序与分发形态一致：exe 同级 `runtimes/`（打包版）
→ exe 上两级 `runtimes/`（仓库内直接跑）。

## powershell 工具

| 参数 | 说明 |
|---|---|
| command | 要执行的 PowerShell 命令 |
| cwd | 工作目录，相对路径按工作区解析（SafeJoin 校验），缺省为工作区根 |
| timeout_ms | 超时毫秒，默认 120000，上限 600000 |

- 内置 pwsh 优先（`tool.Deps.PowerShellExe` 注入），兜底系统 `powershell.exe`；
  `-NoProfile -NonInteractive` 对 pwsh 7 与 5.1 通用
- 子进程统一 `hideConsole`：GUI 进程拉起控制台程序时不设
  `CREATE_NO_WINDOW` 会每次闪黑框，工具调用频繁时不可接受
- 超时终止按进程树（taskkill /T），避免孙进程继续占用文件与端口
- `pip install` 不走 python 工具——需要装包时用本工具（需审批）

## 取舍

不做多版本管理、不做虚拟环境隔离：个人助手要的是「开箱能跑」，
确定性来自内置解释器本身，而不是环境管理。
PowerShell 只内置 pwsh 7.4.2 单版本：系统自带的 5.1 仅作兜底，不承诺行为一致。
