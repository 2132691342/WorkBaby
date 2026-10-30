//! 对话入出参：发送 / 插话 / 追问 / 应答 / 压缩。

use serde::{Deserialize, Serialize};

/// 附件（@ 引用的文件路径，或粘贴的图片）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct AttachmentREQ {
    /// 相对或绝对路径
    pub attach_path: String,
    /// 展示名
    pub attach_name: String,
    /// 粘贴图片的纯 base64（无 data: 前缀）
    pub attach_image_base64: Option<String>,
}

/// 发送消息。
#[derive(Debug, Clone, Deserialize, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ChatSendREQ {
    /// 目标会话；空 = 现场建新会话
    pub chat_session_id: Option<String>,
    /// 用户输入文本
    pub chat_text: String,
    /// 附件
    pub chat_attachments: Vec<AttachmentREQ>,
}

/// 发送响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ChatSendRESP {
    /// 本轮运行 id（RUN_ 前缀）
    pub resp_run_id: String,
    /// 会话 id（新会话时是现场生成的）
    pub resp_session_id: String,
    /// true = 进入插话队列而不是立即开跑
    pub resp_queued: bool,
}

/// 运行中插话。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ChatSteerREQ {
    /// 目标会话
    pub chat_session_id: String,
    /// 插话文本
    pub chat_text: String,
}

/// 空闲追问（按完整一轮处理）。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ChatFollowupREQ {
    /// 目标会话
    pub chat_session_id: String,
    /// 追问文本
    pub chat_text: String,
}

/// 内核提问应答。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ChatAnswerREQ {
    /// 提问 id
    pub answer_ask_id: String,
    /// select/input/editor 的应答值
    pub answer_value: Option<String>,
    /// confirm 的应答
    pub answer_confirmed: Option<bool>,
    /// 取消提问
    pub answer_cancelled: Option<bool>,
}

/// 压缩请求。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ChatCompactREQ {
    /// 目标会话
    pub chat_session_id: String,
    /// 压缩指引（可空）
    pub chat_instructions: Option<String>,
}
