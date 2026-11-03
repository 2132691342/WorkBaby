# MCP 客户端 · MCP

`wb-mcp` 把外部 MCP 服务器的工具并入助手的工具注册表。
配置驱动、按需连接、失败隔离。

MCP 是承接第三方扩展的主通道——它加进来的是「工具声明」，不是「可执行代码」，
所以扩展带来的风险面比运行时加载插件小得多。

## 配置

`{数据目录}\mcp.json`：

```jsonc
{ "<server_name>": {
    "transport": "stdio",                      // 或 "http"
    "command": "…", "args": [], "env": {},     // stdio
    "url": "…", "headers": {},                 // http（streamable）
    "enabled": true,
    "exposure": "direct",                      // direct / deferred / hidden
    "timeout": 10                              // 启动等待秒；未配置或 0 时默认 10
} }
```

工作区级 `{工作区}\.workbaby\mcp.json` 可覆盖同名 server。

配置文件兼容两种形态：平铺的 `{ "name": {...} }`，
以及包一层的 `{ "mcpServers": { "name": {...} } }`——两种都能读，写回时保持原形态。

## 连接编排

- service 持有连接池：启动时对 enabled 且 `exposure != hidden` 的 server 并发连接
- 连接失败（启动超时 / 握手失败 / 工具清单失败）报 9102，连接标记为 error，
  **但不阻塞会话**——某个 server 挂了不该让整个助手不可用
- **stdio**：spawn server 进程（CREATE_NO_WINDOW，env 明文直传）
- **http**：streamable HTTP 传输
- **不自动重连**：会话运行中重连会让工具行为突变（同一个工具前后返回不同结果）。
  重连只在启动与显式 reload 时发生
- 运行中增删 server 走 `POST /mcp/reload`：断开全部 → 重新连接 → 重建注册表

## 工具并入

- 工具名 `mcp__<server>__<tool>`；描述与 schema 透传；结果作为普通工具结果回灌进会话
- `exposure` 语义：`direct` → 全部工具进模型 schema；`hidden` → 不连接；
  `deferred` → 当前与 `direct` 同义
- 单次工具调用超时 60 秒；超时归 7001（外部调用超时），取消归 7002，
  调用失败（传输错误或 `is_error` 结果）归 9103
- 工具开关对 MCP 工具同样生效（按完整名 `mcp__server__tool`）

## 凭据

`env` 与 `headers` 当前只支持明文。配置文件里的密钥是明文，
`mcp.json` 与会话数据同目录，依赖 NTFS 用户级权限保护。

### OAuth 登录

远端鉴权的 server 可以走授权码 + PKCE，不必手写 header：

1. `POST /mcp/login` → 从 `/.well-known/oauth-authorization-server` 取授权端点
2. 本地起一个回环端口（15232），生成 PKCE 的 verifier 与 challenge
3. 打开默认浏览器让用户授权，MCP server 把 code 回调到本地端口
4. 用 code 换 access_token，**写进该 server 的 `headers.Authorization`**

三条设计取舍：

- **token 落在 mcp.json 的 headers 里**，不另建存储：MCP 桥本来就从 headers 读凭据，
  写同一处才能让「改了配置立刻生效」这条纪律成立
- **登出就是删那个 header**（`POST /mcp/logout`），没有服务端会话可销毁
- **不支持 PKCE 就不做**：非 PKCE 的授权码流要额外处理 secret，不值得为此加一条分支；
  这时报错并指引用户改用 headers

回调端口固定为 15232：用户要在 MCP server 侧登记 redirect_uri，固定端口省去配置。

## 错误码

| 码 | 含义 |
|---|---|
| 9101 | server 配置非法（含 OAuth 这类未支持的能力） |
| 9102 | server 启动失败 / 超时（连接标记为 error，但不阻塞会话） |
| 9103 | 工具调用失败（传输错误或 `is_error` 结果；单次调用超时归 7001） |

完整错误码表见 [ERROR-CODES.md](ERROR-CODES.md)。

## 取舍

| 取舍 | 理由 | 代价 |
|---|---|---|
| 用官方 Rust SDK | 覆盖两传输与初始化协商 | 依赖版本要跟随 |
| 不自动重连 | 行为可预期；桌面场景重启会话即可 | 长 run 中途 server 挂会失败 |
| 不做 OAuth 流程 | 桌面单机助手的凭据以配置为主 | 远端鉴权 server 要手写 header |
| 工具名加 `mcp__` 前缀 | 与内置工具明确区分，开关按完整名 | 名字变长，模型要多花一点 token |
| 连接失败不阻塞会话 | 一个 server 挂了不该让整个助手不可用 | 用户要自己去 MCP 页看状态 |
