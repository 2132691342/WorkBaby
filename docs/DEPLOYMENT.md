# 构建与分发

## 产物

```
build/bin/
├── WorkBaby.exe                                   # 主程序（前端已 embed，单文件可跑）
└── runtimes/python-<版本>-win-x64.tar.gz          # 内置 Python（可选，见下）
```

## 构建

```powershell
wails build
powershell -File scripts/copy-runtimes.ps1 -Bin build/bin/WorkBaby.exe
```

## 内置 Python（可选增强）

1. 把 embeddable Python 3.12 打成 `runtimes/python-3.12.13-win-x64.tar.gz`，
   放在 exe 同级的 `runtimes/` 下
2. `scripts/copy-runtimes.ps1` 负责把仓库 `runtimes/` 下的压缩包与 manifest 拷到产物旁
3. 首次调用 `python` 工具时自动解压到数据目录并写 `.version` 标记，之后秒开

没有内置运行时时应用照常运行：`python` 工具会返回 8002 明确错误，
设置页的状态灯显示未就绪，其余功能不受影响。
系统 PATH 上的 Python 会被自动采用。

## 用户数据

首启自动创建 `%APPDATA%/WorkBaby/`（DB / 配置 / 日志 / 运行时 / 技能目录）。
卸载只需删 exe 与该目录。

## 分发注意

- 单实例锁 + 本地 TCP IPC：二次启动会唤起已有窗口并转交文件路径
- 端口随机绑定 127.0.0.1，无外网暴露面
- API Key 以 AES-256-GCM 加密存库，MasterKey 在 config.yaml（首启生成）
- 生成的 Wails 绑定会把 `*api.Handler` 上的方法一并导出，
  真正的系统能力只有 `internal/api/system_windows.go` 里的那几个
