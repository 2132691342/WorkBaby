# 工具体系 · TOOLS

所有可被模型调用的能力都是同一个 `Tool` 实例。内置工具、内置功能、MCP 外部服务器
产出的工具走同一条契约并入同一张注册表——内核只认 `Arc<dyn Tool>`。

## Tool 契约

```rust
#[async_trait]
pub trait Tool: Send + Sync {
    /// 工具名（身份，小写 + 下划线；MCP 工具为 mcp__<server>__<tool>）
    fn name(&self) -> &str;
    /// 界面显示名
    fn label(&self) -> &str;
    /// 给模型的能力说明（决定模型会不会用它）
    fn description(&self) -> &str;
    /// 参数 JSON Schema
    fn parameters(&self) -> Value;
    /// 行为注解（只读 / 破坏性）
    fn annotations(&self) -> ToolAnnotations { ToolAnnotations::default() }

    /// 执行：args 已过 schema 校验；cancel 触发应尽快返回 7002
    async fn execute(&self, args: Value, cancel: CancellationToken, workdir: &Path)
        -> Result<ToolOutput>;
}
```

`description` 不是装饰——模型据此决定调用谁。写得含糊，模型就不调用；
写得太长，占上下文。所以它写「什么时候该用它」，不写「它怎么实现」。

`annotations().read_only` 决定并发分组：相邻只读工具并行执行，
写工具与未标注注解的工具各自串行（见 [KERNEL.md](KERNEL.md)）。

`ToolOutput { text, is_error, duration_ms, file_path, diff }`：`text` 进模型上下文，
`is_error` 让模型知道这次调用失败了（而不是当成正常结果继续往下走）。

`file_path` / `diff` 是写给界面的改动快照：写文件类工具（`write` / `edit`）自己算行级差异，
随 `chat:tool_end` 推给前端、并跟着工具结果落进会话 JSONL。它**不进模型上下文**——
模型不需要看自己刚写的 diff，那只是重复消耗。其他工具保持 `None`。
行级比对在 `wb-tools::difftext`：裁掉公共前后缀后逐行标注 `add` / `del` / `ctx`，
只回答「改了哪几行」，不产出可 patch 的补丁。

## 注册表

`ToolRegistry` 是名字 → 实例的映射。开关状态在 service 层（`wb-repo` 的 `tool_toggle` 表），
注册表本身只管「当前可用集合」。

- 注册同名工具后到者胜——功能工具与 MCP 工具可覆盖内置工具
- `snapshot()` 产出独立快照（新映射 + `Arc` 共享工具实例）给内核线程独占，
  避免运行期被功能开关改动影响
- 停用的工具不进 schema；若仍被调用（模型编造名字或开关在轮间变化），
  内核回灌错误文本「[工具不存在或已停用]」（`is_error`），不抛错误码

## 内置工具

| 工具 | 参数 | 要点 |
|---|---|---|
| `read` | path, offset?(1 起), limit? | 文本按行截断；二进制文件只报类型与字节数 |
| `bash` | command, timeout?(秒，默认 120，上限 600) | 经内置 Git Bash 执行；CREATE_NO_WINDOW；非零退出码以 `[退出码 N]` 尾注、stderr 折叠进输出；输出走截断 |
| `python` | code 或 file(二选一), timeout? | 走内置 Python 3.13；`code` 以 `-c` 直传，`file` 按工作区相对路径解析；cwd 即工作区 |
| `edit` | path, edits[{old_text(唯一), new_text}] | 多处编辑一次调用；old_text 不唯一即拒绝 |
| `write` | path, content | 自动建父目录；原子写 |
| `grep` | pattern, path?, glob?, ignore_case?, literal?, context?, limit?(100) | 尊重 .gitignore |
| `find` | pattern(glob), path?, limit?(1000) | 尊重 .gitignore |
| `ls` | path?, limit?(500) | 目录在前，目录名带 `/` 后缀 |

内置工具全部默认启用，可经工具开关或装载预设关掉。

`task`（派子代理）不属内置工具集，由 service 单独装配（见下）。

## 功能工具

功能工具随对应内置功能一起装配，关掉功能即从工具表撤出（见 [FEATURES.md](FEATURES.md)）。

| 工具 | 所属功能 | 要点 |
|---|---|---|
| `todo` | todo | 待办增删改查与依赖图 |
| `ask_user_question` | ask_user | ≤4 题结构化提问，阻塞等回答 |
| `fffind` / `ffgrep` | fff | frecency 排序的模糊搜索 |
| `memory_save` / `memory_search` / `memory_remove` / `memory_consolidate` / `session_search` | memory | 持久记忆、自动去重整合、历史会话检索 |
| `web_search` / `fetch_content` / `pdf_read` / `github_clone` / `video_transcript` | web | 多搜索源检索、网页抓取、PDF 取文、仓库浅克隆、视频字幕 |
| `ctx_execute` / `ctx_batch_execute` / `ctx_index` / `ctx_search` | context_mode | 沙箱执行与本地知识库 |
| `lsp_diagnostics` / `lsp_fix` | lsp | 静态诊断与快速修复 |
| `plan_submit` / `plan_mark_done` / `plan_inbox` / `plan_annotate` / `review_diff` | plan | 计划评审、逐步标注、分支差异审查 |
| `create_goal` / `get_goal` / `update_goal` | goal | 目标状态机与步骤清单 |

`task` 常驻（不属任何功能）：它是把大块活摘出主上下文的手段。

## 输出截断

| 限制 | 值 |
|---|---|
| 行数 | 2000 |
| 字节 | 50KB |
| grep 单行 | 500 字符 |

超限时在输出尾部附「已截断」标记。截断是**按字符边界**切的——
按字节切会把多字节字符切成乱码。

## 命令执行纪律

模型直接调用的 bash 统一走 `wb-tools::exec`；git、MCP stdio、LSP、github_clone
各有独立进程出口，但全部遵守同一纪律：

| 纪律 | 实现 |
|---|---|
| 不闪控制台 | Windows 下加 `CREATE_NO_WINDOW`（`wb_pkg::proc` 的常量） |
| 不进 shell 解释 | argv 数组直传，命令字符串永不拼接（唯一例外是 `bash -c <command>`，命令串在这一层才进 shell） |
| 工作目录 | `AgentConfig.workdir`（当前工作区）随内核配置传入，与进程 cwd 无关 |
| 可超时可取消 | 超时或取消令牌触发即杀进程；取消返回 7002；超时不是错误码——`exec` 返回 `timed_out` 结果，由工具层以 `is_error` 文本呈现 |
| 输出有上限 | 单次执行 1MB 上限，再交给工具层截断 |
| 探测程序不起进程 | `wb_pkg::proc::resolve_command` 纯路径查找并自展开 `.exe` / `.cmd`，禁止跑 `where` / `which` |

代价：纪律靠约定不靠类型收口——新增进程出口要自觉对齐这张表。

**内置运行时与子进程环境**：`exec` 为每条子进程注入同一份环境——`PATH` 由
`wb_domain::runtime::child_path` 构造（内置 cmd / bin / usr/bin / mingw64/bin / python 在前，
用户 PATH 在后）、`PYTHONUTF8` / `PYTHONIOENCODING` 锁 UTF-8、
`GIT_TERMINAL_PROMPT=0` 防凭据提示挂起。

**bash 工具**的定位顺序：内置 `PortableGit\bin\bash.exe`（`usr\bin\sh.exe` 同源兜底）→
系统 PATH 的 `bash.exe` → 常见 Git 安装位 → 都找不到时把 `bash` 交给系统解析。

**python 工具**优先用内置解释器，其次系统 PATH；源码走 `-c` 直传，脚本走工作区相对路径；
需要第三方库时由模型在 bash 里 `python -m pip install` 装到内置运行时。

## 扩展工具如何并入

| 来源 | 机制 | 命名 |
|---|---|---|
| 内置工具 | `wb_tools::builtin::install` 装进注册表 | 工具自带名 |
| 内置功能 | service 装配时把启用功能的 Tool 实例追加进注册表 | 功能自带名 |
| MCP 服务器 | 连接成功后把 server 工具包成 Tool 实例并入 | `mcp__<server>__<tool>` |
| 技能 | **不产生工具** | — |

技能是「提示词注入 + 用 read 加载正文」，不是工具——把技能做成工具会让模型
多一层无谓的调用往返。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 不实现独立 glob 工具 | find 覆盖 glob；批量任务属子代理 | — |
| JSON Schema 手写常量 | 工具参数固定，不引 schema 生成宏 | 改参数要同步 schema 与解析代码两处 |
| edit 用唯一 old_text 而非补丁语法 | 模型友好、失败可归因（不唯一即报错） | 大改文件不如直接 write |
| bash 统一经执行器起进程 | 超时、取消、截断纪律收在一处 | 其余进程出口靠约定对齐纪律 |
| 改动快照不进上下文 | 看自己写的 diff 只是重复消耗 | 界面与模型各看一份数据 |
