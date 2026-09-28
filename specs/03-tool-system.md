# 03 · 工具系统

## 定位

`internal/tool` 是内核的「手」：注册表 + 一组内置工具。
工具是纯接口实现，参数用 JSON Schema 自描述，注册表负责校验、元数据与
启动期自检。工具面越小越安全——能用已有工具拼出来的事，不新增工具。

## 接口

```go
type Tool interface {
    Name() string
    Label() string                  // 给小白看的名字，如「读文件」
    Description() string
    PromptSnippet() string          // 注入 system 的一行说明
    PromptGuidelines() []string     // 注入 system 的使用准则
    Parameters() map[string]any     // JSON Schema
    ExecutionMode() ExecutionMode
    RequiresApproval() bool
    Execute(ctx context.Context, in Input) (*Result, error)
}

type Input struct {
    Args      map[string]any
    Workspace string
    Deps      Deps   // 已读记录 / 知识库检索 / 临时目录 / 内置解释器
}

type Result struct {
    Content string   // 回给模型
    Title   string   // 卡片上的一行摘要
    Detail  string   // 展开后的完整输出，不进上下文
    IsError bool
}
```

`Content` 与 `Detail` 的双通道是硬约束：**结构给 UI，内容给模型**。
`Detail` 永远不进上下文，否则一次 `grep` 就能撑爆窗口。

## 注册表

| 职责 | 说明 |
|---|---|
| `Register` | 注册即编译 Schema；空名 / 重名 / Schema 不合法当场报错 |
| `Get` / `All` / `Enabled` | 查工具、按注册顺序列出、按停用名单过滤 |
| `Defs` | 转上游声明，按名字排序——顺序稳定才能命中 prompt 缓存 |
| `ValidateSchemas` | 启动期再全量自检一次，坏 schema 不许进运行期 |
| `ValidateArgs` | 执行前校验；`nil` 参数按空对象放行 |

Schema 编译结果按工具名缓存，`ValidateArgs` 复用，不重复编译。
注册期编译是为了让错误**左移到最早能发现的那一刻**。

## ExecutionMode

| 模式 | 行为 | 典型工具 |
|---|---|---|
| sequential | 整批退化为串行 | write / edit / powershell / python |
| parallel | 调用数 > 1 时信号量并发，结果仍按调用顺序回填 | read / ls / find / grep / web_* |

`Config.Parallel` 只是并发度上限，不改变工具自己的声明——
一个批次里只要有一个 sequential，全批串行。

## 审批

`RequiresApproval() == true` 的工具在执行前过 `Gate`：

```text
Gate → 落 approvals 行 → 推 chat:approval → 等用户决策
     → 放行：once 只放这一次，session 把该工具名记进本会话白名单
     → 拒绝：生成一条 IsError 的工具结果，模型自行换方案
```

等待超时（5 分钟）按拒绝处理，**绝不自动放行危险操作**。
审批记录落库，因此应用重启后仍能恢复未决的决策卡。

## 内置工具（11 个）

| 工具 | Label | 模式 | 审批 | 说明 |
|---|---|---|---|---|
| `read` | 读文件 | parallel | 否 | 带行号；支持 offset/limit；截断时给可翻页提示 |
| `write` | 写文件 | sequential | 是 | 全量覆写，自动建父目录，结果区分新建 / 覆盖 |
| `edit` | 改文件 | sequential | 是 | `edits[]` 逐条替换，任一条失败整体失败 |
| `ls` | 看目录 | parallel | 否 | 单层列表，目录在前 |
| `find` | 找文件 | parallel | 否 | 递归通配，尊重 .gitignore |
| `grep` | 搜内容 | parallel | 否 | 正则搜索，输出 `file:line: text` |
| `powershell` | 跑命令 | sequential | 是 | Windows 命令行（见 spec 05） |
| `python` | 跑脚本 | sequential | 是 | 内置 Python（见 spec 06） |
| `web_search` | 搜网页 | parallel | 否 | DuckDuckGo HTML 检索 |
| `web_fetch` | 打开网页 | parallel | 否 | 抓网页正文转纯文本 |
| `knowledge_search` | 搜索知识库 | parallel | 否 | 私有资料检索，由 service 装配时注入（见 spec 09） |

前 10 个在 `tool.RegisterBuiltins` 注册；`knowledge_search` 依赖 knowledge 服务，
由 `service.New` 在装配期注册，**除此之外任何地方不得注册工具**。

## 约束

- 工具不得自行读配置 / 开日志 / 起 goroutine；一切依赖从 `Deps` 注入
- 工具不得 `recover` 自己的 panic
- 输出统一走 `tool.Cut`（2000 行 / 50KB 双上限，超出落临时文件并把路径告诉模型）
- 路径安全（穿越拒绝、Unicode 规整）统一走 `pkg.SafeJoin`，规则只写一遍
- 错误用 `pkg.New(4xxx, ...)`，前端按 code 分流
