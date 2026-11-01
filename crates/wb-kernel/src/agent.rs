//! Agent 主循环：采样 → 流式事件 → 工具批 → 回灌；转向队列、自动重试、压缩挂钩。

use futures::StreamExt;
use std::sync::Arc;
use std::time::Instant;

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};
use wb_provider::{
    ContentPart, ModelClient, ModelEvent, ReqMessage, ReqToolCall, Role, StopReason, StreamRequest,
    ThinkingLevel, ToolDef, Usage,
};

use crate::blocks::{execute_tool, push_text, push_thinking, upsert_tool};
use crate::compaction;
use crate::events::{EmitFn, KernelEvent};
use crate::session::{now_ms, Block, Entry, Msg, SessionFile};

/// 一轮运行的装配参数。
#[derive(Clone)]
pub struct AgentConfig {
    /// 模型客户端
    pub client: Arc<dyn ModelClient>,
    /// 工具注册表
    pub registry: Arc<wb_tools::ToolRegistry>,
    /// 本轮启用的工具名（service 按开关合成）
    pub enabled_tools: Vec<String>,
    /// 组装好的系统提示词
    pub system_prompt: String,
    /// provider 键
    pub provider_id: String,
    /// 模型 id
    pub model_id: String,
    /// 上下文窗口
    pub context_window: i64,
    /// 最大输出
    pub max_tokens: i64,
    /// 思考档位
    pub thinking: ThinkingLevel,
    /// 自动重试上限（0 = 关）
    pub max_retries: i64,
    /// 工具执行的工作目录（= 当前工作区，与进程 cwd 无关）
    pub workdir: std::path::PathBuf,
    /// 压缩后回填的活状态（如待办清单）；service 决定内容，内核不做假设
    pub reinforce: Option<std::sync::Arc<crate::compaction::ReinforceFn>>,
}

/// 单轮运行结果。
pub struct RunOutcome {
    /// 是否被 abort
    pub aborted: bool,
    /// 最后一轮助手正文（无正文为空串）
    pub last_text: String,
    /// 各轮用量之和（供 service 入统计日表）
    pub usage_total: Usage,
}

/// 跑一轮：先落用户消息，随后「采样→工具→回灌」直到停止/中止/失败。
/// steering_rx：运行中插话（one-at-a-time，每个 turn 边界取一条）。
pub async fn run(
    mut session: SessionFile,
    cfg: AgentConfig,
    emit: EmitFn,
    cancel: CancellationToken,
    mut steer_rx: mpsc::Receiver<String>,
    first_message: String,
) -> Result<RunOutcome> {
    session
        .append(Entry::Message {
            message: Msg::User {
                content: first_message,
                timestamp: now_ms(),
            },
        })
        .map_err(loop_err)?;
    emit(&KernelEvent::AgentStart {
        run_id: wb_pkg::ulid::new_id(wb_pkg::Prefix::Run),
    });

    let mut retry: i64 = 0;
    let mut last_text = String::new();
    let mut aborted = false;
    let mut failed: Option<AppError> = None;
    let mut usage_total = Usage::default();

    'outer: loop {
        // turn 边界收插话（one-at-a-time）
        while let Ok(text) = steer_rx.try_recv() {
            session
                .append(Entry::Message {
                    message: Msg::User {
                        content: text,
                        timestamp: now_ms(),
                    },
                })
                .map_err(loop_err)?;
        }
        emit(&KernelEvent::TurnStart);

        // 预采样压缩
        let msgs = session.replay_messages();
        let tokens = compaction::estimate_tokens(&msgs);
        if compaction::should_compact(tokens, cfg.context_window) {
            emit(&KernelEvent::CompactionStart {
                reason: "threshold".into(),
            });
            match compaction::compact(
                &mut session,
                cfg.client.as_ref(),
                &cfg.model_id,
                cfg.max_tokens,
                cfg.reinforce.as_deref(),
                cancel.clone(),
            )
            .await
            {
                Ok((before, _)) => emit(&KernelEvent::CompactionEnd {
                    tokens_before: Some(before),
                }),
                Err(e) => {
                    emit(&KernelEvent::CompactionEnd {
                        tokens_before: None,
                    });
                    failed = Some(e);
                }
            }
        }

        // 请求上下文：系统提示词 + 回放消息
        let mut req_msgs = vec![ReqMessage {
            role: Role::System,
            parts: vec![ContentPart::Text(cfg.system_prompt.clone())],
            tool_calls: Vec::new(),
            tool_call_id: None,
        }];
        for m in session.replay_messages() {
            req_msgs.push(to_req(m));
        }
        let tools: Vec<ToolDef> = cfg
            .enabled_tools
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

        // 流式采样：累积内容块
        let started = Instant::now();
        let mut blocks: Vec<Block> = Vec::new();
        let mut usage: Option<Usage> = None;
        let mut stop = StopReason::Stop;
        let mut stream_err: Option<String> = None;
        {
            let mut stream = match cfg.client.stream(req, cancel.clone()).await {
                Ok(s) => s,
                Err(e) => {
                    stream_err = Some(e.message);
                    stop = StopReason::Error;
                    Box::pin(futures::stream::empty())
                }
            };
            // 事件索引 → 块槽位映射：文本/思考各一段，工具按 provider index
            while let Some(ev) = stream.next().await {
                match ev {
                    ModelEvent::Start => {}
                    ModelEvent::TextDelta(_, t) => {
                        emit(&KernelEvent::TextDelta { delta: t.clone() });
                        push_text(&mut blocks, t);
                    }
                    ModelEvent::ThinkingDelta(_, t) => {
                        emit(&KernelEvent::ThinkingDelta { delta: t.clone() });
                        push_thinking(&mut blocks, t);
                    }
                    ModelEvent::ToolCallDelta(_, _) => {
                        // 参数分片由 provider 侧累积，定稿事件带的是完整参数——这里不重复累加。
                    }
                    ModelEvent::ToolCallEnd(_, done) => {
                        upsert_tool(&mut blocks, done);
                    }
                    ModelEvent::Done(u, s) => {
                        usage = Some(u);
                        stop = s;
                    }
                    ModelEvent::Error(retryable, m) => {
                        stream_err = Some(format!("{}（可重试：{retryable}）", m));
                        stop = StopReason::Error;
                    }
                }
            }
        }

        // 错误处理：可重试 → 退避重来；不可重试 → 失败收场
        if stop == StopReason::Error || stop == StopReason::Aborted {
            if stop == StopReason::Aborted {
                aborted = true;
            } else {
                let msg = stream_err.unwrap_or_default();
                let retryable = msg.contains("可重试：true") || retryable_text(&msg);
                if cfg.max_retries > 0 && retryable && retry < cfg.max_retries {
                    retry += 1;
                    let delay = (2000i64 * (1 << (retry - 1).min(5))).min(60_000);
                    emit(&KernelEvent::AutoRetry {
                        attempt: retry,
                        max: cfg.max_retries,
                        delay_ms: delay,
                    });
                    if tokio::time::timeout(
                        std::time::Duration::from_millis(delay as u64),
                        cancel.cancelled(),
                    )
                    .await
                    .is_ok()
                    {
                        aborted = true;
                    } else {
                        continue 'outer;
                    }
                } else {
                    failed = Some(AppError::new(wb_domain::error::kernel::STREAM_BROKEN, msg));
                    break 'outer;
                }
            }
        }

        // 落盘助手消息
        if let Some(u) = &usage {
            usage_total.input += u.input;
            usage_total.output += u.output;
            usage_total.cache_read += u.cache_read;
            usage_total.cache_write += u.cache_write;
            usage_total.reasoning += u.reasoning;
        }
        let final_blocks = blocks.clone();
        let assistant_entry = session
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
            .map_err(loop_err)?;
        let blocks_json = serde_json::to_value(&final_blocks).unwrap_or(serde_json::Value::Null);
        emit(&KernelEvent::AssistantFinal {
            entry_id: assistant_entry.entry_id.clone(),
            blocks: blocks_json,
            usage: usage.clone(),
            stop_reason: stop,
        });
        emit(&KernelEvent::MessageEnd {
            entry_id: assistant_entry.entry_id.clone(),
        });

        if aborted {
            break 'outer;
        }

        // 工具批执行
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

        let stop2 = stop;
        if stop2 == StopReason::ToolUse {
            // 同批调用并发跑：只读的并组一起执行，带副作用的单独排队。
            // join_all 保持入参顺序，会话里的 ToolResult 次序与串行版一致。
            for group in crate::blocks::group_calls_for_concurrency(&cfg.registry, &calls) {
                let mut tasks = Vec::with_capacity(group.len());
                for idx in group {
                    let (call_id, name, arguments) = calls[idx].clone();
                    emit(&KernelEvent::ToolExecutionStart {
                        args: serde_json::from_str(&arguments).unwrap_or(serde_json::Value::Null),
                        call_id: call_id.clone(),
                        name: name.clone(),
                    });
                    let run_cfg = cfg.clone();
                    let run_cancel = cancel.clone();
                    tasks.push(async move {
                        let out = execute_tool(&run_cfg, &name, &arguments, &run_cancel).await;
                        (call_id, name, out)
                    });
                }
                for (call_id, name, out) in futures::future::join_all(tasks).await {
                    emit(&KernelEvent::ToolExecutionEnd {
                        call_id: call_id.clone(),
                        name: name.clone(),
                        result: out.0.clone(),
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
                        .map_err(loop_err)?;
                }
            }
            emit(&KernelEvent::TurnEnd {
                usage: usage.clone(),
                stop_reason: stop2,
            });
            continue 'outer;
        }

        // 正文收尾
        last_text = final_blocks
            .iter()
            .filter_map(|b| match b {
                Block::Text { text } => Some(text.clone()),
                _ => None,
            })
            .collect::<Vec<_>>()
            .join("\n");
        emit(&KernelEvent::TurnEnd {
            usage: usage.clone(),
            stop_reason: stop2,
        });
        break 'outer;
    }

    if let Some(e) = failed {
        emit(&KernelEvent::Failed {
            code: e.code.clone(),
            message: e.message.clone(),
        });
    }
    emit(&KernelEvent::AgentEnd {
        will_retry: false,
        stop_reason: if aborted {
            StopReason::Aborted
        } else {
            StopReason::Stop
        },
    });
    emit(&KernelEvent::Settled { aborted });
    Ok(RunOutcome {
        aborted,
        last_text,
        usage_total,
    })
}

/// 内核消息 → provider 请求消息。
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

fn stop_name(s: StopReason) -> &'static str {
    match s {
        StopReason::Stop => "stop",
        StopReason::Length => "length",
        StopReason::ToolUse => "tool_use",
        StopReason::Error => "error",
        StopReason::Aborted => "aborted",
    }
}

/// 粗判可重试文本（provider 未标注时的兜底）。
fn retryable_text(msg: &str) -> bool {
    let m = msg.to_lowercase();
    [
        "rate limit",
        "overloaded",
        "timeout",
        "429",
        "500",
        "502",
        "503",
        "504",
        "network",
        "connection",
    ]
    .iter()
    .any(|k| m.contains(k))
}

fn loop_err(e: AppError) -> AppError {
    e
}

/// ToolCall 供外部引用（service 构造运行态调用时用）。
pub use wb_tools::ToolCall as RuntimeToolCall;
