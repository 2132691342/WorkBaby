//! 内核事件：内核 → service 的唯一出参语言（docs/KERNEL.md 事件表）。

use serde::Serialize;

use wb_provider::{StopReason, Usage};

/// 内核事件流条目。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case", tag = "event")]
pub enum KernelEvent {
    /// 一轮 run 开始
    AgentStart {
        /// 运行 id
        run_id: String,
    },
    /// 每次模型请求前
    TurnStart,
    /// 消息定稿（assistant 或 user/工具结果的落盘通知）
    MessageEnd {
        /// 条目 id
        entry_id: String,
    },
    /// 正文增量
    TextDelta {
        /// 文本
        delta: String,
    },
    /// 思考增量
    ThinkingDelta {
        /// 文本
        delta: String,
    },
    /// 工具调用参数增量
    ToolCallDelta {
        /// 调用 id
        call_id: String,
        /// 增量
        delta: String,
    },
    /// 工具批开始（参数已完整）
    ToolExecutionStart {
        /// 调用 id
        call_id: String,
        /// 工具名
        name: String,
        /// 参数对象
        args: serde_json::Value,
    },
    /// 工具批结束（单个）
    ToolExecutionEnd {
        /// 调用 id
        call_id: String,
        /// 工具名
        name: String,
        /// 结果文本
        result: String,
        /// 是否错误
        is_error: bool,
        /// 耗时毫秒
        duration_ms: i64,
    },
    /// 一个 turn 结束（含用量与窗口读数）
    TurnEnd {
        /// 本 turn 助手用量
        usage: Option<Usage>,
        /// 停止原因
        stop_reason: StopReason,
    },
    /// 队列变化
    QueueUpdate {
        /// 插话队列长度
        steering: usize,
        /// 追问队列长度
        follow_up: usize,
    },
    /// 压缩开始
    CompactionStart {
        /// manual / threshold / overflow
        reason: String,
    },
    /// 压缩结束（成功才有 tokens_before）
    CompactionEnd {
        /// 压缩前 token
        tokens_before: Option<i64>,
    },
    /// 自动重试
    AutoRetry {
        /// 第几次
        attempt: i64,
        /// 上限
        max: i64,
        /// 退避毫秒
        delay_ms: i64,
    },
    /// 助手消息定稿（含完整块序与用量，供界面直接渲染）
    AssistantFinal {
        /// 条目 id
        entry_id: String,
        /// 内容块（JSON 数组）
        blocks: serde_json::Value,
        /// 用量
        usage: Option<Usage>,
        /// 停止原因
        stop_reason: StopReason,
    },
    /// 循环终点
    AgentEnd {
        /// 是否会重试
        will_retry: bool,
        /// 停止原因
        stop_reason: StopReason,
    },
    /// 完全结算（chat:done 挂这里）
    Settled {
        /// 是否因 abort 结束
        aborted: bool,
    },
    /// 错误终止
    Failed {
        /// 错误码
        code: String,
        /// 消息
        message: String,
    },
}

/// 事件回调：service 注入（转 SSE）。
pub type EmitFn = std::sync::Arc<dyn Fn(&KernelEvent) + Send + Sync>;
