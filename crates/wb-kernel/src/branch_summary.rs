//! 分支摘要：从旧分支切回时，把被放弃的那条路径压成一条可读摘要。
//!
//! 只是把叶子指回去不够——旧分支上做过的事会**整段消失**，
//! 模型接着往下走时不知道自己之前试过什么、为什么放弃。
//! 摘要以一条 user 消息插在新分支开头，让被放弃的工作留下可追溯的结论。

use futures::StreamExt;

use wb_pkg::Result;
use wb_provider::{ContentPart, ReqMessage, Role, StreamRequest, ThinkingLevel};

use crate::session::{Entry, Msg, SessionFile};

/// 摘要请求的输出上限：摘要本身不需要长。
const SUMMARY_MAX_TOKENS: i64 = 2048;

/// 被放弃分支上一条可读记录。
pub struct AbandonedTurn {
    /// 谁说的：user / assistant / tool
    pub who: &'static str,
    /// 正文（工具结果已截断）
    pub text: String,
}

/// 分支摘要提示词。
fn prompt() -> &'static str {
    "You summarize an abandoned conversation branch. Output ONLY a compact Markdown brief \
     with these sections:\n## Tried\n## Findings\n## Open Questions\n\
     Be terse. Keep file paths, commands, identifiers and error strings verbatim. \
     Do not restate the whole conversation and do not invent anything."
}

/// 收集从 `from_leaf` 往上到（不含）`fork_point` 之间的条目，顺序从老到新。
///
/// 分叉点本身是两条分支的公共祖先，不写进摘要：否则同一段话会在摘要里
/// 和当前上下文里各出现一次。
pub fn abandoned_entries<'a>(
    sf: &'a SessionFile,
    from_leaf: &str,
    fork_point: &str,
) -> Vec<&'a crate::session::EntryWrap> {
    let by_id: std::collections::HashMap<&str, &crate::session::EntryWrap> = sf
        .entries
        .iter()
        .map(|e| (e.entry_id.as_str(), e))
        .collect();
    let mut chain = Vec::new();
    let mut cur = Some(from_leaf);
    while let Some(id) = cur {
        if id == fork_point {
            break;
        }
        let Some(e) = by_id.get(id) else { break };
        chain.push(*e);
        cur = e.parent_entry_id.as_deref();
    }
    // 从叶往上收集，回放顺序需要从老到新
    chain.reverse();
    chain
}

/// 把条目压成可读记录（工具调用本身不写进来，它属于助手那一轮）。
pub fn readable_turns(entries: &[&crate::session::EntryWrap]) -> Vec<AbandonedTurn> {
    let mut out = Vec::new();
    for e in entries {
        match &e.payload {
            Entry::Message {
                message: Msg::User { content, .. },
            } => out.push(AbandonedTurn {
                who: "user",
                text: content.clone(),
            }),
            Entry::Message {
                message: Msg::Assistant {
                    content: blocks, ..
                },
            } => {
                let text: String = blocks
                    .iter()
                    .filter_map(|b| match b {
                        crate::session::Block::Text { text } => Some(text.as_str()),
                        _ => None,
                    })
                    .collect::<Vec<_>>()
                    .join("\n");
                if !text.trim().is_empty() {
                    out.push(AbandonedTurn {
                        who: "assistant",
                        text,
                    });
                }
            }
            Entry::Message {
                message:
                    Msg::ToolResult {
                        tool_name, content, ..
                    },
            } => out.push(AbandonedTurn {
                who: "tool",
                text: format!("[{tool_name}] {}", head(content, 500)),
            }),
            _ => {}
        }
    }
    out
}

fn head(s: &str, n: usize) -> String {
    if s.chars().count() <= n {
        return s.to_string();
    }
    s.chars().take(n).collect::<String>() + "…"
}

/// 拼摘要请求正文。
fn serialize(turns: &[AbandonedTurn]) -> String {
    let mut out = String::from("<abandoned_branch>\n");
    for t in turns {
        out.push_str(&format!("[{}]\n{}\n", t.who, t.text));
    }
    out.push_str("</abandoned_branch>");
    out
}

/// 生成摘要文本（失败返回错误，调用方决定是放弃摘要还是放弃整次切换）。
pub async fn summarize(
    client: &dyn wb_provider::ModelClient,
    model_id: &str,
    turns: &[AbandonedTurn],
    cancel: tokio_util::sync::CancellationToken,
) -> Result<String> {
    let req = StreamRequest {
        model_id: model_id.to_string(),
        messages: vec![
            ReqMessage {
                role: Role::System,
                parts: vec![ContentPart::Text(prompt().to_string())],
                tool_calls: Vec::new(),
                tool_call_id: None,
            },
            ReqMessage {
                role: Role::User,
                parts: vec![ContentPart::Text(serialize(turns))],
                tool_calls: Vec::new(),
                tool_call_id: None,
            },
        ],
        tools: Vec::new(),
        max_tokens: SUMMARY_MAX_TOKENS,
        thinking: ThinkingLevel::Off,
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
    Ok(summary.trim().to_string())
}

/// 把摘要写进新分支开头（`/` 起头的独立一行做分隔，避免和用户原话混在一起）。
pub fn inject(sf: &mut SessionFile, summary: &str) -> Result<()> {
    if summary.trim().is_empty() {
        return Ok(());
    }
    sf.append(Entry::Message {
        message: Msg::user(format!(
            "[已离开的分支摘要] 你在另一条分支上做过下面这些事，随后切回来了。\
             这些内容不在当前上下文里，但结论仍然有效：\n\n{summary}"
        )),
    })
    .map(|_| ())
}
