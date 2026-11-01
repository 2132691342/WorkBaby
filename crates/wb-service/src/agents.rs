//! 子代理观测：活跃表 + 事件翻译成 `agent:*` SSE。
//! 子代理的中间过程不进父上下文，但用户在界面上要看得见它们在干什么。

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

use wb_domain::subagent::{SubagentRunVO, SubagentTranscriptVO};
use wb_kernel::SubagentEvent;

/// 单个子代理的活跃形态：界面上「正在跑」的那一行。
#[derive(Debug, Clone)]
pub struct AgentLive {
    /// 子会话 id
    pub session_id: String,
    /// 父会话 id
    pub parent_session_id: String,
    /// 角色码
    pub role_code: String,
    /// 任务描述
    pub task: String,
    /// 起建毫秒
    pub started_at: i64,
}

/// 活跃子代理表：按子会话 id 索引，跑完即摘。
#[derive(Default)]
pub struct AgentBoard {
    /// 子会话 id → 活跃记录
    live: Mutex<BTreeMap<String, AgentLive>>,
    /// 子会话 id → 已拼好的过程正文（结束后供回看）
    transcript: Mutex<BTreeMap<String, String>>,
}

/// 当前毫秒时间戳。
fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

impl AgentBoard {
    /// 空表。
    pub fn new() -> Arc<Self> {
        Arc::new(Self::default())
    }

    /// 活跃清单（按起建时间升序，界面按这个顺序排）。
    pub fn list(&self) -> Vec<SubagentRunVO> {
        self.live
            .lock()
            .unwrap()
            .values()
            .map(|a| SubagentRunVO {
                run_session_id: a.session_id.clone(),
                run_parent_session_id: a.parent_session_id.clone(),
                run_role_code: a.role_code.clone(),
                run_task: a.task.clone(),
                run_conclusion: String::new(),
                run_failed: false,
                run_started_at: a.started_at,
                run_finished_at: 0,
            })
            .collect()
    }

    /// 某个父会话下的活跃子代理数量。
    pub fn count_of(&self, parent_session_id: &str) -> usize {
        self.live
            .lock()
            .unwrap()
            .values()
            .filter(|a| a.parent_session_id == parent_session_id)
            .count()
    }

    /// 取一段已经跑完（或正在跑）的过程正文。
    pub fn transcript(&self, session_id: &str) -> SubagentTranscriptVO {
        SubagentTranscriptVO {
            resp_session_id: session_id.to_string(),
            resp_transcript: self
                .transcript
                .lock()
                .unwrap()
                .get(session_id)
                .cloned()
                .unwrap_or_default(),
        }
    }

    /// 摘掉一个活跃记录（进程退出或异常路径兜底）。
    pub fn forget(&self, session_id: &str) {
        self.live.lock().unwrap().remove(session_id);
    }

    /// 把内核事件翻成 SSE 载荷并更新表；`emit(session_id, event, data)` 是出口。
    pub fn on_event(
        &self,
        parent_session_id: &str,
        ev: SubagentEvent,
        emit: &dyn Fn(&str, &str, serde_json::Value),
    ) {
        match ev {
            SubagentEvent::Started {
                session_id,
                role_code,
                task,
            } => {
                let started_at = now_ms();
                self.live.lock().unwrap().insert(
                    session_id.clone(),
                    AgentLive {
                        session_id: session_id.clone(),
                        parent_session_id: parent_session_id.to_string(),
                        role_code: role_code.clone(),
                        task: task.clone(),
                        started_at,
                    },
                );
                self.transcript
                    .lock()
                    .unwrap()
                    .insert(session_id.clone(), String::new());
                emit(
                    parent_session_id,
                    "agent:start",
                    serde_json::json!({
                        "agent_session_id": session_id,
                        "agent_parent_session_id": parent_session_id,
                        "agent_role_code": role_code,
                        "agent_task": task,
                        "agent_started_at": started_at,
                    }),
                );
            }
            SubagentEvent::TextDelta { session_id, delta } => {
                if let Some(buf) = self.transcript.lock().unwrap().get_mut(&session_id) {
                    buf.push_str(&delta);
                }
                emit(
                    parent_session_id,
                    "agent:delta",
                    serde_json::json!({
                        "agent_session_id": session_id,
                        "agent_kind": "text",
                        "agent_delta": delta,
                    }),
                );
            }
            SubagentEvent::ThinkingDelta { session_id, delta } => {
                emit(
                    parent_session_id,
                    "agent:delta",
                    serde_json::json!({
                        "agent_session_id": session_id,
                        "agent_kind": "thinking",
                        "agent_delta": delta,
                    }),
                );
            }
            SubagentEvent::ToolStart {
                session_id,
                call_id,
                name,
                args,
            } => {
                emit(
                    parent_session_id,
                    "agent:tool_start",
                    serde_json::json!({
                        "agent_session_id": session_id,
                        "agent_tool_call_id": call_id,
                        "agent_tool_name": name,
                        "agent_tool_args": args,
                    }),
                );
            }
            SubagentEvent::ToolEnd {
                session_id,
                call_id,
                name,
                result,
                is_error,
                duration_ms,
            } => {
                emit(
                    parent_session_id,
                    "agent:tool_end",
                    serde_json::json!({
                        "agent_session_id": session_id,
                        "agent_tool_call_id": call_id,
                        "agent_tool_name": name,
                        "agent_tool_result": result,
                        "agent_tool_error": is_error,
                        "agent_duration_ms": duration_ms,
                    }),
                );
            }
            SubagentEvent::Finished {
                session_id,
                failed,
                conclusion,
                usage,
                duration_ms,
            } => {
                self.live.lock().unwrap().remove(&session_id);
                emit(
                    parent_session_id,
                    "agent:done",
                    serde_json::json!({
                        "agent_session_id": session_id,
                        "agent_failed": failed,
                        "agent_conclusion": conclusion,
                        "agent_duration_ms": duration_ms,
                        "agent_usage": {
                            "input": usage.input,
                            "output": usage.output,
                            "cached": usage.cache_read,
                            "cache_write": usage.cache_write,
                        },
                    }),
                );
            }
        }
    }
}
