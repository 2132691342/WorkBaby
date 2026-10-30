//! 上下文占用分解：context_view 的数据源。估算口径 = 字符数 / 4（usage 优先时另算）。

use std::sync::Arc;

use wb_domain::resource::{ContextBreakdownRESP, ContextSegmentVO};
use wb_pkg::Result;

use super::store;
use super::Services;

/// 每工具定义的估算 token（schema JSON 长度主导，取常量均值）。
const TOOL_DEF_TOKENS: i64 = 350;

/// 分解当前会话的上下文占用。
pub fn breakdown(sv: &Arc<Services>, session_id: &str) -> Result<ContextBreakdownRESP> {
    let (_, prompt) = super::inputs(sv);
    let tool_count = sv
        .registry
        .read()
        .unwrap()
        .all()
        .iter()
        .filter(|t| sv.repo.tool_enabled(t.name()).unwrap_or(true))
        .count() as i64;
    let mut segments = vec![
        ContextSegmentVO {
            segment_name: "系统提示词".into(),
            segment_tokens: prompt.len() as i64 / 4,
        },
        ContextSegmentVO {
            segment_name: "工具定义".into(),
            segment_tokens: tool_count * TOOL_DEF_TOKENS,
        },
    ];
    let mut messages_tokens = 0i64;
    if !session_id.is_empty() {
        if let Ok(detail) = store::detail(sv, session_id) {
            for m in &detail.detail_messages {
                let mut chars = m.message_content.as_deref().unwrap_or("").len();
                chars += m.message_thinking.as_deref().unwrap_or("").len();
                for b in &m.message_blocks {
                    chars += b.block_text.as_deref().unwrap_or("").len();
                }
                messages_tokens += chars as i64 / 4;
            }
        }
    }
    segments.push(ContextSegmentVO {
        segment_name: "历史消息".into(),
        segment_tokens: messages_tokens,
    });
    let total: i64 = segments.iter().map(|s| s.segment_tokens).sum();
    // 窗口取当前默认模型配置
    let context_window = super::chat::resolve_default(sv)
        .map(|(_, _, w, _)| w)
        .unwrap_or(128_000);
    let ratio = if context_window > 0 {
        total as f64 / context_window as f64
    } else {
        0.0
    };
    Ok(ContextBreakdownRESP {
        resp_segments: segments,
        resp_total_tokens: total,
        resp_context_window: context_window,
        resp_ratio: ratio.clamp(0.0, 1.0),
    })
}
