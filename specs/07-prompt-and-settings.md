# 07 · 提示词与设置

WorkBaby 不再自己拼系统提示、不再自己管工具声明——这两件事都在 PI 里。
本层只剩三件事：**把配置写进 PI 的文件**、**把工具开关映射成 `defaultTools`**、
**把拓展模块的内容送到 PI 能读到的位置**。

## 1. 分层

| 内容 | 归属 | 位置 |
|---|---|---|
| 系统提示正文、工具声明、cwd 段、skills 段 | PI | PI 内部生成，WorkBaby 不参与 |
| 用户自定义指令 | 用户 | `<agent-dir>/AGENTS.md` 或 `APPEND_SYSTEM.md` |
| 项目级指令 | 用户 | `<workspace>/.pi/APPEND_SYSTEM.md` |
| WorkBaby 拓展模块（记忆 / 知识库 / 技能） | WorkBaby | 见 §3 |
| 模型、思考档、压缩参数 | WorkBaby 界面 → PI `settings.json` | 见 [18](18-pi-bootstrap.md) |

**不覆盖 PI 的系统提示**（不写 `SYSTEM.md`）。PI 的提示与它的工具集、压缩策略、
权限模型是配套的，覆盖掉等于把内核的假设全拆了。要加东西就用 `APPEND_SYSTEM.md`。

## 2. 工具的开关

WorkBaby 的工具面板映射到 PI 的 `settings.json` → `defaultTools`：

| 界面状态 | `defaultTools` 写法 |
|---|---|
| 全部默认 | 不写（PI 用它的默认集） |
| 关掉 `bash` | `["+read","+edit","+write","+grep","+find","+ls","+powershell"]` |
| 只留只读 | `["read","grep","find","ls"]`（纯名字 = 替换整个选择） |

规则来自 PI：纯名字替换整个集，`+name` / `-name` 在继承集上增减。
WorkBaby 的界面语义是「勾选集合」，所以写**纯名字列表**，不用增减语法——
增减语法依赖 PI 的默认集，PI 加了新工具会自动出现在用户界面上，与勾选不符。

MCP 工具不进 `defaultTools`：它们在 `mcp.json` 里按 server 启用，PI 自己合并。

## 3. 拓展模块怎么进 PI

三个模块的内容都在 WorkBaby 手里，PI 通过文件系统读：

| 模块 | 落地 | PI 怎么读 |
|---|---|---|
| 技能（skill） | `<agent-dir>/skills/<name>/SKILL.md` | PI 原生的 skill 加载器 |
| 知识库 | 不落 PI 的文件，WorkBaby 注册成 MCP server | PI 通过 `mcp.json` 调它 |
| 记忆 | `<agent-dir>/APPEND_SYSTEM.md` 的一个片段 | PI 每轮随提示带上 |

### 3.1 技能

WorkBaby 的技能面板做的就是「管理 `<agent-dir>/skills/`」：导入、启停、删除。
停用一个技能 = 把它从目录移走（PI 只看目录里有什么，没有"禁用"概念）。

**不用 PI 的 skill 命令语法**（`/skill:name`）：WorkBaby 的技能卡片点击后
把 SKILL.md 正文作为上下文注入当前轮，不依赖斜杠命令。两者都指向同一个文件。

### 3.2 知识库

WorkBaby 的知识库是它自己的 FTS5 索引，PI 无法直接读。
接法是把自己暴露成一个 stdio MCP server（`workbaby-knowledge`），写进 `mcp.json`：

```json
{"mcpServers":{"workbaby-knowledge":{"command":"<exe>","args":["--mcp-knowledge"]}}}
```

PI 因此看到一个 `knowledge_search` 工具。这正是 PI 的 MCP 通路的标准用法，
不需要 PI 侧的改动。

### 3.3 记忆

记忆写进 `<agent-dir>/APPEND_SYSTEM.md` 的一个受控片段：

```markdown
<!-- workbaby:memory:begin -->
- 用户偏好简短回答
- 该仓库的测试命令是 pwsh scripts/check.ps1
<!-- workbaby:memory:end -->
```

改记忆 = 重写这个片段。**只改标记块内的内容**，块外是用户自己写的，不能被覆盖。

## 4. 设置项的归属

| 设置 | 存在哪 | 谁读 |
|---|---|---|
| 主题、字号、背景图、关闭行为 | SQLite `settings` KV | 只有 WorkBaby |
| 默认模型、思考档、`defaultTools` | PI `settings.json` | PI 读，WorkBaby 也读 |
| 压缩参数（`reserveTokens` / `keepRecentTokens`） | PI `settings.json` | 只有 PI |
| Provider / 模型 / Key | PI `models.json` | PI 读，WorkBaby 也读 |
| MCP server | PI `mcp.json` | PI 读（WorkBaby 的面板写它） |
| 知识库路径、索引参数 | SQLite | 只有 WorkBaby |

界面只显示一个「设置」页，但底层分两个存放点。这是有意的：
凡是 PI 需要知道的东西就写 PI 的文件，纯界面偏好留在 SQLite。

## 5. 启动时的配置同步

```text
Bootstrap
  ├─ 若 models.json 不存在 → 从 SQLite 生成（首次）
  ├─ 若存在 → 读进界面（接管老用户的配置）
  ├─ 同步 settings.json 的 defaultTools（界面的勾选状态为准）
  └─ 重写 APPEND_SYSTEM.md 的记忆片段与 mcp.json 的知识库条目
```

每次启动都重写这三处是幂等的：内容由 WorkBaby 的当前状态决定，
不依赖上次写了什么。

## 6. 不变量

| 不变量 | 锁死原因 |
|---|---|
| 不写 `SYSTEM.md` | PI 的系统提示与它的工具集/压缩策略配套，覆盖等于拆掉内核的假设 |
| `defaultTools` 用纯名字列表 | 增减语法依赖 PI 默认集，PI 新增工具会绕过用户勾选出界面 |
| `APPEND_SYSTEM.md` 只改标记块 | 块外是用户手写内容，整体覆盖会静默删掉用户的东西 |
| 技能停用 = 移出目录 | PI 只看目录内容，没有"禁用"标志位，写个 enabled 字段等于自欺 |
| 知识库走 MCP 而不是塞提示 | 塞提示会把检索结果整段灌进上下文，走工具是模型按需取 |
| 配置同步必须幂等 | 每次启动重写三处文件，非幂等会累积重复内容 |
| 密钥写进 `models.json` 用字面量 | `!command` 形态要在 Windows 上造打印密钥的小程序，比明文写更危险 |
| `models.json` 单写者 | 两边都能写会让「改了没生效」无法归因 |

## 7. 代价

- 高级用户不能手改 PI 配置生效（`models.json` 被界面接管）。
- 记忆走 `APPEND_SYSTEM.md` 意味着每轮都带上全文，记忆写多了会吃上下文。
  缓解：界面限制条数，超出的按最近使用保留。
- 技能停用靠移动文件，移动失败会留下"看起来还在"的目录。
