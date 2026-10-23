# 11 · PI 引导

## 目标

用户机器上可能什么都没装，也可能已经装好了一套 PI。
引导层的策略只有一条：**用户有的先用，缺了才装**。

## 环境闸门的三步

| 步骤 | 判定 | 缺失时的动作 |
|---|---|---|
| Node | 系统 `node` ≥ 22.19.0（PI 1.4 的 engines）→ 内置 | 直接用内置（首次用时解压） |
| PI | 用户 PATH → 本应用自装前缀 → 其它 npm 全局前缀 | 装一份（npm 跟随当前生效 node） |
| 模型 | `models.json` 里有带模型与密钥的服务 | 引导去设置页填一个 |

闸门按上述顺序报**首个**阻塞项并给原因。前端 `bootstrap.pi_env.ready = false` 时强制进向导。

**下界是完整版本号而不是主版本**：22.15 这类「看着够」的版本会让 PI 1.4 的依赖
一路 `EBADENGINE`，所以系统 Node 不够时选内置那份，而不是将就着用。

**用户 PATH 上的 pi 优先于自装前缀**：用户自己装过 PI 就复用它，WorkBaby 绝不装出
第二份。PATH 扫描按目录顺序进行，本应用生成的终端转发目录（`<data>/bin`）要跳过——
它只是指向当前生效 PI 的入口，不是一份安装。

## 装 PI

```
<当前生效 node> <npm-cli.js> install -g --ignore-scripts --no-fund --no-audit
  --fetch-retries=3 --fetch-timeout=120000 --prefix <runtime/npm-global> --registry <源> @earendil-works/pi-coding-agent
```

| 规则 | 原因 |
|---|---|
| npm 跟随当前生效 node（`npmFor`）：用户 node 合规就用它旁边的 `npm-cli.js`，没有才解压内置 | 用户的 node 能用就不该白解压一份内置；装出来的包要在这份解释器上跑，npm 与 node 必须同源。两处都没有才报 9707 |
| 装到应用目录前缀 `runtime/npm-global`，不改用户 npm 前缀 | 单一权威副本：nvm/fnm 切版本不会让 pi 凭空消失或出现多份；写系统前缀通常还要管理员权限 |
| npm 起进程用 `node + npm-cli.js`，且 node 用绝对路径 | `exec.Command` 只按本进程的 PATH 找可执行文件；内置 node 不在 PATH 上时，字面量 `node` 就是一句 `executable file not found` |
| npm 缓存落在数据目录 | 装失败不牵走用户的全局缓存，重试也干净 |
| 输出按行回传（`chat:setup`） | npm 可能跑几十秒，没有进度界面会被当成卡死 |
| 失败把最后一行输出带进错误 | 界面只剩「安装失败」时用户无从下手 |
| `--ignore-scripts` | 不在安装期执行三方脚本 |

**全局可达靠终端入口**：装在应用目录前缀里的 pi，cmd / PowerShell / Git Bash 都能
直接敲 `pi` 拉起，机制见「终端入口」一节。

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

WorkBaby 在启动 / 装完 / 手动修复时跑一次探测（`pi/env/shell.go`）：

| 步骤 | 判定 |
|---|---|
| 用户指定的 `shellPath` | 真跑 `bash -c "echo …"` 验证 |
| Git / MSYS2 / Cygwin | 从 `ProgramFiles`、注册表 `SOFTWARE\GitForWindows`、PATH 上的 `git.exe` 反推安装根 |
| PATH 上的 bash | 按顺序试跑；WSL 启动器（`%SystemRoot%\System32\bash.exe`、`WindowsApps\bash.exe`）按路径直接剔除，不参与试跑 |
| 内置 Git Bash | 以上全都没有才解压 `runtime/gitbash` 并真跑验证 |
| 展示兜底 | PowerShell（pwsh 7 优先）——只用于状态展示，不写任何配置 |

落盘只改 `shellPath` 一个键：

- 选中的 bash 是 PI 自己找不到的（非标准位置或内置）→ 写 `shellPath` 指过去
- PI 自己找得到（如装在 ProgramFiles）→ 不写；我们以前写过、现在被更好的 bash 取代 → 撤掉
- `defaultTools` 永远不碰：工具集归工具面板（掌管键）管，WorkBaby 不替 PI 换工具
- 自己写过的值记在 `<data>/runtime/shell.json`：**只有自己写的才敢撤回**，用户手写的配置不碰

## 终端入口

WorkBaby 装的 pi 不在用户 PATH 上，用户开个终端敲 `pi` 会得到「不是内部或外部命令」。
`EnsureTerminal` 做两件事：

| 动作 | 细节 |
|---|---|
| 写 `<data>/bin/pi.cmd` 与 `<data>/bin/pi` | `.cmd` 给 CMD / PowerShell；`pi`（sh 脚本，路径转正斜杠）给 Git Bash。两者都是绝对路径的**当前生效 node** + PI 的 `cli.js`。不写 `.ps1`：PowerShell 会优先跑它，而执行策略为 Restricted 的机器上那会直接报「禁止运行脚本」 |
| 把 `<data>/bin` **前置**到 **HKCU** `Environment\Path` | 只改当前用户，不动系统级 PATH；保持原值类型（REG_SZ / REG_EXPAND_SZ）；去掉重复项；写后广播 `WM_SETTINGCHANGE` |

**为什么前置**：Windows 的有效 PATH 是「系统级 + 用户级」。用户级里排在后面的 `pi`
会被前面的 node 全局目录抢先；前置后，用户级这一段里永远是我们这份先被找到。

**系统级 PATH 改不了**（需要管理员）：`TerminalStatusOf` 会按「系统级 + 用户级」的
真实顺序扫一遍，认出排在入口目录之前、也带着 `pi` 入口的目录（`terminal_shadowed_by`），
界面直接说「终端会敲到那一份，请在系统环境变量里清掉」，不谎报「已就绪」。
同一份状态还会列出 PATH 上其它装过 pi 的目录（`other_pis`）。

**界面与终端钉同一份**：入口指向 `PiExe` 当前生效的那份 PI 与 node。用户后来自己
装了 PI、生效份漂移时，启动会重刷入口（自装或入口已存在/已挂 PATH 时每次启动都刷），
终端跟着界面走，不会两个内核互相打架。新开的终端立即生效，已经开着的终端要重开——
这一点在界面提示里写清楚。

## 更新

`POST /engine/update`（别名 `POST /env/update`）升级 PI 自身：

| 情况 | 动作 |
|---|---|
| WorkBaby 自己装的那份 | `npm install -g --prefix <应用目录> @latest`，不另起一份 |
| 系统里那份 | 从包目录反推前缀（`…/node_modules/<pkg>` 的父目录）后直装，装回原处 |
| 直装失败 | 兜底交给 PI 自己的 `pi update` |

升级完重启全部 PI 进程、刷新终端入口并重扫技能：配置与扩展都是启动期读的。

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
```

内置运行时解压失败只告警不阻断：系统里那份可能好使。但原因必须写清楚——
只说「不可用」，用户无从判断是包没嵌进来、解压失败还是被安全软件拦了。

## 取舍

- **用户已有 pi / node 一律优先**：不装第二份内核、不白解压内置运行时；代价是用户那份版本太旧时只能靠引擎页的升级按钮解决。
- **装到应用目录前缀而不是用户 npm 前缀**：免疫 nvm/fnm 切版本导致的「多份 pi」；代价是要靠终端入口 + HKCU PATH 前置实现全局可达，用户得重开终端才看得见。
- **装包跟随当前生效 node**：不无故解压内置；代价是用户 node 旁边缺 npm 时要回退内置 npm-cli.js（罕见路径）。
- **双源并联探测 + 失败换源**：无代理网络下也能装上；代价是每次装包多一次探测（约 0.3~4 秒）。
- **只改自己写过的配置**：`shellPath` 的撤回有据可查；代价是要多维护一份自愈记录。
- **不替 PI 换工具集**：没有 bash 时解压内置 Git Bash 兜住，而不是把 bash 工具换成 powershell——PI 的工具语义归 PI；代价是没有 bash 的机器第一次要多等几秒解压。
- **不预置模型配置**：密钥是用户的东西；代价是首次启动必须自己填一次（向导兜住）。
- **升级走 npm 直装而不是只靠 `pi update`**：前缀说得准，代价是要自己维护这份参数。
