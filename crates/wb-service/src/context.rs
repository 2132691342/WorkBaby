//! 上下文占用分解：context_view 的数据源。估算口径与压缩一致（按字符类别加权）。

use std::sync::Arc;

use wb_domain::resource::{ContextBreakdownRESP, ContextSegmentVO};
use wb_pkg::Result;

use super::store;
use super::Services;

/// 分解当前会话的上下文占用。
pub fn breakdown(sv: &Arc<Services>, session_id: &str) -> Result<ContextBreakdownRESP> {
    let overhead = super::context_overhead(sv, session_id);
    let mut segments = vec![
        ContextSegmentVO {
            segment_name: "系统提示词".into(),
            segment_tokens: overhead.system_tokens,
        },
        ContextSegmentVO {
            segment_name: "工具定义".into(),
            segment_tokens: overhead.tool_tokens,
        },
    ];
    let mut messages_tokens = 0i64;
    // 上游报的真实用量：最近一轮的实际上下文占用，比估算准得多
    let mut reported_tokens = 0i64;
    if !session_id.is_empty() {
        if let Ok(detail) = store::detail(sv, session_id) {
            for m in &detail.detail_messages {
                let mut text = String::new();
                text.push_str(m.message_content.as_deref().unwrap_or(""));
                text.push_str(m.message_thinking.as_deref().unwrap_or(""));
                for b in &m.message_blocks {
                    text.push_str(b.block_text.as_deref().unwrap_or(""));
                }
                messages_tokens += wb_kernel::tokenest::estimate_text_tokens(&text);
                if let Some(u) = &m.message_usage {
                    if u.usage_total > 0 {
                        reported_tokens = u.usage_total;
                    }
                }
            }
        }
    }
    // 历史消息段：有真实用量就用真实值，否则用估算值
    segments.push(ContextSegmentVO {
        segment_name: "历史消息".into(),
        segment_tokens: if reported_tokens > 0 {
            reported_tokens
        } else {
            messages_tokens
        },
    });
    let total: i64 = segments.iter().map(|s| s.segment_tokens).sum();
    // 窗口取当前默认模型配置
    let context_window = super::chat_model::resolve_default(sv)
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
