# 构建与分发

## 产物

```
build/bin/
├── WorkBaby.exe                                    # 主程序（前端已 embed，单文件可跑）
└── runtimes/
    ├── python-<版本>-win-x64.tar.gz                # 内置 Python（可选，见下）
    ├── PowerShell-<版本>-win-x64.zip               # 内置 PowerShell 7（可选）
    └── manifest.json                               # 归档清单（版本 / sha256 / 解压上限）
```

## 构建

```powershell
wails build
powershell -File scripts/copy-runtimes.ps1 -Bin build/bin/WorkBaby.exe
```

`copy-runtimes.ps1` 把仓库 `runtimes/` 下的归档拷到**两处**：
产物旁 `build/bin/runtimes/`（绿色版直接跑）与 NSIS 安装器源目录
`build/windows/runtimes/`（`installer/project.nsi` 引用）。

## 内置运行时（可选增强）

1. 仓库 `runtimes/` 下放 `python-3.12.13-win-x64.tar.gz`（embeddable Python 打包，
   Git LFS）与 `PowerShell-7.4.2-win-x64.zip`（官方 win-x64 zip）
2. 启动装配期（`Handler.Startup` → `NewToolDeps`）自动解压到数据目录并写
   `.version` 标记，命中版本跳过解压；网络/文件系统问题不影响启动
3. 解压校验：逐条目路径检查（拒绝绝对路径与 `..`）、512MB 解压上限、
   LFS 指针识别（未拉取的归档按缺失报错而不是当压缩包喂给解压器）

没有内置 Python 时应用照常运行：`python` 工具返回 8002 明确错误，
设置页状态灯显示未就绪，系统 PATH 上的 Python 会被自动采用。
内置 PowerShell 缺失时静默兜底系统 `powershell.exe`（win5.1 或 7），工具不断档。

## 用户数据

首启自动创建 `%APPDATA%/WorkBaby/`（DB / 配置 / 日志 / 运行时 / 技能目录）。
卸载只需删 exe 与该目录。

## 分发注意

- 单实例锁 + 本地 TCP IPC：二次启动会唤起已有窗口并转交文件路径
- 端口随机绑定 127.0.0.1，无外网暴露面
- API Key 以 AES-256-GCM 加密存库，MasterKey 在 config.yaml（首启生成）
- 生成的 Wails 绑定会把 `*api.Handler` 上的方法一并导出，
  真正的系统能力只有 `internal/api/system_windows.go` 里的那几个
