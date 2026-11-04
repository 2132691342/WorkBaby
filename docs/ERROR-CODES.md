# 错误码 · ERROR-CODES

码值为数字字符串，随信封 `code` 下发；`"0"` 为成功，恒 HTTP 200。
常量唯一落点 `electron/main/domain/errors.ts`——新增码先改本文档再进常量。
`message` 是用户直接可读的文案，`detail` 带原话（stderr / 路径 / 上游响应）。

## 段位

| 段位 | 域 |
|---|---|
| 1000–1999 | 通用 / 文件 / 路径 / git |
| 2000–2999 | 配置 / 持久化 / DB |
| 3000–3999 | 服务商 / 模型 |
| 5000–5999 | 会话运行 |
| 7000–7999 | 外部进程 |
| 8000–8999 | Skill |
| 9100–9199 | MCP |
| 9700–9799 | 引擎（RPC / 预装） |

## 码表

### 通用 / 文件 / 路径 / git（1000–1999）

| 码 | 常量 | 场景 |
|---|---|---|
| 1000 | CODE_INTERNAL | 未归类内部错误（原话进 detail 与日志） |
| 1001 | CODE_FILE_READ | 文件读取失败 |
| 1002 | CODE_FILE_WRITE | 文件写入失败 |
| 1003 | CODE_PATH_ESCAPE | 路径越出工作区（安全边界） |
| 1004 | CODE_NOT_FOUND | 文件 / 目录 / 文档不存在 |
| 1017 | CODE_GIT_UNAVAILABLE | git 不可用（内置运行时未就绪） |
| 1020 | CODE_GIT_FAILED | git 命令失败（原话在 detail） |
| 1021 | CODE_PATH_MISSING | 未选择工作区 |
| 1401 / 1404 | —（路由内直写） | 令牌无效 / 接口不存在 |

### 配置 / 持久化 / DB（2000–2999）

| 码 | 常量 | 场景 |
|---|---|---|
| 2001 | CODE_CONFIG_READ | 配置读取失败 |
| 2002 | CODE_CONFIG_WRITE | 配置写入失败 |
| 2004 | CODE_DB | 数据库操作失败 |

### 服务商 / 模型（3000–3999）

| 码 | 常量 | 场景 |
|---|---|---|
| 3001 | CODE_PROVIDER_BAD | 服务商入参不完整（缺 Base URL / 名称等） |
| 3002 | CODE_PROVIDER_TEST | 连通性测试失败 |
| 3003 | CODE_PROVIDER_FETCH | 远端模型清单拉取失败 |
| 3005 | CODE_PROVIDER_LOGIN | 登录流不支持（引导走模型页填 Key） |

### 会话运行（5000–5999）

| 码 | 常量 | 场景 |
|---|---|---|
| 5001 | CODE_ENGINE_MISSING | 引擎未安装 |
| 5002 | CODE_ENGINE_DOWN | 引擎未运行 / 子进程已退出 |
| 5003 | CODE_ENGINE_BUSY | 上一轮还在跑 / 操作不支持 |
| 5004 | CODE_SESSION_BIND | 会话绑定失败（未选工作区等） |
| 5005 | CODE_PROMPT_REJECTED | 消息被引擎拒绝 |
| 5006 | CODE_SESSION_IO | 会话文件读写失败 |

### 外部进程（7000–7999）

| 码 | 常量 | 场景 |
|---|---|---|
| 7001 | CODE_SPAWN_FAILED | 子进程启动失败 |
| 7002 | CODE_PROC_TIMEOUT | 子进程超时被杀 |
| 7003 | CODE_PROC_EXIT | 子进程异常退出 |
| 7004 | CODE_RUNTIME_UNPACK | 运行时解压失败 / 未就绪 |

### Skill / MCP / 引擎

| 码 | 常量 | 场景 |
|---|---|---|
| 8001 | CODE_SKILL_BAD | 技能路径越界 / 名称非法 / 已存在 |
| 8002 | CODE_SKILL_SAVE | 技能写入 / 开关 / 复制失败 |
| 8004 | CODE_SKILL_DELETE | 技能删除失败 |
| 9101 | CODE_MCP_READ | mcp.json 读取失败 |
| 9102 | CODE_MCP_WRITE | mcp.json 写入失败 / 条目不存在 |
| 9701 | CODE_RPC_DOWN | pi 未运行 / 未安装 / 正在关停 |
| 9703 | CODE_RPC_TIMEOUT | RPC 命令超时 |
| 9704 | CODE_RPC_CMD | RPC 命令被引擎拒绝（success:false） |
| 9705 | CODE_PROVISION | 安装任务冲突 / 安装失败 |

### 前端本地码

| 码 | 场景 |
|---|---|
| 8003 | 连不上后端 / 响应超时（axios 层映射，不来自后端） |
