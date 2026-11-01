//! SSE Hub：订阅 / seq / 重放。实现 service::Emitter，是事件的唯一出口。

use std::collections::HashMap;
use std::sync::atomic::{AtomicI64, Ordering};
use std::sync::{Arc, Mutex};

use tokio::sync::mpsc;

use wb_service::Emitter;

/// 每个会话的重放缓冲上限：一轮长回答很容易超过这个数，
/// 超了就从最早的帧开始丢——丢掉的是已验证过的前缀，不是结论。
const REPLAY_CAP: usize = 4096;

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

    /// 广播：全局帧（session_id 为空）发给所有订阅者；会话帧发给本会话 + 全局订阅者。
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
            // 新一轮开始就丢掉上一轮的重放缓冲：重放的目的只有「接上还在跑的这一轮」，
            // 把历史轮次整段重发会让界面先闪一遍旧回答再回到现在。
            if event == "chat:start" {
                buf.clear();
            }
            buf.push(frame.clone());
            if buf.len() > REPLAY_CAP {
                buf.drain(..buf.len() - REPLAY_CAP);
            }
        }
        let mut subs = self.subs.lock().unwrap();
        for (key, list) in subs.iter_mut() {
            // 全局帧（启动时补发的待答提问等）没有会话归属，所有连接都得看到，
            // 否则用户切到对话页时那些卡片永远等不到回答。
            if key == session_id || key.is_empty() || session_id.is_empty() {
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
