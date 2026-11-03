//! 上下文压缩：阈值触发 + 预算切点（绝不切在工具批中间）+ 结构化摘要。

use wb_pkg::Result;
use wb_provider::Role;

use crate::session::{Entry, Msg, SessionFile};

/// 消息序列的 token 估算（按字符类别加权）。
/// 重导出到本模块：压缩与切点计算都在这里做，调用方不必跨模块找。
pub use crate::tokenest::estimate_tokens;

/// 触发保留（窗口 - reserve 即压缩）
pub const RESERVE_TOKENS: i64 = 16_384;
/// 保留最近
pub const KEEP_RECENT_TOKENS: i64 = 20_000;

/// 固定开销：每轮都重发、但不在消息序列里的部分。
/// 系统提示词与工具定义占着窗口却不在回放消息里，不算它们就会高估剩余空间。
#[derive(Debug, Clone, Copy, Default)]
pub struct ContextOverhead {
    /// 系统提示词 token
    pub system_tokens: i64,
    /// 工具定义 token
    pub tool_tokens: i64,
}

impl ContextOverhead {
    /// 合计开销。
    pub fn total(&self) -> i64 {
        self.system_tokens + self.tool_tokens
    }
}

/// 是否需要压缩。
pub fn should_compact(total_tokens: i64, context_window: i64) -> bool {
    context_window > 0 && total_tokens > context_window - RESERVE_TOKENS
}

/// 消息序列压成纯文本（进摘要提示词）。
pub fn serialize_for_summary(msgs: &[Msg]) -> String {
    let mut out = String::from("<conversation>\n");
    for m in msgs {
        match m {
            Msg::User { content, .. } => {
                out.push_str(&format!("[user]\n{content}\n"));
            }
            Msg::Assistant { content, .. } => {
                out.push_str("[assistant]\n");
                for b in content {
                    match b {
                        crate::Block::Text { text } => out.push_str(&format!("{text}\n")),
                        crate::Block::Thinking { .. } => {}
                        crate::Block::ToolCall {
                            name, arguments, ..
                        } => {
                            out.push_str(&format!("(tool call {name} {arguments})\n"));
                        }
                    }
                }
            }
            Msg::ToolResult {
                tool_name, content, ..
            } => {
                let head: String = content.chars().take(500).collect();
                out.push_str(&format!("[{tool_name} result]\n{head}\n"));
            }
        }
    }
    out.push_str("</conversation>\n");
    out
}

/// 摘要提示词：结构化六段（目标/约束/进度/决策/下一步/关键上下文）。
pub fn summary_prompt() -> &'static str {
    "You are a context summarization assistant. Read the conversation and output ONLY a structured summary in Markdown with exactly these sections:\n## Goal\n## Constraints & Preferences\n## Progress\n### Done\n### In Progress\n### Blocked\n## Key Decisions\n## Next Steps\n## Critical Context\nBe terse; keep file paths, commands and verbatim identifiers."
}

/// 找切点：从最新往回累加估算 token，≥ keep_recent 的消息边界；
/// 绝不把 assistant(带工具调用) 与其 toolResult 拆开。
fn find_cut(session: &SessionFile, keep_recent: i64) -> Option<String> {
    let path = session.path_to_leaf();
    let mut acc = 0i64;
    let mut cut_idx: Option<usize> = None;
    let mut i = path.len();
    while i > 0 {
        i -= 1;
        let e = &path[i];
        if let Entry::Message { message } = &e.payload {
            acc += estimate_tokens(std::slice::from_ref(message));
            // 工具批保护：切点若落在 toolResult 上，上移到其 assistant
            if matches!(message, Msg::ToolResult { .. }) {
                continue;
            }
            if acc >= keep_recent {
                cut_idx = Some(i);
                break;
            }
        }
    }
    cut_idx.map(|i| path[i].entry_id.clone())
}

/// 关外状态回填：压缩丢掉旧消息后，把活状态（如待办清单）重新写回上下文。
/// 返回 None 表示无需回填。
pub type ReinforceFn = Box<dyn Fn() -> Option<String> + Send + Sync>;

/// 执行压缩：先算切点 → 摘要（单次 LLM 调用，经给定 client）→ 追加 compaction 条目。
/// 返回 (tokens_before, summary)。
pub async fn compact(
    session: &mut SessionFile,
    client: &dyn wb_provider::ModelClient,
    model_id: &str,
    max_tokens: i64,
    reinforce: Option<&ReinforceFn>,
    cancel: tokio_util::sync::CancellationToken,
) -> Result<(i64, String)> {
    let msgs = session.replay_messages();
    let tokens_before = estimate_tokens(&msgs);
    let cut = find_cut(session, KEEP_RECENT_TOKENS);
    // 摘要内容 = 全部回放消息（已有更早摘要时它们已折叠成一条 user 摘要消息）
    let conversation = serialize_for_summary(&msgs);
    let req = wb_provider::StreamRequest {
        model_id: model_id.to_string(),
        messages: vec![
            wb_provider::ReqMessage {
                role: Role::System,
                parts: vec![wb_provider::ContentPart::Text(summary_prompt().to_string())],
                tool_calls: Vec::new(),
                tool_call_id: None,
                tool_name: None,
            },
            wb_provider::ReqMessage {
                role: Role::User,
                parts: vec![wb_provider::ContentPart::Text(conversation)],
                tool_calls: Vec::new(),
                tool_call_id: None,
                tool_name: None,
            },
        ],
        tools: Vec::new(),
        max_tokens,
        thinking: wb_provider::ThinkingLevel::Off,
    };
    let mut stream = client.stream(req, cancel).await?;
    let mut summary = String::new();
    while let Some(ev) = stream.next().await {
        match ev {
            wb_provider::ModelEvent::TextDelta(_, t) => summary.push_str(&t),
            wb_provider::ModelEvent::Done(..) => break,
            wb_provider::ModelEvent::Error(_, m) => {
                return Err(wb_pkg::AppError::new(
                    wb_domain::error::session::COMPACT_FAILED,
                    m,
                ));
            }
            _ => {}
        }
    }
    let first_kept = cut.unwrap_or_else(|| {
        session
            .entries
            .last()
            .map(|e| e.entry_id.clone())
            .unwrap_or_default()
    });
    session
        .append(Entry::Compaction {
            summary: summary.trim().to_string(),
            first_kept_entry_id: first_kept,
            tokens_before,
        })
        .map_err(|e| wb_pkg::AppError::new(wb_domain::error::session::COMPACT_FAILED, e.message))?;
    // 压缩把旧消息整段丢掉，此刻把仍在生效的活状态口头钉回上下文
    if let Some(text) = reinforce.and_then(|f| f()) {
        let trimmed = text.trim();
        if !trimmed.is_empty() {
            session
                .append(Entry::Message {
                    message: Msg::user(trimmed),
                })
                .map_err(|e| {
                    wb_pkg::AppError::new(wb_domain::error::session::COMPACT_FAILED, e.message)
                })?;
        }
    }
    Ok((tokens_before, summary.trim().to_string()))
}

use futures::StreamExt;
