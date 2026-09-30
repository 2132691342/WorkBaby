# HTTP API 契约

基础路径 `/api/v1`；统一响应 `{code, message, data}`，`code=0` 为成功。
方法白名单：GET / POST；删除一律 `POST .../delete`。
事件流：`GET /api/v1/events?session_id={sid}`（SSE）。

前端调用全部收口在 `frontend/src/src/api/index.ts`，与 `internal/server/routes.go` 一一对应。

## 系统

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /health | 存活探测 |
| GET | /bootstrap | 启动引导（版本 / 契约版本 / 默认模型 / 工作目录 / 设置全集 / python_ready） |

## 会话

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /sessions | 会话列表（按更新时间倒序） |
| POST | /sessions | 新建 `{title?, workspace?, provider_id?, model?}` |
| GET | /sessions/:id | 详情：会话 + 当前分支全量消息 |
| POST | /sessions/:id/rename | `{title}` |
| POST | /sessions/:id/delete | 删除（级联 entries / approvals / token_usages） |
| POST | /sessions/:id/model | `{provider_id, model}` |
| POST | /sessions/:id/permission | `{permission}`：`ask` / `auto_edit` / `yolo` |
| POST | /sessions/:id/branch | `{entry_id}` 把 leaf 指回历史某条 |

## 对话

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /chat/send | `{session_id, content, attachments?}` → `{run_id, entry_id, session_id}`。附件：`{path, name}` 引用工作目录文件；`{name, image_base64}` 直接带粘贴图片。图片走识图通道：单张 ≤5MB、每条 ≤4 张，模型不支持识图时报 3113 |
| POST | /chat/stop | `{session_id}` 停止当前 run |
| POST | /chat/steer | `{session_id, content}` 插话：轮间注入，立刻生效 |
| POST | /chat/followup | `{session_id, content}` 排队：与插话同队列，轮间注入 |

`attachments` 是 `@` 引用文件：`[{path, name}]`，`path` 必须是**相对工作目录**的路径。
后端现读文件正文拼到用户消息最前面（上限 5 个文件 / 每个 64KB），
读不到会在正文里写明「读不到」而不是静默跳过。

`followup` 与 `steer` 共用一个队列（见 `specs/01`）：内核不分二者，
「排到下一轮」还是「立刻生效」由 UI 决定。运行中与空闲时调用等价。

## 审批

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /approvals?session_id= | 待决审批（含跨重启恢复项） |
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
| POST | /skills/:id/toggle | `{enabled}` |
| POST | /skills/:id/delete | 删除用户技能（builtin 不可删） |
| GET | /skills/:id/content | 技能正文 |
| GET | /knowledge/docs | 文档列表 |
| POST | /knowledge/docs/add | `{paths[]}` 批量添加并建索引 → `{added}` |
| POST | /knowledge/docs/:id/delete | 删除（级联分块） |
| POST | /knowledge/reindex | 全量重建索引 → `{reindexed}` |
| POST | /knowledge/search | `{query, limit?}` → `{hits[]}` |
| GET | /tools | 工具清单：助手当前能干什么、风险多大、是否启用 |
| POST | /tools/:name/toggle | `{enabled}`，下一轮生效 |
| GET | /models/capability?model=&provider_id= | 模型能力画像：上下文窗口、是否支持思考 / 识图 / 工具调用 |
| GET | /models/config?model=&provider_id= | 单个模型配置（目录 + 覆写合并后的最终值） |
| GET | /models/configs?provider_id= | 一个服务下的全部模型配置 |
| POST | /models/config | 保存模型配置 `{provider_id, model, context_window?, max_output?, temperature, top_p, vision, tool_call}` |
| GET | /runtime | 内置运行时状态（Python 是否就绪 + 失败原因 + 归档路径） |
| GET | /settings | KV 全集 |
| POST | /settings | `{key, value}` |
| GET | /stats?days=14 | 仪表盘统计（见下） |

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
{ "id": "MiniMax-M3", "context_window": 1000000, "max_output": 32768,
  "thinking": true, "vision": false, "tool_call": true, "known": true, "note": "" }
```

`known=false` 时 `note` 带一句人话说明。能力解析优先级：
**按「服务 + 模型」的用户配置（model_configs 表）→ 全局 `context_window` 设置（兼容旧数据）→ 内置目录**。

模型级配置是用户对单个模型的覆写：上下文窗口（0 = 跟随目录）、温度（缺省 0.25）、
top_p（缺省 0.75）、最大输出，以及识图 / 工具调用两项能力。会话发起请求时按这里的
值算上下文水位与采样参数；代理改名的私有模型靠它闭环。

## 运行时状态

```jsonc
{ "python_exe": "…\\runtime\\python\\python.exe", "python_source": "bundled",
  "python_version": "3.12.13", "python_error": "", "archive_path": "…\\runtimes\\python-3.12.13-win-x64.tar.gz" }
```

`python_error` 非空时带上可执行的原因（归档缺失 / LFS 指针未拉取 / 解压失败），
而不是只给一句「不可用」——用户要能据此判断该把文件放哪。

## 统计

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /stats?days=14 | `{days, totals, daily[], models[], sessions[]}`；`days` 缺省 14、上限 90 |

```jsonc
{
  "days": 14,
  "totals": { "input": 0, "output": 0, "total": 0, "calls": 0,
              "sessions": 0, "latency_ms": 0, "avg_latency_ms": 0 },
  "daily":    [{ "date": "2026-09-28", "input": 0, "output": 0, "total": 0 }],
  "models":   [{ "model": "deepseek-chat", "total": 0, "calls": 0 }],
  "sessions": [{ "session_id": "SESSION_…", "title": "…", "total": 0 }]
}
```

`daily` 长度恒等于 `days`，没有调用的天也补零——否则柱状图 x 轴会随数据有无跳动。

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
| chat:context | `{used, window, ratio, known, reserve?}`：每轮广播一次上下文占用，`ratio` 是 0-100 整数，`known=false` 表示窗口是缺省估算值，界面须显示「未知」而不是具体数字 |
| chat:done | `{entry_id, stop_reason, usage?}` |
| chat:stopped | `{reason}`：用户点了停止。单独发是因为停止可能来自托盘或快捷键，由后端广播一次权威信号，界面不会停在「后端已停、还在转圈」的状态 |
| chat:error | `{code, message}` |
| chat:gap | `{reason}`：重放窗口已滚过，前端应拉会话快照对账 |

### 断线对账

四条规则，前端必须照做：

1. **重连带 `Last-Event-ID`**：浏览器 `EventSource` 自动带，服务端补发 `seq` 之后的事件
2. **慢客户端直接断连**：缓冲（256 条）写满即断开连接，
   宁可让前端重连补齐，也不拖慢内核。断的是连接不是会话
3. **`chat:gap` 意味着有洞**：立刻 `GET /sessions/:id` 拉权威快照重建消息流，
   之后的事件继续正常应用
4. **订阅必须在握手之后建立**：端口是异步注入的，握手完成前 `getBaseURL()` 为空串。
   此时直接放弃订阅，事件流就永远建不起来，表现为「发消息一直转圈、没有回复」，
   且界面上没有任何错误——因为连接压根没发起。前端须在握手就绪后补订阅，断线也要自建重连

正常路径下前端在 `chat:done` 后也会拉一次权威快照——
**前端不做增量合并**，服务端是唯一真相。
