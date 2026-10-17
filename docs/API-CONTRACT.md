# HTTP API 契约

基础路径 `/api/v1`；统一响应 `{code, message, data}`，`code="0"` 为成功（`code` 是数字字符串，
与 HTTP 状态码的 int 从类型上分开）。
方法白名单：GET / POST；变量参数必须落在路径末尾：`/{resource}/{action}/:id`，
删除一律 `POST /{resource}/delete/:id`。
事件流：`GET /api/v1/events?session_id={sid}`（SSE）。

前端调用全部收口在 `frontend/src/api/index.ts`，与 `backend/server/routes.go` 一一对应。

## 系统

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /health | 存活探测 |
| GET | /bootstrap | 启动引导（版本 / 契约版本 / 默认服务 / 默认模型 / 权限档 / 工作目录 / 设置全集 / `pi_env` / `expired_approvals[]`）。`pi_env` 是环境闸门（`node_ready` / `pi_ready` / `model_ready` / `ready` / `blocking`），`ready=false` 时前端进向导且不 spawn PI。`expired_approvals` 是本次启动被按拒绝收口的上次残留确认 |

## 会话

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /sessions?offset= | 会话列表（按更新时间倒序，一页 200 条；`offset` 用于「加载更多」翻页） |
| POST | /sessions | 新建 `{title?, workspace?, provider_id?, model?}` |
| GET | /sessions/:id | 详情：会话元数据 + **从 PI 的 JSONL 投影出的消息**。不做上下文重建，只显示文件里有什么 |
| POST | /sessions/rename/:id | `{title}`（同时写 SQLite 与 PI 的 `set_session_name`） |
| POST | /sessions/delete/:id | 删除（SQLite 行 + PI 的 JSONL 文件 + approvals + token_usages）。会话在跑时先 `abort` 等 `agent_settled` 再删 |
| POST | /sessions/model/:id | `{provider_id, model}`（同时发 PI 的 `set_model`） |
| POST | /sessions/thinking/:id | `{level}`：`off` / `minimal` / `low` / `medium` / `high` / `xhigh` / `max`，发 PI 的 `set_thinking_level` |
| POST | /sessions/permission/:id | `{permission}`：`auto` / `ask_write` / `readonly`。写进 `workbaby-policy.json` 供审批扩展读取（specs/19） |
| POST | /sessions/workspace/:id | `{workspace}` 切换本会话工作目录（必须真实存在）。因为 `cwd` 是进程级属性，切换会按新工作区重新 spawn（下一条消息生效） |
| POST | /sessions/fork/:id | `{entry_id}` 从历史某条分叉成新会话 → `SessionVO`（发 PI 的 `fork`）。**不是移动 leaf**：PI 没有改 leaf 的协议面 |

## 对话

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /chat/send | `{session_id, content, attachments?}` → `{run_id, session_id}`。会话空闲发 PI 的 `prompt`，流式中带 `streamingBehavior` 发 `prompt`。附件：`{path, name}` 引用工作目录文件；`{name, image_base64}` 直接带粘贴图片。图片走识图通道：单张 ≤5MB、每条 ≤4 张，模型不支持识图时报 3113 |
| POST | /chat/stop | `{session_id}`：先 `clear_queue` 取回未消费文本，再 `abort` |
| POST | /chat/steer | `{session_id, content}` 插话：发 PI 的 `steer`，当前轮工具跑完后立即生效 |
| POST | /chat/followup | `{session_id, content}` 排队：发 PI 的 `follow_up`，等助手自然停下才投递 |

`attachments` 是 `@` 引用文件：`[{path, name}]`，`path` 相对路径按工作目录解析，
绝对路径必须落在工作目录内——越界的文件**不读取**，在正文里写明原因后继续发送
（只有 `/files` 列举路径越界才报 1004；发消息本身不因一个附件失败而失败）。
后端现读文件正文拼到用户消息最前面（上限 5 个文件 / 每个 64KB），
读不到会在正文里写明「读不到」而不是静默跳过。

`followup` 与 `steer` **不再共用队列**：它们在 PI 里是两种语义。
`steer` 在当前轮的工具跑完后介入（“别这么干了”），`follow_up` 等助手彻底停下才投递（“做完了再干这个”）。
这个区别是用户真能感觉到的，所以 UI 必须给两个入口而不是一个。

## 审批

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /approvals?session_id= | 待决审批。**真相在 PI 的等待中 `extension_ui_request` 上**，这里返回的是的内存等待表快照 |
| POST | /approvals/decide/:id | `{action, scope?}`：`action` 取 `allow` / `deny`，`scope` 取 `once` / `session`。回一条 `extension_ui_response` |

两个动作走同一条路由 `POST /approvals/decide/:id`。动作不能拼成路径段——那会把 `:id`
挤到中间，与「变量参数落在末尾」冲突，因此 `action` 走请求体。
决策「弹出即消费」：这张审批仍在等待中才会生效；超时 / 会话被取消 / 重复点击之后到达的
决策返回 4102，不改状态。

`scope=session` 只作用于当前会话（写 `sessions.allowed_tools`），不写全局。
全局放行等于把 `ask_write` 档位废掉。完整链路见 [specs/19](../specs/19-approval-bridge.md)。

## 模型服务

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /providers | 列表（不出密钥，只出 has_key） |
| POST | /providers | 新增 `{name, api, base_url, api_key?, models[]}` |
| POST | /providers/update/:id | 更新（api_key 留空 = 不改） |
| POST | /providers/delete/:id | 删除 |
| POST | /providers/test/:id | 连通测试，返回 `{ok, model, detail}` |
| POST | /providers/default/:id | 设为默认 |
| GET | /providers/models?provider_id= | 拉上游模型列表 |
| POST | /providers/models/fetch | `{api, base_url, api_key?}`：未保存的新服务也能拉列表 |

## 技能 / 知识库 / 设置

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /skills | 技能列表（含 source 与 enabled） |
| POST | /skills | 新建技能 `{name, description, body}` |
| POST | /skills/import | 从磁盘导入 `{paths[]}`（SKILL.md 或含它的文件夹）→ `{imported, skipped[]}` |
| POST | /skills/import-zip | 导入技能压缩包 `{filename, data}`（zip 字节 base64）→ `{imported, skipped[]}` |
| GET | /files | 工作区目录列举（`?path=` 相对目录，空即根）→ `{path, entries[{name, dir}]}`；供 @ 引用面板浏览 |
| POST | /skills/toggle/:id | `{enabled}` |
| POST | /skills/delete/:id | 删除用户技能（builtin 不可删） |
| GET | /skills/content/:id | 技能正文 |
| GET | /knowledge/docs | 文档列表（`error` 字段兼作提示位：超长截断索引等「已成功但有话要说」也写这里） |
| POST | /knowledge/docs/add | `{paths[]}` 批量添加并建索引 → `{added}`；支持 md / txt / csv / log / json / yml / yaml / html / pdf / docx / xlsx / pptx |
| POST | /knowledge/docs/delete/:id | 删除（级联分块） |
| POST | /knowledge/reindex | 启动后台重建 → `{started}`；已在跑时 started=false，不排队也不重入 |
| GET | /knowledge/reindex | 重建进度 → `{running, total, done, failed, started_at, finished_at}` |
| POST | /knowledge/reindex/cancel | 中断重建。大库重建要跑几十秒，同步等一个请求会把连接占死且无法停止，因此走后台任务 + 轮询 |
| POST | /knowledge/search | `{query, limit?}` → `{hits[]}` |

### 记忆

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /memories | 记忆列表 → `{items[]}`（`source`: manual / auto） |
| POST | /memories | `{content}` → `MemoryVO`，界面上写的一律标为 manual |
| POST | /memories/update/:id | `{content?, enabled?}` → `MemoryVO`；两者都空时报错 |
| POST | /memories/delete/:id | 删除（幂等） |
| GET | /tools | 工具清单：PI 当前声明的工具（内置 + MCP + 扩展），带风险与启用态 |
| POST | /tools/toggle/:name | `{enabled}`：写 PI `settings.json` 的 `defaultTools`（纯名字列表），下一轮生效 |
| GET | /models/capability?model=&provider_id= | 模型能力画像：上下文窗口、派生输出预算、是否支持思考 / 识图 / 工具调用 |
| POST | /models/capabilities | 批量能力画像 `{provider_id, models[]}` → `ModelCapability[]`（换模型下拉一次列几十上百个模型，逐个查即 N+1） |
| GET | /models/configs?provider_id= | 一个服务下的全部模型配置（目录 + 覆写合并后的最终值）。展开「模型参数」面板时用它一次取回，逐个 `models/config` 会打成 N+1 |
| GET | /models/config?model=&provider_id= | 单个模型配置；批量场景一律走 configs，这一条留给外部调用与排障 |
| POST | /models/config | 保存模型配置 `{provider_id, model, context_window, max_output, temperature, top_p, vision, tool_call, thinking}`。哨兵：`context_window=0` 跟随内置目录、`max_output=0` 按窗口派生、`temperature`/`top_p=-1` 跟随上游默认（`0` 是合法取值）；`thinking` 与目录取「或」，只能补成推理型、关不掉 |
| GET | /runtime | 运行时状态：Node / Python 各自的 exe / source(bundled\|system\|空) / version / error，另带 PI 的 exe / source / version |
| POST | /runtime/redetect | 重新检测内置运行时：清探测缓存后重跑，并把新路径写回 PI 进程的环境 |

#### 环境引导（specs/18）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /env | 环境闸门快照 `PiEnvVO`：`node_ready` / `node_path` / `node_version` / `pi_ready` / `pi_path` / `pi_version` / `model_ready` / `ready` / `blocking` |
| POST | /env/install | 安装 PI（`npm install -g --ignore-scripts @earendil-works/pi-coding-agent`），输出以 `chat:setup` 事件流式回传 → `{started}` |
| POST | /env/update | 更新 PI 到最新版 |
| POST | /env/recheck | 重新跑一遍探测（安装完成后调它刷新闸门） |

`ready=false` 时前端进向导，**不 spawn PI**：没配模型就 spawn 的话，失败点会推到
第一次对话，错误还埋在协议流里。`blocking` 给出首个不满足项（`node` / `pi` / `model` / 空），
向导据此直接跳到对应步骤。
| GET | /settings | KV 全集。bootstrap 之外的一条轻量读路径：bootstrap 一次给全量（服务 / 技能 / 文档 / 设置），任一子项出错整体失败；而主题 / 字体 / 字号只依赖设置表，不该被别的子项连坐 |
| POST | /settings | `{key, value}` |
| GET | /settings/background | 自定义背景 → `{image, opacity, blur}`；`image` 为空表示未设置 |
| POST | /settings/background | `{path, opacity, blur}`；图片先复制进数据目录再记路径（引用用户原文件，原文件一挪背景就没了）。`path` 为空表示清除并删除副本 |
| GET | /assets/background | 背景图本体。前端拿不到数据目录里的路径，只能走这个 URL |

## 工作区版本（只读）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /git/status?session_id= | 版本状态 → `GitStatusVO`。没装 Git / 不是仓库**不报错**，返回 `available` / `is_repo` 与一句 `reason`（处置动作是「去装」或「去初始化」，不是重试） |
| GET | /git/diff?path=&session_id= | 差异 → `GitDiffVO`。`path` 为空表示整个工作区；相对路径按工作目录解析，越界报 1004 |

`session_id` 决定看哪个目录：**给了就看该会话的工作目录**（会话级目录才是这次改动真正发生的地方），
没给或会话查不到则退回全局工作目录——只按全局看，用户在某个会话里换过目录就会看错地方。

```jsonc
// /git/status
{ "available": true, "reason": "", "is_repo": true, "workspace": "D:\\work",
  "branch": "main", "ahead": 1, "behind": 0, "git": "git version 2.47.0",
  "files": [{ "path": "src/main.go", "status": "M", "staged": false,
              "additions": 12, "deletions": 3 }] }

// /git/diff
{ "path": "src/main.go", "truncated": false, "dropped": 0,
  "lines": [{ "kind": "file", "old_no": 0, "new_no": 0, "text": "src/main.go" },
            { "kind": "hunk", "old_no": 0, "new_no": 0, "text": "@@ -4,3 +4,4 @@ func main() {" },
            { "kind": "add", "old_no": 0, "new_no": 5, "text": "fmt.Println(\"hi\")" }] }
```

`status` 取 `M` 改过 / `A` 新增 / `D` 删除 / `R` 改名 / `?` 未跟踪；`staged` 区分「已放进待提交区」与
「只改了工作区」。`kind` 取 `file | hunk | add | del | ctx`，`old_no` / `new_no` 为 0 表示该侧没有对应行。

两条都是只读：提交与回滚不进这套接口——写版本历史是不可逆操作，由用户在自己的 Git 工具里决定。
差异有 3000 行 / 256KB 双上限，超出按行截断并回报 `dropped`，界面写明「后面还有多少行」。

## 工具目录

`GET /tools` 投影 PI 当前声明的工具集（内置 + MCP + 扩展），与模型每轮拿到的声明是同一份真相。
数据来自 PI 进程：`get_state` 与 `tool_execution_*` 事件累积。**PI 未就绪时返回空列表 + `pi_ready=false`**，
不编造一份静态表。

```jsonc
[{ "name": "powershell", "label": "跑命令", "category": "shell",
   "description": "在 Windows 上执行一条命令", "risk": "high",
   "approval": true, "mode": "sequential", "params": ["command"],
   "enabled": true, "builtin": true }]
```

`category` 取值 `file | shell | code | web | data`；`risk` 取值 `low | medium | high`。
MCP 工具名字带 `mcp__{server}__{tool}` 前缀，`builtin=false`，风险一律 `high`。

开关写 PI 的 `defaultTools`：勾选集合写成**纯名字列表**（替换整个集），
不用 `+name` / `-name` 增减语法——那会依赖 PI 的默认集，PI 新增工具会绕过用户勾选出界面。

## 模型能力

上游 `/models` 只返回模型 ID，不带窗口大小与思考能力——**拉一次列表就"知道"上下文和
思考能力是不成立的**。能力来自内置目录 `domain.modelcap.go`，认不出来的模型返回
`known=false` 与缺省值，由界面显示「未知」并允许用户手填。

```jsonc
{ "id": "MiniMax-M3", "context_window": 1000000, "max_output": 125000,
  "window_override": false, "thinking": true, "vision": false, "tool_call": true, "known": true, "note": "" }
```

`max_output` 是派生值（窗口 1/8），不是目录里的独立声明。窗口来自内置目录时再与厂商
硬上限取小（有的模型单次输出上限只有 32K，1M 窗口按 1/8 会算出 12.5 万）；窗口被用户
手填时（`window_override=true`）硬上限不参与——目录里的硬上限描述的是同名模型在另一个
服务上的样子。被厂商上限钳住时界面补一句「已按厂商上限收紧」。

`known=false` 时 `note` 带一句人话说明。能力解析优先级：
**按「服务 + 模型」的用户配置（model_configs 表）→ 全局 `context_window` 设置（没配模型级时的兜底）→ 内置目录**。

模型级配置是用户对单个模型的覆写，共七项：

| 字段 | 说明 |
|---|---|
| `context_window` | 0 = 跟随目录；>0 即覆写，覆写后 `window_known=true` 且厂商硬上限不再参与钳制 |
| `max_output` | 0 = 按窗口派生（窗口 1/8，非 override 时再与厂商硬上限取小）；>0 即覆写，直接作为下发预算 |
| `tool_call` | 布尔覆写，关掉后该模型不再收到工具声明 |
| `vision` | 布尔覆写，关掉后带图发送被拒（3113） |
| `thinking` | 布尔覆写，与目录取「或」：只能把目录认不出的推理模型补成推理型 |
| `temperature` | **-1 = 未设置，不下发**；0 是合法的确定性取值，两者必须能分开 |
| `top_p` | 同上，夹取 [0,1] |

`POST /models/config` 接受以上七项；`GET /models/config` 与 `GET /models/configs`
出参是「目录 + 覆写」合并后的最终值（`max_output` 即实际下发预算，`thinking` 是合并结果）。
会话发起请求时按这里的值算上下文水位、输出预算与采样参数；代理改名的私有模型靠它闭环。

## 运行时状态

```jsonc
{ "node_exe": "…\\runtime\\node\\node.exe", "node_source": "bundled",
  "node_version": "24.21.0", "node_error": "",
  "pi_exe": "…\\runtime\\npm-global\\pi.cmd", "pi_source": "npm-global",
  "pi_version": "1.3.0", "pi_error": "",
  "python_exe": "…\\runtime\\python\\python.exe", "python_source": "bundled",
  "python_version": "3.12.8", "python_error": "" }
```

`*_source` 三种取值：`bundled`（内置就绪）/ `system`（内置缺失，用系统已装的那份）/
空（未就绪）。`*_error` 非空时带上可执行的原因（解压失败 / 内嵌字节是 LFS 指针 /
版本不一致），而不是只给一句「不可用」——用户要能据此判断下一步做什么。

## 帮助文档

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /docs | 内置帮助文档目录 `[{name, title}]`；按「新手先读」排序，首篇固定 getting-started |
| GET | /docs/:name | 单篇帮助文档 `{name, title, content}`；content 是原始 Markdown，渲染在前端；name 只放行 `[a-z0-9-_]`，越界与不存在都报 1012 |

文档随 exe 打包（`assets/docs/*.md`，首行 `# 标题` 即目录标题），只读、不落库。

## 密钥查看

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /providers/reveal/:id | 返回解密后的 `{api_key}`；只在用户点「显示」时调用，没存密钥时报码 `3114` |

## 统计

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /stats?days=14 | `{days, totals, daily[], models[], sessions[]}`；`days` 缺省 14、上限 90 |

```jsonc
{
  "days": 14,
  "totals": { "input": 0, "output": 0, "cached": 0, "total": 0, "calls": 0, "sessions": 0,
              "latency_ms": 0, "avg_latency_ms": 0, "cache_hit_rate": 0,
              "avg_context": 0, "peak_context": 0 },
  "daily":    [{ "date": "2026-09-28", "input": 0, "output": 0, "cached": 0, "total": 0 }],
  "models":   [{ "model": "deepseek-chat", "input": 0, "output": 0, "cached": 0, "total": 0, "calls": 0 }],
  "sessions": [{ "session_id": "SESSION_…", "title": "…", "total": 0 }]
}
```

`daily` 长度恒等于 `days`，没有调用的天也补零——否则趋势图 x 轴会随数据有无跳动。

## 指挥台（MCP / 运行观测 / 评估 / 历史 / 架构图）

### MCP 外部工具

配置载体是 `<agent-dir>/mcp.json`（`<agent-dir>` = `{数据目录}/pi-agent`），不是数据库表。
**客户端是 PI**：它自己拉子进程、自己握手、自己把工具声明并进请求。WorkBaby 只编辑那个文件。
界面是「编辑 JSON → 保存（校验）→ 重载 → 看工具清单」四步。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /mcp/config | `{path, raw, servers[], parse_error}`：`raw` 是文件原文，`servers[]` 是解析后的条目 |
| POST | /mcp/config | 保存 `{raw}`：先校验再落盘；解析不过返回 `9106` 且不写文件 |
| GET | /mcp/template | 全部内置模板 `[{kind,label,hint,raw}]` |
| POST | /mcp/reload | 重启当前工作区的 PI 进程（`mcp.json` 是启动期读的，不重启等于没改） |
| GET | /mcp/tools | 当前 PI 报上来的 MCP 工具（名字带 `mcp__{server}__` 前缀） |
| GET | /mcp/servers | 配置里声明的所有 server：`name` / `command` / `args` / `env_keys`（只给键名，不给值）/ `cwd` / `disabled` / `timeout` |

这里只有**声明**，没有连接结果：MCP 服务由 PI 连接，而 PI 的 `get_state` 不回报每个服务的
工具清单与成败。所以出参里没有 `ok` / `error` / `工具数` 这类字段——给了就只能靠猜。
界面因此显示「已声明 / 内核已注册」（后者来自 `/mcp/tools` 里有没有这个服务的前缀条目），
并提供「重新加载」动作而不是探测按钮。

条目字段以 PI 的 `mcp.md` 为准：`command`（必填）· `args[]` · `env{}` · `cwd` · `disabled`。
**未知字段原样保留**：从别的客户端拷来的配置带 `timeout` / `headers` 等，裁剪会让它失效。

内置的 `workbaby-knowledge` 条目由 WorkBaby 自动维护（知识库的 MCP 出口），用户不能删；
它单列成「内置能力」区，不与用户的 server 混在一起。

### 运行观测

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /runs?session_id=&limit= | 运行记录列表（不给 `session_id` 就是跨会话，最多 200 条） |
| GET | /runs/:id | 一次运行的详情：记录 + 时间线 + 上下文快照 + 可用检查点 |

`RunVO`：`{id, session_id, session_title, provider_id, model, status, stop_reason, turns,
tokens, input, output, cached, context, max_tokens, compressed, started_at, finished_at,
duration_ms, can_resume}`。

`status` 取 `running | done | failed | aborted`；`stop_reason` 取 PI 的
`stop | length | toolUse | aborted | error`。两者分开：前者是「这一轮结局」，
后者是「模型为什么停下」——合并成一列就分不清「被用户停掉」与「正常说完」。

`RunDetailVO` 在 `RunVO` 上追加：

```jsonc
{ "timeline": { "chars": 1820, "steps": [
      { "kind": "tool", "name": "read", "label": "读文件",
        "at": 1739790000000, "ms": 21, "ok": true, "detail": "…" } ] },
  "snapshots": [ { "turn": 1, "used": 4200, "window": 128000, "compressed": 0 } ],
  "checkpoints": [ { "id": "CKPT_…", "pi_entry_id": "a0b1c2d3", "turn": 0, "prompt": "…" } ] }
```

`timeline` 从 PI 的 JSONL 投影，不进库：复制一份就要在两处保持一致。

### 检查点（回到某一轮 / 从某一轮分叉）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /checkpoints?session_id= | 一个会话的全部检查点（按轮次升序） |
| POST | /checkpoints/fork/:id | 从这一刻重开 `{session_id}`：发 PI 的 `fork`（entryId = 检查点里的），**新建一条会话**，原会话完整保留。PI 没有「把会话叶子挪回去」的协议面，所以「回去」只能是分叉，界面因此不叫「撤销」也不叫「回退」 |

### 评估与回归

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /evals?scope=&scope_id= | 评估列表（含与基线的差值列） |
| POST | /evals | 登记主观分 `{run_id, score, note?}`（客观指标收尾时已自动采集） |
| GET | /evals/baselines · POST /evals/baselines · POST /evals/baselines/delete/:id | 基线列表 / 存基线 `{eval_id, scope}` / 删除 |
| GET | /evals/compare?baseline_id=&run_id= | 对比：指标差 + 结果摘要并排 |
| POST | /archive | 手动按保留期归档隐藏会话 → `{retention_days, sessions, detached_runs}` |

一次 run 最多一条评估；客观指标只读。`score` 1–5 分。`scope` 目前只有 `session`：
评估挂在一次对话上，再细到 run 级基线没有意义（同一会话里两次 run 的输入本来就不同）。

### 通用历史

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /history/entries?source=&q= | 扫描结果（每次实时扫，不缓存） |
| GET | /history/entry?id= | 一段会话的完整消息（只读）。id 带盘符与斜杠，走 query |
| GET | /history/folders · POST /history/folders · POST /history/folders/delete | 自定义文件夹注册管理 |
| POST | /history/continue | 接着聊 `{entry_id, next?}` → 新建会话并注入上下文摘要 |

v1 来源：`claude-code`（`~/.claude/projects/**/*.jsonl`）、
`codex`（`~/.codex/sessions/**/*.jsonl`）、`custom`（注册文件夹，通用 JSONL 兜底）。
扫描限时 5s / 500 文件，超限在结果里明说。

### 项目架构图

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /archmap/analyze | `{path?}` → 分层依赖图数据（限时 10s / 10 万文件） |
| GET | /archmap/entrypoints | 入口文件清单（main / index / App 等） |

```jsonc
// /archmap/analyze
{ "root": "D:\\work\\app",
  "nodes": [{ "id": "src/renderer", "label": "renderer", "files": 42, "lines": 5310,
              "indegree": 3, "depth": 1 }],
  "edges": [{ "from": "src/renderer", "to": "src/shared" }],
  "coverage": { "parsed": 118, "dynamic": 6, "skipped": 3 } }
```

`coverage` 必须随图返回——把「哪些 import 没解析出来」画在界面上，
比画一张貌似完整的图负责。只读分析，不写项目任何文件。

## 通用契约：数组字段永不为 null

所有接口的数组 / 对象字段**一律返回空数组 / 空对象，不返回 `null`**
（`nodes: []` 而不是 `nodes: null`）。Go 的 nil slice 编码成 `null`，
前端按数组读就是一句崩在页面上的报错；出口由 `pkg.FillNilSlices` 兜底，
构造处仍应显式初始化——兜底只保证漏掉的那一次不至于崩掉整个页面。

## SSE 事件（`/api/v1/events`）

信封：`{seq, event, data}`，`seq` 是该会话内单调递增的整数。

| event | data |
|---|---|
| chat:start | `{run_id, session_id}`（PI 的 `agent_start`） |
| chat:delta | `{entry_id, kind: text\|thinking, delta}` |
| chat:tool_start | `{tool_call_id, tool, label, args}` |
| chat:tool_end | `{tool_call_id, ok, title, output, duration_ms}` |
| chat:approval | `{approval_id, tool_call_id, tool, label, args, risk, reason}`：来自 PI 的 `extension_ui_request`，用户必须回应否则工具会被 PI 阻塞 |
| chat:compressed | `{tokens_before, tokens_after}`（PI 的 `compaction_end`） |
| chat:retry | `{attempt, max, message}`：上游拒绝/溢出时 PI 正在自动重试。**没有这个事件，界面会在重试期间显示“没反应”** |
| chat:user | `{entry_id, content}`：插话 / 排队消息已被 PI 接受，界面据此把它补进时间线 |
| chat:context | `{used, window, ratio, known, reserve?}`：每轮广播一次上下文占用，`ratio` 是 0-100 整数，`known=false` 表示窗口是缺省估算值，界面读数加「约」前缀 |
| chat:done | `{entry_id, stop_reason, usage?, max_tokens}`：**只在 `agent_settled` 发**。`stop_reason` 取 `stop` / `length`（撞输出上限）/ `toolUse` / `aborted` / `error`。`length` 时界面必须报出 `max_tokens` 数字：我们给的额度用尽和厂商硬限制在信号上长得一模一样（都是 `length`），但该做的事不同（调额度 vs 换模型） |
| chat:stopped | `{reason}`：用户点了停止。单独发是因为停止可能来自托盘或快捷键，由后端广播一次权威信号 |
| chat:error | `{code, message}`（含 PI 进程死亡的 `9701`） |
| chat:switched | `{session_id, reason}`：会话被切走了（回退分叉 / 工作区重建进程）。前端必须拉新会话的快照 |
| chat:gap | `{reason}`：重放窗口已滚过，前端应拉会话快照对账 |
| chat:setup | `{phase, progress, message}`：环境安装进度（npm 装 PI）。**安装可能几十秒，没有进度界面会被当成卡死** |

### 为什么 `chat:done` 只挂在 `agent_settled`

PI 的 `agent_end` 只关闭一次低层 run。自动重试、上下文溢出恢复、压缩重试、
插话与跟进都可能接在它后面。把 `done` 挂在 `agent_end` 会让界面在自动续跑时
先显示“完成”再继续吐字。

代价：`agent_settled` 之前业务层必须一直持着会话锁，用户在此期间看到的是“还在跑”。

## 断线对账

五条规则，前端必须照做：

1. **重连带 `Last-Event-ID`**：浏览器 `EventSource` 自动带，服务端补发 `seq` 之后的事件；
   补发段与实时段按 `seq` 去重（订阅与快照之间的窗口事件两条路都会到达），
   前端只需照常应用
2. **慢客户端先挤 delta**：缓冲（256 条）写满时先挤掉积压的 delta 保住关键事件，
   仍满才断开连接——宁可让前端重连补齐，也不拖慢内核。断的是连接不是会话；
   挤 delta 时若有关键事件被并发写入挤掉，服务端同样直接断开（静默吞掉
   done/error 等于让前端永远停在「运行中」）
3. **`chat:gap` 意味着有洞**：立刻 `GET /sessions/:id` 拉权威快照重建消息流，
   之后的事件继续正常应用。两种情况都会发：重放窗口整体滚过（缓冲里没有
   `Last-Event-ID` 之后的事件），或缓冲裁剪掉了中段（首条补发事件的 seq
   与请求位点不连续）——两种情况前端处理方式相同
4. **订阅必须在握手之后建立**：端口是异步注入的，握手完成前 `getBaseURL()` 为空串，
   此时订阅会静默失败（EventSource 压根没建起来）。前端须在握手就绪后补订阅，
   断线也要自建重连（EventSource 自带重连不带会话维度语义）
5. **重连成功即对账**：`chat:gap` 只在服务端确认丢帧时才发得到，慢客户端被断开时
   通道已满，这个事件很可能发不出来。所以前端在「断过又接上」时无条件对账一次
   （退出运行态 → 拉权威快照 → 重新对齐未决审批），不能只等 `chat:gap`

正常路径下前端在 `chat:done` 后也会拉一次权威快照——
**前端不做增量合并**，服务端是唯一真相。

打开 / 切换 / 新建会话时，前端不等 `chat:context`：`GET /sessions/:id` 的投影里
每个 assistant 消息已带 `usage.context`，用最近一条加 `/models/capability` 的窗口值，
先把输入框的水位环填上（`stores/chat.ts` 的 `seedContext`）。事件只负责对话进行中的实时更新。

会话切走（`chat:switched`）时必须先退出运行态再拉快照：不退出的话新会话的
第一条 delta 会被当成旧会话的增量拼进去。
