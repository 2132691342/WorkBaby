//! 会话回放修补：坏行跳过后补齐悬空的工具结果，让消息序始终对上游合法。

use crate::session::{now_ms, Block, Msg};

/// 补齐悬空的工具结果。
/// 进程在工具执行中被杀（含提问途中等不到回答）会留下没有结果的 tool_call，
/// 直接发给模型是非法消息序，上游会报 400 且带不了任何调试信息。
pub fn backfill_dangling_tool_results(msgs: &mut Vec<Msg>) {
    use std::collections::HashSet;
    let answered: HashSet<String> = msgs
        .iter()
        .filter_map(|m| match m {
            Msg::ToolResult { tool_call_id, .. } => Some(tool_call_id.clone()),
            _ => None,
        })
        .collect();
    // 逐条检查助手消息里的调用，缺结果的补在它后面（紧跟其后位置最稳）
    let mut extra: Vec<(usize, Msg)> = Vec::new();
    for (idx, m) in msgs.iter().enumerate() {
        let Msg::Assistant { content, .. } = m else {
            continue;
        };
        for b in content {
            if let Block::ToolCall {
                tool_call_id, name, ..
            } = b
            {
                if !answered.contains(tool_call_id) {
                    extra.push((
                        idx,
                        Msg::ToolResult {
                            tool_call_id: tool_call_id.clone(),
                            tool_name: name.clone(),
                            content: "[中断] 这条工具调用没有产生结果：运行进程在它完成前退出了。"
                                .into(),
                            is_error: true,
                            timestamp: now_ms(),
                            duration_ms: 0,
                            file_path: None,
                            diff: None,
                            images: Vec::new(),
                        },
                    ));
                }
            }
        }
    }
    // 从后往前插，避免前面的插入挪动后面的下标
    for (idx, m) in extra.into_iter().rev() {
        msgs.insert(idx + 1, m);
    }
}
