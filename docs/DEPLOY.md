# 打包与发布

## 产物形态

安装态是**一个目录**，不是单文件：

```
WorkBaby\
├── WorkBaby.exe          # Go 编译的壳（前端已 embed 进 exe）
├── assets\
│   ├── runtime\          # node / PortableGit / Python 三个 zip
│   └── skills\           # 内置技能包（22 个目录）
└── 卸载程序等 NSIS 产物
```

前端产物经 `//go:embed all:frontend/dist` 编进 exe，**不额外分发静态文件**。
运行时 zip 走 `assets\runtime\`，首启解压到数据目录——不预解压进安装包，
否则安装体积翻倍且用户重装才能升级运行时。

## 版本号

```bash
go build -ldflags "-X main.version=1.2.3" -o bin/WorkBaby.exe .
```

版本跟着 git tag 走；不注入时是 `dev`，本地 `go run` 出来的就是开发版，
不会误当成正式版。前端可从 `Endpoint().version` 读到它显示在关于页。

## 打包命令

```bash
task build      # 只产 exe，用于本机验证
task package    # NSIS 安装包，per-user 安装、可选安装目录
```

`task package` 内部走 `build/windows/Taskfile.yml`。安装器要点：

- **per-user 安装**，不要求管理员权限（数据目录本来就在 `%LOCALAPPDATA%`）
- 允许用户改安装目录
- **必须把 `assets/` 一起装进去**：`build/windows/nsis/project.nsi` 是手维护文件，
  上游模板只装 exe，不带 assets——少了运行时 zip 装完就是一堆不能跑的文件。
  该文件只能写 ASCII（NSIS 按 ANSI 代码页读脚本），详见 `build/windows/nsis/README.md`
- 卸载不删数据目录——用户的会话与配置在 `%LOCALAPPDATA%\WorkBaby`，
  要删由用户在设置页手动触发

产物体检：安装包应在 **120MB 上下**（= exe 32MB + assets 118MB 压缩后）。
只有十几 MB 说明 assets 没打进去，装完的机器上不会有任何运行时。

## CI

| 工作流 | 触发 | 做什么 |
|---|---|---|
| `.github/workflows/ci.yml` | push / PR → main | gofmt 检查 → `go vet` → `go test` → `npm ci` + 前端类型检查与产物 → `go build ./...` |
| `.github/workflows/release.yml` | push tag `v*` | 装 wails3 → 校验 tag 与 HEAD 一致 → `go test` → `wails3 package` → `gh release create` 上传安装包 |

CI 需要 `windows-latest`：Wails3 的 cgo 胶水与 WebView2 依赖只在 Windows 上编。

## 发布前自查

```bash
gofmt -l backend main.go    # 应无输出
go vet ./...                # 应无输出
go test ./... -count=1      # 全绿
task build                  # exe 产出并可启动
```

安装后验证清单：

1. 首次启动窗口按时出现（不黑屏、不假死）
2. 概览页进度条走完：运行时 → 技能 → 插件
3. 设置页诊断：node / git / python / pi 四个版本号都有值
4. 对话页发一句话能流式回来
5. 插件页能停用一个、再启用回来
6. 技能页能停用一个、再启用回来（`skills\` 与 `skills_disabled\` 之间搬移）
7. 关窗收托盘、托盘菜单能唤回
8. 卸载后 `%LOCALAPPDATA%\WorkBaby` 仍在（数据未丢）

## 局限

- 只支持 Windows x64
- 插件安装依赖 npm 源可达；离线机器只能用到随包的 pi 本体与技能
- 引擎能力以 pi 为准
