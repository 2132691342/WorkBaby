//! MCP 客户端类型：server 清单 VO、配置读写请求。配置文件形态见 docs/MCP.md。

use serde::{Deserialize, Serialize};

/// 一个 MCP server 的视图条目。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct McpServerVO {
    /// server 名（身份；同名工作区覆盖全局）
    pub mcp_name: String,
    /// global / workspace
    pub mcp_scope: String,
    /// 配置文件路径
    pub mcp_source: String,
    /// 是否启用
    pub mcp_enabled: bool,
    /// direct / deferred / hidden
    pub mcp_exposure: String,
    /// stdio / http
    pub mcp_transport: String,
    /// 连接目标：stdio 为启动命令行，http 为 URL
    pub mcp_target: String,
    /// connected / error / unknown
    pub mcp_state: String,
    /// 连接失败原因
    pub mcp_error: String,
    /// 已并入的工具名（mcp__server__tool 全名）
    pub mcp_tools: Vec<String>,
}

/// /mcp 出参：清单 + 两份配置原文（面板直接编辑）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct McpListRESP {
    /// server 清单
    pub resp_servers: Vec<McpServerVO>,
    /// 全局配置文件路径
    pub resp_global_path: String,
    /// 工作区配置文件路径（不存在也给出预期路径）
    pub resp_workspace_path: String,
    /// 全局配置原文
    pub resp_global_raw: String,
    /// 工作区配置原文
    pub resp_workspace_raw: String,
}

/// 保存配置原文入参（先校验后落盘，解析不过一个字符都不写）。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct McpSaveREQ {
    /// global / workspace
    pub mcp_scope: String,
    /// 配置原文
    pub mcp_raw: String,
}

/// 按 server 名动作入参（移除 / 登录 / 登出）。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct McpNameREQ {
    /// server 名
    pub mcp_name: String,
    /// global / workspace
    pub mcp_scope: String,
}

/// 单 server 的 wire 配置（mcp.json 里每个条目的形态）。
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
#[serde(rename_all = "snake_case", default)]
pub struct McpServerConfig {
    /// stdio / http
    pub transport: String,
    /// stdio 可执行文件
    pub command: String,
    /// stdio 参数
    pub args: Vec<String>,
    /// stdio 环境变量（支持 ${VAR} 展开）
    pub env: std::collections::BTreeMap<String, String>,
    /// http 地址（streamable）
    pub url: String,
    /// http 请求头
    pub headers: std::collections::BTreeMap<String, String>,
    /// 是否启用
    pub enabled: bool,
    /// direct / deferred / hidden（v1 一律按 direct 并入）
    pub exposure: String,
    /// 启动等待秒
    pub timeout: u64,
}

/// 校验单 server 配置；不合法返回 9101。
pub fn validate(name: &str, cfg: &McpServerConfig) -> Result<(), wb_pkg::AppError> {
    if name.trim().is_empty() || name.contains(['/', '\\', ':', '*']) {
        return Err(wb_pkg::AppError::new(
            crate::error::mcp::CONFIG_INVALID,
            format!("server 名非法：{name}"),
        ));
    }
    match cfg.transport.as_str() {
        "stdio" if cfg.command.trim().is_empty() => Err(wb_pkg::AppError::new(
            crate::error::mcp::CONFIG_INVALID,
            format!("{name}: stdio 传输必须给出 command"),
        )),
        "http" if cfg.url.trim().is_empty() => Err(wb_pkg::AppError::new(
            crate::error::mcp::CONFIG_INVALID,
            format!("{name}: http 传输必须给出 url"),
        )),
        "stdio" | "http" => Ok(()),
        other => Err(wb_pkg::AppError::new(
            crate::error::mcp::CONFIG_INVALID,
            format!("{name}: 不认识的 transport {other}"),
        )),
    }
}
