# 工具体系 · TOOLS

工具是模型唯一能「动手」的地方。所有工具（内置 / 功能 / MCP）实现同一个 `Tool` trait，
注册表只认 `Arc<dyn Tool>`——内核不知道一个工具来自哪一层。

## 契约

```rust
#[async_trait]
pub trait Tool: Send + Sync {
    fn name(&self) -> &str;
    fn label(&self) -> &str;
    fn description(&self) -> &str;
    fn parameters(&self) -> Value;
    fn annotations(&self) -> ToolAnnotations { .. }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        workdir: &Path,
        ctx: &ToolCtx,
    ) -> Result<ToolOutput>;
}
```

| 项 | 作用 |
|---|---|
| `name` | 身份（小写+下划线），进 schema，模型按它调用 |
| `label` | 界面显示名 |
| `description` | 给模型的能力说明，也决定它什么时候用这个工具 |
| `parameters` | JSON Schema |
| `annotations` | `read_only` 决定能否与其他只读工具并发；`destructive` 给界面提示 |
| `workdir` | 本次运行的工作区（不是进程 cwd） |
| `ctx` | `ToolCtx { session_id, run_id }`：**需要回话的工具必须用它** |

`ToolCtx` 存在的理由：功能工具要发 `chat:ask` / `chat:status` / `chat:widget`。
写进全局通道的话，会话 A 的提问卡会出现在会话 B 的界面上，而且用户切到别的
会话时那张卡根本没法回填。工具拿不到会话就发不出可回填的事件。

## 输出形态

```rust
pub struct ToolOutput {
    pub text: String,                        // 进模型上下文（已截断）
    pub is_error: bool,
    pub duration_ms: i64,
    pub file_path: Option<String>,           // 改动文件（相对工作区）
    pub diff: Option<Vec<DiffLine>>,         // 行级差异，界面就地着色
    pub images: Vec<ToolImage>,              // 读到的图片，随结果交给视觉模型
}
```

`file_path` / `diff` 只给界面看，不进模型上下文——它们随工具结果落进 JSONL，
重开会话时改动同样可见，但不会占模型的 token。

`images` 非空时，wire 层会把它排成「tool 消息 + 紧跟一条 user 图片消息」。
标准协议里 tool 消息只能带文本，图片只能另起一条，丢在这里模型就永远看不到。

## 内置工具

| 工具 | 只读 | 作用 |
|---|---|---|
| `read` | ✓ | 读文本（2000 行 / 50KB 截断）；**图片按 image 段交给视觉模型** |
| `write` | | 整文件写入（临时文件 + rename，避免半截文件） |
| `edit` | | 多处精确替换，`old_text` 必须唯一且不重叠 |
| `bash` | | 经内置 PortableGit 的 Git Bash 执行命令 |
| `python` | | 用内置 Python 3.13 跑代码或脚本 |
| `grep` | ✓ | 正则 / 字面量内容搜索，尊重 `.gitignore` |
| `find` | ✓ | glob 文件名搜索 |
| `ls` | ✓ | 列目录一层 |

`read` 的图片分流按扩展名（png / jpg / gif / webp / bmp，≤5MB）：先判扩展名再看
二进制嗅探。反过来做的话所有图片都会被判成「二进制文件 N 字节」，视觉模型
永远看不到用户截图。

## 输出截断与归档

| 上限 | 值 |
|---|---|
| 行数 | 2000 |
| 字节 | 50KB |
| grep 单行 | 500 字符 |

截断本身有代价：被砍掉的内容模型再也拿不回来，只能重跑一遍命令。
所以 `bash` / `python` 的超长输出会**先落盘再截断**：全文写进
`{数据目录}\tool-output\{毫秒}_{工具名}.txt`，回给模型的文本尾部带一句
`[完整输出已归档：<路径>]`，模型需要时用 `read` 取。

触发判据与截断判据**必须一致**（`len > 50KB || 行数 > 2000`）。只看字节会漏掉
「行数超限但字节不多」的情况——4000 行短日志才 40KB，那份内容会被静默丢掉。
落盘失败就退回纯截断，归档本身不能成为故障点。

## 执行纪律

所有外部进程（bash / python / git / MCP stdio / LSP / 沙箱）共用一套纪律：

| 纪律 | 做法 |
|---|---|
| 不进 shell | argv 数组直传，用户输入永不进解释器 |
| 不闪窗口 | 每个 `Command` 出口挂 `CREATE_NO_WINDOW`（直接引 `wb_pkg::proc` 常量） |
| 可超时 | 统一超时，超时杀进程 |
| 可取消 | `CancellationToken` 触发即杀进程，报 7002 |
| 内置运行时优先 | 子进程 PATH 首位指向内置 PortableGit / Python，不污染用户环境 |
| 编码统一 | `PYTHONUTF8=1` / `PYTHONIOENCODING=utf-8` / `LC_ALL=C.UTF-8` |
| 不走交互 | `GIT_TERMINAL_PROMPT=0`、`GIT_PAGER=cat`——无 TTY 会永久挂起 |

探测程序用 `wb_pkg::proc::resolve_command`（纯路径查找），**禁止跑 `where` / `which`**
——那会闪一个控制台窗口。

## 注册表与开关

`ToolRegistry` 是名字 → 实例的表，同名后到者覆盖（功能 / MCP 可替换内置）。

装配顺序（`wb_service::features::build_registry`）：内置工具打底 → `task` 常驻 →
功能工具按开关并入 → MCP 桥最后。**关掉的功能不进表**，模型根本看不到它的 schema；
开关变化后重建整个注册表，而不是在运行时判断。

单次运行拿的是注册表**快照**：跑着的时候改开关不影响这一轮，避免一轮里工具表变两次。

## 测试

`crates/wb-tools/tests/tools_test.rs` 守三类容易回退的行为：

| 用例 | 守什么 |
|---|---|
| 行级差异 | 写文件类工具必须成对给出删除行与新增行（界面着色的唯一来源） |
| 读图 | 图片必须产出 `images`，二进制不能被当成图片 |
| 超长输出归档 | 截断必须带可再读的句柄，且归档文件是全文 |
