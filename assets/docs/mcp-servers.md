# MCP 服务器接入

MCP（Model Context Protocol）是 WorkBaby 接入外部工具的标准协议。任何实现 MCP stdio 的服务器都能被加载到 WorkBaby 中，作为模型可调用的工具。

## 1. MCP 配置位置

MCP 配置写在 `mcp.json`（应用根目录的全局配置）：

- 配置文件来源优先级：内嵌默认 → `mcp.json`（用户在设置页导出 / 改写）
- 真实运行时从 SQLite 的 `mcp_servers` 表读取（启动期同步）

设置页 → **能力 → MCP** 提供图形化编辑。

## 2. 配置结构

```json
{
  "servers": {
    "<server-name>": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/workspace"],
      "env": {
        "<key>": "<value>"
      },
      "enabled": true
    }
  }
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `command` | ✅ | 启动服务器的二进制（`npx` / `uvx` / 绝对路径）|
| `args` | — | 字符串数组参数，无 shell 拼接（与 `exec` 工具同口径）|
| `env` | — | 注入到子进程的环境变量（敏感值走 AES-256-GCM 加密）|
| `enabled` | — | 是否启用（默认 true） |

## 3. 生命周期

1. **启动**：WorkBaby 启动时按 `enabled=true` 的服务器 spawn 子进程（stdio 双工）
2. **工具发现**：握手 `initialize` → `tools/list` 拉取服务器提供的工具，注册进 `tool.Registry`
3. **过滤**：受 Agent 工具策略与全局启停清单约束（与内置工具同管线）
4. **调用**：模型选 tool → `tools/call` 转发 → 响应回填
5. **停服**：服务器启停 / 应用退出时优雅关闭子进程

## 4. 失败兜底

| 现象 | 处理 |
|---|---|
| 子进程启动失败 | 该服务器所有工具不可用，其他服务器不受影响；设置页底部展示红色「启动失败」|
| `tools/list` 超时 | 同上 |
| 单次 `tools/call` 失败 | 工具结果回填错误，模型可改道；不影响后续调用 |
| 服务器主动断开 | WorkBaby 标记 `disconnected`，设置页展示「已断开」+ 一键重连按钮 |

## 5. 权限与隔离

- MCP 服务器在受限子进程内运行（与 Skill 脚本同一安全边界）
- 工具调用经过完整的护栏链：ExposeGuard / SchemaGuard / PolicyGuard / 用户钩子
- 危险工具（`exec` / `file_write`）受权限四档约束，不可被 MCP 工具名绕过
- 审批决策走同一套审批门（跨 run 跨服务器统一）

## 6. 常见服务器推荐

| 服务器 | 用途 |
|---|---|
| `@modelcontextprotocol/server-filesystem` | 跨工作区文件访问 |
| `@modelcontextprotocol/server-git` | Git 操作（diff / log / commit）|
| `@modelcontextprotocol/server-github` | GitHub PR / Issue |
| `@modelcontextprotocol/server-sqlite` | 本地 SQLite 查询 |

设置页「能力 → MCP」一键预填推荐服务器。

## 7. 与 Skill 的差异

| 维度 | Skill | MCP 服务器 |
|---|---|---|
| 启动方式 | 按需加载（命中方法论时）| 启动期常驻子进程 |
| 暴露形式 | 方法论文本 + 可选脚本 | 工具 schema + 调用 |
| 进程模型 | 内嵌 Python 解释器 | 独立 stdio 子进程 |
| 适用场景 | 教 AI 「怎么思考」| 教 AI 「能调什么」|

两者互补：Skill 决定策略，MCP 决定能力边界。

## 8. 故障排查

| 症状 | 排查方向 |
|---|---|
| 工具列表为空 | 服务器是否真的启动成功；`mcp.json` 是否 `enabled: true` |
| 工具调用立刻超时 | 服务器是否阻塞在 `tools/call` 上；尝试手动跑一次 `command + args` 看输出 |
| 设置页显示「连接失败」| 查看应用日志（`%APPDATA%\WorkBaby\logs\`）的 MCP stderr 行 |

设置页 → MCP → 服务器行右侧的「日志」按钮可看该服务器最近 200 行 stdio 输出。