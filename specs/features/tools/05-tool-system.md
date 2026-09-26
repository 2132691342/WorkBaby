# 05 · 工具契约与注册表

## 1. 工具契约

```go
type Tool interface {
	Name() string
	Description() string
	Schema() ToolSchema                      // JSON Schema（draft 2020-12）
	RiskLevel() RiskLevel                    // readonly / write_local / exec / network / destructive
	Execute(ctx context.Context, args json.RawMessage) ToolResult
}

type ToolResult struct {
	Content string
	Data    map[string]any      // 结构化结果（前端按类型渲染来源卡等）
	Err     error               // 执行故障
	Meta    map[string]string   // 元数据（cwd / same_failure_count / truncated_bytes…）
	Refused bool                // 被护栏拒绝（不是故障）
}
```

### 可选接口（实现了才启用对应能力，不破坏已有实现）

| 接口 | 用途 |
|---|---|
| `MetaProvider` | 声明只读 / 破坏性、路径参数、超时、截断上限、UI 提示、分组、类别 |
| `ActivityProvider` | 生成「正在做什么」的活动描述（前端时间线文案） |
| `RiskClassifier` | 按**本次入参**给出命令级风险描述与级别（exec / 技能脚本用） |

`MetaOf(tool)` 统一读取：实现 `MetaProvider` 用声明值，否则按 `RiskLevel` 兜底推导。

## 2. 注册表

`tool.Registry`：

| 方法 | 行为 |
|---|---|
| `Register(t)` | 重名 → 4005；**schema 编译失败 → 4004（注册即编译）** |
| `Unregister(name)` | MCP 服务器停用时移除其工具 |
| `Get(name)` / `List()` | 按名取 / 全量列出 |
| `AllParallel(names)` | 整批并发裁决：全部存在且执行模式为 Parallel（未声明视同 Parallel）才为真，任一 `Sequential` 即整批串行 |
| `SelfCheckSchema()` | 启动期自检，不合法直接启动失败 |

**工具启停不在注册表**：由 `service.ToolService` 结合 `system_settings` 决定（键 `tool.enabled.{name}`，值 `"true"/"false"`）。

| 环节 | 行为 |
|---|---|
| 默认值 | 未配置时按分组计算：functools 组默认 `false`（数量多、易干扰工具选择），其余默认 `true`；无启动期种子写入 |
| 修改 | `POST /api/v1/tools/:name/enabled`（工具名未注册 → 4005）；设置页 ToolsView 开关与命令面板快捷开关都走它 |
| 生效 | 即改即生效：下次 run 组装工具列表时重读 KV，无需重启；注册表本身无启停概念 |
| 消费 | `ToolService.EnabledTools()` 过滤 → `LLMDefinitionsFiltered()` 转 LLM ToolDefinition → run 装配唯一入口 `ChatService.exposedToolDefs`（再经 Agent 策略 FilterTools） |

参数校验用 `santhosh-tekuri/jsonschema/v6`，资源 URL 走自定义 scheme `workbaby://`，
并剥离 MCP 带来的 `$schema`（避免启动期联网拉元 schema）。

## 3. 入参解析与终局语义

### DecodeArgs

`tool.DecodeArgs(args, &req)` 是工具入参解析的唯一写法：失败即返回 4004 错误结果，调用方直接 `return res`。
各工具不再各写一份 `json.Unmarshal` + 错误拼装。

### 终局语义（MetaTerminate）

`ToolResult.Meta[tool.MetaTerminate] = "1"` 声明「本结果已是终局」：
同一轮**全部**结果都置位时，内核跳过下一轮模型调用直接以 `end_turn` 收尾，省掉一次纯粹用于复述的往返。

判据是「工具已经把用户要的结论交付完毕，模型再跑一轮也只会复述」——
`exit_plan_mode` 被用户拒绝即属此类。批次里只要有一个结果需要模型继续处理，就照常进入下一轮。

## 4. 内置工具清单

| 分组 | 工具 | 能力 |
|---|---|---|
| 文件 | `file_read` `file_write` `file_list` | 读 / 覆盖写（≤2MB）/ 列目录，路径限定工作区沙箱 |
| 文件 | `file_edit` | 精确字符串替换，要求唯一匹配，回执带 unified diff |
| 文件 | `file_grep` `file_glob` | 正则内容检索（含行号）/ 通配列文件，尊重 `.gitignore` |
| 文件 | `archive_manager` | zip 压缩与解压（工作区相对路径） |
| 命令 | `exec` | 白名单内二进制，参数数组（无 shell 拼接），输出头 120KB + 尾 80KB 双端限流 |
| 网络 | `websearch` `webfetch` `http` | 网页搜索（默认 DuckDuckGo，免配置）/ 抓正文（≤5MB）/ 通用请求（仅 GET/POST） |
| 文档 | `doc_reader` | 抽取 pdf/docx/txt/md/html/csv/json 纯文本 |
| 检索 | `knowledge_search` | 本地知识库检索唯一入口，返回带出处的片段 |
| 记忆 | `memory_write` | 模型主动追加一条长期记忆 |
| 计划 | `todo` `enter_plan_mode` `exit_plan_mode` | 会话待办 / 计划模式进出（退出走审批门） |
| 交互 | `request_input` | 缺关键信息时向用户提问并阻塞等待 |
| 技能 | `run_skill_script` | 运行技能内置脚本（解释器固定映射，2 分钟超时） |
| 委派 | `delegate_task` | 委派子 Agent（见 `capability/20-subagent.md`） |
| 纯函数 | 30 个 functools | 数学 / 日期时间 / 文本 / 正则 / JSON / CSV / 哈希 / 编码 / 随机 / 数据清洗 / IP，无 IO 无副作用，默认分组禁用（清单见 §4.1） |
| 外接 | `mcp_*` | 来自 MCP 服务器 |

注册时机：`api.Handler.Startup` 中统一注册（能力注册表暴露的 `knowledge_search` / `memory_write` 与 functools 一并注册），
最后 `SelfCheckSchema` 统一自检。

### 4.1 functools 纯函数工具目录（30 个）

`FuncTool` 骨架统一封装：结果字符串原样返回、其余 JSON 序列化后放 `Data.value`；30 个工具全部 `RiskReadOnly`。

**data 组（18 个，`tools_data.go`）**

| 工具 | 用途 | 参数（**加粗必填**） |
|---|---|---|
| `math_eval` | 求值算术表达式（四则与括号） | **expression** |
| `math_stats` | 逗号分隔数字的 count/sum/min/max/avg | **numbers** |
| `json_parse` | 解析并美化 JSON（顺带校验） | **json** |
| `json_get` | 按点分路径（含 `[0]` 下标）取值 | **json, path** |
| `json_validate` | 校验是否合法 JSON | **json** |
| `csv_read` | CSV 文本 → JSON 数组（有表头则每行成对象） | **csv**, has_header |
| `csv_write` | JSON 二维数组（可选表头）→ CSV 文本 | **json**, header |
| `hash_md5` | 文本 MD5 hex 摘要 | **text** |
| `hash_sha256` | 文本 SHA 摘要（可选 SHA-1/256/512，默认 256） | **text**, algorithm |
| `hash_hmac` | HMAC-SHA256 hex 摘要 | **key, text** |
| `base64_encode` / `base64_decode` | 文本 ↔ Base64（UTF-8） | **text** / **base64** |
| `url_encode` / `url_decode` | URL 组件百分号编码 / 解码 | **text** |
| `data_clean` | 清洗对象数组：去首尾空格、可丢 null/空字段/空对象 | **json**, drop_empty |
| `data_aggregate` | 按 key 分组聚合数值字段（sum/avg/count/min/max） | **json, group_by, field**, op |
| `data_validate` | 校验数组内对象必需字段，返回缺失明细 | **json, required** |
| `ip_lookup` | 本机主机名与非回环 IP 列表 | 无参 |

**text 组（9 个，`tools_text.go`）**

| 工具 | 用途 | 参数 |
|---|---|---|
| `current_time` | 当前日期时间（默认 `2006-01-02 15:04:05`） | format |
| `date_add` / `date_diff` | 日期加减天数 / 两日期天数差 | **date**, days / **date1, date2** |
| `text_replace` | 正则全文替换（支持 `$1` 组引用） | **text, regex**, replacement |
| `text_count` | 字符数 / 词数 / 行数 | **text** |
| `text_extract` | 提取全部正则匹配（捕获组，默认组 1） | **text, regex**, group |
| `regex_match` | 测试文本是否匹配正则 | **pattern, text** |
| `regex_extract` | 提取首个捕获组（无组则全匹配） | **pattern, text** |
| `regex_replace` | 正则全文替换 | **pattern, replacement, text** |

**io 组（3 个，`tools_io.go`）**

| 工具 | 用途 | 参数 |
|---|---|---|
| `random_uuid` | 随机 UUID v4 | 无参 |
| `random_string` | 指定长度随机字母数字串（默认 16） | length |
| `random_number` | [min,max] 闭区间随机整数（默认 0..100） | min, max |

## 5. 运行身份与工作区

工具需要知道「自己在哪个会话、哪次 run、工作区在哪」：

```go
tool.WithRunIdentity(ctx, runID, sessionID)   // 内核在 run 开始时注入
tool.RootResolver(sessionID) string           // 按会话解析工作区根
tool.ResolveRoot(resolver, defRoot)           // 按 ctx 解析（带默认根回落）
```

`ResolveForSession(resolver, defRoot, sessionID)` 供 service 层复用同一套回落语义。

**回落语义**：解析失败（会话未绑定目录 / ctx 无身份）一律回落默认根——
工具宁可在默认工作区里返回「文件不存在」，也不能因拿不到根而让整个 run 中断。

## 6. UI 呈现信号

| 信号 | 用途 |
|---|---|
| `Meta.Group` / `Category` | 工具列表分组；前端图标与配色 |
| `Meta.ActivityDesc` | 时间线文案（「正在读取 app.ts」） |
| `Meta.UIHint` | 结果渲染提示（如 `diff` → 前端用 `DiffView`） |
| `Meta.PathParams` | 路径参数名（目录信任预检提取目标目录） |
| `Meta.MaxResultChars` / `TimeoutSec` | 执行器参数覆盖 |

## 7. 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| 注册即编译 schema | 非法 schema 在启动期暴露 | 注册稍慢（一次性） |
| 可选接口（非必须实现） | 新工具只需实现 5 个方法即可工作 | 能力发现靠类型断言 |
| 注册表不管理启停 | 注册表纯粹；启停策略可独立演化 | 启停状态需另一处（`ToolService` + KV） |
| 终局工具跳过下一轮 | 省一次纯复述往返（省时省钱） | 判据写错会提前收尾（需谨慎声明） |
| 工具输出按 rune 截断 | 防止单次输出淹没上下文 | 截断处需明确标记，模型要知道内容不全 |
