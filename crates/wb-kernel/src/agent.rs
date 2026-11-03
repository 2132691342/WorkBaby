//! Agent 主循环：采样 → 流式事件 → 工具批 → 回灌；转向队列、自动重试、压缩挂钩。

use futures::StreamExt;
use std::sync::Arc;
use std::time::Instant;

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};
use wb_provider::{
    ContentPart, ModelClient, ModelEvent, ReqMessage, Role, StopReason, StreamRequest,
    ThinkingLevel, ToolDef, Usage,
};

use crate::blocks::{execute_tool, push_text, push_thinking, upsert_tool};
use crate::compaction;
use crate::events::{EmitFn, KernelEvent};
use crate::session::{now_ms, Block, Entry, Msg, SessionFile, UserInput};

/// 单轮运行的模型请求上限：模型反复调工具却不收尾时的总闸。
const MAX_TURNS: i64 = 60;

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
    /// 每轮都重发的固定开销（系统提示词 + 工具定义）；不计入就高估剩余窗口
    pub overhead: crate::compaction::ContextOverhead,
    /// 延迟加载的工具名：模型搜到后加进来，下一轮才有它们的定义。
    /// 空 = 工具表固定，全部工具一开始就在请求里。
    pub deferred_tools: Vec<String>,
    /// 技能正文加载器：用户消息点名技能时按需读正文，内核不认识技能存储
    pub skill_loader: Option<crate::reqbuild::SkillLoader>,
    /// 当前会话 id：工具回话（提问 / 状态行 / 小部件）必须写进发起调用那个会话
    pub session_id: String,
    /// 当前运行 id（RUN_ 前缀）
    pub run_id: String,
    /// 审批闸门：None = 不拦（service 关闭审批时用）
    pub gate: Option<std::sync::Arc<dyn crate::blocks::Gate>>,
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
    first: UserInput,
) -> Result<RunOutcome> {
    session
        .append(Entry::Message {
            message: first.into_msg(),
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
    // 空转防护：模型连着只调工具不说话、任务又没推进时会无限转下去。
    let mut turn_no: i64 = 0;
    // 工具侧的会话归属：功能工具要靠它把事件发回本会话而不是全局通道
    let tool_ctx = wb_tools::ToolCtx::new(cfg.session_id.clone(), cfg.run_id.clone());
    // 本次运行里被加载的额外工具：模型搜到后加进来，下一轮才有它们的定义。
    // 共享一份可变集合：工具在执行中请求加载，下一轮组请求时读得到。
    let loaded_tools: std::sync::Arc<std::sync::Mutex<Vec<String>>> =
        std::sync::Arc::new(std::sync::Mutex::new(cfg.deferred_tools.clone()));
    let tool_ctx = tool_ctx.with_loader(std::sync::Arc::clone(&loaded_tools));

    'outer: loop {
        // turn 边界收插话（one-at-a-time）
        while let Ok(text) = steer_rx.try_recv() {
            session
                .append(Entry::Message {
                    message: Msg::user(text),
                })
                .map_err(loop_err)?;
        }
        emit(&KernelEvent::TurnStart);
        turn_no += 1;
        if turn_no > MAX_TURNS {
            failed = Some(AppError::new(
                wb_domain::error::kernel::LOOP_LIMIT,
                format!("这一轮已连续请求模型 {MAX_TURNS} 次仍未收尾，已停下"),
            ));
            break 'outer;
        }

        // 预采样压缩
        let msgs = session.replay_messages();
        // 系统提示词与工具定义每轮都重发，它们占着窗口却不在回放消息里；
        // 上下文占用优先用上游报的真实用量，没拿到才退回估算
        let tokens = crate::tokenest::context_tokens(&msgs, cfg.overhead.total());
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
            tool_name: None,
        }];
        for m in session.replay_messages() {
            req_msgs.push(crate::reqbuild::to_req(m, cfg.skill_loader.as_ref()));
        }
        // 本轮要发哪些工具：已启用的 + 本次运行里被加载的
        let mut active_tools: Vec<String> = cfg.enabled_tools.clone();
        for name in loaded_tools.lock().unwrap().iter() {
            if !active_tools.contains(name) {
                active_tools.push(name.clone());
            }
        }
        let tools: Vec<ToolDef> = active_tools
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
        // 错误本体：消息 + 上游是否标注可重试（不靠重新解析文案猜）
        let mut stream_err: Option<(bool, String)> = None;
        {
            let mut stream = match cfg.client.stream(req, cancel.clone()).await {
                Ok(s) => s,
                Err(e) => {
                    stream_err = Some((true, e.message));
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
                        stream_err = Some((retryable, m));
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
                let (tagged, msg) = stream_err.take().unwrap_or((false, String::new()));
                // 上游没标注时按文本兜底判：网关五花八门，宁可多试一次
                let retryable = tagged || retryable_text(&msg);
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
                        // 上次尝试的残留块不能带进下一次：否则重试出来的正文会接在
                        // 已经吐了一半的旧文本后面，用户看到的是两段拼一起的内容。
                        continue 'outer;
                    }
                } else {
                    failed = Some(AppError::new(wb_domain::error::kernel::STREAM_BROKEN, msg));
                    break 'outer;
                }
            }
        }
        // 这一轮拿到有效响应：重试计数归零，后续再出错重新从 1 数
        retry = 0;

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
        // 有调用就执行：部分网关把 tool_calls 帧的 finish_reason 报成 stop，
        // 只看 stop_reason 会把模型明确要求的工具调用静默丢掉。
        if !calls.is_empty() || stop2 == StopReason::ToolUse {
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
                    let run_ctx = tool_ctx.clone();
                    // 过程回显：闭包只负责「把这段输出发出去」，节流在 progress_throttle 里
                    let progress = progress_sink(&emit, &call_id);
                    tasks.push(async move {
                        let out = execute_tool(
                            &run_cfg,
                            &name,
                            &arguments,
                            &run_cancel,
                            &run_ctx,
                            Some(progress),
                            run_cfg.gate.clone(),
                        )
                        .await;
                        (call_id, name, out)
                    });
                }
                let outcomes = futures::future::join_all(tasks).await;
                crate::toolbatch::settle(&mut session, outcomes, &emit)?;
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
        // 模型刚说完话、用户恰好在同一瞬间插了一句：文本已进不了这一轮，
        // 但绝不能就这么丢掉——另起一轮接上，用户看到的是一条回应而不是永远的行。
        let mut pending_steer = Vec::new();
        while let Ok(text) = steer_rx.try_recv() {
            pending_steer.push(text);
        }
        if pending_steer.is_empty() || abort_flag(&cancel) {
            break 'outer;
        }
        for text in pending_steer {
            session
                .append(Entry::Message {
                    message: Msg::user(text),
                })
                .map_err(loop_err)?;
        }
        continue 'outer;
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

pub(crate) fn stop_name(s: StopReason) -> &'static str {
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

/// 过程回显的最小间隔：命令刷得快时（如 `yes`、构建日志）逐块发事件会把 SSE 和界面
/// 一起堵死。它们只服务于「看得见在跑」，不需要逐字节保真。
const PROGRESS_MIN_INTERVAL: std::time::Duration = std::time::Duration::from_millis(120);

/// 造一个节流的进度回调：攒够间隔才真发一次，最后一小块随 ToolExecutionEnd 一起到。
/// 节流器是「丢帧不丢内容」——被跳过的块直接舍弃，最终结果仍以完整输出回灌模型。
fn progress_sink(emit: &EmitFn, call_id: &str) -> wb_tools::ProgressFn {
    let emit = emit.clone();
    let call_id = call_id.to_string();
    crate::throttle::new_progress(PROGRESS_MIN_INTERVAL, move |chunk: &str| {
        emit(&KernelEvent::ToolProgress {
            call_id: call_id.clone(),
            chunk: chunk.to_string(),
        });
    })
}

/// 取消令牌是否已触发（插话续跑前要看一眼：用户按了停止就不该再开一轮）。
fn abort_flag(cancel: &CancellationToken) -> bool {
    cancel.is_cancelled()
}

/// ToolCall 供外部引用（service 构造运行态调用时用）。
pub use wb_tools::ToolCall as RuntimeToolCall;
