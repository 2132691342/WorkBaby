# MCP 客户端 · MCP

`wb-mcp` 把外部 MCP 服务器的工具并入助手的工具注册表。
配置驱动、按需连接、失败隔离。

MCP 是本项目承接第三方扩展的主通道——它加进来的是「工具声明」，
不是「可执行代码」，所以扩展带来的风险面比运行时加载插件小得多。

## 配置

`{数据目录}\mcp.json`：

```jsonc
{ "<server_name>": {
    "transport": "stdio",                      // 或 "http"
    "command": "…", "args": [], "env": {},     // stdio
    "url": "…", "headers": {},                 // http（streamable）
    "enabled": true,
    "exposure": "direct",                      // direct / deferred / hidden
    "timeout": 10                              // 启动等待秒（默认 10，只等 direct）
} }
```

工作区级 `{工作区}\.workbaby\mcp.json` 可覆盖同名 server（需受信提示）。

## 连接编排

- service 持有连接池：启动会话时对 enabled 且 exposure ≠ hidden 的 server 并发连接
- 超时未就绪 → 发通知事件（warning）但不阻塞会话——某个 server 挂了不该让整个助手不可用
- **stdio**：spawn server 进程（CREATE_NO_WINDOW，env 支持明文与 `${VAR}` 展开）；日志进独立文件
- **http**：streamable HTTP 传输
- **不自动重连**：会话运行中重连会让工具行为突变（同一个工具前后返回不同结果）。
  下次会话启动时重试
- 运行中增删 server 走 `POST /mcp/reload`：空闲会话才允许

## 工具并入

- 工具名 `mcp__<server>__<tool>`；描述与 schema 透传；结果作为普通 toolResult 回灌
- exposure 语义：
  - `direct` → 全部工具进模型 schema
  - `deferred` → 记入目录，模型能看到存在但调用时才声明
  - `hidden` → 不连接
- 调用超时默认 60 秒，可按 server 配置；调用失败归 9103，服务器崩溃归 9102
- 工具开关对 MCP 工具同样生效（按完整名 `mcp__server__tool`）

## 凭据

`env` 与 `headers` 支持明文与 `${VAR}` 形式的环境变量引用——
配置文件里不必出现真实密钥。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 用官方 Rust SDK | 覆盖两传输与初始化协商 | 依赖版本要跟随 |
| 不自动重连 | 行为可预期；桌面场景重启会话即可 | 长 run 中途 server 挂会失败 |
| 只等 direct 的 server | hidden / deferred 不该拖慢启动 | deferred 的按需声明需要额外机制 |
| 工具名加 `mcp__` 前缀 | 与内置工具明确区分，开关按完整名 | 名字变长，模型要多花一点 token |
