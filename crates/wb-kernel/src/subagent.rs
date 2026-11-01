//! 子代理：隔离子会话跑一段任务，只把结论回灌父上下文。
//! 父上下文不塞中间过程——每轮采样与工具调用都发生在子会话文件里，
//! 父会话只拿到最后一段结论文本。

use std::time::Instant;

use futures::StreamExt;
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};
use wb_provider::{
    ContentPart, ModelEvent, ReqMessage, Role, StopReason, StreamRequest, ToolDef, Usage,
};

use crate::agent::{stop_name, AgentConfig};
use crate::blocks::{execute_tool, push_text, push_thinking, upsert_tool};
use crate::session::{now_ms, Block, Entry, Msg, SessionFile};
use crate::subagent_prompt::{build_sub_prompt, to_req};

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
    /// 角色码（scout / worker / reviewer，只用于界面标识）
    pub role_code: String,
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
    /// 过程事件出口（界面观测用）；None 即不观测
    pub observe: Option<ObserveFn>,
}

/// 子代理过程事件：只服务「看得见他们在干什么」，不参与上下文。
#[derive(Debug, Clone)]
pub enum SubagentEvent {
    /// 开跑（会话已建）
    Started {
        /// 子会话 id
        session_id: String,
        /// 角色码
        role_code: String,
        /// 任务描述
        task: String,
    },
    /// 正文增量
    TextDelta {
        /// 子会话 id
        session_id: String,
        /// 文本
        delta: String,
    },
    /// 思考增量
    ThinkingDelta {
        /// 子会话 id
        session_id: String,
        /// 文本
        delta: String,
    },
    /// 工具开始
    ToolStart {
        /// 子会话 id
        session_id: String,
        /// 调用 id
        call_id: String,
        /// 工具名
        name: String,
        /// 参数
        args: serde_json::Value,
    },
    /// 工具结束
    ToolEnd {
        /// 子会话 id
        session_id: String,
        /// 调用 id
        call_id: String,
        /// 工具名
        name: String,
        /// 结果（已截断）
        result: String,
        /// 是否错误
        is_error: bool,
        /// 耗时毫秒
        duration_ms: i64,
    },
    /// 结束（成功或失败都发）
    Finished {
        /// 子会话 id
        session_id: String,
        /// 是否失败
        failed: bool,
        /// 结论
        conclusion: String,
        /// 累计用量
        usage: Usage,
        /// 耗时毫秒
        duration_ms: i64,
    },
}

/// 子代理事件回调。
pub type ObserveFn = std::sync::Arc<dyn Fn(SubagentEvent) + Send + Sync>;

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
    let observe = |ev: SubagentEvent| {
        if let Some(f) = sc.observe.as_ref() {
            f(ev);
        }
    };
    observe(SubagentEvent::Started {
        session_id: session_id.clone(),
        role_code: sc.role_code.clone(),
        task: sc.task.clone(),
    });

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
                    ModelEvent::TextDelta(_, t) => {
                        observe(SubagentEvent::TextDelta {
                            session_id: session_id.clone(),
                            delta: t.clone(),
                        });
                        push_text(&mut blocks, t);
                    }
                    ModelEvent::ThinkingDelta(_, t) => {
                        observe(SubagentEvent::ThinkingDelta {
                            session_id: session_id.clone(),
                            delta: t.clone(),
                        });
                        push_thinking(&mut blocks, t);
                    }
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
                    observe(SubagentEvent::ToolStart {
                        session_id: session_id.clone(),
                        call_id: call_id.clone(),
                        name: name.clone(),
                        args: serde_json::from_str(&arguments)
                            .unwrap_or(serde_json::Value::String(arguments.clone())),
                    });
                    let run_cfg = cfg.clone();
                    let run_cancel = cancel.clone();
                    tasks.push(async move {
                        let out = execute_tool(&run_cfg, &name, &arguments, &run_cancel).await;
                        (call_id, name, out)
                    });
                }
                for (call_id, name, out) in futures::future::join_all(tasks).await {
                    observe(SubagentEvent::ToolEnd {
                        session_id: session_id.clone(),
                        call_id: call_id.clone(),
                        name: name.clone(),
                        result: crate::subagent::truncate_observe(&out.0),
                        is_error: out.1,
                        duration_ms: out.2,
                    });
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
    observe(SubagentEvent::Finished {
        session_id: session_id.clone(),
        failed,
        conclusion: conclusion.clone(),
        usage: usage_total.clone(),
        duration_ms: started.elapsed().as_millis() as i64,
    });
    Ok(SubagentOutcome {
        session_id,
        conclusion,
        failed,
        usage: usage_total,
    })
}

/// 观测用的结果截断：界面只看开头一段，完整结果仍在子会话文件里。
fn truncate_observe(text: &str) -> String {
    const CAP: usize = 4000;
    if text.chars().count() <= CAP {
        return text.to_string();
    }
    let head: String = text.chars().take(CAP).collect();
    format!("{head}\n…（已截断，完整结果在子会话文件里）")
}
