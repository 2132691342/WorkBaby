//! 对话编排：run 生命周期（忙闲 / 取消 / 转向）与 KernelEvent→SSE 翻译。

use std::collections::HashMap;
use std::sync::Arc;

use tokio_util::sync::CancellationToken;

use wb_domain::chat::{ChatSendREQ, ChatSendRESP};
use wb_domain::error::session as code;
use wb_kernel::{Entry, KernelEvent};
use wb_pkg::{AppError, Result};
use wb_provider::StopReason;

use super::chat_events::translate;
use super::store;
use super::{steer_channel, Services};

/// 目标连续续跑上限：超过就停下来并告知用户，不让模型空转。
const MAX_GOAL_RESUMES: i64 = 20;

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
    // 用户自己发消息：续跑计数归零（人介入了，计数从头算）。
    if let Some(id) = &req.chat_session_id {
        if !id.is_empty() {
            sv.goal_resumes.lock().unwrap().remove(id);
        }
    }
    start_run(sv, req).await
}

/// 起一轮运行（发送与目标续跑共用）。
async fn start_run(sv: Arc<Services>, req: ChatSendREQ) -> Result<ChatSendRESP> {
    let (pid, mid, context_window, resolved) = resolve_default(&sv)?;
    if resolved.api_key.is_empty() {
        return Err(AppError::new(code::NOT_READY, "该服务商还没有配置 API key"));
    }
    let client = wb_provider::create_client(resolved)?;

    // 会话：已有则加载；无则建（工作区同时是工具执行的工作目录）
    let workspace = sv.workspace.lock().unwrap().clone();
    let (session_id, session_file) = match &req.chat_session_id {
        Some(id) if !id.is_empty() => {
            let path = store::find_session_file(&sv, id)?;
            (id.clone(), wb_kernel::SessionFile::load(&path)?)
        }
        _ => {
            let sid = wb_pkg::ulid::new_id(wb_pkg::Prefix::Sess);
            let mut sf = wb_kernel::SessionFile::create(&sv.sessions_dir(), &workspace, &sid);
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
    let cfg = Arc::new(wb_kernel::AgentConfig {
        client,
        registry,
        enabled_tools,
        system_prompt: prompt,
        provider_id: pid.clone(),
        model_id: mid.clone(),
        context_window,
        max_tokens: 16_384,
        // 压缩会把旧消息整段丢掉，此刻由 service 现场读一遍活状态回填进去
        reinforce: Some(Arc::new(Box::new({
            let svr = sv.clone();
            move || super::features::live_state(&svr)
        }))),
        thinking: wb_provider::ThinkingLevel::Off,
        max_retries: 3,
        workdir: std::path::PathBuf::from(&workspace),
    });
    let run_session_id = session_id.clone();
    let svc = sv.clone();
    let run_id = wb_pkg::ulid::new_id(wb_pkg::Prefix::Run);
    // 运行上下文：task 工具据此知道父会话是谁、用哪个模型。
    *sv.scope.lock().unwrap() = Some(super::RunScope {
        session_id: session_id.clone(),
        cfg: cfg.clone(),
    });
    tokio::spawn(async move {
        let emit_fn: wb_kernel::EmitFn = {
            let sid = run_session_id.clone();
            let emitter = emitter.clone();
            Arc::new(move |ev: &KernelEvent| {
                if let Some(e) = &emitter {
                    translate(e.as_ref(), &sid, ev, context_window);
                }
            })
        };
        let outcome = wb_kernel::run(
            session_file,
            (*cfg).clone(),
            emit_fn,
            cancel.clone(),
            steer_rx,
            req.chat_text.clone(),
        )
        .await;
        svc.chat.lock().unwrap().finish(&run_session_id);
        // 运行结束：清掉运行上下文，避免 task 工具拿到已结束的父会话参数。
        *svc.scope.lock().unwrap() = None;
        match outcome {
            Ok(o) => {
                refresh_index(&svc, &run_session_id);
                // 用量入日表：按 天/provider/model/session 聚合（无定价配置，成本恒 0）。
                let u = &o.usage_total;
                let _ = svc.repo.usage_add(
                    &wb_repo::usage::today_key(),
                    &cfg.provider_id,
                    &cfg.model_id,
                    &run_session_id,
                    u.input,
                    u.output,
                    u.cache_read,
                    u.cache_write,
                    0.0,
                );
                // 目标续跑：活跃目标还有未完成步骤就自动开下一轮。
                if !o.aborted {
                    maybe_resume_goal(&svc, &run_session_id);
                }
            }
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

/// 目标续跑：一轮结束后若活跃目标还有未完成步骤，自动开下一轮。
/// 走内部 run 路径，不占用界面输入框；被中止的轮次不续跑。
/// 连续续跑有上限——模型没推进目标却一直挂着目标时，不能无限转下去。
fn maybe_resume_goal(sv: &Arc<Services>, session_id: &str) {
    let Some(prompt) = super::features::goal_continuation(sv) else {
        // 目标没了（达成 / 放弃 / 功能关掉）：清计数。
        sv.goal_resumes.lock().unwrap().remove(session_id);
        return;
    };
    // 同一会话已在跑（用户又发了消息）就不插手。
    if sv.chat.lock().unwrap().is_running(session_id) {
        return;
    }
    let mut count = sv.goal_resumes.lock().unwrap();
    let n = count.entry(session_id.to_string()).or_insert(0);
    if *n >= MAX_GOAL_RESUMES {
        count.remove(session_id);
        tracing::info!(
            session_id,
            "目标连续续跑达到上限，停止自动续跑（目标仍未完结）"
        );
        if let Some(em) = sv.emitter.lock().unwrap().clone() {
            em.emit(
                session_id,
                "chat:notify",
                serde_json::json!({
                    "level": "warning",
                    "text": format!("目标已连续续跑 {MAX_GOAL_RESUMES} 轮仍未完结，已停止自动续跑。"),
                }),
            );
        }
        return;
    }
    *n += 1;
    drop(count);

    let sv2 = sv.clone();
    let sid = session_id.to_string();
    tokio::spawn(async move {
        let req = ChatSendREQ {
            chat_session_id: Some(sid),
            chat_text: prompt,
            chat_attachments: Vec::new(),
        };
        // 走 start_run 而非 send：续跑不重置计数，否则上限形同虚设。
        if let Err(e) = start_run(sv2, req).await {
            tracing::warn!(code = %e.code, "目标续跑启动失败");
        }
    });
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
    let reinforce: wb_kernel::compaction::ReinforceFn = {
        let svr = sv.clone();
        Box::new(move || super::features::live_state(&svr))
    };
    let (tokens_before, summary) = wb_kernel::compaction::compact(
        &mut session_file,
        client.as_ref(),
        &format!("{pid}/{mid}"),
        4096,
        Some(&reinforce),
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
