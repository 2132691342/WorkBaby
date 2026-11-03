//! 执行过程回显的节流器：把高频输出块压成人类可读的刷新率。
//!
//! 命令刷得快时（`yes`、构建日志、进度条）逐块发事件会把 SSE 通道和界面一起堵死。
//! 这里丢帧不丢内容：被跳过的块直接舍弃，最终结果仍以完整输出回灌模型。

use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

/// 造一个节流的回调：距上次发送不足 `min_interval` 的块被丢弃。
pub fn new_progress<F>(min_interval: Duration, send: F) -> crate::tools_progress::ProgressFn
where
    F: Fn(&str) + Send + Sync + 'static,
{
    // 起点回拨一个间隔，让第一块立刻发出去
    let last = Mutex::new(Instant::now() - min_interval);
    Arc::new(move |chunk: &str| {
        if chunk.is_empty() {
            return;
        }
        let Ok(mut at) = last.lock() else { return };
        if at.elapsed() < min_interval {
            return;
        }
        *at = Instant::now();
        send(chunk);
    })
}
