# 11 · PI 引导

## 目标

用户机器上可能什么都没装，也可能已经装好了一套 PI。
引导层的策略只有一条：**能复用就复用，缺了才装**。

## 环境闸门的三步

| 步骤 | 判定 | 缺失时的动作 |
|---|---|---|
| Node | 系统 `node` ≥ 22.19.0（PI 1.4 的 engines）→ 内置 | 直接用内置（首次用时解压） |
| PI | `pi` 在 PATH 或 npm 全局前缀里 | 用内置 npm 全局装到应用目录 |
| 模型 | `models.json` 里有带模型与密钥的服务 | 引导去设置页填一个 |

闸门按上述顺序报**首个**阻塞项并给原因。前端 `bootstrap.pi_env.ready = false` 时强制进向导。

**下界是完整版本号而不是主版本**：22.15 这类「看着够」的版本会让 PI 1.4 的依赖
一路 `EBADENGINE`，所以系统 Node 不够时选内置那份，而不是将就着用。

## 装 PI

```
<内置 node> <内置 npm-cli.js> install -g --ignore-scripts --no-fund --no-audit
  --fetch-retries=3 --fetch-timeout=120000 --prefix <prefix> --registry <源> @earendil-works/pi-coding-agent
```

| 规则 | 原因 |
|---|---|
| npm 只认内置那份（`runtime.EnsureNode` + `runtime.NpmCmd`） | 系统 node 可能低于 PI 的下界；装出来的包最终也在这份解释器上跑，两者必须一致。内置 Node 解压失败就直接报 9707，不退到系统 npm |
| 前缀显式给出（首次装 = 应用目录） | 系统前缀通常要管理员权限，写失败时用户完全看不懂错误 |
| npm 起进程用 `node + npm-cli.js`，且 node 用绝对路径 | `exec.Command` 只按本进程的 PATH 找可执行文件；内置 node 不在 PATH 上时，字面量 `node` 就是一句 `executable file not found` |
| 内置 npm 的缓存落在数据目录 | 装失败不牵走用户的全局缓存，重试也干净 |
| 输出按行回传（`chat:setup`） | npm 可能跑几十秒，没有进度界面会被当成卡死 |
| 失败把最后一行输出带进错误 | 界面只剩「安装失败」时用户无从下手 |
| `--ignore-scripts` | 不在安装期执行三方脚本 |

**源**：官方源（`registry.npmjs.org`）与国内镜像（`registry.npmmirror.com`）并联探测
「PI 的最新版本元数据」，先返回的那个就是当前网络下更快的那个，结果缓存 10 分钟。
一趟装失败就换另一个源整跑一遍，并把两趟的输出都打进进度流。
`WORKBABY_NPM_REGISTRY` 可以钉死源，跳过探测。

装完再探一次：找不到 `pi` 就报「装好像成功了但找不到命令」，并提示检查前缀可写。
失败原因同时记在引擎服务里，向导下一轮轮询就能看到 `pi_env.install_error`。

装完顺手办两件事（`EngineService.prepareRuntime`）：修命令执行环境、生成终端入口。见下两节。

## 命令执行环境

PI 的 `bash` 工具在 Windows 上**只认 bash**：`shellPath` → `%ProgramFiles%\Git\bin\bash.exe` →
PATH 上的 `bash.exe` → 报错。问题在于 `C:\Windows\System32\bash.exe` 是 WSL 启动器，
**存在且可执行，但没装发行版时每条命令都失败**（`execvpe(/bin/bash) failed`），
而 Git 装在自定义目录（`D:\...\Git`）时 PI 又找不到。

WorkBaby 在启动 / 装完 / 手动修复时跑一次自愈（`pi/env/shell.go`）：

| 步骤 | 判定 |
|---|---|
| 用户指定的 `shellPath` | 真跑 `bash -c "echo …"` 验证 |
| Git / MSYS2 / Cygwin | 从 `ProgramFiles`、注册表 `SOFTWARE\GitForWindows`、PATH 上的 `git.exe` 反推安装根 |
| WSL 启动器 | 按路径直接剔除（`%SystemRoot%\System32\bash.exe`、`WindowsApps\bash.exe`），不参与试跑 |
| 兜底 | 系统 PowerShell（pwsh 7 优先） |

落盘只改自己该改的键：

- 找到真 bash → 写 `shellPath`（PI 自己找得到的那份不写）；撤回之前塞的 `+powershell` / `-bash`
- 只有 PowerShell → 往 `defaultTools` 追加 `+powershell`、`-bash`（增量语义，不动用户自己的清单）
- 自己写过的值记在 `<data>/runtime/shell.json`：**只有自己写的才敢撤回**，用户手写的配置不碰

## 终端入口

应用目录里的 `pi` 不在用户 PATH 上，用户开个终端敲 `pi` 会得到「不是内部或外部命令」。
`EnsureTerminal` 做两件事：

| 动作 | 细节 |
|---|---|
| 写 `<data>/bin/pi.cmd` | 绝对路径的**内置 node** + PI 的 `cli.js`；不依赖 PATH 上的 node，也不依赖 npm 生成的 shim |
| 把 `<data>/bin` 追加到 **HKCU** `Environment\Path` | 只改当前用户，不动系统级 PATH；保持原值类型（REG_SZ / REG_EXPAND_SZ）；写后广播 `WM_SETTINGCHANGE` |

启动时只在本应用自己装的那份（`OwnsInstall`）上自动挂 PATH；系统里那份归用户自己管。
新开的终端立即生效，已经开着的终端要重开——这一点在界面提示里写清楚。

## 更新

`POST /engine/update`（别名 `POST /env/update`）升级 PI 自身：

| 情况 | 动作 |
|---|---|
| WorkBaby 自己装的那份 | `npm install -g --prefix <应用目录> @latest`，不另起一份 |
| 系统里那份 | 从包目录反推前缀（`…/node_modules/<pkg>` 的父目录）后直装，装回原处 |
| 直装失败 | 兜底交给 PI 自己的 `pi update` |

升级完重启全部 PI 进程并重扫技能：配置与扩展都是启动期读的。

## 版本对账

| 读的是什么 | 来源 |
|---|---|
| 本地版本 | 包目录里 `package.json` 的 `version`（`pi --version` 只在探测入口时跑一次） |
| 最新版本 | npm 注册表的 `dist-tags.latest`（abbreviated 元数据，缓存 5 分钟） |

查询超时 12 秒、失败只说明「查不到」：网络断了面板仍要把本地状态说清楚。
查询与装包走同一套源选择：当前源查不到就换另一个源再问一次。

## 首次启动留痕

```
startup: WorkBaby 1.4.0 启动（数据目录 <data>，PI 配置目录 <agent>）
startup: 内置 Node 不可用（将回退系统 node）：<原因>
startup: 内置 Python 不可用（将回退系统 python）：<原因>
```

内置运行时解压失败只告警不阻断：系统里那份可能好使。但原因必须写清楚——
只说「不可用」，用户无从判断是包没嵌进来、解压失败还是被安全软件拦了。

## 取舍

- **复用系统安装**：不重复下载 40MB+、终端与界面行为一致；代价是版本差异要靠闸门挡住。
- **应用目录兜底安装**：免管理员权限；代价是要额外写一个终端入口（`<data>/bin` + HKCU PATH），
  并且用户得重开终端才看得见。
- **装包强制走内置 Node**：解释器版本说得准；代价是内置解压失败时装不了（此时报 9707 而不是将就）。
- **双源并联探测 + 失败换源**：无代理网络下也能装上；代价是每次装包多一次探测（约 0.3~4 秒）。
- **只改自己写过的配置**：`shellPath` / `defaultTools` 的撤回有据可查；代价是要多维护一份自愈记录。
- **不预置模型配置**：密钥是用户的东西；代价是首次启动必须自己填一次（向导兜住）。
- **升级走 npm 直装而不是只靠 `pi update`**：前缀说得准，代价是要自己维护这份参数。
