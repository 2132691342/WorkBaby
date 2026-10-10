# HTTP API 契约

基础路径 `/api/v1`；统一响应 `{code, message, data}`，`code=0` 为成功。
方法白名单：GET / POST；删除一律 `POST .../delete`。
事件流：`GET /api/v1/events?session_id={sid}`（SSE）。

前端调用全部收口在 `frontend/src/src/api/index.ts`，与 `backend/server/routes.go` 一一对应。

## 系统

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /health | 存活探测 |
| GET | /bootstrap | 启动引导（版本 / 契约版本 / 默认服务 / 默认模型 / 权限档 / 工作目录 / 设置全集 / python_ready） |

## 会话

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /sessions?offset= | 会话列表（按更新时间倒序，一页 200 条；`offset` 用于「加载更多」翻页） |
| POST | /sessions | 新建 `{title?, workspace?, provider_id?, model?}` |
| GET | /sessions/:id | 详情：会话 + 当前分支全量消息 |
| POST | /sessions/:id/rename | `{title}` |
| POST | /sessions/:id/delete | 删除（级联 entries / approvals / token_usages） |
| POST | /sessions/:id/model | `{provider_id, model}` |
| POST | /sessions/:id/permission | `{permission}`：`ask` / `auto_edit` / `yolo` |
| POST | /sessions/:id/workspace | `{workspace}` 切换本会话工作目录（必须真实存在，下一条消息生效） |
| POST | /sessions/:id/branch | `{entry_id}` 把 leaf 指回历史某条 |

## 对话

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /chat/send | `{session_id, content, attachments?}` → `{run_id, entry_id, session_id}`。附件：`{path, name}` 引用工作目录文件；`{name, image_base64}` 直接带粘贴图片。图片走识图通道：单张 ≤5MB、每条 ≤4 张，模型不支持识图时报 3113 |
| POST | /chat/stop | `{session_id}` 停止当前 run |
| POST | /chat/steer | `{session_id, content}` 插话：轮间注入，立刻生效 |
| POST | /chat/followup | `{session_id, content}` 排队：与插话同队列，轮间注入 |

`attachments` 是 `@` 引用文件：`[{path, name}]`，`path` 相对路径按工作目录解析，
绝对路径必须落在工作目录内（越界拒绝，1004）。
后端现读文件正文拼到用户消息最前面（上限 5 个文件 / 每个 64KB），
读不到会在正文里写明「读不到」而不是静默跳过。

`followup` 与 `steer` 共用一个队列（见 `specs/01`）：内核不分二者，
「排到下一轮」还是「立刻生效」由 UI 决定。运行中与空闲时调用等价。

## 审批

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /approvals?session_id= | 待决审批（仅当前进程内等待中的；启动时会把上次残留的按拒绝收口） |
| POST | /approvals/:id/approve | `{scope}`：`once` / `session` |
| POST | /approvals/:id/deny | 拒绝 |

两个动作走同一条路由 `POST /approvals/:id/:action`，`action` 取 `approve` / `deny`。

## 模型服务

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /providers | 列表（不出密钥，只出 has_key） |
| POST | /providers | 新增 `{name, api, base_url, api_key?, models[]}` |
| POST | /providers/:id/update | 更新（api_key 留空 = 不改） |
| POST | /providers/:id/delete | 删除 |
| POST | /providers/:id/test | 连通测试，返回 `{ok, model, detail}` |
| POST | /providers/:id/default | 设为默认 |
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
| POST | /skills/:id/toggle | `{enabled}` |
| POST | /skills/:id/delete | 删除用户技能（builtin 不可删） |
| GET | /skills/:id/content | 技能正文 |
| GET | /knowledge/docs | 文档列表（`error` 字段兼作提示位：超长截断索引等「已成功但有话要说」也写这里） |
| POST | /knowledge/docs/add | `{paths[]}` 批量添加并建索引 → `{added}`；支持 md / txt / csv / log / json / yml / yaml / html / pdf / docx / xlsx / pptx |
| POST | /knowledge/docs/:id/delete | 删除（级联分块） |
| POST | /knowledge/reindex | 全量重建索引 → `{reindexed}` |
| POST | /knowledge/search | `{query, limit?}` → `{hits[]}` |
| GET | /tools | 工具清单：助手当前能干什么、风险多大、是否启用 |
| POST | /tools/:name/toggle | `{enabled}`，下一轮生效 |
| GET | /models/capability?model=&provider_id= | 模型能力画像：上下文窗口、派生输出预算、是否支持思考 / 识图 / 工具调用 |
| POST | /models/capabilities | 批量能力画像 `{provider_id, models[]}` → `ModelCapability[]`（换模型下拉一次列几十上百个模型，逐个查即 N+1） |
| GET | /models/config?model=&provider_id= | 单个模型配置（目录 + 覆写合并后的最终值） |
| GET | /models/configs?provider_id= | 一个服务下的全部模型配置 |
| POST | /models/config | 保存模型配置 `{provider_id, model, context_window?, temperature, top_p, vision, tool_call}`；temperature / top_p 传 -1 表示跟随上游默认；最大输出不可配置，按「窗口 1/8」派生 |
| GET | /runtime | 内置运行时状态：Python 与 PowerShell 各自的 exe / source(bundled\|system\|空) / version / error |
| POST | /runtime/redetect | 重新检测内置运行时：清探测缓存后重跑，运行期放入归档后点这里生效 |
| GET | /settings | KV 全集 |
| POST | /settings | `{key, value}` |

## 工具目录

`GET /tools` 直接投影 `tool.Registry`，与模型每轮拿到的工具声明是同一份真相，
不会出现「文档说有、实际没有」。

```jsonc
[{ "name": "powershell", "label": "跑命令", "category": "shell",
   "description": "在 Windows 上执行一条命令", "risk": "high",
   "approval": true, "mode": "sequential", "params": ["command"],
   "enabled": true, "builtin": true }]
```

`category` 取值 `file | shell | code | web | data`；`risk` 取值 `low | medium | high`。

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

模型级配置是用户对单个模型的覆写：上下文窗口（0 = 跟随目录）、温度 / top_p
（**-1 = 未设置，跟随上游默认**；0 是合法的确定性取值）、识图 / 工具调用两项能力。
最大输出不可配置：按上面的规则从窗口派生（`max_output` 出参即结果）。
会话发起请求时按这里的值算上下文水位与采样参数；代理改名的私有模型靠它闭环。

## 运行时状态

```jsonc
{ "python_exe": "…\\runtime\\python\\python.exe", "python_source": "bundled",
  "python_version": "3.12.8", "python_error": "",
  "powershell_exe": "…\\powershell\\pwsh.exe", "powershell_source": "bundled",
  "powershell_version": "7.4.2", "powershell_error": "" }
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
| POST | /providers/:id/reveal | 返回解密后的 `{api_key}`；只在用户点「显示」时调用，没存密钥时报 3114 |

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

## SSE 事件（`/api/v1/events`）

信封：`{seq, event, data}`，`seq` 是该会话内单调递增的整数。

| event | data |
|---|---|
| chat:start | `{run_id, session_id}` |
| chat:delta | `{entry_id, kind: text\|thinking, delta}` |
| chat:tool_start | `{tool_call_id, tool, label, args}` |
| chat:tool_end | `{tool_call_id, ok, title, output, duration_ms}` |
| chat:approval | `{approval_id, tool_call_id, tool, label, args, risk, reason}` |
| chat:compressed | `{tokens_before, tokens_after}` |
| chat:user | `{entry_id, content}`：插话 / 排队消息已注入上下文并落库，界面据此把它补进时间线（空闲时直接落库也发本事件） |
| chat:context | `{used, window, ratio, known, reserve?}`：每轮广播一次上下文占用，`ratio` 是 0-100 整数，`known=false` 表示窗口是缺省估算值，界面读数加「约」前缀（数字照常给，不隐藏） |
| chat:done | `{entry_id, stop_reason, usage?, max_tokens}`：`stop_reason` 取 `completed` / `length`（撞输出上限，正文可能只有半截）/ `max_turns` / `aborted` / `error`。`max_tokens` 是这一轮实际下发给上游的输出预算——`length` 时界面必须报出这个数字：我们给的额度用尽和厂商自己的硬限制在信号上长得一模一样（都是 `length`），但该做的事不同（调额度 vs 换模型），不给数字用户无从判断。界面必须照实表达截断，否则表现就是「助手莫名不说话了」：`length` 给说明 + 「继续」，`max_turns` 提示可接着做 |
| chat:stopped | `{reason}`：用户点了停止。单独发是因为停止可能来自托盘或快捷键，由后端广播一次权威信号，界面不会停在「后端已停、还在转圈」的状态 |
| chat:error | `{code, message}` |
| chat:gap | `{reason}`：重放窗口已滚过，前端应拉会话快照对账 |

### 断线对账

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

打开 / 切换 / 新建会话时，前端不等 `chat:context`：用会话快照里最近一条带
`usage.context` 的助手条目加 `/models/capability` 的窗口值，先把输入框的水位环
填上（`stores/chat.ts` 的 `seedContext`）。事件只负责对话进行中的实时更新。
