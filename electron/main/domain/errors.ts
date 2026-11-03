//! 错误码常量：与 docs/ERROR-CODES.md 一一对应，新增码先改文档再进这里。

// 1000–1999 通用 / 文件 / 路径 / git
export const CODE_INTERNAL = '1000'
export const CODE_FILE_READ = '1001'
export const CODE_FILE_WRITE = '1002'
export const CODE_PATH_ESCAPE = '1003'
export const CODE_NOT_FOUND = '1004'
export const CODE_GIT_UNAVAILABLE = '1017'
export const CODE_GIT_FAILED = '1020'
export const CODE_PATH_MISSING = '1021'
export const CODE_NETWORK = '8003'

// 2000–2999 配置 / 持久化 / DB
export const CODE_CONFIG_READ = '2001'
export const CODE_CONFIG_WRITE = '2002'
export const CODE_DB = '2004'

// 3000–3999 服务商 / 模型
export const CODE_PROVIDER_BAD = '3001'
export const CODE_PROVIDER_TEST = '3002'
export const CODE_PROVIDER_FETCH = '3003'
export const CODE_PROVIDER_LOGIN = '3005'

// 5000–5999 会话运行
export const CODE_ENGINE_MISSING = '5001'
export const CODE_ENGINE_DOWN = '5002'
export const CODE_ENGINE_BUSY = '5003'
export const CODE_SESSION_BIND = '5004'
export const CODE_PROMPT_REJECTED = '5005'
export const CODE_SESSION_IO = '5006'

// 7000–7999 外部进程
export const CODE_SPAWN_FAILED = '7001'
export const CODE_PROC_TIMEOUT = '7002'
export const CODE_PROC_EXIT = '7003'
export const CODE_RUNTIME_UNPACK = '7004'

// 8000–8999 Skill
export const CODE_SKILL_BAD = '8001'
export const CODE_SKILL_SAVE = '8002'
export const CODE_SKILL_DELETE = '8004'

// 9100–9199 MCP
export const CODE_MCP_READ = '9101'
export const CODE_MCP_WRITE = '9102'

// 9700–9799 引擎（RPC / 预装）
export const CODE_RPC_DOWN = '9701'
export const CODE_RPC_TIMEOUT = '9703'
export const CODE_RPC_CMD = '9704'
export const CODE_PROVISION = '9705'
