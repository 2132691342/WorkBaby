//! 结构化提问：模型在该猜时改为向用户提问（chat:ask 桥 + oneshot 回答回灌）。

use std::collections::HashMap;
use std::path::Path;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio::sync::oneshot;
use tokio_util::sync::CancellationToken;
use wb_domain::error::{session as code, tool as toolcode};
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

use crate::FeatureCtx;

/// 单题等待上限：无人回答时报 5005，模型自行决策。
const ANSWER_TIMEOUT: Duration = Duration::from_secs(600);

/// 回答板：ask_id → 等待中的回答通道。service 持有，chat/answer 路由回填。
#[derive(Default)]
pub struct AnswerBoard {
    pending: Mutex<HashMap<String, oneshot::Sender<Value>>>,
}

impl AnswerBoard {
    /// 空板。
    pub fn new() -> Arc<Self> {
        Arc::new(Self::default())
    }

    /// 登记等待者，拿接收端。
    pub fn register(&self, ask_id: &str) -> oneshot::Receiver<Value> {
        let (tx, rx) = oneshot::channel();
        self.pending.lock().unwrap().insert(ask_id.to_string(), tx);
        rx
    }

    /// 回填一条回答；没有等待者返回 false。
    pub fn answer(&self, ask_id: &str, value: Value) -> bool {
        match self.pending.lock().unwrap().remove(ask_id) {
            Some(tx) => tx.send(value).is_ok(),
            None => false,
        }
    }
}

/// ask_user_question 工具：≤4 题，每题可带 2-4 个选项；逐题出卡片。
pub struct AskUserTool {
    ctx: FeatureCtx,
    answers: Arc<AnswerBoard>,
}

impl AskUserTool {
    /// 装配。
    pub fn new(ctx: FeatureCtx, answers: Arc<AnswerBoard>) -> Self {
        Self { ctx, answers }
    }
}

#[async_trait]
impl Tool for AskUserTool {
    fn name(&self) -> &str {
        "ask_user_question"
    }

    fn label(&self) -> &str {
        "结构化提问"
    }

    fn description(&self) -> &str {
        "向用户提出结构化问题（最多 4 题，每题 2-4 个选项或自由输入）。当你需要用户在几个方案间做选择、确认破坏性操作或补充缺失信息时使用它，而不是靠猜。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "questions": {
                    "type": "array",
                    "maxItems": 4,
                    "description": "问题清单，每题一次问答",
                    "items": {
                        "type": "object",
                        "properties": {
                            "question": { "type": "string", "description": "问题正文" },
                            "header": { "type": "string", "description": "短标题（可选）" },
                            "options": {
                                "type": "array",
                                "maxItems": 4,
                                "description": "选项；缺省 = 自由输入",
                                "items": {
                                    "type": "object",
                                    "properties": {
                                        "label": { "type": "string" },
                                        "description": { "type": "string", "description": "选项说明（可选）" }
                                    },
                                    "required": ["label"]
                                }
                            }
                        },
                        "required": ["question"]
                    }
                }
            },
            "required": ["questions"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let questions = args
            .get("questions")
            .and_then(|v| v.as_array())
            .cloned()
            .unwrap_or_default();
        if questions.is_empty() || questions.len() > 4 {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                "questions 需要 1-4 题",
            ));
        }
        let mut out = String::new();
        for (i, q) in questions.iter().enumerate() {
            let question = q
                .get("question")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            let header = q
                .get("header")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .to_string();
            let options: Vec<String> = q
                .get("options")
                .and_then(|v| v.as_array())
                .map(|arr| {
                    arr.iter()
                        .map(|o| {
                            let label = o.get("label").and_then(|v| v.as_str()).unwrap_or("");
                            let desc = o.get("description").and_then(|v| v.as_str()).unwrap_or("");
                            if desc.is_empty() {
                                label.to_string()
                            } else {
                                format!("{label} — {desc}")
                            }
                        })
                        .collect()
                })
                .unwrap_or_default();
            if question.is_empty() {
                return Err(AppError::new(
                    toolcode::ARG_INVALID,
                    format!("第 {} 题缺 question", i + 1),
                ));
            }
            let ask_id = wb_pkg::ulid::new_id(wb_pkg::Prefix::Ask);
            let (method, options_payload) = if options.is_empty() {
                ("input", Vec::<Value>::new())
            } else {
                ("select", options.iter().map(|o| json!(o)).collect())
            };
            (self.ctx.emit)(
                "",
                "chat:ask",
                json!({
                    "id": ask_id,
                    "method": method,
                    "title": if header.is_empty() { format!("问题 {}", i + 1) } else { header },
                    "message": question,
                    "options": options_payload,
                    "placeholder": "",
                    "prefill": "",
                    "timeout_ms": ANSWER_TIMEOUT.as_millis() as u64,
                }),
            );
            let rx = self.answers.register(&ask_id);
            let answer = tokio::select! {
                r = rx => match r {
                    Ok(v) => v,
                    Err(_) => {
                        (self.ctx.emit)("", "chat:ask_closed", json!({ "id": ask_id, "reason": "session_gone" }));
                        return Err(AppError::new(code::ASK_EXPIRED, "提问通道已关闭"));
                    }
                },
                _ = tokio::time::sleep(ANSWER_TIMEOUT) => {
                    (self.ctx.emit)("", "chat:ask_closed", json!({ "id": ask_id, "reason": "expired" }));
                    return Err(AppError::new(code::ASK_EXPIRED, "提问等待超时（10 分钟）"));
                }
                _ = cancel.cancelled() => {
                    (self.ctx.emit)("", "chat:ask_closed", json!({ "id": ask_id, "reason": "cancelled" }));
                    return Err(AppError::new(toolcode::EXEC_CANCELLED, "提问已随会话取消"));
                }
            };
            (self.ctx.emit)(
                "",
                "chat:ask_closed",
                json!({ "id": ask_id, "reason": "answered" }),
            );
            let text = if answer
                .get("cancelled")
                .and_then(|v| v.as_bool())
                .unwrap_or(false)
            {
                "（用户跳过了这题）".to_string()
            } else {
                answer
                    .get("value")
                    .and_then(|v| v.as_str())
                    .unwrap_or("")
                    .to_string()
            };
            out.push_str(&format!(
                "{}. {}\n   → {}\n",
                i + 1,
                question,
                if text.is_empty() {
                    "（未回答）"
                } else {
                    &text
                }
            ));
        }
        Ok(ToolOutput {
            text: out,
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}
