//! KernelEvent → SSE 事件翻译（docs/API.md 事件表）。

use wb_kernel::KernelEvent;

use super::Emitter;

/// KernelEvent → SSE 事件。
///
/// 载荷一律走 wb_domain::events 里的结构体——那是前后端契约的唯一出处，
/// 手写 json! 字面量早晚会与前端对不上。
pub fn translate(em: &dyn Emitter, sid: &str, ev: &KernelEvent) {
    match ev {
        KernelEvent::AgentStart { run_id } => {
            emit_data(
                em,
                sid,
                "chat:start",
                &wb_domain::events::StartData {
                    run_id: run_id.clone(),
                    session_id: sid.to_string(),
                },
            );
        }
        KernelEvent::TextDelta { delta } => {
            emit_data(
                em,
                sid,
                "chat:delta",
                &wb_domain::events::DeltaData {
                    kind: "text".into(),
                    delta: delta.clone(),
                },
            );
        }
        KernelEvent::ThinkingDelta { delta } => {
            emit_data(
                em,
                sid,
                "chat:delta",
                &wb_domain::events::DeltaData {
                    kind: "thinking".into(),
                    delta: delta.clone(),
                },
            );
        }
        KernelEvent::ToolExecutionStart {
            call_id,
            name,
            args,
        } => {
            emit_data(
                em,
                sid,
                "chat:tool_start",
                &wb_domain::events::ToolStartData {
                    tool_call_id: call_id.clone(),
                    name: name.clone(),
                    label: None,
                    args: Some(args.clone()),
                },
            );
        }
        KernelEvent::ToolExecutionEnd {
            call_id,
            name,
            result,
            is_error,
            duration_ms,
        } => {
            emit_data(
                em,
                sid,
                "chat:tool_end",
                &wb_domain::events::ToolEndData {
                    tool_call_id: call_id.clone(),
                    name: name.clone(),
                    result: result.clone(),
                    is_error: *is_error,
                    duration_ms: *duration_ms,
                },
            );
        }
        KernelEvent::TurnEnd { usage, stop_reason } => {
            if let Some(u) = usage {
                let total = u.input + u.output + u.cache_read + u.cache_write;
                emit_data(
                    em,
                    sid,
                    "chat:context",
                    &wb_domain::events::ContextData {
                        usage: wb_domain::events::ContextUsage {
                            input: u.input,
                            output: u.output,
                            cached: u.cache_read,
                            cache_write: u.cache_write,
                            total,
                            context: total,
                            cost: 0.0,
                        },
                        context_window: 128000,
                        ratio: total as f64 / 128000.0,
                    },
                );
            }
            let _ = stop_reason;
        }
        KernelEvent::CompactionEnd {
            tokens_before: Some(t),
        } => {
            emit_data(
                em,
                sid,
                "chat:compressed",
                &wb_domain::events::CompressedData {
                    tokens_before: *t,
                },
            );
        }
        KernelEvent::CompactionEnd {
            tokens_before: None,
        } => {}
        KernelEvent::AutoRetry {
            attempt,
            max,
            delay_ms,
        } => {
            emit_data(
                em,
                sid,
                "chat:retry",
                &wb_domain::events::RetryData {
                    attempt: *attempt,
                    max: *max,
                    delay_ms: *delay_ms,
                },
            );
        }
        KernelEvent::Settled { aborted } => {
            if *aborted {
                emit_data(
                    em,
                    sid,
                    "chat:stopped",
                    &wb_domain::events::StoppedData {
                        run_id: String::new(),
                    },
                );
            } else {
                emit_data(
                    em,
                    sid,
                    "chat:done",
                    &wb_domain::events::DoneData {
                        run_id: String::new(),
                        message: None,
                    },
                );
            }
        }
        KernelEvent::Failed { code, message } => {
            emit_data(
                em,
                sid,
                "chat:error",
                &wb_domain::events::ErrorData {
                    run_id: String::new(),
                    code: code.clone(),
                    message: message.clone(),
                },
            );
        }
        _ => {}
    }
}

/// 发一条带结构化载荷的 SSE 事件（序列化失败降级为空对象，不中断事件流）。
fn emit_data<T: serde::Serialize>(em: &dyn Emitter, sid: &str, event: &str, data: &T) {
    let payload = serde_json::to_value(data).unwrap_or(serde_json::Value::Null);
    em.emit(sid, event, payload);
}
