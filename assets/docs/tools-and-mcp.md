# 工具与 MCP

## 内置工具

助手默认带这些工具（可在设置里逐个停用）：

- **read / write / edit**：文件读写与精确编辑
- **bash**：执行命令（输出按 2000 行 / 50KB 截断）
- **grep / find / ls**：内容搜索、文件查找、目录浏览

## 内置功能提供的工具

- **todo**：跨压缩存活的待办清单，多步任务自动维护
- **ask_user_question**：结构化提问，把"该猜"变成"问你"
- **fffind / ffgrep**：模糊文件与内容搜索（frecency 排序）
- **memory_save / memory_search / session_search**：持久记忆与历史会话检索
- **fetch_content / web_search**：联网抓取与搜索（默认关闭，需在设置里启用）

## MCP

在 **MCP**（左侧一级导航） 用 JSON 配置外部工具服务：

```json
{
  "mcpServers": {
    "my-server": {
      "transport": "stdio",
      "command": "node",
      "args": ["C:/path/to/server.js"],
      "enabled": true
    }
  }
}
```

- stdio 与 http（streamable）两种传输；`enabled: false` 可保留配置不启动。
- 连上的工具以 `mcp__服务名__工具名` 出现在工具清单里，开关对其同样生效。
- 断线不自动重连：重开会话或点「重新连接」。
