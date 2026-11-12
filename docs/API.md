# API 契约

- baseURL：`http://127.0.0.1:{port}/api/v1`。端口与令牌由 Wails 绑定的
  `Endpoint()` 回填（`base_url` 已含 `/api/v1`）。
- 鉴权：每个请求带 `X-Desktop-Token` 头；SSE 用 `?token=` 查询参数
  （EventSource 带不了自定义头）。令牌每次启动随机，不匹配回 HTTP 401。
- CORS：WebView2 的页面源（`wails.localhost` 虚拟域）与开发态 vite 端口对回环
  服务就是跨源。OPTIONS 预检不带令牌、绕过鉴权门直接回 204 与允许头；
  允许源限 `wails.localhost` 与 localhost / 127.0.0.1（真正的门禁是令牌）。
- **只允许 GET / POST**；写操作语义靠路径段表达（`/sessions/delete`）。
- **恒回 200**，业务失败看信封 `code`：

```json
{ "code": "0", "message": "", "detail": "可选原话", "data": { } }
```

`code` 是数字字符串，`"0"` 成功；码表见文末「错误码」一节。
用户可读文案在 `message`，`detail` 给诊断面板。未知接口回 `1404`。
鉴权失败是唯一例外：回 401 且不带信封——那不是业务失败，是根本没进门。

## 引导 / 设置 / 偏好

| 端点 | 方法 | 入参 | 出参 data |
|---|---|---|---|
| `/bootstrap` | GET | — | 首屏总包：版本、契约版本、引擎身份、能力表、工作区、设置全量、目录布局、工作区历史、运行时状态、偏好 |
| `/settings` | GET | — | 全部键值 |
| `/settings/save` | POST | 键值对象 | 保存后的全量键值 |
| `/settings/workspace` | POST | `{ workspace }` | `{ workspace }`；切工作区会让会话绑定失效 |
| `/workspaces` | GET | — | `{ workspace, workspace_rows, active_engine }`；每行带目录是否还在、是否 git 仓库、当前分支、未提交改动数、会话数 |
| `/settings/workspace/forget` | POST | `{ workspace_path }` | 剩余历史（与 `/workspaces` 同形） |
| `/logs` | GET | — | 当日日志尾部 |
| `/prefs` | GET | — | 自启 / 恢复会话 / 关窗行为 / 数据目录等 |
| `/prefs/autostart` | POST | `{ enabled }` | 更新后的 prefs |
| `/prefs/restore` | POST | `{ enabled }` | 更新后的 prefs |
| `/prefs/close-action` | POST | `{ close_action }` | 更新后的 prefs |
| `/prefs/data-dir` | POST | — | 打开数据目录 |

## 引擎 / 应用 / 健康

| 端点 | 方法 | 说明 |
|---|---|---|
| `/engines` | GET | 引擎身份 + 目录布局 + 运行快照 + 运行时状态 |
| `/engines/detect` | POST | 重新探测外部工具链，刷新运行时状态 |
| `/engines/restart` | POST | 中止当前运行（内置引擎没有需要重启的进程） |
| `/engines/repair` | POST | 重新探测全部外部依赖（等价于一次全面刷新） |
| `/health/engines` | GET | 健康面板：会话 / 技能 / MCP 计数、内存占用、运行时状态 |
| `/apps` | GET | 外部工具链检测行：git / node / python，带版本、路径与不可用原因 |

**引擎在进程内**：`installed` 恒为真，没有「装没装」这件事，也没有手动指定入口的动作。

## 会话

| 端点 | 方法 | 说明 |
|---|---|---|
| `/sessions` | GET | `?workspace=&q=`；按工作区编码目录扫描，`updated_at` 降序；`q` 非空时按标题与正文过滤（大小写不敏感） |
| `/sessions/detail` | GET | `?path=`；清单行 + 逐条消息 |
| `/sessions/delete` | POST | `{ path }` |
| `/sessions/rename` | POST | `{ session_path, session_title }`；向会话文件追加 `session_info` 条目。该会话正被打开时回 `5002`——两路同时追加会写坏唯一真相 |

## 聊天

| 端点 | 方法 | 说明 |
|---|---|---|
| `/chat/state` | GET | `?session=`；一次给出界面画聊天页要用的全部事实：会话文件、工作区、流式态、当前模型、思考档、排队、历史轮、待办、计划模式、斜杠命令、能力表 |
| `/chat/send` | POST | `{ session_file, message, attachments }` → `{ run_id, disposition }`。`disposition` 是这句话的处理方式：`started` 才真起一轮（有 `run_id`）；`queued` 是排在前一轮后面。界面据此决定要不要进入流式态——误登记会让界面永久停在「正在回答」 |
| `/chat/stop` | POST | 中止当前运行 |
| `/chat/steer` / `followup` | POST | `{ session_file?, message, attachments? }` 插话 / 排队追加 |
| `/chat/clear-queue` | POST | 清空队列，返回被清的话术 |
| `/chat/remove-queued` | POST | `{ queue_kind, queue_index }`；单条移除（引擎侧是本进程内的切片，所以是真删这一条） |
| `/chat/compact` | POST | `{ session_file?, instructions? }` 手动压缩；会话级动作先按 `session_file` 对齐绑定 |
| `/chat/answer` | POST | `{ session_file, ask_id, value?, confirmed?, cancelled?, answers? }` 回答提问卡（确认 / 选择 / 输入 / 编辑器） |
| `/chat/new` / `clone` | POST | 新建 / 克隆会话，返回 `session_file` |
| `/chat/export-html` | POST | `{ session }` → `{ file_name, content }` |
| `/chat/model` | POST | `{ session_file, provider_name, model_id }` 会话内切换模型 |
| `/chat/thinking` | POST | `{ session_file, thinking_level }` 切换思考档 |
| `/chat/autocompact` | POST | `{ session_file, auto_compaction }` 自动压缩开关 |
| `/chat/plan` | POST | `{ plan_enabled }`；开关只读模式，返回生效状态与被拦下的工具清单 |
| `/messages/hide` / `unhide` | POST | `{ session_file, turn_id }`；隐藏台账存设置文件，会话文件不动 |

附件形状：`{ kind: 'file', file_path }` 走文本引用（路径追加进正文，让模型自己用读工具取）；
`{ kind: 'image', image_base64, image_mime }` 以 base64 进内容块——这是唯一的二进制通道。

## 模型与服务商

| 端点 | 方法 | 说明 |
|---|---|---|
| `/models` | GET | 全部服务商的模型平铺（对话页模型下拉的数据源） |
| `/providers` | GET | 服务商视图（含凭据状态，密钥恒为掩码） |
| `/providers/upsert` | POST | `{ provider_name, base_url, api_protocol, auth_kind, api_key, models }`；`api_key` 空串 = 保留原值 |
| `/providers/delete` | POST | `{ provider_name }`；服务商与其模型一并清 |
| `/providers/model/save` | POST | `{ provider_name, model }`；单条模型增改，按 `model_id` 定位（存在即改），服务商必须已存在 |
| `/providers/model/delete` | POST | `{ provider_name, model_id }`；删除服务商下一条模型 |
| `/providers/models` | POST | 按 API 协议拉远端模型清单 |
| `/providers/test` | POST | 连通性测试，回 `{ probe_ok, model_count, probe_message }` |

**协议**：只实现 OpenAI 兼容（`/chat/completions`）。
`api_protocol` 字段保留给将来接别的协议，目前认 `openai`。

## 技能

| 端点 | 方法 | 说明 |
|---|---|---|
| `/skills` | GET | 磁盘上的技能（builtin / bundled / managed / project）+ `available`（推荐清单里还没装的）；带 `recommended_total` / `recommended_installed` |
| `/skills/install` | POST | `{ skill_dir_name }`；把一条推荐技能装进技能区（已存在则拒） |
| `/skills/content` | GET | `{ skill_path }` 读 SKILL.md 正文 |
| `/skills/save` | POST | `{ skill_path, file_text }` 写 SKILL.md |
| `/skills/toggle` | POST | `{ skill_path, skill_enabled }`；实现是目录搬进 / 搬出禁用区 |
| `/skills/create` | POST | `{ skill_name, skill_desc, skill_body }` 新建托管技能 |
| `/skills/import` | POST | `{ skill_from_path }` 数据目录外整份复制进托管区（含 SKILL.md 校验） |
| `/skills/delete` | POST | `{ skill_path }` 删除整个技能目录 |
| `/skills/reset` | POST | `{ skill_path }`；用随包原件覆盖本地改动 |

**技能路径必须是 `SKILL.md` 文件路径**，动作对象是它所在的目录：
深层任意文件算出的目录会把别的技能搬走；传目录路径时删除一律拒——
目录路径正好是技能根的话，删一次就是清空全部内置技能。

**项目技能归那个仓库管**：工作区 `.pi\skills\` 下的技能，壳里的开关与删除对它无效。

## MCP

| 端点 | 方法 | 说明 |
|---|---|---|
| `/mcp` | GET | 用户级 + 项目级合并清单；行里带 `mcp_env_text` / `mcp_work_dir` / `mcp_timeout`（秒）与作用域 |
| `/mcp/save` | POST | 落 `data\mcp.json`；`command_text` 拆 argv；`mcp_scope: project` 写工作区 `.pi\mcp.json`（需已选工作区），默认用户级；编辑整份继承旧条目（手配的 headers 不丢） |
| `/mcp/toggle` | POST | `{ server_name, mcp_enabled }` 开关 |
| `/mcp/probe` | POST | `{ server_name }`；完成一次 MCP initialize 握手（stdio 拉真进程 / http 发真请求），回 `probe_ok` / `probe_message` / `probe_tools` |
| `/mcp/remove` | POST | 只删界面显示的那一条，项目级遮蔽用户级时不连带删除 |

## 联网

| 端点 | 方法 | 说明 |
|---|---|---|
| `/web/status` | GET | 联网状态：当前搜索服务、是否已配密钥、可选服务种类 |
| `/web/settings` | POST | `{ web_kind, web_api_key }`；`web_api_key` 空串 = 保留原值（界面回传的是掩码） |
| `/web/search` | POST | `{ search_query, search_limit }`；返回标题、地址、摘要 |
| `/web/fetch` | POST | `{ fetch_url }`；HTML 转可读文本，PDF 抽文字，JSON 原样 |

搜索服务：`duckduckgo`（免密钥，默认）、`brave`、`exa`、`tavily`。后三个需要密钥。

**地址安全**：只允许 http/https；主机名指向环回、私网、链路本地或 CGNAT 段一律拒。
校验做两遍——`ValidateURL` 看字面，`safeDial` 在真正连接时复核解析出的 IP。
重定向逐跳校验：只查首地址的话，公网地址跳到 `127.0.0.1` 就绕过去了。

## 工作区预索引

| 端点 | 方法 | 说明 |
|---|---|---|
| `/index/status` | GET | 索引状态：条目数、是否已建、建索引时刻 |
| `/index/search` | POST | `{ search_query, search_limit }`；返回文件与符号命中 |
| `/index/rebuild` | POST | 重建当前工作区的索引 |

排序分 = 文本相关度 + frecency（频次 × 时近）。相关度分档：
符号名完全匹配 100 > 符号名包含 60 > 文件名完全匹配 50 > 文件名前缀 40 > 路径包含 30 > 正文包含 10。
同分时按路径排，保证结果顺序稳定。

## 文件 / Git / 系统

| 端点 | 方法 | 说明 |
|---|---|---|
| `/files` | GET | `?path=` 工作区内列目录（目录优先，≤500 条） |
| `/files/search` | GET | `?query=&limit=`；按名递归搜索，跳过 node_modules/.git/dist |
| `/files/read` | GET | 文本读取（1MB 截断，二进制给占位说明） |
| `/files/save` | POST | `{ file_path, file_text }` 新建 / 覆盖同一入口；父目录不存在一并建出；5MB 上限 |
| `/files/mkdir` | POST | `{ file_path }` 新建目录（多级一并建）；已存在即拒 |
| `/files/delete` | POST | `{ file_path }` 删除文件或目录（递归）；工作区根本身拒删 |
| `/files/rename` | POST | `{ file_path, new_file_path }` 改名或移动；源必须存在、目标必须不存在 |
| `/system/reveal` / `open` | POST | 资源管理器定位 / 系统默认程序打开 |
| `/config/files` | GET | 可查看的配置文件清单（白名单） |
| `/config/text` | GET | 配置文件原文；只认白名单里的名字 |

路径约定：全部接受工作区内绝对路径；相对路径（前端的 `/` 斜杠约定）按工作区根解析——
不按进程 CWD，`../` 折算后仍出不了工作区根。

`auth.json` 不在配置清单里：密钥只在服务商页以掩码流转。

| 端点 | 方法 | 说明 |
|---|---|---|
| `/git/status` / `diff` / `history` / `file-history` / `file-diff` / `branches` | GET | 工作区 git 只读面：状态、差异、历史（按条与按文件）、分支 |
| `/git/stage` / `unstage` / `discard` / `resolve` | POST | 暂存 / 取消暂存 / 丢弃改动 / 标记冲突已解决 |
| `/git/conflict/resolve` | POST | `{ file_path, resolve_side }`；`resolve_side` 为 `ours` / `theirs` / 空串（我改完了，只 `git add`）。按指定口径取一边后自动暂存 |
| `/git/commit` | POST | `{ commit_message, is_amend }` 提交或修补 |
| `/git/branch` / `branch/delete` / `switch` / `merge` / `state` / `init` / `remote` | POST | 分支增删切换、合并（可选 rebase）、合并中 continue / abort、初始化、加远程 |
| `/git/push` / `pull` / `fetch` | POST | 推拉取；`push` 带 `confirm` 表达「覆盖远程」的二次确认 |
| `/git/stash/list` / `push` / `pop` / `drop` | POST | stash 四件套；`drop` 的 `confirm` 传 stash ref 防误删 |

git 全部走系统装的 git.exe，凭证走 Git Credential Manager（落 Windows 凭据管理器），
终端交互问询禁用——需要登录时立即失败回 `1102` 并给下一步指引，而不是卡满超时。

## SSE 事件词汇（`/events`）

帧格式 `{ seq, event, data, sent_at }`；聊天帧带 `engine_kind: 'native'` 与 `run_id`。
连接建立时先补一帧 `stream:open`（带当前 seq，不占新号），前端据此决定要不要带 `?since=` 补帧。
心跳是注释行，不占 seq。

| event | 载荷要点 | 用途 |
|---|---|---|
| `stream:open` | `seq`（hub 当前值，不占新号） | 连接建立；前端以它为对齐基准 |
| `chat:start` | `started_at` | 一轮开始；前端进入流式态并起秒表 |
| `chat:live` | `phase('thinking'/'text'/'tool'), tool_name, output_tokens` | 活动指示 |
| `chat:patch` | `mode('delta'/'block'), index, kind, delta?/block?` | 增量追加或整块替换 |
| `chat:turn` | `user_message, turn_blocks` | 回合快照（权威块表） |
| `chat:turn_end` | `turn_usage{input_tokens, output_tokens, cache_read_tokens, cache_write_tokens}` | 回合用量 |
| `chat:queue` | `steering[], follow_up[]` | 排队变化 |
| `chat:todos` | `todos[]` | 待办清单实时更新 |
| `chat:ask` | 提问卡（`ask_id/ask_method/ask_title/ask_options/ask_timeout_ms`…） | 结构化提问 |
| `chat:plan` | `plan_enabled, session_id` | 计划模式开关 |
| `chat:compaction` | `state('start'/'done')` | 压缩中 |
| `chat:error` | `message` | 运行失败（含上游报错） |
| `chat:done` / `chat:settled` | `aborted` | 回合收尾；前端随后对账 `chat/state` |
| `chat:model` | `provider_name, model_id` | 模型切换 |
| `engine:detected` | `runtime` | 外部工具链重新探测完成 |
| `engine:ready` | — | 引擎就绪 |
| `app:close_requested` | — | 首次按窗口关闭，前端弹一次「收进托盘还是退出」 |
| `skill:status` | `skill_dir_name, skill_enabled?/skill_reset?/skill_deleted?/skill_imported?` | 技能落位、开关与清理变化 |
| `session:info` / `session:title` | — | 会话清单变化 / 标题变化 |
| `workspace:changed` | `workspace` | 工作区切换 |

**丢帧不靠服务端事件**：前端逐帧跟踪 `seq`，发现跳号（`seq > last + 1`）就对 `chat/state`
拉一次快照对账。

## 错误码

码值为数字字符串，随信封 `code` 下发；`"0"` 为成功，恒 HTTP 200。
常量唯一落点是 `backend/pkg/codes.go`——**新增码先改本文档再进常量**。
`message` 是用户直接可读的文案，`detail` 带原话（stderr / 路径 / 上游响应）。

段位：1000–1999 通用/文件/路径/git；2000–2999 持久化；3000–3999 服务商/模型；
5000–5999 会话运行；7000–7999 外部进程；8000–8999 技能；9100–9199 MCP。

### 通用 / 文件 / 路径 / git（1000–1999）

| 码 | 常量 | 场景 |
|---|---|---|
| 1000 | `CodeBadRequest` | 入参不合法 |
| 1001 | `CodeFileNotFound` | 文件或目录不存在 |
| 1002 | `CodePathOutside` | 路径越界：不在允许的根目录内，或地址指向内网 |
| 1003 | `CodeFileIO` | 文件读写失败 |
| 1100 | `CodeGitMissing` | git 不可用 |
| 1101 | `CodeGitFailed` | git 命令失败（原话在 detail） |
| 1102 | `CodeGitAuth` | git 凭证被拒或无法登录（提示先登录一次或换 token） |
| 1404 | `CodeUnknownRoute` | 未知路由（`NoRoute` 兜底） |

### 配置 / 持久化（2000–2999）

| 码 | 常量 | 场景 |
|---|---|---|
| 2002 | `CodeStorage` | 本地存储文件或数据库操作失败 |

### 服务商 / 模型（3000–3999）

| 码 | 常量 | 场景 |
|---|---|---|
| 3000 | `CodeProviderBad` | 服务商配置不合法（缺 Base URL / 名称 / 密钥等） |
| 3001 | `CodeNoProvider` | 还没有选择模型 |

> 连通性探测（拉模型清单 / 测试配置）的结果是 `probe_ok` 载荷而非错误码：
> 「上游不通」是探测的正常结果之一，走信封错误会让界面没法把原因摆出来。

### 会话运行（5000–5999）

| 码 | 常量 | 场景 |
|---|---|---|
| 5000 | `CodeSessionBind` | 未绑定到会话或工作区 |
| 5001 | `CodeSessionIO` | 会话文件读写失败 |
| 5002 | `CodeEngineBusy` | 有任务正在跑（改名 / 切会话 / 新建 / 压缩时被拒） |
| 5003 | `CodeNotFound` | 目标不存在（会话 / 条目 / 模型 / 队列项等） |

### 外部进程（7000–7999）

| 码 | 常量 | 场景 |
|---|---|---|
| 7000 | `CodeProvision` | 外部命令执行失败（git / 联网 / 子进程） |

### 技能（8000–8999）

| 码 | 常量 | 场景 |
|---|---|---|
| 8000 | `CodeSkillBad` | 技能路径越界 / 名称非法 / 已存在 |
| 8001 | `CodeSkillSave` | 技能保存 / 开关 / 复制失败 |
| 8002 | `CodeSkillDelete` | 技能删除失败 |

### MCP（9100–9199）

| 码 | 常量 | 场景 |
|---|---|---|
| 9100 | `CodeMcpRead` | `mcp.json` 读取失败 |
| 9101 | `CodeMcpWrite` | `mcp.json` 写入失败 / 条目不存在 |

### 兜底

| 码 | 常量 | 场景 |
|---|---|---|
| 9999 | `CodeInternal` | 未归类的内部错误（原话进 detail 与日志） |

### 前端本地码

| 码 | 场景 |
|---|---|
| `8003` | 连不上后端 / 响应超时（axios 层映射，不来自后端） |
