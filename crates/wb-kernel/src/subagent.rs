//! 子代理：隔离子会话跑一段任务，只把结论回灌父上下文。
//! 父上下文不塞中间过程——每轮采样与工具调用都发生在子会话文件里，
//! 父会话只拿到最后一段结论文本。

use std::time::Instant;

use futures::StreamExt;
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};
use wb_provider::{
    ContentPart, ModelEvent, ReqMessage, ReqToolCall, Role, StopReason, StreamRequest, ToolDef,
    Usage,
};

use crate::agent::AgentConfig;
use crate::blocks::{execute_tool, push_text, push_thinking, upsert_tool};
use crate::session::{now_ms, Block, Entry, Msg, SessionFile};

/// 子代理的运行结果。
#[derive(Debug, Clone)]
pub struct SubagentOutcome {
    /// 子代理会话 id（SUB_ 前缀）
    pub session_id: String,
    /// 结论正文（回灌给父会话）
    pub conclusion: String,
    /// 是否失败
    pub failed: bool,
    /// 累计用量
    pub usage: Usage,
}

/// 子代理的一次运行参数。
pub struct SubagentConfig {
    /// 父会话 id（写进子会话头，便于回溯）
    pub parent_session_id: String,
    /// 角色提示词（拼在子代理系统提示词前）
    pub role_prompt: String,
    /// 任务描述
    pub task: String,
    /// 子代理可用工具名（空 = 与父会话一致）
    pub tool_names: Vec<String>,
    /// 子会话存放目录
    pub sessions_dir: std::path::PathBuf,
    /// 工作区路径
    pub workspace: String,
    /// 最多跑几轮（模型轮次，不含工具回灌）
    pub max_turns: i64,
}

/// 跑一个子代理：独立会话文件、独立上下文，跑完只回结论。
/// 父会话的 `cfg` 提供模型客户端与工具注册表；工具集由 `tool_names` 收窄。
pub async fn run_subagent(
    cfg: &AgentConfig,
    sc: SubagentConfig,
    cancel: CancellationToken,
) -> Result<SubagentOutcome> {
    let session_id = wb_pkg::ulid::new_id(wb_pkg::Prefix::Sub);
    let mut session = SessionFile::create(&sc.sessions_dir, &sc.workspace, &session_id);
    let started = Instant::now();

    let tool_names: Vec<String> = if sc.tool_names.is_empty() {
        cfg.enabled_tools.clone()
    } else {
        sc.tool_names
            .iter()
            .filter(|n| !n.eq_ignore_ascii_case("task"))
            .cloned()
            .collect()
    };

    // 子代理系统提示词：角色职责 + 父提示词的工作区与技能段。
    let system_prompt = build_sub_prompt(&sc.role_prompt, &cfg.system_prompt, &tool_names);
    let mut usage_total = Usage::default();
    let mut conclusion = String::new();
    let mut last_text: Option<String> = None;
    let mut failed = false;

    session
        .append(Entry::Message {
            message: Msg::User {
                content: sc.task.clone(),
                timestamp: now_ms(),
            },
        })
        .map_err(|e| AppError::new(wb_domain::error::kernel::SUBAGENT_FAILED, e.message))?;

    let mut turn = 0i64;
    'outer: loop {
        turn += 1;
        if turn > sc.max_turns {
            break;
        }
        if cancel.is_cancelled() {
            break;
        }

        let mut req_msgs = vec![ReqMessage {
            role: Role::System,
            parts: vec![ContentPart::Text(system_prompt.clone())],
            tool_calls: Vec::new(),
            tool_call_id: None,
        }];
        for m in session.replay_messages() {
            req_msgs.push(to_req(m));
        }
        let tools: Vec<ToolDef> = tool_names
            .iter()
            .filter_map(|n| cfg.registry.get(n))
            .map(|t| ToolDef {
                name: t.name().to_string(),
                description: t.description().to_string(),
                parameters: t.parameters(),
            })
            .collect();
        let req = StreamRequest {
            model_id: cfg.model_id.clone(),
            messages: req_msgs,
            tools,
            max_tokens: cfg.max_tokens,
            thinking: cfg.thinking,
        };

        let mut blocks: Vec<Block> = Vec::new();
        let mut usage: Option<Usage> = None;
        let mut stop = StopReason::Stop;
        {
            let mut stream = match cfg.client.stream(req, cancel.clone()).await {
                Ok(s) => s,
                Err(e) => {
                    return Err(AppError::new(
                        wb_domain::error::kernel::SUBAGENT_FAILED,
                        format!("子代理采样失败：{}", e.message),
                    ));
                }
            };
            while let Some(ev) = stream.next().await {
                match ev {
                    ModelEvent::Start => {}
                    ModelEvent::TextDelta(_, t) => push_text(&mut blocks, t),
                    ModelEvent::ThinkingDelta(_, t) => push_thinking(&mut blocks, t),
                    ModelEvent::ToolCallDelta(_, _) => {
                        // 参数分片由 provider 侧累积，定稿事件带的是完整参数——这里不重复累加。
                    }
                    ModelEvent::ToolCallEnd(_, done) => upsert_tool(&mut blocks, done),
                    ModelEvent::Done(u, s) => {
                        usage = Some(u);
                        stop = s;
                    }
                    ModelEvent::Error(_, m) => {
                        failed = true;
                        let _ = m;
                        stop = StopReason::Error;
                    }
                }
            }
        }
        if let Some(u) = &usage {
            usage_total.input += u.input;
            usage_total.output += u.output;
            usage_total.cache_read += u.cache_read;
            usage_total.cache_write += u.cache_write;
        }

        let final_blocks = blocks.clone();
        // 记下本轮正文：轮次用尽时用它当结论。
        let round_text: String = final_blocks
            .iter()
            .filter_map(|b| match b {
                Block::Text { text } => Some(text.clone()),
                _ => None,
            })
            .collect::<Vec<_>>()
            .join("\n");
        if !round_text.trim().is_empty() {
            last_text = Some(round_text.clone());
        }
        session
            .append(Entry::Message {
                message: Msg::Assistant {
                    content: final_blocks.clone(),
                    provider_id: cfg.provider_id.clone(),
                    model_id: cfg.model_id.clone(),
                    message_id: wb_pkg::ulid::new_id(wb_pkg::Prefix::Msg),
                    usage: usage.clone(),
                    stop_reason: stop_name(stop).into(),
                    timestamp: now_ms(),
                    duration_ms: started.elapsed().as_millis() as i64,
                },
            })
            .map_err(|e| AppError::new(wb_domain::error::kernel::SUBAGENT_FAILED, e.message))?;

        if stop == StopReason::Error || stop == StopReason::Aborted {
            failed = true;
            break 'outer;
        }

        let calls: Vec<(String, String, String)> = final_blocks
            .iter()
            .filter_map(|b| match b {
                Block::ToolCall {
                    tool_call_id,
                    name,
                    arguments,
                } => Some((tool_call_id.clone(), name.clone(), arguments.clone())),
                _ => None,
            })
            .collect();

        if stop == StopReason::ToolUse && !calls.is_empty() {
            // 与主循环同一条纪律：只读并发、有副作用串行，结果按调用顺序入会话
            for group in crate::blocks::group_calls_for_concurrency(&cfg.registry, &calls) {
                let mut tasks = Vec::with_capacity(group.len());
                for idx in group {
                    let (call_id, name, arguments) = calls[idx].clone();
                    let run_cfg = cfg.clone();
                    let run_cancel = cancel.clone();
                    tasks.push(async move {
                        let out = execute_tool(&run_cfg, &name, &arguments, &run_cancel).await;
                        (call_id, name, out)
                    });
                }
                for (call_id, name, out) in futures::future::join_all(tasks).await {
                    session
                        .append(Entry::Message {
                            message: Msg::ToolResult {
                                tool_call_id: call_id,
                                tool_name: name,
                                content: out.0,
                                is_error: out.1,
                                timestamp: now_ms(),
                                duration_ms: out.2,
                            },
                        })
                        .map_err(|e| {
                            AppError::new(wb_domain::error::kernel::SUBAGENT_FAILED, e.message)
                        })?;
                }
            }
            continue 'outer;
        }

        conclusion = final_blocks
            .iter()
            .filter_map(|b| match b {
                Block::Text { text } => Some(text.clone()),
                _ => None,
            })
            .collect::<Vec<_>>()
            .join("\n");
        break 'outer;
    }

    // 轮次用尽时模型可能还停在工具调用上——退回到它最后说的正文，
    // 总比给父会话一句「没有产出结论」有用。
    if conclusion.trim().is_empty() {
        conclusion = last_text.unwrap_or_else(|| "（子代理没有产出结论）".into());
    }
    Ok(SubagentOutcome {
        session_id,
        conclusion,
        failed,
        usage: usage_total,
    })
}

/// 子代理系统提示词：角色职责在前，父提示词的上下文段在后。
fn build_sub_prompt(role_prompt: &str, parent_prompt: &str, tool_names: &[String]) -> String {
    let mut out = String::new();
    out.push_str(
        "You are a subagent running inside WorkBaby. You complete one delegated task and \
         report the conclusion. Work autonomously; the parent agent only sees your final report, \
         so make it self-contained and concrete.\n",
    );
    if !role_prompt.trim().is_empty() {
        out.push_str(&format!("\n<role>\n{}\n</role>\n", role_prompt.trim()));
    }
    out.push_str(&format!(
        "\n<tools>\n{}\n</tools>\n",
        tool_names
            .iter()
            .map(|n| format!("- {n}\n"))
            .collect::<String>()
    ));
    // 父提示词里的 cwd / skills 段对子代理同样有效。
    for tag in ["<cwd>", "<skills>"] {
        if let Some(part) = extract_tag(parent_prompt, tag) {
            out.push('\n');
            out.push_str(&part);
            out.push('\n');
        }
    }
    out.push_str(
        "\nReport format: state the outcome first, then the key facts you verified \
         (file paths, line numbers, command output). If the task failed, say so and why.\n",
    );
    out
}

/// 从父提示词里取出一个标签段（含首尾标签）。
fn extract_tag(text: &str, tag: &str) -> Option<String> {
    let start = text.find(tag)?;
    let end = text[start..].find("</")? + start;
    Some(text[start..end].to_string())
}

/// 内核消息 → provider 请求消息（与主循环同形）。
fn to_req(m: Msg) -> ReqMessage {
    match m {
        Msg::User { content, .. } => ReqMessage {
            role: Role::User,
            parts: vec![ContentPart::Text(content)],
            tool_calls: Vec::new(),
            tool_call_id: None,
        },
        Msg::Assistant { content, .. } => {
            let text = content
                .iter()
                .filter_map(|b| match b {
                    Block::Text { text } => Some(text.clone()),
                    _ => None,
                })
                .collect::<Vec<_>>()
                .join("\n");
            let calls = content
                .iter()
                .filter_map(|b| match b {
                    Block::ToolCall {
                        tool_call_id,
                        name,
                        arguments,
                    } => Some(ReqToolCall {
                        call_id: tool_call_id.clone(),
                        name: name.clone(),
                        arguments: arguments.clone(),
                    }),
                    _ => None,
                })
                .collect();
            ReqMessage {
                role: Role::Assistant,
                parts: vec![ContentPart::Text(text)],
                tool_calls: calls,
                tool_call_id: None,
            }
        }
        Msg::ToolResult {
            tool_call_id,
            content,
            ..
        } => ReqMessage {
            role: Role::ToolResult,
            parts: vec![ContentPart::Text(content)],
            tool_calls: Vec::new(),
            tool_call_id: Some(tool_call_id),
        },
    }
}

/// 停止原因 → 字符串。
fn stop_name(stop: StopReason) -> &'static str {
    match stop {
        StopReason::Stop => "stop",
        StopReason::Length => "length",
        StopReason::ToolUse => "tool_use",
        StopReason::Error => "error",
        StopReason::Aborted => "aborted",
    }
}

/// 子代理任务参数解析（供 task 工具使用）。
pub fn parse_task_args(args: &serde_json::Value) -> Result<(String, String, i64)> {
    let role = args
        .get("role")
        .and_then(|v| v.as_str())
        .unwrap_or("worker")
        .trim()
        .to_string();
    let task = args
        .get("task")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .trim()
        .to_string();
    let max_turns = args
        .get("max_turns")
        .and_then(|v| v.as_i64())
        .unwrap_or(12)
        .clamp(1, 64);
    if task.is_empty() {
        return Err(AppError::new(
            wb_domain::error::tool::ARG_INVALID,
            "task 不能为空",
        ));
    }
    Ok((role, task, max_turns))
}
