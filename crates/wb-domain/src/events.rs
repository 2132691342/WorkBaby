//! SSE 事件载荷：与前端 types/api.ts 的 ServerEvent 一一对应，字段 snake_case。

use serde::Serialize;

use crate::session::{MessageVO, UsageVO};

/// chat:start 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct StartData {
    /// 本轮运行 id
    pub run_id: String,
    /// 会话 id
    pub session_id: String,
}

/// chat:delta 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct DeltaData {
    /// 增量类型：text / thinking
    pub kind: String,
    /// 增量文本
    pub delta: String,
}

/// chat:tool_start 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ToolStartData {
    /// 调用 id
    pub tool_call_id: String,
    /// 工具名
    pub name: String,
    /// 显示名（可空，前端按 label||name 取）
    pub label: Option<String>,
    /// 完整参数对象（参数补齐后才发事件）
    pub args: Option<serde_json::Value>,
}

/// chat:tool_end 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ToolEndData {
    /// 调用 id
    pub tool_call_id: String,
    /// 工具名
    pub name: String,
    /// 结果文本（已截断）
    pub result: String,
    /// 是否错误
    pub is_error: bool,
    /// 耗时（毫秒）
    pub duration_ms: i64,
}

/// chat:compressed 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct CompressedData {
    /// 压缩前的上下文占用
    pub tokens_before: i64,
}

/// chat:user 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct UserData {
    /// 消息本体
    pub message: MessageVO,
}

/// chat:context 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ContextData {
    /// 用量读数
    pub usage: UsageVO,
    /// 上下文窗口大小
    pub context_window: i64,
    /// 占用比（0-1）
    pub ratio: f64,
}

/// chat:done 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct DoneData {
    /// 本轮运行 id
    pub run_id: String,
    /// 本轮助手消息（定稿形态）
    pub message: Option<MessageVO>,
}

/// chat:stopped 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct StoppedData {
    /// 本轮运行 id
    pub run_id: String,
}

/// chat:error 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct ErrorData {
    /// 本轮运行 id（可能为空串）
    pub run_id: String,
    /// 错误码
    pub code: String,
    /// 错误消息
    pub message: String,
}

/// chat:gap 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct GapData {
    /// 客户端最后的 seq
    pub last_seq: i64,
    /// 服务端当前 seq
    pub current_seq: i64,
}

/// chat:retry 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct RetryData {
    /// 第几次尝试
    pub attempt: i64,
    /// 最多几次
    pub max: i64,
    /// 退避毫秒
    pub delay_ms: i64,
}

/// chat:switched 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct SwitchedData {
    /// 切换到的会话
    pub session_id: String,
}

/// chat:setup 载荷（全局通道，无 session_id）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct SetupData {
    /// 阶段：start / progress / done / error
    pub phase: String,
    /// 人话进度
    pub message: String,
}

/// chat:ask 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct AskData {
    /// 提问 id
    pub id: String,
    /// confirm / select / input / editor / questionnaire
    pub method: String,
    /// 标题
    pub title: String,
    /// 说明（confirm 用）
    pub message: String,
    /// select 选项
    pub options: Vec<String>,
    /// input 占位符
    pub placeholder: String,
    /// editor 预填
    pub prefill: String,
    /// 问卷（method=questionnaire）
    pub questions: Vec<AskQuestion>,
    /// 等待上限毫秒（0 = 不限时）
    pub timeout_ms: i64,
}

/// 问卷单题。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct AskQuestion {
    /// 题面
    pub question_title: String,
    /// 选项（含描述与预览）
    pub question_options: Vec<AskOption>,
    /// 是否多选
    pub question_multi: bool,
}

/// 问卷选项。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct AskOption {
    /// 选项文案
    pub option_label: String,
    /// 选项说明
    pub option_description: String,
    /// markdown 预览
    pub option_preview: Option<String>,
}

/// chat:ask_closed 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct AskClosedData {
    /// 提问 id
    pub id: String,
    /// 关闭原因：answered / cancelled / expired / session_gone
    pub reason: String,
}

/// chat:notify 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct NotifyData {
    /// info / warning / error
    pub notify_type: String,
    /// 内容
    pub message: String,
}

/// chat:status 载荷（空 text 即清除）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct StatusData {
    /// 状态键
    pub status_key: String,
    /// 状态文本
    pub status_text: String,
}

/// chat:widget 载荷（空行组即清除）。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct WidgetData {
    /// 小部件键
    pub widget_key: String,
    /// 行文本组
    pub widget_lines: Vec<String>,
}

/// chat:title 载荷。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct TitleData {
    /// 建议的会话标题
    pub title: String,
}
