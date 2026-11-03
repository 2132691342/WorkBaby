//! 会话读取：列表 / 详情 / 消息形态转换（内核条目 → 界面 VO）。

use std::sync::Arc;

use wb_domain::error::session as code;
use wb_domain::session::{
    MessageBlock, MessageImageVO, MessageVO, SessionDetailRESP, SessionVO, ToolCallVO, UsageVO,
};
use wb_pkg::{AppError, Result};

use super::Services;
use wb_kernel::{Block, Msg, SessionFile};

/// 会话列表（索引表），只列当前工作区的会话：工作区切了，列表就得跟着换。
pub fn list(sv: &Arc<Services>) -> Result<Vec<SessionVO>> {
    let rows = sv.repo.sessions_list_in(&sv.workspace_text())?;
    let running = sv.chat.lock().unwrap().running_ids();
    let dir = sv.sessions_dir();
    // 扫一次目录建 id → 路径索引：逐行 find_session_file 会把列表变成 O(n²) 扫盘，
    // 会话一多展开就卡住。
    let by_id = session_file_index(&dir);
    Ok(rows
        .into_iter()
        .map(|r| {
            let is_running = running.contains(&r.session_id);
            // 索引只做加速：真名与创建时间以会话文件为准，读不到才用索引值
            let created_at = by_id
                .get(&r.session_id)
                .and_then(|p| SessionFile::load(p).ok())
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
    let (provider, model) = model_of(sv, &sf);
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
        detail_tree: Default::default(),
    };
    // 顺序保持原有字段语义：先说清可分支点，再谈现叶
    out.detail_alt_leaf_ids = wb_kernel::session::alternate_leaves(&sf);
    out.detail_tree = tree_vo(&wb_kernel::session_tree::build_tree(&sf));
    Ok(out)
}

/// 内核树 → 视图树。
fn tree_vo(node: &wb_kernel::session_tree::TreeNode) -> wb_domain::session::SessionTreeNodeVO {
    wb_domain::session::SessionTreeNodeVO {
        node_entry_id: node.node_entry_id.clone(),
        node_parent_entry_id: node.node_parent_entry_id.clone(),
        node_preview: node.node_preview.clone(),
        node_is_user: node.node_is_user,
        node_on_path: node.node_on_path,
        node_children: node.node_children.iter().map(tree_vo).collect(),
    }
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
                vec![Msg::user(format!("[已压缩的早期对话摘要]\n{summary}"))]
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
            Msg::User {
                content,
                timestamp,
                skill,
                images,
            } => {
                let mut vo = MessageVO::empty(format!("ENT_U{idx}"), "user", timestamp);
                vo.message_content = Some(content);
                vo.message_skill = skill;
                vo.message_images = images
                    .into_iter()
                    .map(|i| MessageImageVO {
                        image_mime: i.mime,
                        image_base64: i.base64,
                    })
                    .collect();
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
                file_path,
                diff,
                images,
            } => {
                let mut vo = MessageVO::empty(format!("ENT_T{idx}"), "tool", timestamp);
                vo.message_type = "tool".into();
                vo.message_tool_call_id = Some(tool_call_id);
                vo.message_tool_name = Some(tool_name);
                vo.message_is_error = Some(is_error);
                vo.message_content = Some(content);
                // 改动文件与差异行随工具结果落盘：回放会话时界面同样能就地着色
                vo.message_file_path = file_path.clone();
                vo.message_diff = diff.clone();
                // 读图类工具产生的图片回放时要看得到（否则刷新一下图就没了）
                vo.message_images = images
                    .into_iter()
                    .map(|i| MessageImageVO {
                        image_mime: i.mime,
                        image_base64: i.base64,
                    })
                    .collect();
                let _ = duration_ms;
                out.push(vo);
            }
        }
    }
    out
}

/// 会话目录一次性扫成 id → 文件路径的索引（文件名形如 `{ms}_{SESS_xxx}.jsonl`）。
fn session_file_index(
    dir: &std::path::Path,
) -> std::collections::HashMap<String, std::path::PathBuf> {
    let mut out = std::collections::HashMap::new();
    for p in wb_pkg::fsutil::list_dir(dir) {
        if p.extension().and_then(|e| e.to_str()) != Some("jsonl") {
            continue;
        }
        let name = p
            .file_name()
            .unwrap_or_default()
            .to_string_lossy()
            .into_owned();
        if let Some(id) = name.trim_end_matches(".jsonl").split_once('_').map(|x| x.1) {
            out.insert(id.to_string(), p);
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
/// 从指定用户消息之后重开：旧分支完整保留，新消息接在它后面。
/// 被放弃的那段先做成摘要插在新分支开头——否则旧分支上做过的事整段消失。
pub async fn branch(
    sv: &Arc<Services>,
    session_id: &str,
    entry_id: &str,
) -> Result<SessionDetailRESP> {
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
    // 被放弃的那段先摘要成一条消息插进新分支：只把叶子指回去的话，
    // 旧分支上做过的事会整段从模型视野里消失。摘要失败不能挡住回溯本身。
    let abandoned =
        wb_kernel::branch_summary::readable_turns(&wb_kernel::branch_summary::abandoned_entries(
            &sf,
            &sf.leaf.clone().unwrap_or_default(),
            entry_id,
        ));
    let summary = summarize_branch(sv, &sf, &abandoned).await;
    sf.fork_at(entry_id)?;
    if let Some(text) = summary {
        wb_kernel::branch_summary::inject(&mut sf, &text)?;
    }
    detail(sv, session_id)
}

/// 切回某个旧分支的叶子（同样先把当前分支摘要留下）。
pub async fn switch_leaf(
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
    let abandoned =
        wb_kernel::branch_summary::readable_turns(&wb_kernel::branch_summary::abandoned_entries(
            &sf,
            &sf.leaf.clone().unwrap_or_default(),
            leaf_entry_id,
        ));
    let summary = summarize_branch(sv, &sf, &abandoned).await;
    sf.fork_at(leaf_entry_id)?;
    if let Some(text) = summary {
        wb_kernel::branch_summary::inject(&mut sf, &text)?;
    }
    detail(sv, session_id)
}

/// 给被放弃的分支生成摘要：没内容 / 取不到模型 / 上游报错都退回 None，
/// 回溯本身不能因为摘要失败而被拦住。
async fn summarize_branch(
    sv: &Arc<Services>,
    sf: &SessionFile,
    turns: &[wb_kernel::branch_summary::AbandonedTurn],
) -> Option<String> {
    if turns.is_empty() {
        return None;
    }
    let (pid, mid) = model_of(sv, sf);
    let resolved = sv.providers.resolve(&pid, Some(&mid)).ok()?;
    let client = wb_provider::create_client(resolved).ok()?;
    match wb_kernel::branch_summary::summarize(
        client.as_ref(),
        &mid,
        turns,
        tokio_util::sync::CancellationToken::new(),
    )
    .await
    {
        Ok(text) if !text.is_empty() => Some(text),
        Ok(_) => None,
        Err(e) => {
            tracing::warn!(code = %e.code, "分支摘要生成失败，回溯继续");
            None
        }
    }
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

/// 会话当前模型：取最近一次 ModelChange。
/// 会话从未切过模型时回落到默认模型——空串会让需要模型的地方（分支摘要 / 导出）
/// 以为「没有模型可用」，而真相是「用默认那个」。
pub fn model_of(sv: &Arc<Services>, sf: &SessionFile) -> (String, String) {
    for e in sf.path_to_leaf().iter().rev() {
        if let wb_kernel::Entry::ModelChange {
            provider_id,
            model_id,
        } = &e.payload
        {
            return (provider_id.clone(), model_id.clone());
        }
    }
    let dp = sv
        .repo
        .settings_get("default_provider")
        .ok()
        .flatten()
        .unwrap_or_default();
    let dm = sv
        .repo
        .settings_get("default_model")
        .ok()
        .flatten()
        .unwrap_or_default();
    (dp, dm)
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
