# 01 · 内置运行时（Node / Python）

## 定位

内置两种运行时，各有明确用途：

| 运行时 | 用途 | 谁用它 |
|---|---|---|
| Node 24.21.0 | 跑 PI（PI 是 Node 程序，要求 ≥ 22.19） | WorkBaby 本体 + 助手的脚本 |
| Python 3.12.8 | 办公场景（表格 / 文档 / 图片 / 爬虫） | 助手调用的脚本 |

两份归档都**内嵌在 exe 里**（`go:embed`），首次启动解压到数据目录，
用户机器上不需要任何随附文件。**不内置 PowerShell**：Windows 自带
Windows PowerShell 5.1，助手要跑命令时走系统 `powershell.exe` 即可，
再嵌一份 110MB 的 pwsh 只让安装包变大，换不来新能力。其他语言一律走系统 PATH。

## 归档

| 项 | 约定 |
|---|---|
| 归档目录 | `backend/runtime/bundled/`（与代码同包，`//go:embed all:bundled`） |
| Node | `node-v24.21.0-win-x64.zip`：上游发行 zip，**内含单层目录，解压时必须平铺到根** |
| Python | `python-3.12.8-embed-amd64.zip`：embedded 发行包，平铺，不剥层 |
| 版本托管 | Git LFS（`.gitattributes`），仓库里必须存真文件 |
| 一致性 | 版本常量与 `*ArchiveSHA256` 常量必须与归档同步；`TestBundledRuntimeChain` 守护 |

**只有 Node 需要剥一层**。上游 zip 的顶层目录名随版本号变（`node-v24.21.0-win-x64/`），
不剥层的话 `node.exe` 多一层路径，`npm.cmd` 内部的相对路径会指错，
表现为「npm 装好了但 PI 起不来」。Python 的归档本身平铺，剥层反倒会拆坏。

只认 zip：Windows 上 zip 是原生格式，两份上游发行包也都只发 zip。

Python 的 embeddable 包默认关掉 `site`（`python312._pth` 里 `#import site` 是注释），
解压后必须打开它，否则装不了用户包。

升级归档的步骤：下载新版 zip 替换同名文件 → 改版本常量 → 算 SHA-256 填进
`*ArchiveSHA256` → 跑 `go test ./backend/runtime/`。漏改版本常量时
`versionMatches` 会拿旧标记跳过解压，用户永远停在旧运行时，SHA 守护就是拦这个。

## 查找顺序

```go
runtime.NodeExe(paths) :=
  EnsureNode(paths)        // ① 内置：装配期自动解压（幂等，只解析一次）
| lookPath("node.exe")     // ② 系统 PATH

runtime.PythonExe(paths) :=
  EnsurePython(paths)      // ① 内置
| lookPath("python.exe")   // ② 系统 PATH
```

**Node 与 Python 的优先序不同，这是刻意的**：

- Node 内置优先：PI 对 Node 版本有下界且行为随小版本变，用用户 PATH 上某个 18.x
  会让错误出现在协议层而不是启动检查里
- Python 系统优先：用户自己装的 python 有他装好的包，用内置那份反而 import 不到

错误码：

| 码 | 含义 |
|---|---|
| 7001 | 运行时包不可用：未嵌入 / 解压失败 / 解压后找不到可执行文件 |
| 7002 | 内嵌的是 Git LFS 指针文本（构建机没执行 `git lfs pull`） |
| 7004 | 归档条目含不安全路径（绝对路径 / `..`），拒绝解压 |

状态查询（设置页「内置运行时」区）：

- `GET /api/v1/runtime`：Node / Python 与 PI 的 `*_exe / *_source / *_version / *_error`；
  `source` 取 `bundled` / `system` / 空
- `POST /api/v1/runtime/redetect`：清探测缓存后重跑，并把新路径写回 PI 进程的环境。
  不写回去的话会出现「设置页显示已就绪、助手跑脚本却报找不到」
- 探测结果有缓存（一次进程生命周期只解析一次），缓存可被重新检测清空
- **缓存与解压闸门分离**：解压可能耗时数十秒，放在缓存锁里会让缓存命中的读路径
  一起排队。缓存命中只碰微秒级锁，未命中时在独立闸门下 double-check

## 解压

1. 目标目录没有 `.version` 或版本不一致 → 解压；版本不一致时**先整目录清掉**——
   旧版本的 DLL 留在新目录里，用户会以为还在用旧版本
2. Node 剥掉归档的顶层单目录，其余逐条目原样落盘
3. 逐条目校验路径，拒绝绝对路径与 `..`（7004）
4. 总体积上限 512MB，防压缩包炸弹
5. Python 打开 `site`；写 `.version` 标记；命中版本跳过解压
6. 内嵌字节是 LFS 指针文本时按「包不可用」报错（7002），不喂给解压器

测试侧：装配类测试用 `backend/runtime/runtimetest.SeedMarkers` 预置 `.version` 与
可执行文件占位，跳过上述第 1~5 步。真解压只在 `runtime` 包的 `TestBundledRuntimeChain`
里做一次（`-short` 跳过）。

## 目录布局

```
backend/runtime/bundled/node-v24.21.0-win-x64.zip     # go:embed 进 exe
backend/runtime/bundled/python-3.12.8-embed-amd64.zip

%APPDATA%/WorkBaby/runtime/node/
├── node.exe
├── npm.cmd
└── node_modules/npm/
%APPDATA%/WorkBaby/runtime/npm-global/            # npm 全局前缀（可写，装 PI 用）
└── node_modules/@earendil-works/pi-coding-agent/
%APPDATA%/WorkBaby/runtime/python/
├── python.exe
├── python312._pth（已打开 import site）
└── ...
```

`npm-global` 单独一层是必要的：系统 npm 全局前缀通常需要管理员权限，
写失败时 npm 报的是权限错，用户完全看不懂。指向应用目录就没有这个问题。

## PATH 注入

PI 进程与它派生的所有工具共享一份注入后的环境：

```text
PATH = <runtime>/node ; <runtime>/npm-global ; <runtime>/python ; <原 PATH>
```

三件事同时成立：PI 自己用内置 node 跑；PI 的 shell 工具里 `node` / `npm` /
`python` 都能直接调；助手写的脚本用的是内置那份解释器。

**必须前置拼接**。后置拼接会被用户 PATH 上的旧版本抢先，内置运行时等于没生效。

## 取舍

- exe 因此明显变大（两份归档，Node 一份就 36MB）。换来的是「双击即用」。
- 不做多版本管理、不做虚拟环境隔离：个人助手要的是开箱能跑，
  确定性来自内置解释器本身，而不是环境管理。
- pip / npm 的包安装走内置运行时自己的前缀，不碰用户全局环境。
- 不内置 PowerShell：系统 5.1 兜底够用，且它本来就在 Windows 上。
- 内置 node 优先意味着用户全局升级 node 不改变 WorkBaby 行为（这既好也坏：
  可预测，但用户以为的升级没有生效）。
