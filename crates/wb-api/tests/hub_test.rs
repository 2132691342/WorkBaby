//! wb-api 场景测试：SSE 重放缓冲的取舍。
//! 重放的目的是「接上还在跑的这一轮」；挤掉对话帧会让重连后的界面少内容。

use wb_api::hub::Hub;
use wb_service::Emitter;

/// 高频过程帧不进重放：否则长命令一秒钟八帧，几秒就把真正的对话帧冲出缓冲。
#[test]
fn progress_frames_do_not_displace_replayable_frames() {
    let hub = Hub::new();
    hub.emit("SESS_1", "chat:start", serde_json::json!({}));
    hub.emit("SESS_1", "chat:delta", serde_json::json!({"delta": "正文"}));

    // 模拟一段长命令刷出远超缓冲容量的过程帧
    for _ in 0..8000 {
        hub.emit(
            "SESS_1",
            "chat:tool_progress",
            serde_json::json!({"chunk": "x"}),
        );
    }
    hub.emit(
        "SESS_1",
        "chat:tool_end",
        serde_json::json!({"result": "ok"}),
    );

    let frames = hub.replay_after("SESS_1", 0);
    let names: Vec<&str> = frames.iter().map(|f| f.event.as_str()).collect();
    assert!(
        names.contains(&"chat:delta"),
        "正文帧必须还在重放缓冲里：got {} 帧 {names:?}",
        frames.len()
    );
    assert!(
        names.contains(&"chat:tool_end"),
        "工具结果帧必须还在重放缓冲里"
    );
    assert!(
        !names.contains(&"chat:tool_progress"),
        "过程帧不该进重放缓冲"
    );
}
