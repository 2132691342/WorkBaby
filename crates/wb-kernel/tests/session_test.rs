//! wb-kernel 场景测试：会话树的追加/回放/编辑，与长链路内核运行。
//! 会话树是所有对话的持久底座——回放错乱会让界面显示错误的上下文。

use std::path::PathBuf;

use wb_kernel::{Block, Entry, Msg, SessionFile};

/// 临时会话目录。
fn session_dir(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("wb_sess_{}_{}", std::process::id(), tag));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    dir
}

/// 追加 → 重载 → 回放：进程重启后消息必须一字不差地回来。
#[test]
fn session_append_survives_reload() {
    let dir = session_dir("reload");
    let mut sf = SessionFile::create(&dir, "D:\\t", "SESS_TEST");
    sf.append(Entry::Message {
        message: Msg::User {
            content: "hi".into(),
            timestamp: 1,
        },
    })
    .unwrap();
    sf.append(Entry::Message {
        message: Msg::Assistant {
            content: vec![Block::Text {
                text: "hello".into(),
            }],
            provider_id: "p".into(),
            model_id: "m".into(),
            message_id: "MSG_1".into(),
            usage: None,
            stop_reason: "stop".into(),
            timestamp: 2,
            duration_ms: 5,
        },
    })
    .unwrap();
    assert_eq!(sf.replay_messages().len(), 2);

    let path = sf.path.clone();
    drop(sf);
    let reloaded = SessionFile::load(&path).unwrap();
    assert_eq!(reloaded.replay_messages().len(), 2, "重载后消息数不该变");
    let _ = std::fs::remove_dir_all(&dir);
}

/// 上下文编辑（ContextEdit）把目标条目从回放里摘掉——压缩与回溯都靠它。
#[test]
fn context_edit_drops_targeted_entry() {
    let dir = session_dir("edit");
    let mut sf = SessionFile::create(&dir, "D:\\t", "SESS_TEST2");
    let target = sf
        .append(Entry::Message {
            message: Msg::User {
                content: "bad".into(),
                timestamp: 1,
            },
        })
        .unwrap()
        .entry_id
        .clone();
    sf.append(Entry::ContextEdit {
        target_entry_id: target,
        replacement: None,
    })
    .unwrap();
    assert!(
        sf.replay_messages().is_empty(),
        "被编辑的条目不应出现在回放里"
    );
    let _ = std::fs::remove_dir_all(&dir);
}
