//! 错误码常量与段位定义；全表与处置建议见 docs/ERROR-CODES.md。

/// 通用 / 文件 / 路径
pub mod common {
    /// 参数缺失或非法
    pub const PARAM_INVALID: &str = "1001";
    /// 文件读写失败
    pub const IO_FAILED: &str = "1002";
    /// 路径越界（逃出允许根）
    pub const PATH_ESCAPE: &str = "1003";
    /// 路径不存在
    pub const PATH_MISSING: &str = "1004";
    /// 压缩包解析失败
    pub const ARCHIVE_INVALID: &str = "1030";
    /// git 命令失败
    pub const GIT_FAILED: &str = "1017";
    /// git 提交失败
    pub const GIT_COMMIT_FAILED: &str = "1020";
    /// 冲突未解决
    pub const GIT_CONFLICT: &str = "1021";
    /// 远端领先
    pub const GIT_DIVERGED: &str = "1024";
}

/// 配置 / 持久化 / DB
pub mod config {
    /// 配置文件损坏
    pub const CONFIG_INVALID: &str = "2001";
    /// 数据库失败
    pub const DB_FAILED: &str = "2002";
    /// 数据目录不可写
    pub const DATA_DIR_READONLY: &str = "2004";
}

/// Provider / 模型配置
pub mod provider {
    /// provider 不存在
    pub const NOT_FOUND: &str = "3001";
    /// 模型不存在
    pub const MODEL_NOT_FOUND: &str = "3002";
    /// API key 缺失
    pub const KEY_MISSING: &str = "3003";
    /// baseUrl 非法 / wire api 不支持
    pub const BASE_URL_INVALID: &str = "3005";
    /// 上游响应解析失败
    pub const VALIDATE_FAILED: &str = "3109";
    /// 名称重复
    pub const NAME_DUP: &str = "3111";
    /// 连通测试失败
    pub const TEST_FAILED: &str = "3200";
}

/// 会话运行
pub mod session {
    /// 会话忙
    pub const BUSY: &str = "5001";
    /// 会话不存在
    pub const NOT_FOUND: &str = "5002";
    /// 会话文件被占用
    pub const FILE_BUSY: &str = "5003";
    /// 队列已满
    pub const QUEUE_FULL: &str = "5004";
    /// 提问已过期
    pub const ASK_EXPIRED: &str = "5005";
    /// 压缩失败
    pub const COMPACT_FAILED: &str = "5006";
    /// 内核未就绪
    pub const NOT_READY: &str = "5100";
}

/// 工具执行与外部进程
pub mod tool {
    /// 后台任务失败 / 外部调用超时（功能任务、LSP、MCP 调用、克隆）
    pub const EXEC_TIMEOUT: &str = "7001";
    /// 命令被取消
    pub const EXEC_CANCELLED: &str = "7002";
    /// 工具不存在或已停用 / 外部进程启动失败
    pub const NOT_AVAILABLE: &str = "7003";
    /// 工具参数校验失败
    pub const ARG_INVALID: &str = "7004";
    /// LSP server 启动失败
    pub const LSP_START_FAILED: &str = "7100";
    /// LSP server 返回错误响应
    pub const LSP_SERVER_ERROR: &str = "7101";
    /// 语言服务器未安装
    pub const LSP_NOT_INSTALLED: &str = "7102";
    /// 没有匹配该文件类型的语言服务器
    pub const LSP_NO_MATCH: &str = "7103";
    /// 记忆条目不存在
    pub const MEMORY_NOT_FOUND: &str = "7301";
}

/// Skill
pub mod skill {
    /// SKILL.md 解析失败
    pub const PARSE_FAILED: &str = "8001";
    /// description 缺失或超长
    pub const DESC_INVALID: &str = "8002";
    /// 技能名非法
    pub const NAME_INVALID: &str = "8003";
    /// 技能名冲突
    pub const NAME_DUP: &str = "8004";
    /// 内置技能不可删改
    pub const BUILTIN_LOCKED: &str = "8005";
}

/// MCP
pub mod mcp {
    /// server 配置非法
    pub const CONFIG_INVALID: &str = "9101";
    /// server 启动失败
    pub const START_FAILED: &str = "9102";
    /// 工具调用失败
    pub const CALL_FAILED: &str = "9103";
}

/// 内核
pub mod kernel {
    /// 会话文件损坏
    pub const FILE_CORRUPT: &str = "9701";
    /// 模型流中断（不可重试）
    pub const STREAM_BROKEN: &str = "9703";
    /// 子代理启动失败
    pub const SUBAGENT_FAILED: &str = "9704";
    /// 单轮请求数超过上限（模型反复调工具不收尾）
    pub const LOOP_LIMIT: &str = "9705";
}
