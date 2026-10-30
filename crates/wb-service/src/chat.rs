//! 对话编排：run 生命周期（忙闲 / 取消 / 转向）与 KernelEvent→SSE 翻译。

use std::collections::HashMap;
use std::sync::Arc;

use tokio_util::sync::CancellationToken;

use wb_domain::chat::{ChatSendREQ, ChatSendRESP};
use wb_domain::error::session as code;
use wb_kernel::{Entry, KernelEvent};
use wb_pkg::{AppError, Result};
use wb_provider::StopReason;

use super::store;
use super::{steer_channel, Emitter, Services};

/// 一次运行句柄。
pub struct RunHandle {
    /// 取消令牌（stop / 窗口关闭时触发）
    pub cancel: CancellationToken,
    /// 运行中插话入口
    pub steer: tokio::sync::mpsc::Sender<String>,
}

/// 全部会话的运行态板。
#[derive(Default)]
pub struct ChatBoard {
    runs: HashMap<String, RunHandle>,
}

impl ChatBoard {
    /// 空板。
    pub fn new() -> Self {
        Self::default()
    }

    /// 是否在跑。
    pub fn is_running(&self, session_id: &str) -> bool {
        self.runs.contains_key(session_id)
    }

    /// 运行中的会话集合。
    pub fn running_ids(&self) -> Vec<String> {
        self.runs.keys().cloned().collect()
    }

    /// 登记运行（占用报 5001），返回取消令牌与转向接收端；发送端留在板上给 steer 用。
    pub fn begin(
        &mut self,
        session_id: &str,
    ) -> Result<(CancellationToken, tokio::sync::mpsc::Receiver<String>)> {
        if self.runs.contains_key(session_id) {
            return Err(AppError::new(
                code::BUSY,
                "这条对话正在运行，先等它结束或停止它",
            ));
        }
        let (tx, rx) = steer_channel();
        let cancel = CancellationToken::new();
        self.runs.insert(
            session_id.to_string(),
            RunHandle {
                cancel: cancel.clone(),
                steer: tx,
            },
        );
        Ok((cancel, rx))
    }

    /// 收尾清理。
    pub fn finish(&mut self, session_id: &str) {
        self.runs.remove(session_id);
    }

    /// 停止（先撤转向入口再触发取消——排队的话不再送达）。
    pub fn stop(&mut self, session_id: &str) -> bool {
        if let Some(h) = self.runs.remove(session_id) {
            h.cancel.cancel();
            true
        } else {
            false
        }
    }

    /// 插话（满队列报 5004）。
    pub fn steer(&self, session_id: &str, text: String) -> Result<()> {
        match self.runs.get(session_id) {
            Some(h) => h
                .steer
                .try_send(text)
                .map_err(|_| AppError::new(code::QUEUE_FULL, "插话队列已满")),
            None => Err(AppError::new(code::NOT_FOUND, "这条对话没有在运行")),
        }
    }
}

/// 解析默认模型：设置键 → 第一个有 key 的 provider。
pub(crate) fn resolve_default(
    sv: &Arc<Services>,
) -> Result<(String, String, i64, wb_provider::config::ResolvedProvider)> {
    let dp = sv.repo.settings_get("default_provider")?;
    let dm = sv.repo.settings_get("default_model")?;
    let models = sv.providers.load_models()?;
    let (pid, mid) = match (dp, dm) {
        (Some(p), Some(m)) if models.providers.contains_key(&p) => (p, m),
        _ => {
            let (p, cfg) = models
                .providers
                .iter()
                .find(|(_, c)| !c.api_key.is_empty() || !c.models.is_empty())
                .ok_or_else(|| {
                    AppError::new(
                        code::NOT_READY,
                        "还没有可用的模型配置，先去设置页添加服务商",
                    )
                })?;
            let m = cfg.models.first().map(|m| m.id.clone()).unwrap_or_default();
            (p.clone(), m)
        }
    };
    let resolved = sv.providers.resolve(&pid, Some(&mid))?;
    let thinking = resolved.context_window;
    Ok((pid, mid, thinking, resolved))
}

/// 发送（新会话现场创建）。
pub async fn send(sv: Arc<Services>, req: ChatSendREQ) -> Result<ChatSendRESP> {
    let (pid, mid, context_window, resolved) = resolve_default(&sv)?;
    if resolved.api_key.is_empty() {
        return Err(AppError::new(code::NOT_READY, "该服务商还没有配置 API key"));
    }
    let client = wb_provider::create_client(resolved)?;

    // 会话：已有则加载；无则建
    let (session_id, session_file) = match &req.chat_session_id {
        Some(id) if !id.is_empty() => {
            let path = store::find_session_file(&sv, id)?;
            (id.clone(), wb_kernel::SessionFile::load(&path)?)
        }
        _ => {
            let sid = wb_pkg::ulid::new_id(wb_pkg::Prefix::Sess);
            let mut sf = wb_kernel::SessionFile::create(
                &sv.sessions_dir(),
                &sv.workspace.lock().unwrap().clone(),
                &sid,
            );
            sf.append(Entry::ModelChange {
                provider_id: pid.clone(),
                model_id: mid.clone(),
            })
            .ok();
            (sid, sf)
        }
    };

    let (cancel, steer_rx) = sv.chat.lock().unwrap().begin(&session_id)?;
    let emitter = sv.emitter.lock().unwrap().clone();
    let (enabled_tools, prompt) = super::inputs(&sv);
    let registry: Arc<wb_tools::ToolRegistry> = {
        let reg = sv.registry.read().unwrap();
        Arc::new(wb_tools::ToolRegistry::snapshot(&reg))
    };
    let cfg = wb_kernel::AgentConfig {
        client,
        registry,
        enabled_tools,
        system_prompt: prompt,
        provider_id: pid.clone(),
        model_id: mid.clone(),
        context_window,
        max_tokens: 16_384,
        thinking: wb_provider::ThinkingLevel::Off,
        max_retries: 3,
    };
    let run_session_id = session_id.clone();
    let svc = sv.clone();
    let run_id = wb_pkg::ulid::new_id(wb_pkg::Prefix::Run);
    tokio::spawn(async move {
        let emit_fn: wb_kernel::EmitFn = {
            let sid = run_session_id.clone();
            let emitter = emitter.clone();
            Arc::new(move |ev: &KernelEvent| {
                if let Some(e) = &emitter {
                    translate(e.as_ref(), &sid, ev);
                }
            })
        };
        let outcome = wb_kernel::run(
            session_file,
            cfg,
            emit_fn,
            cancel.clone(),
            steer_rx,
            req.chat_text.clone(),
        )
        .await;
        svc.chat.lock().unwrap().finish(&run_session_id);
        match outcome {
            Ok(_) => refresh_index(&svc, &run_session_id),
            Err(e) => {
                if let Some(em) = &emitter {
                    em.emit(
                        &run_session_id,
                        "chat:error",
                        serde_json::json!({ "run_id": "", "code": e.code, "message": e.message }),
                    );
                }
            }
        }
    });
    Ok(ChatSendRESP {
        resp_run_id: run_id,
        resp_session_id: session_id,
        resp_queued: false,
    })
}

/// 停止。
pub fn stop(sv: &Arc<Services>, session_id: &str) -> bool {
    sv.chat.lock().unwrap().stop(session_id)
}

/// 插话。
pub fn steer(sv: &Arc<Services>, session_id: &str, text: String) -> Result<()> {
    sv.chat.lock().unwrap().steer(session_id, text)
}

/// 空闲追问：完整一轮，复用 send 的既有会话路径。
pub async fn followup(sv: Arc<Services>, session_id: &str, text: String) -> Result<ChatSendRESP> {
    send(
        sv,
        ChatSendREQ {
            chat_session_id: Some(session_id.to_string()),
            chat_text: text,
            chat_attachments: Vec::new(),
        },
    )
    .await
}

/// 取消自动重试等待：与停止同义（取消令牌会中断重试间隔）。
pub fn retry_cancel(sv: &Arc<Services>, session_id: &str) -> bool {
    stop(sv, session_id)
}

/// 手动压缩：空闲会话才允许；完成后发 chat:compressed。
pub async fn compact(
    sv: Arc<Services>,
    session_id: &str,
    _instructions: Option<String>,
) -> Result<(i64, String)> {
    if sv.chat.lock().unwrap().is_running(session_id) {
        return Err(AppError::new(code::BUSY, "对话正在运行，停止后再压缩"));
    }
    let (pid, mid, _window, resolved) = resolve_default(&sv)?;
    let client = wb_provider::create_client(resolved)?;
    let path = store::find_session_file(&sv, session_id)?;
    let mut session_file = wb_kernel::SessionFile::load(&path)?;
    let (tokens_before, summary) = wb_kernel::compaction::compact(
        &mut session_file,
        client.as_ref(),
        &format!("{pid}/{mid}"),
        4096,
        tokio_util::sync::CancellationToken::new(),
    )
    .await?;
    if let Some(em) = sv.emitter.lock().unwrap().clone() {
        em.emit(
            session_id,
            "chat:compressed",
            serde_json::json!({ "tokens_before": tokens_before }),
        );
    }
    Ok((tokens_before, summary))
}

/// KernelEvent → SSE 事件（docs/API.md 事件表）。
fn translate(em: &dyn Emitter, sid: &str, ev: &KernelEvent) {
    match ev {
        KernelEvent::AgentStart { run_id } => {
            em.emit(
                sid,
                "chat:start",
                serde_json::json!({ "run_id": run_id, "session_id": sid }),
            );
        }
        KernelEvent::TextDelta { delta } => {
            em.emit(
                sid,
                "chat:delta",
                serde_json::json!({ "kind": "text", "delta": delta }),
            );
        }
        KernelEvent::ThinkingDelta { delta } => {
            em.emit(
                sid,
                "chat:delta",
                serde_json::json!({ "kind": "thinking", "delta": delta }),
            );
        }
        KernelEvent::ToolExecutionStart {
            call_id,
            name,
            args,
        } => {
            em.emit(
                sid,
                "chat:tool_start",
                serde_json::json!({ "tool_call_id": call_id, "name": name, "label": null, "args": args }),
            );
        }
        KernelEvent::ToolExecutionEnd {
            call_id,
            name,
            result,
            is_error,
            duration_ms,
        } => {
            em.emit(
                sid,
                "chat:tool_end",
                serde_json::json!({ "tool_call_id": call_id, "name": name, "result": result, "is_error": is_error, "duration_ms": duration_ms }),
            );
        }
        KernelEvent::TurnEnd { usage, stop_reason } => {
            if let Some(u) = usage {
                let total = u.input + u.output + u.cache_read + u.cache_write;
                em.emit(
                    sid,
                    "chat:context",
                    serde_json::json!({
                        "usage": { "input": u.input, "output": u.output, "cached": u.cache_read, "cache_write": u.cache_write, "total": total, "context": total, "cost": 0.0 },
                        "context_window": 128000,
                        "ratio": total as f64 / 128000.0,
                    }),
                );
            }
            let _ = stop_reason;
        }
        KernelEvent::CompactionEnd {
            tokens_before: Some(t),
        } => {
            em.emit(
                sid,
                "chat:compressed",
                serde_json::json!({ "tokens_before": t }),
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
            em.emit(
                sid,
                "chat:retry",
                serde_json::json!({ "attempt": attempt, "max": max, "delay_ms": delay_ms }),
            );
        }
        KernelEvent::Settled { aborted } => {
            if *aborted {
                em.emit(sid, "chat:stopped", serde_json::json!({ "run_id": "" }));
            } else {
                em.emit(
                    sid,
                    "chat:done",
                    serde_json::json!({ "run_id": "", "message": null }),
                );
            }
        }
        KernelEvent::Failed { code, message } => {
            em.emit(
                sid,
                "chat:error",
                serde_json::json!({ "run_id": "", "code": code, "message": message }),
            );
        }
        _ => {}
    }
}

/// 收尾刷新索引（消息数 / 活动时间 / 预览）。
fn refresh_index(sv: &Arc<Services>, session_id: &str) {
    let Ok(path) = store::find_session_file(sv, session_id) else {
        return;
    };
    let Ok(sf) = wb_kernel::SessionFile::load(&path) else {
        return;
    };
    let mut count = 0i64;
    let mut summary = String::new();
    for m in sf.replay_messages() {
        count += 1;
        let text = match &m {
            wb_kernel::Msg::User { content, .. } => Some(content.clone()),
            wb_kernel::Msg::Assistant { content, .. } => Some(
                content
                    .iter()
                    .filter_map(|b| match b {
                        wb_kernel::Block::Text { text } => Some(text.clone()),
                        _ => None,
                    })
                    .collect::<String>(),
            ),
            wb_kernel::Msg::ToolResult { .. } => None,
        };
        if let Some(t) = text.filter(|t| !t.trim().is_empty()) {
            summary = t.chars().take(80).collect();
        }
    }
    let _ = sv
        .repo
        .sessions_upsert(&wb_domain::session::SessionIndexDO {
            session_id: session_id.to_string(),
            session_name: String::new(),
            session_workspace: sf.header.workspace.clone(),
            session_provider: String::new(),
            session_model: String::new(),
            session_message_count: count,
            session_last_active_at: wb_pkg::fsutil::mtime_ms(&path)
                .unwrap_or_else(wb_kernel::now_ms),
            session_summary: summary,
        });
    let _ = StopReason::Stop; // 保持导入（映射表后续补 turn_end 终态语义）
}

/// 就绪检查：默认模型可解析即就绪（就绪判定在发送之前）。
pub fn default_model_state(sv: &Arc<Services>) -> Result<(String, String)> {
    let (pid, mid, _, _) = resolve_default(sv)?;
    Ok((pid, mid))
}
