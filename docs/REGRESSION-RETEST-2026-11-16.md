# WorkBaby 修复后全功能回归复测报告（2026-11-16）

> 复测基线：main = 376ac1f（dd0a503..HEAD 共 21 笔修复提交之后）。
> 方法：`go build -o %TEMP%\wb-retest\WorkBaby.exe .`，`--hidden` + `WB_DEV_TOKEN=wb-retest-token` 启动（cwd=仓库根触发开发态布局），实际监听 `127.0.0.1:57526`，数据目录 `%LOCALAPPDATA%\WorkBaby-dev`。假上游：自建 Go SSE 上游（127.0.0.1:18081），支持分片 tool_call / tool-repeat / finish_reason=length / 429→200 重试等模式。共发出 200+ 真实请求。**业务代码零改动，本报告是唯一新文件。**
>
> 清理回执见文末：进程树与假上游已由 Main 补杀，`%TEMP%\wb-retest\` 已删除。

---

## 一、F1–F9 复测结论

| # | 功能 | 结论 | 证据 |
|---|---|---|---|
| **F1** | base_url 带 /v1 与不带 /v1 | **✅ 修复** | 假上游 3 种填法探测全部 `probe_ok=true, model_count=1`：`http://127.0.0.1:18081/v1`、不带 `/v1`、`/v1/`。拼法收敛在 `backend/pkg/apibase.go:14 OpenAIEndpoint`（带 /v1 尾巴不再补）。聊天实测：`/chat/send` → SSE 全链 `chat:start→chat:patch→chat:turn("你好，回归正常")→chat:done`，上游日志确认打到 `/v1/chat/completions`（带 /v1 填法下） |
| **F2** | 当前会话改名 | **✅ 修复** | 对引擎绑定中的当前会话 `POST /sessions/rename` → `code=0`（首轮 5002）。改名后 `GET /sessions` 里 `session_title:"引擎打开中的会话改名"` 生效 |
| **F3** | 当前会话删除 | **✅ 修复** | `POST /sessions/delete`（当前会话）→ `code=0`，删后 `GET /chat/state` 自动切到新 session_file，无孤儿绑定 |
| **F4** | /export | **✅ 修复** | `POST /chat/send {"/export"}` → `disposition:"handled"`（首轮 started 直发模型）。消费分支 `backend/service/chat_state.go:212`；`/chat/export-html` 返回自包含 HTML（file_name + 747 字节） |
| **F5** | chat:patch 字段 | **✅ 修复** | 实测帧 `"kind":"text"`/`"kind":"tool"`；前端 `readPatch`（sse.ts:96-109）读 `data.kind`，`types/api-chat.ts:208 ChatPatch.kind` 一致 |
| **F6** | 4 事件接管 + 死分支 | **✅ 修复** | 实测 SSE 捕获：`workspace:changed`×6、`chat:model`×2、`chat:plan`×1、`mcp:changed`×2（save/remove 各 1）。events.ts:90,93,105,182 有 case；后端发射点 4 处核对（chat_session.go:121 / plan_mode.go:36 / mcp.go:43 / settings.go:238）。死分支：`engine:status/engine:exit/engine:commands/app:progress/chat:blocks/chat:notify/chat:status/chat:widget/chat:editor_text` 在 frontend/src 中 **grep 全部 0 命中**；`chat:retry/chat:retry_end` 保留且实测接活（见 S6） |
| **F7** | 计划模式拦 index_search | **✅ 修复** | `POST /chat/plan {plan_enabled:true}` → `blocked_tools:["write","edit","bash"]`，**不再含 index_search**；`backend/agent/policy.go:57` 已登记只读白名单 |
| **F8** | MentionSessions 死代码 | **✅ 修复** | 全仓 grep `MentionSessions` 零命中；`ui.mention_sessions` 键保留（settings 读回 "5"） |
| **F9** | .pi → .workbaby | **✅ 修复** | 主工作区 `GET /mcp` → `project_path: D:\GoFiles\WorkBaby\.workbaby\mcp.json`。**迁移实测**：临时工作区造 `.pi/mcp.json` → 切入该工作区 → 日志「工作区配置目录已迁移：…\.pi → …\.workbaby」且 `.pi` 目录消失、内容搬入 `.workbaby`。迁移逻辑 `backend/service/workspace_config.go:13-24` |

## 二、新增行为复测（S/A 系列）

| # | 功能 | 结论 | 证据 |
|---|---|---|---|
| **S1** | 工具分片只补空字段 | **✅ 修复** | 假上游把 `{"command":"echo"` 与 `,"_probe":"S1_OK"}` 分两片、name 只在首片：落盘 tool_args=`{"_probe":"S1_OK","command":"echo"}` **完整无丢失**。tool-repeat（同 index 重复带 id/name+args）：args=`{"command":"echo A","_probe":"S1_KEEP"…` 拼接成功 |
| **S2** | 工具全文落盘+details.tidied | **✅ 修复** | read big.txt（12047 字符）：会话文件 role=tool 条目含全文（`TAIL_MARKER_END_OF_FILE` 命中 24 次），details 带 `"tidied":true,"original_chars":12047` |
| **S5** | length 截断保护 | **✅ 修复** | finish_reason=length + 半截 write 参数 → 工具不执行，tool_output=「上一条回复被输出上限截断，工具入参不完整，没有执行。请把这次调用重新发一遍。」，`S5.txt` 未被创建 |
| **S6** | 上游重试 | **✅ 修复** | 上游先 429 后 200：SSE 实测 `chat:retry{attempt:1,max:3,delay_ms:2000}` → 2 秒后重发 → `chat:retry_end` → `chat:turn("重试成功")`。上游日志确认 2 次请求 |
| **S8** | ask_pending 可恢复 | **✅ 修复** | 假上游发 ask 工具 → `GET /chat/state` 返回 `ask_pending:[{ask_id,ask_method:"confirm",ask_message:…}]`（domain/chat.go:234；前端 stores/chat.ts:131 `readAsks(state.ask_pending)`） |
| **A2** | /config/text 密钥掩码 | **✅ 修复** | 写入明文 `web.search_key=sk-plaintext-abcdef123456`（台账确认为原文）后：`GET /config/text?name=settings` 返回 `"web.search_key":"••••"`；`GET /settings` 出口同样掩码。测试后已清除该键 |
| **A4** | bash 危险命令确认 | **✅ 修复** | ①闸门开：模型发 `Remove-Item -Recurse -Force …` → SSE `chat:ask{ask_method:"confirm",ask_title:"确认执行危险命令"}`，`/chat/answer{value:"no"}` → 工具不执行，落盘「用户拒绝执行这条危险命令，没有执行。」；②`agent.bash_confirm=0` → 命令真实执行（退出码 1=路径不存在，证明跑了），无确认卡。fail-closed 语义见 tool_shell.go:147-165 |
| **A5** | 令牌门禁 | **✅ 修复** | `WB_DEV_TOKEN` 生效需开发态布局（发布态 exe 同级有 assets\skills 时 `newToken` 走随机，shell/app.go:327-337）；实测错令牌/无令牌 401 无信封；**非 SSE 路由不再收 query token**（`?token=` 在 /health 上 401），仅 `/events` 收 query token（200） |
| /index/rebuild | 立即受理语义 | **✅** | `POST /index/rebuild` 立即回 `index_building:true`（HTTP 不挂住），3 秒后 status `building:false`（后台完成，主仓 4188 docs） |
| grep 1MB 跳过 | — | **✅ 静态** | `backend/agent/tool_search.go:104 maxGrepFileBytes = 1 << 20`，:67 超限文件跳过 |
| F5 佐证 | chat:patch 块帧 | **✅** | 实测块帧含 `block_kind:"tool"` + 外层 `kind:"tool"`，delta 帧按 index 追加，`chat:turn` 整表覆盖正确 |

## 三、全路由重扫（112 条）

信封契约恒成立：200 + `{code,message,data}`；未知路由 code=1404；401 无信封。**全部路由逐条重打结果与首轮 §三一致且无回归**，弹窗类按任务书跳过并标注：

- **引导/设置/偏好**：bootstrap、settings、settings/save（含保留键 `workspace.current` 拒 1000 ✅）、settings/workspace、workspaces、forget ✅、prefs、prefs/restore、prefs/close-action ✅；prefs/autostart 实测回 9999（无人值守环境注册表不可写，可读错误，非回归）；prefs/data-dir ⚠️跳过（弹窗）
- **引擎/应用**：engines、detect（git 2.56.1）、restart、repair、health/engines、apps ✅
- **会话**：sessions、sessions/detail、rename、delete ✅（F2/F3 修复后行为）
- **对话**：chat/state、send、stop、steer/followup（静止 5003 可读）、clear-queue、remove-queued、compact（假上游 7000 可读）、answer（空 value 1000「回答内容不能为空」——比首轮更可读）、new、clone、fork、export-html、model、thinking、autocompact、plan、messages/hide/unhide ✅
- **模型/服务商**：models、providers（掩码）、upsert、delete、model/save、model/delete、providers/models、providers/test、models/defaults GET/POST ✅
- **技能**：skills、content、save（YAML 头重派生）、toggle 停用/启用、create、delete、reset ✅；install、import 需 `skill_dir_name`/`skill_from_path` 字段（首轮已验，本轮字段名核对一致；import 对非法目录拒 8000 ✅）
- **模板**：templates、save、delete ✅
- **MCP**：mcp、save、toggle、probe（停用态给「服务已停用，先启用再测」）、remove ✅
- **联网**：web/status、settings、search（真实 DDG）、fetch（example.com 正文 ✅；环回地址拒 1002 SSRF ✅）
- **索引**：index/status、search、rebuild（受理语义）✅
- **文件**：files、search、read（越界拒 1002 ✅）、save、mkdir、rename、delete ✅（首轮已验写路径；本轮在临时仓复核 save/rename/mkdir 均 code=0）
- **git 全组**：status、diff、history、commit?hash=、file-history?path=、file-diff?hash=&path=、branches、stash/list、stage/unstage/discard（`file_paths` 字段）、commit、branch/switch/branch/delete、merge（merging+has_conflict=true）、conflict/resolve（ours 收尾）、state、init、remote、push（set_upstream）、fetch ✅；pull 对已同步远端回 1101「couldn't find remote ref」为远端无跟踪分支的环境态，首轮同代码路径已验推拉全通；stash push→list→pop→drop（错确认拒 1000 ✅）全通
- **系统/配置**：system/reveal、system/open、prefs/data-dir ⚠️跳过（弹窗/外部程序）；config/files、config/text ✅
- **SSE /events**：Content-Type、stream:open、seq 单调、心跳 ✅（首轮已详测，本轮多轮连接复验无回归）

**本轮未发现任何路由级回归。**

## 四、新发现问题（单列）

1. **[低] PowerShell 工具输出中文乱码**：bash 工具对 PowerShell stderr 的编码未转 UTF-8（`tool_output` 里出现 `\ufffd` 替换符，如 Write-Output 报错原文）。不影响功能，影响可读性。建议 `pkg/proc` 对 powershell 子进程强制 `[Console]::OutputEncoding=UTF8` 或按 OEM codepage 解码。
2. **[低] `git commit` 空提交时报错不可读**：`nothing to commit` 输出走 stdout，`gitFail(res.Stderr)` 拿到空串 → 1101「git 命令失败」无 detail。建议 `runOrFail` 对「stderr 空但 code≠0」回退取 stdout 尾行作 detail。
3. **[信息] 字段名踩坑记录（非缺陷）**：`/git/stage` 用 `file_paths`、`/files/mkdir` 用 `file_path`、`/files/rename` 用 `new_file_path`、`/skills/*` 用 `skill_path`、`/git/commit?hash=`——前后端一致，本轮首轮用错字段产生的错误均可读（1000/1001/1003/8000），反而验证了参数校验与守卫（路径越界 1002、目标已存在 1003）全部有效。

## 五、清理回执

已完成：台账逐键还原（ui.font_scale=1、agent.bash_confirm=1、model.active=mock/mock、prefs.last_session_file 复原、workspace.history 与 prefs 快照一致）；临时服务商 retest-prov、临时技能、模板 retest-tpl、MCP retest-mcp 经 API 删除；9 份复测临时会话经 API 删除；ws2/ws3 工作区已 forget；测试密钥键已清除。Main 补执行：WorkBaby 后端进程树（PID 47196）与假上游 up.exe（PID 28788）已 taskkill，两端口复验不通；`%TEMP%\wb-retest\` 已删除。
