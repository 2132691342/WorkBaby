# 工具参考

模型能调用的全部工具：名字、入参、硬上限、是否只读、计划模式下的行为。
定义在 `backend/agent/tool_*.go`，装配在 `backend/agent/tool.go`（`NewRegistry` 与 `AddWebTools` / `AddIndexTool`）。

五条全局规则先记住：

1. **路径必须先过 `Registry.resolve`**（`backend/agent/tool.go`）：相对路径按工作区根拼、`Clean` 后必须仍在工作区内，
   否则回 `1002`「路径不在工作区内」。没有工作区时一律回 `5000`。
   校验同时比字符串位置与**真实路径**（`Inside` + `InsideResolved`）：工作区里放一个指向别处的 junction / 符号链接
   逃不出去，读写都按解开后的位置判。
   **唯一的放行例外是技能正文**：路径命中所属会话当前已启用技能的 `SKILL.md` 时，允许读工作区外的这个文件
   （数据目录里的技能不在工作区里，不放行模型就永远读不到它要遵循的说明）。
2. **技能正文只读**：`write` / `edit` 的 `path` 指向任何一个已启用技能的 `SKILL.md` 时，`Run` 直接拒绝
   「技能正文只读，请在技能页里修改」。`read` 不受此限（见上一条）。技能有原件、启用副本与台账三份状态，
   工具写哪一份都说不清。
3. **工具输出会折叠**：单条超过 4000 字时，进上下文的是「头 3000 字 + 折叠标记 + 尾 1000 字」
   （`backend/agent/tidy.go`，`defaultTidyMaxChars/HeadChars/TailChars`）。**界面与会话文件里是完整原文**，
   折叠只影响模型这一轮看到的上下文。折叠发生时会往结果 `details` 写 `tidied` 与 `original_chars`。
4. **子进程输出还有一层更早的截断**：每条流（stdout / stderr 各算一条）最多留 **8 MB**
   （`maxCapturedBytes`，`backend/pkg/proc.go`）。触顶后缓冲不再增长并标记 `Truncated`；
   `bash` 会把它写进结果首行（「输出超过保留上限，已截断」），模型据此知道自己看到的不是全部。
   这一层在工具自己的 40000 字上限之前发生。
5. **计划模式按白名单拦执行**（`backend/agent/policy.go`）：只读白名单是
   `read` `grep` `find` `ls` `todo` `ask` `web_search` `web_fetch` `index_search`；其余一律按「会改」处理 ——
   **包括全部 MCP 工具**。被拦时回一句「现在是计划模式…退出后（/plan off）再执行」，
   带 `details.blocked_by="plan_mode"`。

---

## 1. 总表

| 工具 | 一句话 | 只读 | 计划模式 | 定义 |
|---|---|---|---|---|
| `read` | 按行号读文本文件 | 是 | 放行 | `tool_fs.go` |
| `write` | 整体覆盖写文件 | 否 | 拦下 | `tool_fs.go` |
| `edit` | 精确文本替换 | 否 | 拦下 | `tool_fs.go` |
| `ls` | 列目录 | 是 | 放行 | `tool_fs.go` |
| `grep` | 文件内容正则搜索 | 是 | 放行 | `tool_search.go` |
| `find` | 文件名通配搜索 | 是 | 放行 | `tool_search.go` |
| `bash` | 跑一条 PowerShell 命令 | 否 | 拦下 | `tool_shell.go` |
| `todo` | 整份替换待办清单 | 是 | 放行 | `tool_todo.go` |
| `ask` | 向用户提问并等回答 | 是 | 放行 | `tool_ask.go` |
| `index_search` | 在工作区预索引里检索 | 是 | 放行 | `tool_index.go` |
| `web_search` | 联网搜索 | 是 | 放行 | `tool_web.go` |
| `web_fetch` | 抓网页正文 | 是 | 放行 | `tool_web.go` |
| `mcp_<服务>_<工具>` | 调用 MCP 服务上的工具 | 未知（按写处理） | 拦下 | `tool_mcp.go` |

---

## 2. 文件类

### `read`

读工作区内一个文本文件，返回带行号的内容。改文件前先读原文 —— 行号是 `edit` 与差异视图的锚点。

| 入参 | 必填 | 说明 |
|---|---|---|
| `path` | 是 | 相对工作区或绝对路径 |
| `start` | 否 | 起始行号，从 1 起；`< 1` 按 1 处理 |
| `end` | 否 | 结束行号（含）；超过总行数或小于 `start` 时按总行数处理 |

硬上限与拒绝条件：

| 条件 | 结果 |
|---|---|
| 文件 > 512 KB | 拒绝：`文件太大（N KB），超过单次读取上限 512 KB`（`maxReadBytes = 512 << 10`，`tool_fs.go`） |
| 路径是目录 | 拒绝，并提示改用 `ls` |
| 内容含 NUL 字节 | 判为二进制，拒绝按文本读 |
| 路径在工作区外 | 拒绝 `1002`，**除非**它正是某个已启用技能的 `SKILL.md` —— 技能正文在数据目录里，模型要读它必须放行（`Registry.resolve` → `Skills.IsFile`） |

头一行会说明总行数与本次范围，便于模型接着读下一段。

### `write`

整体覆盖写。父目录不存在会自动建。改已有文件的局部内容应该用 `edit`。

| 入参 | 必填 | 说明 |
|---|---|---|
| `path` | 是 | 目标路径（工作区内） |
| `content` | 是 | 完整内容 |

走 `pkg.WriteFileAtomic`（同目录临时文件 → `Sync` → 改名），中途中断不会留下半个文件。
**没有体积上限** —— 这是与 `read` 的不对称，大文件写进去以后 `read` 读不回来，只能分段读。
返回结果带 `details.file_path`（相对路径），供回放层渲染「改了什么文件」。

`path` 指向某个已启用技能的 `SKILL.md` 时直接拒绝：「技能正文只读，请在技能页里修改」——
技能有原件、副本、台账三份状态，工具写哪一份都说不清。

### `edit`

把一段原文替换成新文本。先 `read` 看清原文再改。

| 入参 | 必填 | 说明 |
|---|---|---|
| `path` | 是 | 目标文件 |
| `old_text` | 是 | 要被替换的原文，**必须在文件里唯一出现** |
| `new_text` | 是 | 替换后的文本 |

三种拒绝，都是让模型自己纠正而不是猜：

| 情况 | 结果 |
|---|---|
| `old_text` 为空 | 拒绝 |
| `old_text == new_text` | 拒绝（没有可改的内容） |
| 出现 0 次 | 拒绝，提示先 `read` 确认缩进与换行 |
| 出现多次 | 拒绝，要求带上更多上下文让它唯一 |

成功后写 `details.file_path` 与 `details.diff`（统一差异文本，`unifiedDiff`），
回放层据此画逐行差异与轮尾的改动芯片。

`path` 指向某个已启用技能的 `SKILL.md` 时直接拒绝：「技能正文只读，请在技能页里修改」。
（这条拦截在参数解析之后、工具执行之前，所以 `old_text` 不会被消费。）

### `ls`

列一个目录，目录在前、文件在后，各按名字排序；文件带人读的大小。

| 入参 | 必填 | 说明 |
|---|---|---|
| `path` | 否 | 目录路径；不传就列工作区根 |

**没有条数上限** —— 一次列一个超大目录会把全部条目写进上下文（再由折叠策略截断）。
结果首行给「N 个目录，M 个文件」。

---

## 3. 搜索类

### `grep`

按正则在工作区文件内容里搜，返回 `相对路径:行号: 整行`。

| 入参 | 必填 | 说明 |
|---|---|---|
| `pattern` | 是 | 正则表达式 |
| `path` | 否 | 搜索范围（目录或单个文件）；默认工作区根 |
| `ignore_case` | 否 | `true` 时加 `(?i)` 前缀 |

| 约束 | 值 |
|---|---|
| 命中上限 | **200**（`maxGrepHits`，`tool_search.go`）；到顶就停，头一行标注「已截断到上限」 |
| 单文件体积 | 超过 **1 MB**（`maxGrepFileBytes`）直接跳过，不参与内容搜索 |
| 单行长度 | 截到 **300** 字 |
| 跳过的目录 | `node_modules` `.git` `dist` `build` `.next` `target` `vendor` `__pycache__` `.venv`（`skipDirs`） |
| 非文本文件 | 跳过：按扩展名排除图片/压缩包/可执行文件/PDF/Office/字体/音视频（`isTextPath`，`tool_fs.go`）；内容含 NUL 的也跳过 |
| 权限不足的目录 | 跳过该目录，不中断整趟 |

正则编译失败会回原文错误，模型据此改表达式。

### `find`

按文件名通配符找文件或目录。

| 入参 | 必填 | 说明 |
|---|---|---|
| `pattern` | 是 | 通配符，如 `*.go`、`main.*` |
| `path` | 否 | 起始目录；默认工作区根 |

**只匹配基名**：传 `**/*.ts` 时取 `filepath.Base`，效果等于「全工作区所有 `.ts`」——
不是 shell 的目录层级语义。命中上限 **300**（`maxFindHits`）；目录命中带尾 `/`。

### `index_search`

在工作区预索引里检索文件名与符号名。快，但要读内容/找具体行还得 `grep`。

| 入参 | 必填 | 说明 |
|---|---|---|
| `query` | 是 | 文件名、符号名或路径片段 |
| `limit` | 否 | 默认 **15**；`<= 0` 按 15 |

工具描述里写着「上限 50」，但代码**没有做这个截断** —— 传 500 就是真给 500 条。
`backend/index/search.go` 的 `Search` 只在 `limit <= 0` 时兜底成 20，其余照传。

排序是相关度叠 frecency（`backend/index/search.go`）。命中行含路径、符号名与行号。

注意：工具描述写「上限 50」，但**代码只兜底 `<= 0`，没有上限裁剪**（`tool_index.go`）；
传更大的 `limit` 会真的返回更多条，直到索引里没有更多命中。
索引为空（切工作区后第一次检索之前）时返回「没找到」，并提示改用 `find`。

---

## 4. 执行类

### `bash`

在工作区里跑一条命令并返回输出。Windows 上没有 bash，实际走
`powershell -NoProfile -NonInteractive -Command <整条语句>`（`tool_shell.go`），argv 直传不进 shell 拼接。

| 入参 | 必填 | 说明 |
|---|---|---|
| `command` | 是 | 要执行的语句 |
| `timeout_ms` | 否 | 默认 **120000**，上限 **600000**；`<= 0` 按默认 |

| 约束 | 值 |
|---|---|
| 输出上限 | **40000 字**（`maxShellOutput`），超出截断（再经折叠进上下文） |
| 采集上限 | stdout 与 stderr **各 8 MB**（`maxCapturedBytes`，`backend/pkg/proc.go`）：触顶后缓冲不再增长并置 `Truncated` |
| 截断如何呈现 | 结果首行变成「退出码 N（输出超过保留上限，已截断）」，模型据此知道自己看到的不是全部 |
| 工作目录 | 当前工作区根 |
| 危险命令闸门 | 命令命中不可逆特征（递归删除 / 格式化 / 注册表写入 / 停机重启 / 磁盘擦除，`destructivePatterns`）时先经 ask 通道要确认；拒绝或无确认通道一律不执行。设置键 `agent.bash_confirm` 设为 `"0"` 可关（默认开，fail-closed） |
| 找不到 PowerShell | 拒绝并说明 |
| 非零退出码 | 结果标 `isError`，正文带「退出码 N」与 stderr 段 |
| 取消 | 引擎的 `ctx` 直接杀进程树（`RunProc` 收到取消即 `KillTree`），不等超时；`ctx` 先于超时到期时按剩余时间缩短超时 |
| 超时 | 默认 120s，上限 600s；超时后同样杀树，等 `Wait` 收尾最多 5s（`killWaitBudget`） |

子进程环境追加 `NO_COLOR=1` 与 `GIT_TERMINAL_PROMPT=0`（`pkg.Layout.ChildEnv`）：
终端问询立即失败，命令行程序不会卡在等输入上。

---

## 5. 交互与协作类

### `todo`

整份替换待办清单。传入的就是当前全部任务，不是增量。

| 入参 | 必填 | 说明 |
|---|---|---|
| `tasks[]` | 是 | 数组，按顺序给出 |
| `tasks[].subject` | 是 | 要做什么，一句话 |
| `tasks[].status` | 否 | `pending` / `in_progress` / `completed`；缺省 `pending` |
| `tasks[].activeForm` | 否 | 进行中的现在进行时描述 |

ID 由工具侧自增分配（1 起）。`status` 只接受 `pending` / `in_progress` / `completed`，其余值按 `pending` 处理。
执行完广播 `chat:todos`，界面待办面板实时替换。清单同时写进工具结果的 `details.tasks`，回放层据此重建当前待办；
回放时 `status` 为空或为 `deleted` 的行被剔除（`parser/todos.go`），所以想「删掉」一项就把清单里那一项去掉。

### `ask`

向用户提问并阻塞等待回答。只在确实需要用户做选择、且猜错代价高时用 ——
能从文件或命令拿到答案就别问。

| 入参 | 必填 | 说明 |
|---|---|---|
| `method` | 是 | `confirm` / `select` / `input` / `editor`；缺省按 `input` |
| `title` | 否 | 标题，一句话 |
| `message` | 否 | 正文：为什么问、问什么 |
| `placeholder` | 否 | 输入框占位 |
| `prefill` | 否 | 预填内容 |
| `options[]` | `select` 时必填 | 可选项 |

- 缺 `method` 按 `input`；不认识的 `method` 拒绝。
- `select` 没给 `options` 直接拒绝。
- 用户取消**不算失败**：返回「用户取消了这次提问」，让模型自己决定继续还是收手。
- 没有提问通道（未装配 `Asker`）时拒绝。
- 没有自动超时：只等回答与 `ctx`。

---

## 6. 联网类

两者在装配时**无条件装上**（`backend/service/app.go` 的 `AddWebTools`），
可用性取决于运行时配置能不能调通，不取决于装配条件。

### `web_search`

| 入参 | 必填 | 说明 |
|---|---|---|
| `query` | 是 | 搜索词；带具体关键词与限定条件效果更好 |
| `limit` | 否 | 工具侧默认 8；`<= 0` 按 8 |

工具描述写「上限 20」，实际生效的条数由适配器决定：所有提供方都忽略传进来的 `limit`，
改用配置里的 `MaxResults`（`backend/web/search.go` 的 `limitOf`），而服务层把它固定成 **8**
（`service/web.go` 的 `config()`）。所以**无论如何最多 8 条**；
`limitOf` 的合法区间是 1~20，超出就回落到 8。

结果格式是编号列表：标题 / 地址 / 摘要（摘要截到 400 字）。零结果时回一句「换个说法再试」。

按当前配置的服务类型：`duckduckgo`（免密钥，默认）/ `brave` / `exa` / `tavily`（后三者需要密钥）。
没配密钥时 Brave 会明确回「Brave 搜索需要密钥」。

### `web_fetch`

抓一个网址并读正文：HTML 转可读文本，PDF 抽文字，JSON 原样。

| 入参 | 必填 | 说明 |
|---|---|---|
| `url` | 是 | 必须是 `http` / `https` |

| 约束 | 说明 |
|---|---|
| 内网地址 | 拒绝（`1002`）。字面校验拦环回、私有段、链路本地、未指定地址与 CGNAT `100.64.0.0/10`；真正连接前再按 DNS 解析结果复核一次（`backend/web/client.go` 的 `ValidateURL` / `safeDial`） |
| 非 200 | 报「上游返回 N」 |
| PDF 抽不出文字 | 报「这个 PDF 没有抽出文字（可能是扫描件）」 |
| 抽不出正文 | 退回原文；仍为空则回「没有读到可读内容」 |

结果 `details` 带 `url` 与 `char_count`（字符数）。

---

## 7. MCP 工具

名字是 `mcp_<服务名>_<工具名>`（`McpToolName`，`backend/agent/mcp_client.go`）。
服务名只允许字母、数字、`_`、`-`，因为它要拼进这个名字。

| 方面 | 行为 |
|---|---|
| 入参 | 服务端 `tools/list` 给的 JSON Schema **原样透传**，壳不解释也不改 |
| 描述 | 服务端描述；为空时写「MCP 工具（服务：X）」 |
| 只读判定 | **按写工具处理**，不向白名单自荐 —— 计划模式会拦下全部 MCP 工具（预期行为） |
| 服务已退出 | 回「MCP 服务 X 已退出（9103）」，提示确认服务进程再重试 |
| 调用失败 | 工具结果正文里带错误码与原因（`9102` / `9103` / `9104`），**不进 HTTP 信封** |
| 结果 | 文本原样进上下文，`details` 带 `mcp_server` 与 `mcp_tool` |

只有 **stdio** 型条目（`command` 存在）会被装载成工具；`url` 型条目能保存、能测连接，但不产生工具。
单个服务装载失败只记日志，其它服务照常（`mcpRuntime.Rebuild`）。

---

## 8. 相关文档

- 计划模式与准入：`docs/MODULES.md` §1
- 输出折叠与回放：`docs/DATA-MODEL.md` §1.4（`details.tidied` / `original_chars` / `diff`）
- 文件接口的上限（与工具不同一套）：`docs/MODULES.md` §7
- 联网服务商与地址安全：`docs/MODULES.md` §10
- 报错怎么处理：`docs/TROUBLESHOOTING.md`
