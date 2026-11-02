//! 结构化提问：模型在该猜时改为向用户提问（chat:ask 桥 + oneshot 回答回灌）。
//! 待答问题同时落盘一份：进程重启后能重新弹出来，而不是让会话卡在一个没人能答的调用上。

use std::path::Path;
use std::sync::Arc;
use std::time::{Duration, Instant};

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::{session as code, tool as toolcode};
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

pub use crate::answer_board::AnswerBoard;

use crate::FeatureCtx;

/// 单题等待上限：无人回答时报 5005，模型自行决策。
const ANSWER_TIMEOUT: Duration = Duration::from_secs(600);

/// 单题问答：出卡 → 等回答 → 关卡片；返回（答案文本，是否取消）。
impl AskUserTool {
    async fn ask_once(
        &self,
        cancel: CancellationToken,
        question: String,
        header: String,
        options: Vec<String>,
    ) -> Result<(String, bool)> {
        let ask_id = wb_pkg::ulid::new_id(wb_pkg::Prefix::Ask);
        let (method, options_payload) = if options.is_empty() {
            ("input", Vec::<Value>::new())
        } else {
            ("select", options.iter().map(|o| json!(o)).collect())
        };
        let card = json!({
            "id": ask_id,
            "method": method,
            "title": if header.is_empty() { "提问".to_string() } else { header },
            "message": question,
            "options": options_payload,
            "placeholder": "",
            "prefill": "",
            "timeout_ms": ANSWER_TIMEOUT.as_millis() as u64,
        });
        // 先登记再发事件：否则用户手快点下回答时，监听者还没挂上
        let rx = self.answers.register_with_card(&ask_id, card.clone());
        (self.ctx.emit)("", "chat:ask", card);
        let outcome = tokio::select! {
            r = rx => Some(r),
            _ = tokio::time::sleep(ANSWER_TIMEOUT) => None,
            _ = cancel.cancelled() => None,
        };
        // 无论怎么退出都从待答名单里删掉：重启后不该再弹一个已经作废的问题
        self.answers.forget(&ask_id);
        let answer = match outcome {
            Some(Ok(v)) => v,
            Some(Err(_)) => {
                (self.ctx.emit)(
                    "",
                    "chat:ask_closed",
                    json!({ "id": ask_id, "reason": "session_gone" }),
                );
                return Err(AppError::new(code::ASK_EXPIRED, "提问通道已关闭"));
            }
            None if cancel.is_cancelled() => {
                (self.ctx.emit)(
                    "",
                    "chat:ask_closed",
                    json!({ "id": ask_id, "reason": "cancelled" }),
                );
                return Err(AppError::new(toolcode::EXEC_CANCELLED, "提问已随会话取消"));
            }
            None => {
                (self.ctx.emit)(
                    "",
                    "chat:ask_closed",
                    json!({ "id": ask_id, "reason": "expired" }),
                );
                return Err(AppError::new(code::ASK_EXPIRED, "提问等待超时（10 分钟）"));
            }
        };
        (self.ctx.emit)(
            "",
            "chat:ask_closed",
            json!({ "id": ask_id, "reason": "answered" }),
        );
        let cancelled = answer
            .get("cancelled")
            .and_then(|v| v.as_bool())
            .unwrap_or(false);
        let text = answer
            .get("value")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        Ok((text, cancelled))
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
        "向用户提出结构化问题（最多 4 题，每题 2-4 个选项或自由输入）。\
         当你需要用户在几个方案间做选择、确认破坏性操作或补充缺失信息时使用它，而不是靠猜。\
         一次可以给 1-4 题：多题会合并成一张问卷一次问完，不必分多次调用。"
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
        // 先解析全部题面：任何一题非法都直接拒，不要问一半才发现第三题缺 question
        let mut parsed: Vec<(String, String, Vec<String>)> = Vec::new();
        for (i, q) in questions.iter().enumerate() {
            let question = q
                .get("question")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .trim()
                .to_string();
            if question.is_empty() {
                return Err(AppError::new(
                    toolcode::ARG_INVALID,
                    format!("第 {} 题缺 question", i + 1),
                ));
            }
            let header = q
                .get("header")
                .and_then(|v| v.as_str())
                .unwrap_or("")
                .trim()
                .to_string();
            let options: Vec<String> = q
                .get("options")
                .and_then(|v| v.as_array())
                .map(|arr| {
                    arr.iter()
                        .filter_map(|o| {
                            let label =
                                o.get("label").and_then(|v| v.as_str()).unwrap_or("").trim();
                            if label.is_empty() {
                                return None;
                            }
                            let desc = o
                                .get("description")
                                .and_then(|v| v.as_str())
                                .unwrap_or("")
                                .trim();
                            Some(if desc.is_empty() {
                                label.to_string()
                            } else {
                                format!("{label} — {desc}")
                            })
                        })
                        .collect()
                })
                .unwrap_or_default();
            if options.len() > 4 {
                return Err(AppError::new(
                    toolcode::ARG_INVALID,
                    format!("第 {} 题选项超过 4 个", i + 1),
                ));
            }
            parsed.push((question, header, options));
        }

        // 一题：沿用单卡形态（界面最熟，也最省一次点击）
        if parsed.len() == 1 {
            let (question, header, options) = parsed.remove(0);
            let (text, cancelled) = self
                .ask_once(cancel, question.clone(), header, options)
                .await?;
            let out = if cancelled {
                format!("1. {question}\n   → （用户跳过了这题）\n")
            } else {
                format!(
                    "1. {question}\n   → {}\n",
                    if text.is_empty() {
                        "（未回答）"
                    } else {
                        &text
                    }
                )
            };
            return Ok(ToolOutput::plain(
                out,
                false,
                started.elapsed().as_millis() as i64,
            ));
        }

        // 多题：合并成一张问卷，一次问完再统一回灌
        let ask_id = wb_pkg::ulid::new_id(wb_pkg::Prefix::Ask);
        let form_questions: Vec<Value> = parsed
            .iter()
            .map(|(question, header, options)| {
                json!({
                    "question_title": if header.is_empty() { question.clone() } else { format!("{header}｜{question}") },
                    "question_options": options.iter().map(|o| json!({
                        "option_label": o.clone(),
                        "option_description": "",
                        "option_preview": Value::Null,
                    })).collect::<Vec<_>>(),
                    "question_multi": false,
                })
            })
            .collect();
        let titles: Vec<String> = parsed
            .iter()
            .map(|(question, header, _)| {
                if header.is_empty() {
                    question.clone()
                } else {
                    format!("{header}｜{question}")
                }
            })
            .collect();
        let card = json!({
            "id": ask_id,
            "method": "questionnaire",
            "title": format!("{} 个问题", parsed.len()),
            "message": "",
            "options": [],
            "questions": form_questions,
            "placeholder": "",
            "prefill": "",
            "timeout_ms": ANSWER_TIMEOUT.as_millis() as u64,
        });
        let rx = self.answers.register_with_card(&ask_id, card.clone());
        (self.ctx.emit)("", "chat:ask", card);
        let outcome = tokio::select! {
            r = rx => Some(r),
            _ = tokio::time::sleep(ANSWER_TIMEOUT) => None,
            _ = cancel.cancelled() => None,
        };
        self.answers.forget(&ask_id);
        let answer = match outcome {
            Some(Ok(v)) => v,
            Some(Err(_)) => {
                (self.ctx.emit)(
                    "",
                    "chat:ask_closed",
                    json!({ "id": ask_id, "reason": "session_gone" }),
                );
                return Err(AppError::new(code::ASK_EXPIRED, "提问通道已关闭"));
            }
            None if cancel.is_cancelled() => {
                (self.ctx.emit)(
                    "",
                    "chat:ask_closed",
                    json!({ "id": ask_id, "reason": "cancelled" }),
                );
                return Err(AppError::new(toolcode::EXEC_CANCELLED, "提问已随会话取消"));
            }
            None => {
                (self.ctx.emit)(
                    "",
                    "chat:ask_closed",
                    json!({ "id": ask_id, "reason": "expired" }),
                );
                return Err(AppError::new(code::ASK_EXPIRED, "提问等待超时（10 分钟）"));
            }
        };
        (self.ctx.emit)(
            "",
            "chat:ask_closed",
            json!({ "id": ask_id, "reason": "answered" }),
        );
        // 问卷答案是逐题文本，用换行分隔；条数对不上就如实说明，不硬凑
        let cancelled = answer
            .get("cancelled")
            .and_then(|v| v.as_bool())
            .unwrap_or(false);
        let raw = answer
            .get("value")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let answers: Vec<&str> = if raw.is_empty() {
            Vec::new()
        } else {
            raw.split('\n').collect()
        };
        let mut out = String::new();
        for (i, title) in titles.iter().enumerate() {
            let value = answers.get(i).copied().unwrap_or("").trim();
            let shown = if cancelled {
                "（用户取消了整张问卷）"
            } else if value.is_empty() {
                "（未回答）"
            } else {
                value
            };
            out.push_str(&format!("{}. {}\n   → {}\n", i + 1, title, shown));
        }
        if !cancelled && answers.len() != titles.len() {
            out.push_str(&format!(
                "\n（注意：问卷有 {} 题，只收到 {} 行答案）\n",
                titles.len(),
                answers.len()
            ));
        }
        Ok(ToolOutput::plain(
            out,
            false,
            started.elapsed().as_millis() as i64,
        ))
    }
}
