# API 契约 · API

baseURL：`http://127.0.0.1:{port}/api/v1`。端口由壳启动后经 Tauri 事件 `app:ready` 注入前端。
信封统一 `Resp { code: string, message: string, data: T }`，`code === "0"` 成功。

**CORS**：仅放行本机来源——`localhost` / `127.0.0.1`（任意端口，覆盖 dev 端口漂移）与
`tauri.localhost` / `tauri://localhost`（webview）；方法限 GET / POST，头放行
`Content-Type` 与 `Last-Event-ID`。服务只绑 127.0.0.1，外部站点读不到响应。

**只允许 GET / POST**；路径形状 `/{resource}/{action}/:id`，id 永远最后一段。

**业务前缀豁免**：HTTP 出入参字段一律带业务前缀，仅三处例外——
`/chat/stop` 响应是裸字段 `{"stopped": bool}`，`/chat/retry-cancel` 响应是裸字段
`{"cancelled": bool}`，`/providers/models/fetch` 请求体是裸字段
`{id, api, base_url, api_key}`。

本表与前端 `src/api/index.ts` 一一对应。

## 引导与设置

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /bootstrap | 启动数据：版本 / 数据目录 / 工作目录 / 引导状态（`boot_ready` / `boot_block_reason`） |
| GET | /warmup | 一次拉齐四路清单（providers / skills / tools / features） |
| GET | /settings | 设置 KV 全量 |
| POST | /settings | 批量写 KV |
| GET/POST | /settings/background | 背景图读取 / 设置 |
| GET | /assets/background | 背景图本体（直引 URL） |

工作区没有历史清单端点：启动时按设置 KV `workspace` 解析（存过且目录存在
则用之，否则用壳传入目录并写回），界面改选后重启生效。

## 会话与对话

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /sessions | 会话列表（session_index） |
| GET | /sessions/:id | 单会话详情（消息回放） |
| POST | /sessions/rename/:id | 重命名 |
| POST | /sessions/delete/:id | 删除（空闲会话才允许） |
| POST | /sessions/model/:id | 切模型 |
| POST | /sessions/thinking/:id | 切思考档位 |
| POST | /sessions/export/:id | 导出（body 带 `session_export_path`，兼容 `path`；空 = 存数据目录 exports） |
| POST | /chat/send | 发送（新会话时现场创建） |
| POST | /chat/stop | 停止（先取回队列再 abort；响应裸字段 `{"stopped"}`） |
| POST | /chat/steer | 运行中插话 |
| POST | /chat/followup | 空闲追问（完整一轮） |
| POST | /chat/retry-cancel | 取消自动重试等待（响应裸字段 `{"cancelled"}`） |
| POST | /chat/compact | 手动压缩 |
| POST | /chat/answer | 回答内核提问（支持结构化问卷应答） |

## Provider / 模型

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /providers | models.json 视图（密钥掩码） |
| POST | /providers/upsert | 增改 provider |
| POST | /providers/delete | 删 provider（默认模型连带清理） |
| POST | /providers/test | 连通性测试 |
| POST | /providers/reveal | 明文查看密钥 |
| POST | /providers/default | 设默认模型 |
| POST | /providers/models/fetch | 拉 /models 列表（请求体裸字段 `{id, api, base_url, api_key}`） |
| GET | /models/available | 全部可用模型（跨 provider 平铺） |

## 技能 / 工具 / 内置功能

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /skills | 技能清单（含开关状态与来源） |
| POST | /skills/toggle/:id | 开关 |
| GET | /skills/content/:id | 技能正文 |
| POST | /skills/create | 新建用户技能 |
| POST | /skills/import | 从目录导入 |
| POST | /skills/import-zip | 从 zip 导入（base64 字节） |
| POST | /skills/delete/:id | 删除用户技能（内置不可删） |
| GET | /tools | 工具清单（内置 + 功能提供 + MCP） |
| POST | /tools/toggle | 工具开关 |
| GET | /features | 12 项内置功能清单 |
| POST | /features/toggle | 功能开关（切换后重建工具注册表） |
| GET/POST | /features/:code/config | 功能级配置读写（KV） |
| GET | /commands | 斜杠命令清单（启用技能 → /skill:名） |
| GET | /loadout · POST /loadout/save · /loadout/apply · /loadout/delete | 装载预设（full / minimal 内置） |
| GET | /inspect?session_id= | 自省快照：工具/技能/命令/系统提示词/会话元 |

## MCP / Git / 文件 / 统计

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /mcp | server 清单 + 两份配置原文（全局 / 工作区） |
| POST | /mcp/save · /mcp/remove · /mcp/reload | 配置维护（save / remove 后自动重连并重建注册表） |
| POST | /mcp/login · /mcp/logout | OAuth 登录登出 |
| GET | /git/status · /git/diff?path= · /git/history?limit= · /git/commit?hash=&path= · /git/file-history?path=&limit= · /git/branches | 读（history / file-history 的 `limit` 缺省 50） |
| POST | /git/stage · /git/unstage · /git/discard · /git/resolve · /git/commit · /git/branch · /git/branch/delete · /git/switch · /git/merge · /git/state · /git/pull · /git/fetch · /git/push · /git/init · /git/remote · /git/message | 写（白名单动作） |
| GET | /files?path= | 文件树 |
| GET | /files/bytes?path= | 单文件字节（base64；文档预览用，上限 32MB） |
| GET | /stats?span=day7\|day30\|all | 用量聚合 |
| GET | /docs · /docs/:name | 内置帮助文档 |

## 内核状态

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | /kernel/status | 就绪状态（`{ready, data_dir}`；无模型配置即 ready=false） |
| GET | /context/breakdown?session_id= | 上下文占用分解 |
| GET | /subagents/roles | 子代理角色预设清单（code / 名称 / 职责 / 工具白名单 / 轮次上限） |

阻塞原因不在 /kernel/status 里：无模型配置时 `ready=false`，
具体原因看 /bootstrap 的 `boot_block_reason`。

## SSE

`GET /events?session_id={sid}`。帧：`id: {seq}` + `event: {name}` + `data: {…}`，
seq 是服务端全局递增序号。

断线对账只有重放一条路：重连时把最后收到的 seq 带回（前端以 `last_event_id`
查询参数，事件源原生重连走 `Last-Event-ID` 请求头），服务端把该序号之后的帧
先补推一遍，再进入正常流。

| 事件 | 载荷要点 | 来源 |
|---|---|---|
| chat:start | run_id, session_id | 内核 |
| chat:delta | kind(text/thinking), delta | 内核 |
| chat:tool_start / chat:tool_end | tool_call_id, name, label?, args / result, is_error, duration_ms | 内核 |
| chat:context | usage{input,output,cached,cache_write,total,context,cost}, context_window, ratio | 内核 |
| chat:compressed | tokens_before | 内核（阈值压缩）/ service（手动压缩） |
| chat:retry | attempt, max, delay_ms | 内核 |
| chat:stopped | run_id | 内核 |
| chat:done | run_id, message | 内核 |
| chat:error | run_id, code, message | 内核 |
| chat:ask / chat:ask_closed | 提问卡（confirm/select/input/editor/问卷）与关闭 | ask_user / plan 功能 |
| chat:notify | level(info/warning/error), text | service |
| chat:status | key, text（空 text 即清除该 key） | goal 功能 |
| chat:widget | key, lines（空行组即清除该 key） | todo / plan 功能 |

载荷字段名是 SSE 线格式，不走 `xxx_` 业务前缀规则——前端按 `data.kind` / `data.name` 读。
事件名与载荷结构体定义在 `wb-domain/src/events.rs`，那是前后端契约的唯一出处。

## 错误响应

业务失败时 HTTP 状态码仍是 200，靠信封里的 `code` 表达：

```json
{ "code": "7003", "message": "工具不存在或已停用", "data": null }
```

完整错误码表见 [ERROR-CODES.md](ERROR-CODES.md)。
