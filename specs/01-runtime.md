# 01 · 运行时与路径

## 两个根，不要混

| 根 | 谁的数据 | 内容 |
|---|---|---|
| **Agent 目录** | PI | 会话、settings / models / mcp、技能、扩展、包 |
| **数据目录** | WorkBaby | 内置运行时、SQLite（只有设置 KV）、日志、背景图、人设文件 |

Agent 目录按这个顺序解析：

1. `PI_CODING_AGENT_DIR` 环境变量（用户显式指定时绝不覆盖）；
2. `~/.pi/agent`。

**为什么不自己开一个目录**：那等于把用户的技能、插件、模型、会话全部隔离掉。
界面会显示「什么都没有」，而命令行里一切正常——两本账必然分叉。

## 内置运行时：只在缺的时候用

| 运行时 | 选择顺序 | 说明 |
|---|---|---|
| Node | 系统 `node`（≥ 22）→ 内置 `runtime/node` | 复用系统版本可以省一次解压，也让 `pi` 在终端与界面里跑在同一个 Node 上 |
| Python | 系统 `python`（3.x）→ 内置 `runtime/python` | 内置的是官方 embeddable 包：解压即用，`._pth` 里加进 `site-packages` |
| npm | 与 node 同一套 | 装 PI 与装包都用它 |

内置归档随二进制 `go:embed`（`backend/runtime/bundled/`），首次用到时解压到数据目录，
版本标记命中就跳过解压。Node 归档带顶层单目录，解压必须剥掉那一层——否则 `npm.cmd` 找错 node。

embeddable 包自带 `._pth` 而不带 pip：需要第三方包时由技能/用户在运行时自己决定怎么装，
WorkBaby 不代替用户联网装 pip（一个启动期就联网的动作，在离线机器上只能变成启动慢）。

运行时探测不缓存：它只是读一次版本标记，而「重新检测」按钮就要看到当场结果。
缺归档、磁盘满、安全软件拦下解压这三类失败都会出现在 `GET /runtime` 的 `*_error` 字段里。

## PATH 注入

spawn PI 时前置拼接选中运行时的目录（内置的那份才需要），并清掉继承来的 `npm_config_prefix`：

| 规则 | 原因 |
|---|---|
| 前置而不是追加 | 后置会被用户 PATH 上的旧版本抢先，选中的运行时等于没生效 |
| 不覆盖 `PI_CODING_AGENT_DIR` | 换了它，用户已有的技能 / 插件 / 模型 / 会话全部看不见 |
| 注入 `PYTHONIOENCODING=utf-8` / `PYTHONUTF8=1` | Windows 下 Python 默认 GBK，工具结果会变成乱码 |

## PI 的发现

```
WORKBABY_PI_EXE（显式/测试）
  → PATH 上的 pi.cmd / pi.exe / pi
  → 各 npm 全局前缀（系统 npm 的 prefix，其次应用内 npm-global）
```

找到之后跑一次 `pi --version` 填进环境闸门；版本号只用于展示与排障。

找到的是 npm 的 shim（`pi.cmd` / `pi`）时，**起进程前先还原成 `node.exe` + 包内
`cli.js`**：经 `cmd.exe /c` 起进程每调一次闪一个控制台窗口，且
`CREATE_NO_WINDOW` 只作用于 cmd.exe 本身，它再拉起的 node 仍会带窗口。
`npm.cmd` 同理还原成 `npm-cli.js`。解析结果带缓存（shim 文本不会自己变）。

## 环境闸门

按 `node → pi → model` 的顺序报**首个**阻塞项，并给原因。`ready` 三者全满足才为真；
前端据此决定是否强制进向导——没有 PI 就没有对话，留一个空壳只会让用户以为程序坏了。

`model` 判定看的是 `models.json` 里是否有「至少一个带模型与密钥的服务」，
而不是 `settings.json` 有没有默认模型：PI 自己会在缺省时自动选第一个可用服务。

## 取舍

- **复用系统运行时**：省一次解压、与终端行为一致；代价是版本差异要靠闸门挡住（≥ 22 才放行）。
- **内置运行时只做兜底**：用户机器上什么都没装也能起来；代价是安装包大 48MB。
- **不覆盖 agent 目录**：与 `pi` 共享一份状态；代价是卸载 WorkBaby 不会清掉 PI 的数据（这是对的）。
