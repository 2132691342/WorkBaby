//! 回答板：ask_id → 等待中的回答通道，并把待答问题落一份到盘上。
//! 进程重启后待答问题能重新弹出来，而不是让会话卡在一个没人能答的调用上。

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::sync::oneshot;

/// 当前毫秒。
fn now_ms() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

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
