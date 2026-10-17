# 18 · 环境引导

用户双击 exe 就该能用。这条链路负责把「有 Node 吗、有 PI 吗、模型配了吗」
三步全部收成界面上的一次向导。

## 1. 三级探测

每样东西都有三个来源，按序取第一个可用的。

| 优先级 | 来源 | 用途 |
|---|---|---|
| 1 | 内置运行时（`backend/runtime/bundled/`，go:embed） | 打包即用，不碰用户环境 |
| 2 | 系统 PATH | 尊重用户已装好的、更新的版本 |
| 3 | 向导安装 | 内置缺失且系统没有时兜底 |

内置 node 与内置 python 不同：**内置 node 优先于系统 node**。
理由是 PI 对 Node 版本有下界（≥ 22.19）且行为随小版本变，
用用户 PATH 上某个 18.x 会让错误出现在协议层而不是启动检查里。
内置 python 则相反——用户自己装的 python 有他的包，优先用他的。

## 2. 内置运行时

| 运行时 | 归档 | 落地 |
|---|---|---|
| Python 3.12.8 | `python-3.12.8-embed-amd64.zip` | `<data>/runtime/python/` |
| Node 24.21.0 | `node-v24.21.0-win-x64.zip` | `<data>/runtime/node/` |

PowerShell 不内置：Windows 自带 5.1，助手要跑命令时走系统 `powershell.exe`。

首次启动解压一次，写版本标记文件。标记存在即跳过，版本号变了才重解。
归档走 Git LFS，SHA 与代码里的常量对齐（不符即拒绝解压）。

Node 归档解压后是 `node-v24.21.0-win-x64/` 单层目录，**必须平铺到根**：
不平铺的话 `node.exe` 的实际路径多一层，`npm.cmd` 里的相对路径会指错，
装 PI 时表现为 "npm 找不到 node"。

## 3. PI 安装

PI 有四个可用来源，按序取第一个可用的：

```text
WORKBABY_PI_EXE（可指定） > 内置 npm 前缀 > 系统 PATH（pi.cmd / pi） > 安装
```

**系统已装的 PI 优先于内置那一份**：用户自己用 npm 装的那份带他自己的包、他自己的版本（配合 pi update），多养一份只会让改过的配置不生效。


```text
检测 pi --version
  ├─ 有且 >= 最低版本 → 可用
  ├─ 有但太旧 → 提示更新
  └─ 没有 → 安装
```

安装走 npm，参数固定：

```text
<npm> install -g --ignore-scripts @earendil-works/pi-coding-agent
```

- `--ignore-scripts` 是 PI 官方推荐：它的正常安装不需要依赖的生命周期脚本
- npm 全局前缀必须**可写**：系统前缀通常需要管理员权限，因此优先用
  `<data>/runtime/npm-global` 作为前缀（`npm_config_prefix`）
- 安装过程输出流式回传前端（`chat:setup` 事件），用户要看到在动

### 3.1 为什么不用在线安装脚本

PI 官方的 `install.ps1` 会自己决定装哪个 Node、装到哪里、是否改用户 PATH。
WorkBaby 需要的恰好是「别改我的环境」：全部落在应用数据目录，
卸载即删干净。代价是要自己维护安装参数，PI 改了包名就要跟。

## 4. PATH 注入

PI 进程与它派生的所有工具共享一份注入后的环境：

```text
PATH = <内置 node> ; <内置 npm global/bin> ; <内置 python> ; <原 PATH>
```

这样有三件事同时成立：

1. PI 自己是内置 node 跑的
2. PI 的 `bash` / `powershell` 工具里 `node` / `npm` / `python` 都能直接调
3. 助手写的脚本用它内置的那份解释器，用户不用装环境

非 Windows 平台不适用（本期只考虑 Windows，`//go:build windows` 分文件）。

## 5. WorkBaby ↔ PI 配置互通

两边的配置文件是同一份，WorkBaby 是它唯一的写入者，PI 是读取者。

| WorkBaby 界面 | PI 文件 | 写法 |
|---|---|---|
| Provider 增删改 | `<agent-dir>/models.json` → `providers` | 只写 PI 认识的字段 |
| 模型列表 / 上下文窗口 / 最大输出 | `models.json` → `providers[].models[]` | 同上 |
| 默认模型、思考档位、压缩参数 | `<agent-dir>/settings.json` | 同上 |
| MCP server | `<agent-dir>/mcp.json` | 同上 |
| API Key | `models.json` → `apiKey` 字段 | 见下 |

`<agent-dir>` 由 `PI_CODING_AGENT_DIR` 指定，默认 `~/.pi/agent`。
WorkBaby 显式设置它指向自己的数据目录，把 PI 的状态收进应用目录，卸载即清。

### 5.1 API Key 存法

PI 接受三种形态：字面量、`$ENV_NAME`、`!command`（命令输出）。

WorkBaby 存 AES-GCM 加密的密文，写 `models.json` 时解出来写**字面量**。
不用 `!command`：那要在 Windows 上造一个"打印密钥"的小程序，比直接写更危险。

**代价**：密钥在 `models.json` 里是明文。缓解办法是把这个文件的权限设成仅当前用户
可读，并在界面上明说这件事；不假装它安全。

### 5.2 谁拥有 `models.json`

WorkBaby 是唯一写入者。用户如果手工改了它，下次启动会被界面里的配置覆盖。

这是刻意的：两边都能写会导致「改了没生效」。代价是高级用户失去手改这条路——
他们的出路是用 PI 原生 CLI，不走 WorkBaby。

### 5.3 首次回填

应用第一次跑且 `models.json` 已存在时，把它读进界面（用户可能是先装了 PI 的
老用户）。之后以 WorkBaby 为准。

## 6. 环境闸门

启动阶段一次判定，结果进 `BootstrapVO`：

```go
type PiEnvVO struct {
    NodeReady    bool   `json:"node_ready"`
    NodePath     string `json:"node_path"`
    NodeVersion  string `json:"node_version"`
    PiReady      bool   `json:"pi_ready"`
    PiPath       string `json:"pi_path"`
    PiVersion    string `json:"pi_version"`
    ModelReady   bool   `json:"model_ready"`   // 至少一个 provider 有可用 key 与模型
    Ready        bool   `json:"ready"`
    Blocking     string `json:"blocking"`      // node / pi / model / ""（首个不满足项）
}
```

`Ready == false` 时前端进向导，**不 spawn PI**。
spawn 一个没有模型配置的 PI 会在第一次 prompt 时才失败，错误信息还埋在协议流里。

## 7. 不变量

| 不变量 | 锁死原因 |
|---|---|
| 内置 node 解压必须平铺到根 | 多一层目录会让 `npm.cmd` 找错 node，表现为"npm 装了但 PI 起不来" |
| 归档 SHA 必须校验后解压 | 归档走 LFS，损坏或 pointer 文件会解出一堆乱码路径 |
| npm 全局前缀必须可写且落在应用目录 | 系统前缀要管理员权限，写失败时 npm 报的是权限错，用户完全看不懂 |
| 安装过程必须流式回传输出 | 一次 `npm install` 可能几十秒，界面无反馈会被当成卡死 |
| 注入 PATH 用前置拼接 | 后置拼接会被用户自己那份旧 node 抢先，内置 node 等于没生效 |
| `Ready` 判定在 spawn 之前 | 没配模型就 spawn，失败点会推到第一次对话，错误难以归因 |
| 密钥文件权限收紧 | 明文落盘是既定代价，不加权限就是这一代价的白给 |
| 版本标记存在即跳过解压 | 每次启动都解压 100MB+ 归档，冷启动会从 1s 变 20s |

## 8. 代价

- exe 体积 +36MB（Node）——换来"双击即用"。
- 内置 node 优先意味着用户全局升级 node 不会改变 WorkBaby 的行为（这既好也坏）。
- `models.json` 单写者原则让高级用户无法手改配置生效。
