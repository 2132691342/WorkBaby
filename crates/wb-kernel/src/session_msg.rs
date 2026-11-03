//! 会话消息类型：内容块、用户输入、图片与四类消息体。

use serde::{Deserialize, Serialize};

use wb_provider::Usage;

use crate::session::now_ms;

/// 会话文件首行。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SessionHeader {
    /// 类型标记（恒为 "session"）
    #[serde(rename = "type")]
    pub type_: String,
    /// 格式版本
    pub version: i64,
    /// 会话 id（SESS_ 前缀）
    pub session_id: String,
    /// 起建毫秒
    pub created_at: i64,
    /// 工作目录
    pub workspace: String,
}

/// 消息内容块。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum Block {
    /// 正文
    Text {
        /// 文本
        text: String,
    },
    /// 思考
    Thinking {
        /// 思考内容
        thinking: String,
    },
    /// 工具调用
    ToolCall {
        /// 调用 id（TC_ 前缀）
        tool_call_id: String,
        /// 工具名
        name: String,
        /// 参数 JSON 文本
        arguments: String,
    },
}

/// 用户消息携带的图片（纯 base64，不含 data: 前缀）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct ImagePart {
    /// MIME 类型（image/png 等）
    pub mime: String,
    /// 图片字节的纯 base64
    pub base64: String,
}

/// 消息体（role 区分四类）。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "role", rename_all = "snake_case")]
pub enum Msg {
    /// 用户消息
    User {
        /// 文本
        content: String,
        /// 毫秒
        timestamp: i64,
        /// 这条消息显式调用的技能名；容器只记名字，正文在发请求时按需读
        #[serde(default, skip_serializing_if = "Option::is_none")]
        skill: Option<String>,
        /// 随消息发送的图片
        #[serde(default, skip_serializing_if = "Vec::is_empty")]
        images: Vec<ImagePart>,
    },
    /// 助手消息
    Assistant {
        /// 内容块（有序）
        content: Vec<Block>,
        /// provider id
        provider_id: String,
        /// 模型 id
        model_id: String,
        /// 消息 id（MSG_ 前缀）
        message_id: String,
        /// 用量
        #[serde(default)]
        usage: Option<Usage>,
        /// 停止原因：stop / length / tool_use / error / aborted
        stop_reason: String,
        /// 毫秒
        timestamp: i64,
        /// 耗时毫秒
        #[serde(default)]
        duration_ms: i64,
    },
    /// 工具结果
    ToolResult {
        /// 对应调用 id
        tool_call_id: String,
        /// 工具名
        tool_name: String,
        /// 结果文本（已截断）
        content: String,
        /// 是否错误
        is_error: bool,
        /// 毫秒
        timestamp: i64,
        /// 耗时
        #[serde(default)]
        duration_ms: i64,
        /// 改动的文件（相对工作区）；没有改动的工具为 None
        #[serde(default, skip_serializing_if = "Option::is_none")]
        file_path: Option<String>,
        /// 改动差异行；回放时界面直接着色
        #[serde(default, skip_serializing_if = "Option::is_none")]
        diff: Option<Vec<wb_domain::events::DiffLine>>,
        /// 工具读到的图片（read 读图 / 截图）；只有视觉模型才带，否则工具不会产生它们
        #[serde(default, skip_serializing_if = "Vec::is_empty")]
        images: Vec<ImagePart>,
    },
}

/// 一条用户输入：正文 + 显式技能 + 图片。发送与续跑共用一个形态。
#[derive(Debug, Clone, Default)]
pub struct UserInput {
    /// 正文
    pub text: String,
    /// 显式调用的技能名
    pub skill: Option<String>,
    /// 随消息发送的图片
    pub images: Vec<ImagePart>,
}

impl UserInput {
    /// 纯文本输入。
    pub fn text(text: impl Into<String>) -> UserInput {
        UserInput {
            text: text.into(),
            ..Default::default()
        }
    }

    /// 转成内核消息（时间取当前）。
    pub fn into_msg(self) -> Msg {
        Msg::user_with(self.text, self.skill, self.images)
    }
}

impl Msg {
    /// 用户消息：纯文本，无技能、无图。
    pub fn user(content: impl Into<String>) -> Msg {
        Msg::user_with(content, None, Vec::new())
    }

    /// 用户消息：带显式技能与图片。
    pub fn user_with(
        content: impl Into<String>,
        skill: Option<String>,
        images: Vec<ImagePart>,
    ) -> Msg {
        Msg::User {
            content: content.into(),
            timestamp: now_ms(),
            skill,
            images,
        }
    }
}
