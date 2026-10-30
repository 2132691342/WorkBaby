//! 请求与流事件的统一形态：内核只认这里的类型，不看各家 wire 细节。

use serde::{Deserialize, Serialize};
use serde_json::Value;

use wb_pkg::Result;

/// 消息角色。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Role {
    /// 系统
    System,
    /// 用户
    User,
    /// 助手
    Assistant,
    /// 工具结果
    ToolResult,
}

/// 消息内容片段。
#[derive(Debug, Clone)]
pub enum ContentPart {
    /// 文本
    Text(String),
    /// 图片（纯 base64）
    Image { mime: String, base64: String },
}

/// 请求侧消息：多角色共用一个形态，tool 结果带 tool_call_id。
#[derive(Debug, Clone)]
pub struct ReqMessage {
    /// 角色
    pub role: Role,
    /// 内容片段（文本 / 图片）
    pub parts: Vec<ContentPart>,
    /// assistant 消息携带的工具调用（结果按 id 对应回灌）
    pub tool_calls: Vec<ReqToolCall>,
    /// ToolResult 消息对应的调用 id
    pub tool_call_id: Option<String>,
}

/// 请求侧工具调用（历史回放形态）。
#[derive(Debug, Clone)]
pub struct ReqToolCall {
    /// 调用 id
    pub call_id: String,
    /// 工具名
    pub name: String,
    /// 参数 JSON 文本
    pub arguments: String,
}

/// 工具声明（进请求的 schema）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ToolDef {
    /// 工具名
    pub name: String,
    /// 给模型的说明
    pub description: String,
    /// JSON Schema
    pub parameters: Value,
}

/// 思考档位。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ThinkingLevel {
    /// 关
    Off,
    /// 低
    Low,
    /// 中
    Medium,
    /// 高
    High,
}

/// 一次采样请求。
#[derive(Debug, Clone)]
pub struct StreamRequest {
    /// 模型 id
    pub model_id: String,
    /// 消息序列（系统提示词在最前）
    pub messages: Vec<ReqMessage>,
    /// 工具声明
    pub tools: Vec<ToolDef>,
    /// 最大输出 token
    pub max_tokens: i64,
    /// 思考档位
    pub thinking: ThinkingLevel,
}

/// 一次工具调用的完整形态（流结束前定稿）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ToolCallDone {
    /// 调用 id（provider 返回的原样）
    pub call_id: String,
    /// 工具名
    pub name: String,
    /// 参数 JSON 文本
    pub arguments: String,
}

/// 服务端返回的用量读数。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct Usage {
    /// 输入 token
    pub input: i64,
    /// 输出 token
    pub output: i64,
    /// 缓存命中
    pub cache_read: i64,
    /// 缓存写入
    pub cache_write: i64,
    /// 推理 token（openai 系；anthropic 计入 output）
    pub reasoning: i64,
}

/// 停止原因。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum StopReason {
    /// 正常结束
    Stop,
    /// 输出被截断
    Length,
    /// 要调工具
    ToolUse,
    /// 错误终止
    Error,
    /// 被取消
    Aborted,
}

/// 模型流事件：provider → 内核的唯一语言。
#[derive(Debug, Clone)]
pub enum ModelEvent {
    /// 流开始
    Start,
    /// 正文增量（内容索引, 文本）
    TextDelta(usize, String),
    /// 思考增量（内容索引, 文本）
    ThinkingDelta(usize, String),
    /// 工具调用参数增量（内容索引, 文本）
    ToolCallDelta(usize, String),
    /// 工具调用定稿（内容索引, 完整调用）
    ToolCallEnd(usize, ToolCallDone),
    /// 流完成（用量, 停止原因）
    Done(Usage, StopReason),
    /// 流错误（是否可重试, 消息）
    Error(bool, String),
}

/// 单个 wire 客户端的抽象：一次请求返回一条事件流。
#[async_trait::async_trait]
pub trait ModelClient: Send + Sync {
    /// 发起流式采样；事件流在 cancel 触发后尽快收尾（Error(Aborted)）。
    async fn stream(
        &self,
        req: StreamRequest,
        cancel: tokio_util::sync::CancellationToken,
    ) -> Result<ProviderStream>;
}

/// 事件流的别名（内核消费它）。
pub type ProviderStream = std::pin::Pin<Box<dyn futures::Stream<Item = ModelEvent> + Send>>;
