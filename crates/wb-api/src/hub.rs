//! SSE Hub：订阅 / seq / 重放。实现 service::Emitter，是事件的唯一出口。

use std::collections::HashMap;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::{Arc, Mutex};

use tokio::sync::mpsc;

use wb_service::Emitter;

/// 单帧（SSE 行）：id=seq，event=事件名，data=JSON。
#[derive(Debug, Clone)]
pub struct Frame {
    /// 全局递增序号
    pub seq: i64,
    /// 事件名（chat:*）
    pub event: String,
    /// 载荷 JSON
    pub data: serde_json::Value,
}

/// 订阅端：接收帧的通道。
pub type SubTx = mpsc::UnboundedSender<Frame>;

/// Hub：按会话键订阅 + 每会话重放缓冲。
pub struct Hub {
    /// session_id（空 = 全局）→ 订阅者
    subs: Mutex<HashMap<String, Vec<SubTx>>>,
    /// 重放缓冲（session_id → 最近 256 帧）
    replay: Mutex<HashMap<String, Vec<Frame>>>,
    /// 全局 seq
    seq: AtomicI64,
}

impl Hub {
    /// 新 Hub。
    pub fn new() -> Arc<Self> {
        Arc::new(Self {
            subs: Mutex::new(HashMap::new()),
            replay: Mutex::new(HashMap::new()),
            seq: AtomicI64::new(0),
        })
    }

    /// 订阅：返回（退订句柄, 帧接收端）。
    pub fn subscribe(
        self: &Arc<Self>,
        session_id: &str,
    ) -> (SubTx, mpsc::UnboundedReceiver<Frame>) {
        let (tx, rx) = mpsc::unbounded_channel();
        self.subs
            .lock()
            .unwrap()
            .entry(session_id.to_string())
            .or_default()
            .push(tx.clone());
        (tx, rx)
    }

    /// 退订（连接断开时调用）。
    pub fn unsubscribe(&self, session_id: &str, tx: &SubTx) {
        if let Some(v) = self.subs.lock().unwrap().get_mut(session_id) {
            v.retain(|t| !t.same_channel(tx));
        }
    }

    /// 取 seq > last 的重放帧。
    pub fn replay_after(&self, session_id: &str, last: i64) -> Vec<Frame> {
        self.replay
            .lock()
            .unwrap()
            .get(session_id)
            .map(|v| v.iter().filter(|f| f.seq > last).cloned().collect())
            .unwrap_or_default()
    }

    /// 广播：会话订阅者 + 全局订阅者都收；进重放缓冲。
    fn push(&self, session_id: &str, event: &str, data: serde_json::Value) {
        let seq = self.seq.fetch_add(1, Ordering::SeqCst) + 1;
        let frame = Frame {
            seq,
            event: event.to_string(),
            data,
        };
        {
            let mut replay = self.replay.lock().unwrap();
            let buf = replay.entry(session_id.to_string()).or_default();
            buf.push(frame.clone());
            if buf.len() > 256 {
                buf.drain(..buf.len() - 256);
            }
        }
        let mut subs = self.subs.lock().unwrap();
        for (key, list) in subs.iter_mut() {
            if key == session_id || key.is_empty() {
                list.retain(|tx| tx.send(frame.clone()).is_ok());
            }
        }
    }
}

impl Emitter for Hub {
    fn emit(&self, session_id: &str, event: &str, data: serde_json::Value) {
        self.push(session_id, event, data);
    }
}
