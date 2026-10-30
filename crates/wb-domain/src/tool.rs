//! 工具视图与开关类型。

use serde::{Deserialize, Serialize};

/// 工具视图。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ToolVO {
    /// 工具名（身份）
    pub tool_name: String,
    /// 显示名
    pub tool_label: String,
    /// 给模型的能力说明
    pub tool_description: String,
    /// 来源：builtin / feature / mcp
    pub tool_source: String,
    /// 开关
    pub tool_enabled: bool,
    /// 只读工具（不产生副作用）
    pub tool_read_only: bool,
}

/// 工具开关请求。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ToolToggleREQ {
    /// 工具名
    pub req_tool_name: String,
    /// 开关目标态
    pub req_enabled: bool,
}
