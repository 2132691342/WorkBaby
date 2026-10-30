//! 会话与消息类型：列表 VO、详情消息、索引 DO。时间一律毫秒整数。

use serde::{Deserialize, Serialize};

/// 单条消息里的内容块（正文 / 思考 / 工具调用，按原始顺序排列）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct MessageBlock {
    /// text = 正文；thinking = 思考；tool = 工具调用（结果在紧随其后的 tool 消息里）
    pub block_kind: String,
    /// 正文或思考内容（block_kind 为 text/thinking 时）
    pub block_text: Option<String>,
    /// 工具调用 id（block_kind 为 tool 时）
    pub tool_call_id: Option<String>,
    /// 工具名
    pub tool_name: Option<String>,
    /// 工具显示名
    pub tool_label: Option<String>,
    /// 工具参数对象
    pub tool_args: Option<serde_json::Value>,
}

/// 消息用量读数（全部 token 数，cost 为美元）。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct UsageVO {
    /// 输入 token（不含缓存命中部分）
    pub usage_input: i64,
    /// 输出 token
    pub usage_output: i64,
    /// 命中上游缓存的输入量（与 usage_input 并列）
    pub usage_cached: i64,
    /// 写入上游缓存的输入量
    pub usage_cache_write: i64,
    /// 总量
    pub usage_total: i64,
    /// 这一轮发出时的上下文窗口占用
    pub usage_context: i64,
    /// 成本（美元）
    pub usage_cost: f64,
}

/// 消息视图对象：会话详情与实时流的统一形态。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct MessageVO {
    /// 消息 id（MSG_ 前缀；用户消息为对应条目 id）
    pub message_id: String,
    /// user / assistant / tool / notice
    pub message_role: String,
    /// 条目类型：text / tool / notice
    pub message_type: String,
    /// 思考全文（assistant）
    pub message_thinking: Option<String>,
    /// 正文全文（user / assistant；tool 消息为结果文本）
    pub message_content: Option<String>,
    /// 正文 / 思考 / 工具调用的原始顺序；渲染按它来，不按字段类型分组
    pub message_blocks: Vec<MessageBlock>,
    /// 工具调用组（assistant 消息）
    pub message_tool_calls: Vec<ToolCallVO>,
    /// 被答复的工具调用 id（tool 消息）
    pub message_tool_call_id: Option<String>,
    /// 被答复的工具名（tool 消息）
    pub message_tool_name: Option<String>,
    /// 工具结果是否错误（tool 消息）
    pub message_is_error: Option<bool>,
    /// assistant 的停止原因：stop / length / tool_use / aborted / error
    pub message_stop_reason: Option<String>,
    /// 用量读数（assistant）
    pub message_usage: Option<UsageVO>,
    /// 时间（毫秒）
    pub message_created_at: i64,
    /// 不进上下文也不渲染正文（如命令回显）
    pub message_hidden: bool,
    /// 非空时渲染成分隔提示条（已压缩 / 换模型）而不是气泡
    pub message_notice: Option<String>,
}

impl MessageVO {
    /// 构造空块消息（构造处初始化数组，永不发 null）。
    pub fn empty(message_id: String, message_role: &str, message_created_at: i64) -> Self {
        Self {
            message_id,
            message_role: message_role.to_string(),
            message_type: "text".into(),
            message_thinking: None,
            message_content: None,
            message_blocks: Vec::new(),
            message_tool_calls: Vec::new(),
            message_tool_call_id: None,
            message_tool_name: None,
            message_is_error: None,
            message_stop_reason: None,
            message_usage: None,
            message_created_at,
            message_hidden: false,
            message_notice: None,
        }
    }
}

/// 工具调用视图（assistant 消息内）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ToolCallVO {
    /// 调用 id（TC_ 前缀）
    pub tool_call_id: String,
    /// 工具名
    pub tool_name: String,
    /// 显示名
    pub tool_label: Option<String>,
    /// 参数对象
    pub tool_args: Option<serde_json::Value>,
}

/// 会话视图对象：列表与详情共用。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SessionVO {
    /// 会话 id（SESS_ 前缀）
    pub session_id: String,
    /// 会话文件相对数据目录的路径
    pub session_file: String,
    /// 显示名
    pub session_title: String,
    /// 工作目录
    pub session_workspace: String,
    /// 当前 provider id
    pub session_provider: String,
    /// 当前模型 id
    pub session_model: String,
    /// 思考档位
    pub session_thinking_level: String,
    /// 消息数
    pub session_message_count: i64,
    /// 累计 token
    pub session_total_tokens: i64,
    /// 累计成本（美元）
    pub session_cost: f64,
    /// 是否有 run 在跑
    pub session_running: bool,
    /// 起建时间（毫秒）
    pub session_created_at: i64,
    /// 最后活动时间（文件 mtime，毫秒）
    pub session_updated_at: i64,
}

/// 会话详情响应。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SessionDetailRESP {
    /// 会话本体
    pub detail_session: SessionVO,
    /// 消息回放（leaf→root 展开后的界面形态）
    pub detail_messages: Vec<MessageVO>,
}

/// 会话索引 DO（session_index 表）。
#[derive(Debug, Clone, Default)]
pub struct SessionIndexDO {
    /// 会话 id
    pub session_id: String,
    /// 显示名
    pub session_name: String,
    /// 工作目录
    pub session_workspace: String,
    /// provider id
    pub session_provider: String,
    /// 模型 id
    pub session_model: String,
    /// 消息数
    pub session_message_count: i64,
    /// 最后活动（毫秒）
    pub session_last_active_at: i64,
    /// 列表预览（最后一条正文前 80 字）
    pub session_summary: String,
}
