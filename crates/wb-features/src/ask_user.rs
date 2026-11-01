//! 结构化提问：模型在该猜时改为向用户提问（chat:ask 桥 + oneshot 回答回灌）。
//! 待答问题同时落盘一份：进程重启后能重新弹出来，而不是让会话卡在一个没人能答的调用上。

use std::collections::HashMap;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio::sync::oneshot;
use tokio_util::sync::CancellationToken;
use wb_domain::error::{session as code, tool as toolcode};
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

use crate::FeatureCtx;

/// 单题等待上限：无人回答时报 5005，模型自行决策。
const ANSWER_TIMEOUT: Duration = Duration::from_secs(600);

/// 落盘的问题记录（进程死后还能重新弹给用户）。
#[derive(Debug, Clone, Serialize, Deserialize)]
struct PendingAsk {
    /// 提问 id
    ask_id: String,
    /// 卡片载荷（原样重发，界面不需要另一套解析）
    ask_card: Value,
    /// 登记时间（毫秒）
    ask_created_at: i64,
}

impl PendingAsk {
    /// 是否已超过等待上限（过期的不再弹，直接当无结果处理）。
    fn is_stale(&self, now: i64) -> bool {
        now - self.ask_created_at > ANSWER_TIMEOUT.as_millis() as i64
    }
}

/// 回答板：ask_id → 等待中的回答通道；同时把待答问题写一份到盘上。
#[derive(Default)]
pub struct AnswerBoard {
    /// 内存等待者
    pending: Mutex<HashMap<String, oneshot::Sender<Value>>>,
    /// 待答问题的落盘文件（None = 不落盘，测试用）
    store_path: Option<PathBuf>,
}

impl AnswerBoard {
    /// 空板（不落盘）。
    pub fn new() -> Arc<Self> {
        Arc::new(Self::default())
    }

    /// 落盘版：待答问题写在给定文件里。
    pub fn with_store(store_path: PathBuf) -> Arc<Self> {
        Arc::new(Self {
            pending: Mutex::new(HashMap::new()),
            store_path: Some(store_path),
        })
    }

    /// 读取上次没答完的问题（跳过已过期的）。
    pub fn load_pending(&self) -> Vec<Value> {
        let Some(path) = &self.store_path else {
            return Vec::new();
        };
        let Ok(text) = std::fs::read_to_string(path) else {
            return Vec::new();
        };
        let Ok(list) = serde_json::from_str::<Vec<PendingAsk>>(&text) else {
            return Vec::new();
        };
        let now = now_ms();
        list.into_iter()
            .filter(|a| !a.is_stale(now))
            .map(|a| a.ask_card)
            .collect()
    }

    /// 全量重写落盘文件；失败只告警，不影响问答本身。
    /// 注意必须把调用方已改好的列表传进来：若在这里重新读文件，
    /// 就会把调用方刚做的删除又盖回去。
    fn write_all(&self, list: &[PendingAsk]) {
        let Some(path) = &self.store_path else {
            return;
        };
        match serde_json::to_string(list) {
            Ok(text) => {
                if let Err(e) = wb_pkg::fsutil::write_atomic(path, text.as_bytes()) {
                    tracing::warn!(err = %e, "待答问题写盘失败");
                }
            }
            Err(e) => tracing::warn!(err = %e, "待答问题序列化失败"),
        }
    }

    /// 读原始记录（不做过期过滤）。
    fn load_raw(&self) -> Vec<PendingAsk> {
        let Some(path) = &self.store_path else {
            return Vec::new();
        };
        std::fs::read_to_string(path)
            .ok()
            .and_then(|t| serde_json::from_str(&t).ok())
            .unwrap_or_default()
    }

    /// 登记等待者并落盘，拿接收端。
    pub fn register_with_card(&self, ask_id: &str, card: Value) -> oneshot::Receiver<Value> {
        let (tx, rx) = oneshot::channel();
        self.pending.lock().unwrap().insert(ask_id.to_string(), tx);
        if self.store_path.is_some() {
            let mut list = self.load_raw();
            list.retain(|a| a.ask_id != ask_id);
            list.push(PendingAsk {
                ask_id: ask_id.to_string(),
                ask_card: card,
                ask_created_at: now_ms(),
            });
            self.write_all(&list);
        }
        rx
    }

    /// 登记等待者，拿接收端。
    pub fn register(&self, ask_id: &str) -> oneshot::Receiver<Value> {
        let (tx, rx) = oneshot::channel();
        self.pending.lock().unwrap().insert(ask_id.to_string(), tx);
        rx
    }

    /// 回填一条回答并把它从待答名单里删掉；没有等待者返回 false。
    pub fn answer(&self, ask_id: &str, value: Value) -> bool {
        self.forget(ask_id);
        match self.pending.lock().unwrap().remove(ask_id) {
            Some(tx) => tx.send(value).is_ok(),
            None => false,
        }
    }

    /// 只从落盘名单里移除（取消 / 超时 / 已答都用它）。
    pub fn forget(&self, ask_id: &str) {
        if self.store_path.is_none() {
            return;
        }
        let mut list = self.load_raw();
        let before = list.len();
        list.retain(|a| a.ask_id != ask_id);
        if list.len() != before {
            self.write_all(&list);
        }
    }
}

/// 当前毫秒。
fn now_ms() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
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
            let card = json!({
                "id": ask_id,
                "method": method,
                "title": if header.is_empty() { format!("问题 {}", i + 1) } else { header },
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
