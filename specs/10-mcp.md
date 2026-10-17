# 10 · MCP 外部工具接入

## 定位

模型上下文协议（Model Context Protocol）是外部工具与资源的事实标准：一个 MCP server
暴露一组工具，任何支持的客户端都能发现并调用。

**客户端不再是 WorkBaby，是 PI。** PI 原生支持 MCP：它读 `<agent-dir>/mcp.json`、
自己拉子进程、自己握手、自己把工具声明并进模型请求。WorkBaby 这一层只剩两件事：

1. **编辑那个文件**（带校验与模板）
2. **把 PI 报上来的工具列出来给用户看**

不再自己做 JSON-RPC 客户端。理由很直接：一份协议实现两遍，两边对
`initialize` 能力的理解迟早会分叉，而模型只认 PI 那一份。

## 设计

| 决策 | 选择 | 为什么 | 代价 |
|---|---|---|---|
| 谁做客户端 | PI | 工具声明与真实调用必须在同一个进程里，否则「声明了却调不灵」 | WorkBaby 看不到调用细节，只能看 PI 的 `tool_execution_*` 事件 |
| 配置载体 | `<agent-dir>/mcp.json` | PI 的既有格式，从别的客户端拷来就能用 | 字段集由 PI 决定，WorkBaby 只是编辑器 |
| 配置入口 | 编辑器 + 内置模板 + 保存前校验 | 用户手上通常已有一段别人给的配置，要能整段粘贴 | 拼错字段名要自己看出来，界面给即时校验 |
| 工具可见性 | 从 PI 的 `get_state` / 工具事件读 | 真相在 PI 那边，WorkBaby 不复制 | 要 spawn 之后才知道有哪些 |
| 工具开关 | 只按 server 粒度（`disabled`） | 单个工具启停是 PI 的 `defaultTools` 与扩展的地盘 | 用户不能只关掉某个 server 里的一个工具 |
| 风险定级 | 交给 PI 的工具审批机制 | 审批是内核职责，两套闸门会互相打脸 | WorkBaby 的权限档不覆盖 MCP 工具 |

### 为什么不再自建 MCP 客户端

自建版本要自己做 `initialize` 能力协商、`tools/list` 缓存、空闲断开、
崩溃后工具标记不可用，以及"系统提示里不再声明已死 server 的工具"这套联动。
PI 已经做了，而且做得更全（它还要处理扩展注册的工具、`codemode` 的
工具清单预算这些我们根本用不到的东西）。

**代价**：这一层看不见连接结果。PI 的 `get_state` 不回报每个 server 的工具清单与成败，
所以 WorkBaby 只能给出「配置里声明了什么」，加上「重载后清单里有没有这个前缀」。

## 配置文件

路径：`<agent-dir>/mcp.json`，`<agent-dir>` 由 `PI_CODING_AGENT_DIR` 指向
`{数据目录}/pi-agent`（见 [18](18-pi-bootstrap.md)）。

首次启动若文件不存在，按内置模板写一份（只含 `_comment` 与一个示例条目），
让用户打开就知道往哪儿填，而不是面对空编辑器。

```jsonc
{
  "_comment": "MCP 服务配置。改完保存即刻生效；某项不用就删掉或把 disabled 设为 true。",
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "D:\\WorkBaby"],
      "env": { "NODE_OPTIONS": "--no-warnings" }
    },
    "my-tool": {
      "command": "python",
      "args": ["-m", "my_mcp_server"],
      "cwd": "D:\\tools\\my-mcp",
      "disabled": true
    }
  }
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `command` | string | 必填。可执行文件；`PATH` 里能找到就写名字 |
| `args` | string[] | 逐项一个元素；不要拼成一整串靠空格切（路径带空格会切错） |
| `env` | object | 附加环境变量（与 PI 的环境合并）。值里可直接写密钥 |
| `cwd` | string | 子进程工作目录 |
| `disabled` | bool | `true` 时整项跳过 |

字段集以 PI 的 `mcp.md` 为准。WorkBaby 的面板只做**语法与必填项**校验，
未知字段原样保留——按自己的理解裁剪字段会让用户从别处拷来的配置失效。

### `workbaby-knowledge` 是内置条目

WorkBaby 的知识库要把自己暴露给 PI，写的就是这个文件：

```json
{"mcpServers":{"workbaby-knowledge":{"command":"<exe>","args":["--mcp-knowledge"]}}}
```

它由 WorkBaby 在启动时**自动写入并保持**（用户不能删），其余条目归用户。
界面上把它单列成「内置能力」区，不与用户的 server 混在一起。

## 契约

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/mcp/config` | `{path, raw, servers[], parse_error}`：`raw` 是原文，`servers[]` 是解析后条目 |
| POST | `/api/v1/mcp/config` | 保存 `{raw}`：先校验再落盘；解析不过返回 `9106` 且不写文件 |
| GET | `/api/v1/mcp/template` | 内置模板 `[{kind,label,hint,raw}]`：空白 / 本地命令 / 常见示例 |
| POST | `/api/v1/mcp/reload` | 触发 PI 重扫：重启当前工作区进程，重新读 `mcp.json` |
| GET | `/api/v1/mcp/tools` | 当前 PI 报上来的 MCP 工具（名字带 `mcp__{server}__` 前缀） |

没有探测路由：保存成功后前端调 `POST /mcp/reload` 再 `GET /mcp/tools`，
对比服务名前缀是否出现。少一个路由，也少一套只为界面显示而起的临时进程生命周期。

错误码只有 `9106`：配置解析失败（语法或必填字段，保存与内置条目写入都归它）。
重载失败不另开码——它原样返回 PI 重启进程的错（9700 段），那才是真实原因。

**没有探测码，也没有探测动作**：连接由 PI 负责，而它的 `get_state` 不回报每个服务的
工具清单与成败，WorkBaby 无从确证「连上了」。所以出参里没有 `ok` / `error` / `工具数`
这类字段——给了就只能靠猜。界面显示的是「已声明」，
以及从 `/mcp/tools` 里能看到这个服务的前缀条目时显示「内核已注册」，
中间用「重新加载」这个动作连起来。

## 关键流程

1. 设置页「MCP」→ `GET /mcp/config` 拿到原文与解析结果并排显示
2. 点「填入模板」→ 编辑 → 保存：后端校验语法与 `mcpServers` 形状 → 落盘
3. 保存成功后调 `POST /mcp/reload` → PI 重启 → `GET /mcp/tools`
4. 助手发起调用 → **PI 的审批机制**先问 → 放行 → PI 调 `tools/call` → 结果回填
5. server 进程死亡：PI 自己把该 server 的工具标记失效，下一轮不再声明

第 5 步 WorkBaby 只旁观：界面上那个 server 的状态灯从 PI 的
`tool_execution_*` 与工具清单变化推断，不自己探活。

## 不变量

| 不变量 | 锁死原因 |
|---|---|
| 客户端只有 PI 一份实现 | 两份握手实现对 `initialize` 能力的理解迟早分叉，模型只认 PI 那一份 |
| 配置文件是唯一真相源 | 数据库里再存一份会出现「界面显示的和文件里的不一样」 |
| 解析失败一律不落盘 | 写坏一份配置等于把整个面板变成「什么都连不上」，用户还看不出是自己改坏的 |
| 未知字段原样保留 | 从别的客户端拷来的配置带 `timeout` / `headers` 等，裁剪会让它失效 |
| `workbaby-knowledge` 条目由程序维护 | 用户删掉它，知识库工具就消失，而症状是「搜索没结果」不是「配置丢了」 |
| 重载必须重启 PI 进程 | `mcp.json` 是进程启动期读的，改文件不重载等于没改 |
| 工具审批交给 PI | 两套闸门会互相打脸：WorkBaby 放行了 PI 仍会拦，反之亦然 |

## 取舍

- **不做独立探测进程**：靠保存后重载 + 看工具清单，多一次重启（~1s）换掉一整套
  「临时进程 + 临时客户端」的生命周期。
- **不做单工具开关**：server 粒度够用。要精细控制请用 PI 的 `defaultTools`。
- **不缓存工具清单**：每次从 PI 现读，server 升级后立刻可见；代价是打开面板多一次往返。
- **知识库那条不能被删**：它是 WorkBaby 自己的器官，写在用户文件里但归程序管。
  代价是用户的 `mcp.json` 里有一条他删不掉的东西——界面上明说它是什么。
