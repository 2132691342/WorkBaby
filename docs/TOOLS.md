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

`ToolOutput { text, is_error, duration_ms }`：`text` 进模型上下文，
`is_error` 让模型知道这次调用失败了（而不是当成正常结果继续往下走）。

## 注册表

`ToolRegistry` 是名字 → 实例的映射。开关在 service 层（repo），
注册表本身只管「当前可用集合」。

- 注册同名工具后到者胜——功能工具与 MCP 工具可覆盖内置工具
- `snapshot()` 深拷贝一份给内核线程独占，避免运行期被功能开关改动影响
- 停用的工具不进 schema，调用时返回 7003

## 内置工具

| 工具 | 参数 | 要点 |
|---|---|---|
| read | path, offset?(1 起), limit? | 文本按行截断；图片转 image 块 |
| bash | command, timeout?(秒) | 命令执行；CREATE_NO_WINDOW；输出走截断 |
| edit | path, edits[{old_text(唯一), new_text}] | 多处编辑一次调用；old_text 不唯一或区间重叠即拒绝 |
| write | path, content | 自动建父目录；原子写 |
| grep | pattern, path?, glob?, ignore_case?, literal?, context?, limit?(100) | 尊重 .gitignore |
| find | pattern(glob), path?, limit?(1000) | 同上 |
| ls | path?, limit?(500) | 目录在前，目录名带 `/` 后缀 |

默认激活集是 `read / bash / edit / write`，其余可经工具开关或装载预设开启。

## 输出截断

| 限制 | 值 |
|---|---|
| 行数 | 2000 |
| 字节 | 50KB |
| grep 单行 | 500 字符 |

超限时在输出尾部附「已截断」标记。截断是**按字符边界**切的——
按字节切会把多字节字符切成乱码。

## 命令执行纪律

子进程只能经 `wb-tools::exec` 启动，这是全仓唯一的进程出口：

| 纪律 | 实现 |
|---|---|
| 不闪控制台 | Windows 下加 `CREATE_NO_WINDOW` |
| 不进 shell 解释 | argv 数组直传，命令字符串永不拼接 |
| 工作目录 | 当前工作区（服务层保证进程 cwd 等于工作区） |
| 可超时可取消 | 超时或取消令牌触发即杀进程，返回 7001 / 7002 |
| 输出有上限 | 单次执行 1MB 上限，再交给工具层截断 |

**PATH 注入**：用户 PATH 前置当前工作区所需的目录（如 Git Bash 的 bin），
不注入数据目录——数据目录里没有可执行文件，注入了只会让「命令找不到」更难查。

## 扩展工具如何并入

| 来源 | 机制 | 命名 |
|---|---|---|
| 内置功能 | service 装配时把启用功能的 Tool 实例追加进注册表 | 功能自带名 |
| MCP 服务器 | 连接成功后把 server 工具包成 Tool 实例并入 | `mcp__<server>__<tool>` |
| 技能 | **不产生工具** | — |

技能是「提示词注入 + 用 read 加载正文」，不是工具——把技能做成工具会让模型
多一层无谓的调用往返。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 不实现独立 task / glob 工具 | find 覆盖 glob；批量任务属子代理 | — |
| JSON Schema 手写常量 | 工具参数固定，不引 schema 生成宏 | 改参数要同步 schema 与解析代码两处 |
| edit 用唯一 old_text 而非补丁语法 | 模型友好、失败可归因（不唯一即报错） | 大改文件不如直接 write |
| 只经一个执行器起进程 | 便于统一超时、取消与截断纪律 | 执行器本身成为关键路径 |
