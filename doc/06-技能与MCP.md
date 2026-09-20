# 06 · 技能与 MCP

## 技能（Skill）

技能是一份 `SKILL.md`：YAML frontmatter + Markdown 正文，可选带 `scripts/`。它把「一类任务该怎么做」固化成可复用的说明，按需注入而不是常驻上下文。

```markdown
---
name: review
description: 代码审查
when_to_use: 审查|review|检查改动
allowed_tools: file_read, file_grep
version: 1
---
正文：步骤、约定、输出格式
```

**来源**：`builtin`（编译期嵌入） / `download`（导入） / `custom`（用户自建 + 目录发现）

**两级目录叠加**（`service/skill.go`）：

- 全局根 `{home}/skills`：所有会话可用
- 工作区根 `<工作区>/.workbaby/skills`：仅该工作区可用，**同名覆盖全局**
- 切换工作区时整根摘除上一工作区的技能，被覆盖的全局版本自动恢复
- 磁盘上删除的技能从注册表摘除；内置与 UI 自建技能永不被摘除（前者 `SourceRef=assets/…`，后者为空）
- 用户启停状态落 `skills.Enabled`，重扫时继承，不会被重置

**匹配是确定性的** `Registry.Match(content)`：命中触发词最长者优先，等长按名字典序（避免 map 遍历顺序随机导致同输入不同技能）。

**注入**：能力注册表在 run 前把命中的技能正文作为 Section 注入上下文（Order 50），并把 `allowed_tools` 作为本轮工具白名单。脚本执行走 `run_skill_script`。

## MCP

MCP（Model Context Protocol）用于外接任意工具服务器。v1 支持 `stdio` transport。

**客户端** `mcp.StdioClient`：JSON-RPC 2.0 over 子进程 stdin/stdout，协议版本 `2025-06-18`，单行上限 1MB，调用超时 60s，stderr 保留尾部 4KB。

```go
type Client interface {
    Initialize(ctx) (*ServerInfo, error)
    ListTools(ctx) ([]ToolDef, error)   // 跟进 nextCursor 分页
    CallTool(ctx, name, args) (*Result, error)
    Close() error
}
```

`Done()` 通道在子进程死亡时关闭，`Manager` 据此自动注销其工具。

**管理器** `mcp.Manager`：按 `fingerprint = command + args + env` 增量对齐——变化才重启，删除才停止；启动流程 `Dial → Initialize → 监听 Done → ListTools → 逐个注册`。服务器名强制 `^[A-Za-z0-9_-]{1,32}$`（要进工具名）。

**工具暴露**：`MCPToolPrefix = "mcp_"`，`Name() = mcp_{server}_{tool}`，非法字符替换为 `_`；风险级别设为 `network`，`Meta.Group = exec`（避免被当作只读而参与并发执行）；远端 schema 非法时回退到通用 schema；结果截断 50k；image/resource 块降级为占位说明。

## 配置同步

`mcp.json` 是配置真相源：

- `Raw()` 从 DB 导出成可编辑文本，env 值掩码为 `***`
- `SaveRaw(content)`：解析 → 备份 + 原子写文件 → 全量对齐 DB → 热重载子进程；DB 写入失败自动回滚文件
- env 逐项加密存储，值为 `***` 时保留原密文
- `Add` 时用 `exec.LookPath` 预检命令是否存在

设置页直接编辑 `mcp.json` 原文，保存即校验并热重载。

## 取舍

| 取舍 | 优势 | 代价 |
|---|---|---|
| MCP 走独立子进程（stdio） | 崩溃隔离；任意语言实现；协议标准化 | 进程生命周期管理（孤儿回收、PATH 注入）；冷启动慢 |
| 工具以 `mcp_` 前缀暴露 | 与内置工具零冲突，一眼可辨来源 | 名字较长；模型偶尔截断（靠别名/宽松解析兜底） |
| env 加密存储 + 掩码回写 | 密钥不落明文；前端编辑不覆盖原密文 | 依赖主密钥可用（换机或删配置即失效） |
| 技能两级叠加（全局 → 工作区同名覆盖） | 团队共享与个人定制并存 | 覆盖关系需要解释（设置页给出说明） |
| 技能正文按需加载（元数据常驻） | 上下文预算不被技能正文挤占 | 模型需先「读」才知道细节，多一次工具调用 |
