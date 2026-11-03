//! 工具运行时上下文：谁发起调用、往哪个会话回话。

/// 一次工具调用所属的运行时环境。
///
/// 功能工具要发事件（提问 / 状态行 / 小部件）时必须写进**发起调用的那个会话**，
/// 否则全局帧会串到别的会话界面上。`run_id` 让同一会话先后两轮的同名工具调用
/// 能区分开——重启后补发的提问据此判断自己是不是已经作废。
#[derive(Debug, Clone, Default)]
pub struct ToolCtx {
    /// 发起调用的会话 id；空 = 没有会话归属（非对话上下文）
    pub session_id: String,
    /// 发起调用的那一轮运行 id
    pub run_id: String,
}

impl ToolCtx {
    /// 造一个带归属的上下文。
    pub fn new(session_id: impl Into<String>, run_id: impl Into<String>) -> Self {
        Self {
            session_id: session_id.into(),
            run_id: run_id.into(),
        }
    }
}
