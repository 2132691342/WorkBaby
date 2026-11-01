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
/// 索引行已存有名称、provider、model 与活动时间；创建时间要回到会话头读，
/// 否则列表里每条会话的「创建」都等于「最后一次说话」。
pub fn list(sv: &Arc<Services>) -> Result<Vec<SessionVO>> {
    let rows = sv.repo.sessions_list()?;
    let running = sv.chat.lock().unwrap().running_ids();
    let dir = sv.sessions_dir();
    Ok(rows
        .into_iter()
        .map(|r| {
            let is_running = running.contains(&r.session_id);
            // 索引只做加速：真名与创建时间以会话文件为准，读不到才用索引值
            let created_at = find_session_file(sv, &r.session_id)
                .ok()
                .and_then(|p| SessionFile::load(&p).ok())
                .map(|sf| sf.header.created_at)
                .unwrap_or(r.session_last_active_at);
            let title = if r.session_name.trim().is_empty() {
                r.session_summary.chars().take(40).collect()
            } else {
                r.session_name
            };
            SessionVO {
                session_id: r.session_id,
                session_file: dir.display().to_string(),
                session_title: title,
                session_workspace: r.session_workspace,
                session_provider: r.session_provider,
                session_model: r.session_model,
                session_thinking_level: "medium".into(),
                session_message_count: r.session_message_count,
                session_total_tokens: 0,
                session_cost: 0.0,
                session_running: is_running,
                session_created_at: created_at,
                session_updated_at: r.session_last_active_at,
            }
        })
        .collect())
}

/// 会话详情（回放消息）。
/// 同时给出当前叶子与可选分支点，界面据此提供「从这里重开」。
pub fn detail(sv: &Arc<Services>, session_id: &str) -> Result<SessionDetailRESP> {
    let path = find_session_file(sv, session_id)?;
    let sf = SessionFile::load(&path)?;
    let (provider, model) = model_of(&sf);
    let detail_session = SessionVO {
        session_id: session_id.to_string(),
        session_file: path.display().to_string(),
        session_title: session_name(&sf),
        session_workspace: sf.header.workspace.clone(),
        session_provider: provider,
        session_model: model,
        session_thinking_level: thinking_of(&sf),
        session_message_count: 0,
        session_total_tokens: tokens_of(&sf),
        session_cost: 0.0,
        session_running: sv.chat.lock().unwrap().is_running(session_id),
        session_created_at: sf.header.created_at,
        session_updated_at: wb_pkg::fsutil::mtime_ms(&path).unwrap_or(sf.header.created_at),
    };
    let mut messages = message_vos_with_entries(&sf);
    let mut detail_session = detail_session;
    detail_session.session_message_count = messages.len() as i64;
    // 已被其他分支越过、可再次进入的叶子（旧分支的末端）
    let leaves = wb_kernel::session::alternate_leaves(&sf);
    // 消息 id 直接用真实条目 id，不另造假号，否则前端无从指定分支点
    let mut out = SessionDetailRESP {
        detail_session,
        detail_messages: std::mem::take(&mut messages),
        detail_leaf_entry_id: sf.leaf.clone(),
        detail_fork_entry_ids: wb_kernel::session::user_entry_ids(&sf),
        detail_alt_leaf_ids: leaves,
    };
    // 顺序保持原有字段语义：先说清可分支点，再谈现叶
    out.detail_alt_leaf_ids = wb_kernel::session::alternate_leaves(&sf);
    Ok(out)
}

/// 回放消息并带上真实条目 id（与「用户消息可作分支点」一一对应）。
pub fn message_vos_with_entries(sf: &SessionFile) -> Vec<MessageVO> {
    let mut out = Vec::new();
    let path = sf.path_to_leaf();
    // 仅编译期可见的映射：回放顺序 ↔ 目标条目 id
    for e in path.iter() {
        let msgs = match &e.payload {
            wb_kernel::Entry::Message { message } => vec![message.clone()],
            wb_kernel::Entry::Compaction {
                summary,
                first_kept_entry_id,
                tokens_before,
            } => {
                let _ = (first_kept_entry_id, tokens_before);
                vec![Msg::User {
                    content: format!("[已压缩的早期对话摘要]\n{summary}"),
                    timestamp: e.timestamp,
                }]
            }
            _ => Vec::new(),
        };
        let mut vos = to_message_vos(msgs);
        for vo in vos.iter_mut() {
            vo.message_entry_id = e.entry_id.clone();
        }
        out.append(&mut vos);
    }
    out
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

/// 从指定用户消息之后重开：旧分支完整保留，新消息接在它后面。
pub fn branch(sv: &Arc<Services>, session_id: &str, entry_id: &str) -> Result<SessionDetailRESP> {
    let path = find_session_file(sv, session_id)?;
    if sv.chat.lock().unwrap().is_running(session_id) {
        return Err(AppError::new(code::BUSY, "会话正在跑，跑完再回溯"));
    }
    let mut sf = SessionFile::load(&path)?;
    // 只能在用户消息之后分叉，否则会把工具调用和它的结果拆开
    let ok = sf.entries.iter().any(|e| {
        e.entry_id == entry_id
            && matches!(
                &e.payload,
                wb_kernel::Entry::Message {
                    message: Msg::User { .. }
                }
            )
    });
    if !ok {
        return Err(AppError::new(code::NOT_FOUND, "分支点不存在或不是用户消息")
            .with_details(entry_id.to_string()));
    }
    sf.fork_at(entry_id)?;
    detail(sv, session_id)
}

/// 切回某个旧分支的叶子。
pub fn switch_leaf(
    sv: &Arc<Services>,
    session_id: &str,
    leaf_entry_id: &str,
) -> Result<SessionDetailRESP> {
    let path = find_session_file(sv, session_id)?;
    if sv.chat.lock().unwrap().is_running(session_id) {
        return Err(AppError::new(code::BUSY, "会话正在跑，跑完再切分支"));
    }
    let mut sf = SessionFile::load(&path)?;
    if !sf.entries.iter().any(|e| e.entry_id == leaf_entry_id) {
        return Err(AppError::new(code::NOT_FOUND, "目标叶子不存在")
            .with_details(leaf_entry_id.to_string()));
    }
    sf.fork_at(leaf_entry_id)?;
    detail(sv, session_id)
}

pub fn session_name(sf: &SessionFile) -> String {
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

/// 会话当前模型：取最近一次 ModelChange（未切换过则为空，界面回落默认模型）。
pub fn model_of(sf: &SessionFile) -> (String, String) {
    for e in sf.path_to_leaf().iter().rev() {
        if let wb_kernel::Entry::ModelChange {
            provider_id,
            model_id,
        } = &e.payload
        {
            return (provider_id.clone(), model_id.clone());
        }
    }
    (String::new(), String::new())
}

/// 会话当前思考档：取最近一次 ThinkingChange，缺省 medium。
pub fn thinking_of(sf: &SessionFile) -> String {
    for e in sf.path_to_leaf().iter().rev() {
        if let wb_kernel::Entry::ThinkingChange { thinking_level } = &e.payload {
            return thinking_level.clone();
        }
    }
    "medium".into()
}

/// 会话累计上下文读数：最后一条带用量的助手消息即当前水位。
pub fn tokens_of(sf: &SessionFile) -> i64 {
    for e in sf.path_to_leaf().iter().rev() {
        if let wb_kernel::Entry::Message {
            message: Msg::Assistant { usage: Some(u), .. },
        } = &e.payload
        {
            return u.input + u.output + u.cache_read + u.cache_write;
        }
    }
    0
}
