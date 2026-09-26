# 22 · 内置用户手册（DocsService）

## 1. 定位

用户手册随应用二进制分发（`go:embed`），无外网、无文件系统依赖：设置中心「文档」页与命令面板直接阅读。
当前 **12 页**（`assets/docs/*.md`）：getting-started / permissions-and-approval / chat-sampling /
goal-plan-todo / memory / knowledge-rag / mcp-servers / skills-and-commands / subagents /
files-workspace / dashboard-tasks / shortcuts。

## 2. 设计

| 环节 | 实现 |
|---|---|
| 资源 | `assets/docs.go`：`//go:embed docs` 整目录嵌入；**约定：每页首行 `# ` 标题即 title**，无标题回退文件名 |
| 服务 | `service.DocsService`：`List()` 只收 `.md`、name 去扩展名、按名排序；`Get(name)` 读 `docs/{name}.md` |
| 防穿越 | `Get` 拒绝含 `..` 或路径分隔符的 name |

## 3. 契约

| 方法 | 路径 | 出参 |
|---|---|---|
| GET | `/docs` | `DocItemRESP[]`：`{name, title}` |
| GET | `/docs/:name` | `DocDetailRESP`：`{name, title, content}` |

错误：2016 文档不存在（`domain/docs.go`）。

前端消费：`stores/docs.ts`（列表 + 详情缓存）、设置页 `settings/views/DocsView.vue`、命令面板（文档直达）；
`/api/v1/docs` 在 `api/client.ts` 路径白名单内。

## 4. 约束与取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| embed 进二进制 | 零依赖、离线可用、版本与代码严格一致 | 改手册需重新编译（换发行包才生效） |
| 首行标题约定 | 无需元数据文件，纯 Markdown 自描述 | 首行格式写错只影响列表标题显示 |
| 按名排序 | 列表稳定可预期 | 不支持手工排序（靠文件名前缀控制） |
