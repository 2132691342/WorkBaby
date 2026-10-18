# 01 · 运行时与路径

## 两个根，不要混

| 根 | 谁的数据 | 内容 |
|---|---|---|
| **Agent 目录** | PI | 会话、settings / models / mcp、技能、扩展、包 |
| **数据目录** | WorkBaby | 内置运行时、SQLite（只有设置 KV）、日志、背景图、人设文件 |

Agent 目录按这个顺序解析：

1. `PI_CODING_AGENT_DIR` 环境变量（用户显式指定时绝不覆盖）；
2. `~/.pi/agent`。

**为什么不自己开一个目录**：那等于把用户的技能、插件、模型、会话全部隔离掉，
界面会显示「什么都没有」，而 `pi` 命令里一切都好——这正是 v1 最贵的错误。

## 内置运行时：只在缺的时候用

| 运行时 | 选择顺序 | 说明 |
|---|---|---|
| Node | 系统 `node`（≥ 22）→ 内置 `runtime/node` | 复用系统版本可以省一次解压，也让 `pi` 在终端与界面里跑在同一个 Node 上 |
| Python | 系统 `python`（3.x）→ 内置 `runtime/python` | 内置的是官方 embeddable 包：解压即用，`._pth` 里加进 `site-packages` |
| npm | 与 node 同一套 | 装 PI 与装包都用它 |

内置归档随二进制 `go:embed`（`backend/runtime/bundled/`），首次用到时解压到数据目录，
版本标记命中就跳过解压。Node 归档带顶层单目录，解压必须剥掉那一层——否则 `npm.cmd` 找错 node。

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

## 环境闸门

按 `node → pi → model` 的顺序报**首个**阻塞项，并给原因。`ready` 三者全满足才为真；
前端据此决定是否强制进向导——没有 PI 就没有对话，留一个空壳只会让用户以为程序坏了。

`model` 判定看的是 `models.json` 里是否有「至少一个带模型与密钥的服务」，
而不是 `settings.json` 有没有默认模型：PI 自己会在缺省时自动选第一个可用服务。

## 取舍

- **复用系统运行时**：省一次解压、与终端行为一致；代价是版本差异要靠闸门挡住（≥ 22 才放行）。
- **内置运行时只做兜底**：用户机器上什么都没装也能起来；代价是安装包大 48MB。
- **不覆盖 agent 目录**：与 `pi` 共享一份状态；代价是卸载 WorkBaby 不会清掉 PI 的数据（这是对的）。
