//! 会话读取：列表 / 详情 / 消息形态转换（内核条目 → 界面 VO）。

use std::sync::Arc;

use wb_domain::error::session as code;
use wb_domain::session::{
    MessageBlock, MessageVO, SessionDetailRESP, SessionVO, ToolCallVO, UsageVO,
};
use wb_pkg::{AppError, Result};

use super::Services;
use wb_kernel::{Block, Msg, SessionFile};

/// 会话列表（索引表）。
pub fn list(sv: &Arc<Services>) -> Result<Vec<SessionVO>> {
    let rows = sv.repo.sessions_list()?;
    let running = sv.chat.lock().unwrap().running_ids();
    Ok(rows
        .into_iter()
        .map(|r| {
            let is_running = running.contains(&r.session_id);
            SessionVO {
                session_id: r.session_id,
                session_file: String::new(),
                session_title: r.session_name,
                session_workspace: r.session_workspace,
                session_provider: r.session_provider,
                session_model: r.session_model,
                session_thinking_level: "medium".into(),
                session_message_count: r.session_message_count,
                session_total_tokens: 0,
                session_cost: 0.0,
                session_running: is_running,
                session_created_at: r.session_last_active_at,
                session_updated_at: r.session_last_active_at,
            }
        })
        .collect())
}

/// 会话详情（回放消息）。
pub fn detail(sv: &Arc<Services>, session_id: &str) -> Result<SessionDetailRESP> {
    let path = find_session_file(sv, session_id)?;
    let sf = SessionFile::load(&path)?;
    let messages = to_message_vos(sf.replay_messages());
    Ok(SessionDetailRESP {
        detail_session: SessionVO {
            session_id: session_id.to_string(),
            session_file: path.display().to_string(),
            session_title: session_name(&sf),
            session_workspace: sf.header.workspace.clone(),
            session_provider: String::new(),
            session_model: String::new(),
            session_thinking_level: "medium".into(),
            session_message_count: messages.len() as i64,
            session_total_tokens: 0,
            session_cost: 0.0,
            session_running: sv.chat.lock().unwrap().is_running(session_id),
            session_created_at: sf.header.created_at,
            session_updated_at: wb_pkg::fsutil::mtime_ms(&path).unwrap_or(sf.header.created_at),
        },
        detail_messages: messages,
    })
}

/// 内核消息 → 界面 VO（保持原始块序）。
pub fn to_message_vos(msgs: Vec<Msg>) -> Vec<MessageVO> {
    let mut out = Vec::new();
    for (idx, m) in msgs.into_iter().enumerate() {
        match m {
            Msg::User { content, timestamp } => {
                let mut vo = MessageVO::empty(format!("ENT_U{idx}"), "user", timestamp);
                vo.message_content = Some(content);
                out.push(vo);
            }
            Msg::Assistant {
                content,
                provider_id,
                model_id,
                message_id,
                usage,
                stop_reason,
                timestamp,
                duration_ms,
            } => {
                let _ = (&provider_id, &model_id, &duration_ms);
                let mut vo = MessageVO::empty(message_id, "assistant", timestamp);
                vo.message_stop_reason = Some(stop_reason);
                vo.message_usage = usage.map(|u| UsageVO {
                    usage_input: u.input,
                    usage_output: u.output,
                    usage_cached: u.cache_read,
                    usage_cache_write: u.cache_write,
                    usage_total: u.input + u.output + u.cache_read + u.cache_write,
                    usage_context: u.input + u.output + u.cache_read + u.cache_write,
                    usage_cost: 0.0,
                });
                let mut thinking_all = String::new();
                let mut text_all = String::new();
                for b in &content {
                    match b {
                        Block::Thinking { thinking } => thinking_all.push_str(thinking),
                        Block::Text { text } => text_all.push_str(text),
                        Block::ToolCall {
                            tool_call_id,
                            name,
                            arguments,
                        } => {
                            vo.message_blocks.push(MessageBlock {
                                block_kind: "tool".into(),
                                block_text: None,
                                tool_call_id: Some(tool_call_id.clone()),
                                tool_name: Some(name.clone()),
                                tool_label: None,
                                tool_args: serde_json::from_str(arguments).ok(),
                            });
                            vo.message_tool_calls.push(ToolCallVO {
                                tool_call_id: tool_call_id.clone(),
                                tool_name: name.clone(),
                                tool_label: None,
                                tool_args: serde_json::from_str(arguments).ok(),
                            });
                        }
                    }
                }
                if !thinking_all.is_empty() {
                    vo.message_thinking = Some(thinking_all);
                }
                if !text_all.is_empty() {
                    vo.message_content = Some(text_all.clone());
                    vo.message_blocks.insert(
                        0,
                        MessageBlock {
                            block_kind: "text".into(),
                            block_text: Some(text_all),
                            tool_call_id: None,
                            tool_name: None,
                            tool_label: None,
                            tool_args: None,
                        },
                    );
                }
                out.push(vo);
            }
            Msg::ToolResult {
                tool_call_id,
                tool_name,
                content,
                is_error,
                timestamp,
                duration_ms,
            } => {
                let mut vo = MessageVO::empty(format!("ENT_T{idx}"), "tool", timestamp);
                vo.message_type = "tool".into();
                vo.message_tool_call_id = Some(tool_call_id);
                vo.message_tool_name = Some(tool_name);
                vo.message_is_error = Some(is_error);
                vo.message_content = Some(content);
                let _ = duration_ms;
                out.push(vo);
            }
        }
    }
    out
}

/// 按会话 id 定位文件（扫当前工作区目录）。
pub fn find_session_file(sv: &Arc<Services>, session_id: &str) -> Result<std::path::PathBuf> {
    let dir = sv.sessions_dir();
    for p in wb_pkg::fsutil::list_dir(&dir) {
        if p.extension().and_then(|e| e.to_str()) == Some("jsonl") {
            let name = p.file_name().unwrap_or_default().to_string_lossy();
            // 文件名形如 {ms}_{SESS_xxx}.jsonl
            if let Some(id_part) = name.trim_end_matches(".jsonl").split_once('_').map(|x| x.1) {
                if id_part == session_id {
                    return Ok(p);
                }
            }
        }
    }
    Err(AppError::new(code::NOT_FOUND, "会话不存在").with_details(session_id.to_string()))
}

/// HTML 转义。
fn esc(s: &str) -> String {
    s.replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
}

/// 导出会话为 HTML（自包含单文件，无外部依赖）。
pub fn export_html(sv: &Arc<Services>, session_id: &str, out_path: &str) -> Result<String> {
    let detail = detail(sv, session_id)?;
    let mut body = String::new();
    for m in &detail.detail_messages {
        if m.message_hidden {
            continue;
        }
        let who = match m.message_role.as_str() {
            "user" => "用户",
            "assistant" => "助手",
            "tool" => "工具",
            _ => "提示",
        };
        let text = m
            .message_notice
            .clone()
            .or_else(|| m.message_content.clone())
            .unwrap_or_default();
        body.push_str(&format!(
            "<div class=\"m {}\"><span class=\"who\">{}</span><pre>{}</pre></div>\n",
            m.message_role,
            who,
            esc(&text)
        ));
    }
    let html = format!(
        "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>{}</title>\
<style>body{{font-family:system-ui,sans-serif;max-width:820px;margin:0 auto;padding:24px;color:#1e293b}}\
.m{{margin:12px 0;padding:12px;border-radius:8px;background:#f8fafc}}\
.m.assistant{{background:#e0e7ff}}\
.m .who{{font-weight:700;font-size:12px;color:#6366f1;display:block;margin-bottom:4px}}\
pre{{white-space:pre-wrap;word-break:break-word;margin:0;font-family:inherit}}</style></head>\
<body><h1>{}</h1>{body}</body></html>",
        esc(&detail.detail_session.session_title),
        esc(&detail.detail_session.session_title),
    );
    let path = if out_path.is_empty() {
        sv.data_dir
            .join("exports")
            .join(format!("{session_id}.html"))
    } else {
        std::path::PathBuf::from(out_path)
    };
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).map_err(|e| AppError::wrap("1002", "建导出目录失败", e))?;
    }
    std::fs::write(&path, html).map_err(|e| AppError::wrap("1002", "写导出文件失败", e))?;
    Ok(path.display().to_string())
}

fn session_name(sf: &SessionFile) -> String {
    for e in sf.path_to_leaf().iter().rev() {
        if let wb_kernel::Entry::SessionInfo {
            session_name: Some(n),
        } = &e.payload
        {
            return n.clone();
        }
    }
    String::new()
}
