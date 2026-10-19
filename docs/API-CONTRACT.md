# API 契约

## 统一外壳

```json
{ "code": "0", "message": "", "data": { } }
```

- `code` 是**数字字符串**；`"0"` 成功，其余是 AppError 的错误码（见 `docs/ERROR-CODES.md`）。
- HTTP 状态码恒为 200（除 SSE 与静态资源）；前端按 `code` 分流。
- 契约里的切片与 map 字段永远不是 `null`（`api.ok()` 统一兜底，构造处也一律初始化）。

## 路径约定

`/api/v1/{resource}/{action}/:id`：动作段在 id 之前，id 永远是最后一段。
需要两个变量时把第二个放进请求体。只允许 GET 与 POST。

## 路由表

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/health` | 存活探测 |
| GET | `/bootstrap` | 启动引导：版本、契约版本、默认模型、工作目录、环境闸门、设置 KV |
| GET | `/warmup` | 首屏预热聚合：一次拉齐设置页五个面板的清单（技能 / 服务商 / 工具 / 插件 / MCP），不各自去拉 |
| GET | `/sessions?workspace=` | 会话列表（来自 PI 的会话文件，可按工作目录过滤） |
| GET | `/sessions/:id` | 会话详情（元信息 + 投影消息） |
| POST | `/sessions/rename/:id` | 改名（PI 的 `set_session_name`） |
| POST | `/sessions/delete/:id` | 删除（删 PI 的会话文件） |
| POST | `/sessions/model/:id` | 换模型 `{provider, model}` |
| POST | `/sessions/thinking/:id` | 换思考档 `{level}` |
| POST | `/sessions/export/:id` | 导出 HTML `{path}` |
| POST | `/chat/send` | 发送（`session_id` 空 = 新对话，返回体带回新建的 id） |
| POST | `/chat/stop` | 停止（先取回排队文本再中止） |
| POST | `/chat/steer` | 插话 |
| POST | `/chat/followup` | 排队 |
| POST | `/chat/retry-cancel` | 叫停自动重试 |
| POST | `/chat/answer` | 回答引擎提问 `{id, value, confirmed, cancelled}` |
| GET | `/commands?session_id=` | 斜杠命令（扩展 / 提示模板 / 技能） |
| GET | `/providers` | 服务商列表 + 默认模型 + models.json 路径 |
| POST | `/providers/upsert` | 新增或更新服务商 |
| POST | `/providers/delete` | 删除服务商 `{id}` |
| POST | `/providers/test` | 连通测试 `{id}` |
| POST | `/providers/reveal` | 查看密钥明文 `{id}` |
| POST | `/providers/default` | 设默认模型 `{provider, model}` |
| POST | `/providers/models/fetch` | 从上游拉模型清单 |
| GET | `/models/available?workspace=` | PI 眼下的可用模型（含内置服务商与登录态） |
| GET | `/skills` | 技能列表（global / workspace / package） |
| POST | `/skills/create` | 新建技能 |
| POST | `/skills/import` | 从磁盘导入 `{paths}` |
| POST | `/skills/import-zip` | 导入 zip `{filename, data(base64)}` |
| POST | `/skills/toggle/:id` | 启停 `{enabled}`（移目录） |
| POST | `/skills/delete/:id` | 删除（仅用户目录） |
| GET | `/skills/content/:id` | 技能正文 |
| GET | `/extensions` | 扩展条目 + 包清单 |
| POST | `/extensions/toggle` | 启停扩展 `{path, enabled}` |
| POST | `/extensions/import` | 导入扩展 `{paths}` |
| POST | `/extensions/delete` | 删除扩展 `{path}` |
| POST | `/extensions/install` | 装包 `{source}`（进度走 `chat:setup`） |
| POST | `/extensions/remove` | 卸包 `{source}` |
| POST | `/extensions/reload` | 重扫并重启 PI |
| GET | `/tools` | 工具清单（`snapshot` 说明来源） |
| POST | `/tools/toggle` | 启停工具 `{name, enabled}`（写 `defaultTools`） |
| GET | `/mcp?workspace=` | MCP 服务 + 在线状态 + 两份配置原文 |
| POST | `/mcp/save` | 保存配置原文 `{scope, raw}` |
| POST | `/mcp/remove` | 删服务 `{name, scope}` |
| POST | `/mcp/login` / `/mcp/logout` | OAuth 登录 / 登出 `{name}` |
| POST | `/mcp/reload` | 重启 PI |
| GET | `/files?path=` | 工作区目录列举（@ 引用面板） |
| GET | `/runtime` | 运行时状态（Node / Python / PI） |
| POST | `/runtime/redetect` | 重新探测 |
| GET | `/env` | 环境闸门快照 |
| POST | `/env/install` / `/env/update` | 安装或更新 PI（进度走 `chat:setup`） |
| POST | `/env/recheck` | 重新判定闸门 |
| GET | `/git/status` `/git/diff` `/git/history` `/git/commit` `/git/file-history` `/git/branches` | 只读版本视图 |
| POST | `/git/stage` `/git/unstage` `/git/commit` `/git/branch` `/git/switch` `/git/push` `/git/message` | 受控写入（白名单六动作 + 起草提交信息） |
| GET | `/docs` / `/docs/:name` | 内置帮助文档 |
| GET | `/settings` / POST `/settings` | 界面设置 KV |
| GET | `/settings/pi` | PI 配置快照（默认模型、思考档、压缩、全部路径） |
| POST | `/settings/pi/thinking` | 写 PI 的默认思考档 |
| POST | `/settings/pi/compaction` | 写 PI 的压缩参数 |
| GET | `/settings/workspaces` | 最近用过的工作目录 |
| GET/POST | `/settings/background` | 自定义背景 |
| GET | `/assets/background` | 背景图本体 |
| GET | `/stats?span=` | 用量统计；`span` 取 `24h` / `today` / `7d` / `14d` / `30d`（按小时或按天聚合），旧 `days` 数字参数兜底 |
| GET | `/api/v1/events?session_id=` | SSE 事件流（`last_event_id` 重放） |

## SSE

```
id: 42
event: chat:delta
data: {"seq":42,"event":"chat:delta","data":{"entry_id":"run-…","kind":"text","delta":"你"}}
```

- `data` 是信封：`seq` / `event` / `data` 三层，事件名在帧头与 `data.event` 里各出现一次。
- `session_id` 留空表示全局通道（环境安装、包安装进度）。
- 断线重连带 `last_event_id`；窗口溢出时后端先发 `chat:gap`，前端据此拉快照对账。

| 事件 | 载荷要点 |
|---|---|
| `chat:start` | run_id / session_id |
| `chat:delta` | kind=text / thinking + delta |
| `chat:tool_start` / `chat:tool_end` | 工具名、参数、输出、耗时 |
| `chat:user` | 插话 / 排队消息已注入上下文 |
| `chat:context` | 上下文占用（used / window / ratio / known） |
| `chat:compressed` | 压缩前后 token |
| `chat:retry` | 自动重试（attempt / max / message） |
| `chat:ask` | 引擎提问（id / method / title / message / options） |
| `chat:ask_closed` | 提问已结束（answered / cancelled / gone） |
| `chat:done` | stop_reason / usage / max_tokens |
| `chat:stopped` / `chat:error` / `chat:gap` | 停止 / 出错 / 重放缺口 |
| `chat:switched` | 会话被切走（换工作区 / 引擎重启），前端必须拉新快照 |
| `chat:setup` | 安装进度（phase / progress / message） |
| `chat:notify` | 引擎扩展的即时通知（level: info/warning/error，无需应答） |
| `chat:status` | 引擎扩展设的状态条（空 text 即清除） |
| `chat:widget` | 引擎扩展推的小部件文本行（空行即清除） |
| `chat:title` | 引擎请求改标题（写进标题栏，不进会话名） |

## 契约版本

`domain.ContractVersion` 与 `types/api.ts` 同键；改 RESP 字段或路径必须同时 +1。
