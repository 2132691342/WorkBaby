# API 契约 · API

- baseURL：`http://127.0.0.1:{port}/api/v1`。端口与令牌由 Wails 绑定的
  `Endpoint()` 回填（`base_url` 已含 `/api/v1`）。
- 鉴权：每个请求带 `X-Desktop-Token` 头；SSE 用 `?token=` 查询参数
  （EventSource 带不了自定义头）。令牌每次启动随机，不匹配回 HTTP 401。
- CORS：WebView2 的页面源（`wails.localhost` 虚拟域）与开发态 vite 端口对回环
  服务就是跨源。OPTIONS 预检不带令牌、绕过鉴权门直接回 204 与允许头；
  允许源限 `wails.localhost` 与 localhost / 127.0.0.1（真正的门禁是令牌）。
- 只允许 GET / POST；写操作语义靠路径段表达（`/sessions/delete`）。
- **恒回 200**，业务失败看信封 `code`：

```json
{ "code": "0", "message": "", "detail": "可选原话", "data": { } }
```

`code` 是数字字符串，`"0"` 成功；码表见 [ERROR-CODES.md](ERROR-CODES.md)。
用户可读文案在 `message`，`detail` 给诊断面板。未知接口回 `1404`。

## 引导 / 设置 / 偏好

| 端点 | 方法 | 入参 | 出参 data |
|---|---|---|---|
| `/bootstrap` | GET | — | 首屏总包：版本、引擎身份、能力表、激活引擎、工作区、设置全量、目录布局、工作区历史、计数 |
| `/settings` | GET | — | 全部键值 |
| `/settings/save` | POST | 键值对象 | 保存后的全量键值 |
| `/settings/workspace` | POST | `{ workspace }` | `{ workspace }`；切工作区会重建引擎绑定 |
| `/workspaces` | GET | — | `{ workspace, workspace_rows, active_engine }`；每行带目录是否还在、是否 git 仓库、当前分支、未提交改动数、会话数 |
| `/settings/workspace/forget` | POST | `{ workspace_path }` | 剩余历史（与 `/workspaces` 同形） |
| `/logs` | GET | — | `{ lines }` 当日日志尾部 |
| `/prefs` | GET | — | 自启 / 恢复会话 / 关窗行为 / 数据目录等 |
| `/prefs/autostart` `restore` `close-action` | POST | `{ enabled }` / `{ action }` | 更新后的 prefs |
| `/prefs/data-dir` | POST | — | 打开数据目录 |

## 引擎与应用

| 端点 | 方法 | 说明 |
|---|---|---|
| `/engines` | GET | 引擎身份 + 布局 + 存活实例 + 探测时间戳 |
| `/engines/detect` | POST | 重新探测版本，刷新时间戳并广播 |
| `/engines/restart` | POST | 杀 RPC 子进程，下次用到自动重拉 |
| `/engines/repair` | POST | 修复运行时：停引擎 → 三个内置运行时删掉重解压 → 重钉 npmCommand；进度走 `app:progress`，回 `repair_problems` |
| `/health/engines` | GET | 健康面板：安装态、运行态、会话/技能/插件/MCP 计数 |
| `/apps` | GET | 应用行：pi 本体 + 三个内置运行时（安装态 / 版本 / 可执行路径） |
| `/apps/install` `update` `uninstall` | POST | `{ engine: 'pi' }`；内置 npm 全程经 SSE `app:progress` 报进度 |

## 会话

| 端点 | 方法 | 说明 |
|---|---|---|
| `/sessions` | GET | `?workspace=&q=`；按工作区编码目录扫描，`updated_at` 降序；`q` 非空时按标题与正文过滤（大小写不敏感） |
| `/sessions/detail` | GET | `?path=`；清单行 + 逐条消息 |
| `/sessions/delete` | POST | `{ path }` |
| `/sessions/rename` | POST | `{ session_path, session_title }`；向会话文件追加 `session_info` 条目。该会话正被引擎打开时回 `5002`——两路同时追加会写坏唯一真相 |

## 聊天

| 端点 | 方法 | 说明 |
|---|---|---|
| `/chat/state` | GET | `?session=`；会话绑定 + 引擎实况 + JSONL 回放的历史轮 + 待办（`todos`，活动分支最近一次 todo 工具调用派生）+ 能力表 |
| `/chat/send` | POST | `{ engine, session, message, attachments }` → `{ run_id }` |
| `/chat/stop` | POST | 中止当前运行 |
| `/chat/steer` / `followup` | POST | 插话 / 排队追加 |
| `/chat/clear-queue` | POST | 清空两队，返回被清的话术 |
| `/chat/remove-queued` | POST | `{ queue_kind, queue_index }`；单条移除。引擎只有整队清空，壳用「取出全队 → 摘一条 → 其余按原序重投」实现；插队重投失败降级为追加，话术不丢。返回移除后的两队 |
| `/chat/compact` | POST | `{ instructions? }` 手动压缩 |
| `/chat/answer` | POST | `{ session, request }` 回答提问板（确认 / 选择 / 输入 / 编辑器） |
| `/chat/new` / `clone` | POST | 新建 / 克隆会话，返回 `session_file` |
| `/chat/export-html` | POST | `{ session }` → `{ file_name, content }` |
| `/chat/model` / `thinking` / `autocompact` | POST | 会话内切换模型 / 思考档 / 自动压缩 |
| `/messages/hide` / `unhide` | POST | `{ session, hid }`；隐藏台账存壳存储文件，会话文件不动 |

附件形状：`{ kind: 'file', file_path }` 走文本引用；`{ kind: 'image', image_base64, image_mime }`
以 base64 进引擎 `images` 字段——这是唯一的二进制通道。

## 模型与服务商

| 端点 | 方法 | 说明 |
|---|---|---|
| `/models` | GET | 自定义服务商的模型平铺（按服务商名排序），对话页模型下拉的数据源 |
| `/providers` | GET | 自定义 + 内置服务商合并视图（含凭据状态，密钥恒为掩码） |
| `/providers/upsert` | POST | `{ name, provider }`；写 models.json + auth.json；`api_key` 空串=保留原值 |
| `/providers/delete` | POST | 服务商与凭据一并清 |
| `/providers/model/save` | POST | `{ provider_name, model }`；单条模型增改，按 `model_id` 定位（存在即改），服务商必须已存在 |
| `/providers/model/delete` | POST | `{ provider_name, model_id }`；删除服务商下一条模型 |
| `/providers/models` | POST | 按 API 协议拉远端模型清单（openai / anthropic / google 三种） |
| `/providers/test` | POST | 连通性测试，回 `{ ok, model_count, message }` |

## 技能 / 插件 / MCP

| 端点 | 方法 | 说明 |
|---|---|---|
| `/skills` | GET | 内置（builtin / bundled）+ 自建（managed）+ 项目（project）；带 `builtin_total` / `builtin_installed` 进度 |
| `/skills/content` / `save` | GET/POST | 读写 SKILL.md（路径白名单校验） |
| `/skills/toggle` | POST | `{ path, enabled }`；实现是目录搬进/搬出禁用区 |
| `/skills/reset` | POST | `{ path }`；内置技能用随包原件覆盖本地改动 |
| `/skills/create` / `delete` | POST | 新建托管技能 / 删除 |
| `/skills/import` | POST | `{ skill_from_path }` 沙箱外目录整份复制进托管区（含 SKILL.md 校验） |
| `/plugins` | GET | 内置清单 ∪ `settings.json` 的 `packages` 声明；带安装进度、`plugin_upgradable` 与失败原因 |
| `/plugins/install` | POST | `{ source }` 缺 `npm:` 前缀自动补；经内置 npm 装进沙箱 |
| `/plugins/retry` / `retry-all` | POST | 单个 / 全部重试内置插件安装 |
| `/plugins/uninstall` | POST | `{ plugin_name }`；撤声明 + 删包目录（先停引擎） |
| `/plugins/toggle` | POST | `{ plugin_name, enabled }`；只改 `packages` 条目（`autoload:false` = 停用） |
| `/plugins/upgrade` | POST | `{ plugin_name }`；内置按清单 pin 重装，用户插件按声明串重装（先停引擎） |
| `/plugins/upgrade-all` | POST | 只升版本与清单不符的内置插件；回 `{ installed, failed }` |
| `/plugins/check-updates` | POST | 逐包问 npm registry 的 latest，回一份带 `plugin_latest` 的新清单；离线时字段留空 |
| `/mcp` | GET | 用户级 + 项目级合并清单；行里带 `mcp_env_text` / `mcp_work_dir` / `mcp_timeout`（秒） |
| `/mcp/save` / `toggle` / `remove` | POST | 落 pi 的 mcp.json；`command_text` 拆 argv；`mcp_scope: project` 写工作区 `.pi\mcp.json`（需已选工作区），默认用户级；编辑整份继承旧条目（含 `mcp_timeout`，手配的 headers / oauth 不丢）；`remove` 只删界面显示的那一条，项目级遮蔽用户级时不连带删除 |
| `/mcp/probe` | POST | `{ server_name }`；完成一次 MCP initialize 握手（stdio 拉真进程 / http 发真请求），回 `probe_ok` / `probe_message` / `probe_tools` |

**插件身份是包名不是来源串**：`pi:<name>@<ver>` 里的版本会变，包名不会。
启停写对象形态 `{ source, autoload: false }`，启用写回裸来源串——这是 pi 自己的表达法。

## 文件 / Git / 系统

| 端点 | 方法 | 说明 |
|---|---|---|
| `/files` | GET | `?path=` 工作区内列目录（目录优先，≤500 条） |
| `/files/search` | GET | `?query=&limit=`；按名递归搜索，跳过 node_modules/.git/dist |
| `/files/read` | GET | 文本读取（1MB 截断，二进制给占位说明） |
| `/files/save` | POST | `{ file_path, file_text }` 新建/覆盖同一入口；父目录不存在一并建出；5MB 上限 |
| `/files/mkdir` | POST | `{ file_path }` 新建目录（多级一并建）；已存在即拒 |
| `/files/delete` | POST | `{ file_path }` 删除文件或目录（递归）；工作区根本身拒删 |
| `/files/rename` | POST | `{ file_path, new_file_path }` 改名或移动；源必须存在、目标必须不存在 |

路径约定：全部接受工作区内绝对路径；相对路径（前端的 `/` 斜杠约定）按工作区根解析——
不按进程 CWD，`../` 折算后仍出不了工作区根。
| `/git/*` | GET/POST | status / diff / history / commit / file-history / file-diff / branches / stash(list,push,pop,drop) / stage / unstage / discard / resolve / commit / branch(+delete) / switch / merge / state / init / remote / push / pull / fetch；全部走内置 git.exe，凭证问询禁交互（走 GCM，失败回 1102） |
| `/system/reveal` / `open` | POST | 资源管理器定位 / 系统默认程序打开 |
| `/config/files` / `text` | GET | 引擎配置原文（settings/models/mcp/auth/AGENTS + 项目级），白名单校验 |

## SSE 事件词汇（`/events`）

帧格式 `{ seq, event, data, sent_at }`；聊天帧带 `engine_kind: 'pi'` 与 `run_id`。

| event | 载荷要点 | 用途 |
|---|---|---|
| `stream:open` | `seq`（hub 当前值，不占新号） | 连接建立；前端以它为对齐基准决定要不要带 `?since=` 补帧 |
| `chat:start` | `started_at` | 一轮开始；前端进入流式态 |
| `chat:live` | `phase('thinking'/'text'/'tool'), tool_name, output_tokens` | 活动指示 |
| `chat:patch` | `mode('delta'/'block'), index, kind, delta?/block?` | 增量追加或整块替换 |
| `chat:blocks` | `blocks` | 权威块表整体替换（message_end 后） |
| `chat:turn` | `user_message, blocks` | 回合快照 |
| `chat:turn_end` | `usage{input,output,cacheRead,cacheWrite,costUSD}` | 回合用量 |
| `chat:queue` | `steering[], follow_up[]` | 排队变化 |
| `chat:ask` | 提问卡（`ask_id/ask_method/ask_title/ask_options/ask_timeout_ms`…） | 引擎对话框 |
| `chat:notify` | `level, message` | 引擎通知（错误级弹全局提示，其余进行内提示） |
| `chat:status` / `chat:widget` | `key, text` / `key, lines[]` | 状态条 / 引擎小组件 |
| `chat:retry` / `chat:retry_end` | `attempt, max, delay_ms` | 自动重试进度 |
| `chat:compaction` | `state('start'/'done')` | 压缩中 |
| `chat:error` | `message` | 运行失败（含模型报错） |
| `chat:done` / `chat:settled` | — | 回合收尾；前端随后对账 `chat/state` |
| `chat:editor_text` | `text` | 引擎反向预填输入框 |
| `engine:status` / `ready` / `exit` / `detected` | — | 引擎状态变化 |
| `engine:commands` | `commands[]` | 斜杠命令清单更新 |
| `app:progress` | `kind, action, phase('start'/'line'/'done'/'fail'), plugin_name?, line?, message?` | 安装 / 解压 / 插件进度 |
| `app:close_requested` | `close_action` | 首次按窗口关闭，前端弹一次「收进托盘还是退出」 |
| `plugin:status` | `plugin_name?` | 插件安装态变化（无字段 = 整体刷新） |
| `skill:status` | `skill_dir, enabled?/reset?/deleted?/copied?` | 技能落位与开关变化 |
| `session:info` / `session:title` | — | 会话清单变化 / 标题变化 |
| `workspace:changed` | `workspace` | 工作区切换 |

丢帧不靠服务端事件：前端逐帧跟踪 `seq`，发现跳号（`seq > last + 1`）就对 `chat/state`
拉一次快照对账。
