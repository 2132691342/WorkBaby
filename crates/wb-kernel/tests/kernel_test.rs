//! 内核场景测试：对话循环、工具批、会话树、压缩、子代理。断言守两条契约——事件顺序与回放一字不差。

mod common;

use std::path::Path;

use common::*;
use std::sync::{Arc, Mutex};

#[tokio::test]
async fn agent_loop_replays_tool_batches_end_to_end() {
    let tmp = temp_dir("run");
    let provider = FakeProvider::new(vec![
        vec![delta("工具结果已确认"), done(100, StopReason::Stop)],
        vec![
            delta("我先调用工具"),
            tool_end(1, "call_1", r#"{"text":"你好"}"#),
            tool_use(),
        ],
    ]);

    let events: Arc<Mutex<Vec<&'static str>>> = Arc::new(Mutex::new(Vec::new()));
    let sink = events.clone();
    let emit: wb_kernel::EmitFn =
        Arc::new(move |ev: &KernelEvent| sink.lock().unwrap().push(event_name(ev)));
    let (_steer_tx, steer_rx) = tokio::sync::mpsc::channel(4);

    let outcome = wb_kernel::run(
        wb_kernel::SessionFile::create(&tmp, "D:/ws", "SESS_run"),
        agent_cfg(provider.clone()),
        emit,
        CancellationToken::new(),
        steer_rx,
        "请调用 echo 工具".into(),
    )
    .await
    .expect("一轮运行不应失败");
    assert!(!outcome.aborted, "不该被中止");
    assert_eq!(outcome.last_text, "工具结果已确认", "最后一轮正文应回传");

    // 事件顺序契约：final 先于 message_end，工具批夹在两 turn 之间，settled 收尾。
    let expect =
        "agent_start turn_start text_delta assistant_final message_end tool_start tool_end \
        turn_end turn_start text_delta assistant_final message_end turn_end agent_end settled";
    assert_eq!(
        events.lock().unwrap().join(" "),
        expect,
        "事件顺序契约被破坏：界面时序依赖它"
    );

    // 落盘回放：user → assistant(工具调用) → tool(结果) → assistant(正文)。
    let msgs = load_session(&tmp).replay_messages();
    assert_eq!(
        role_names(&msgs),
        ["user", "assistant", "tool", "assistant"],
        "回放序错乱"
    );
    let wb_kernel::Msg::ToolResult { content, .. } = &msgs[2] else {
        panic!("第三条应是工具结果")
    };
    assert_eq!(content, "你好", "工具结果应原样回灌");
    assert_eq!(
        provider.seen.lock().unwrap()[1].messages.len(),
        4,
        "工具结果必须紧跟其调用回灌"
    );
    let _ = std::fs::remove_dir_all(&tmp);

    // 并行工具批——漏掉一个就是丢掉一次调用。
    let tmp = temp_dir("parallel");
    let provider = FakeProvider::new(vec![
        vec![delta("都查完了"), done(0, StopReason::Stop)],
        vec![
            tool_end(0, "c0", r#"{"text":"A"}"#),
            tool_end(1, "c1", r#"{"text":"B"}"#),
            tool_use(),
        ],
    ]);
    let started = Arc::new(Mutex::new(Vec::<String>::new()));
    let sink = started.clone();
    let emit: wb_kernel::EmitFn = Arc::new(move |ev: &KernelEvent| {
        if let KernelEvent::ToolExecutionStart { args, .. } = ev {
            sink.lock()
                .unwrap()
                .push(args["text"].as_str().unwrap_or("").to_string());
        }
    });
    let (_tx, rx) = tokio::sync::mpsc::channel(4);
    wb_kernel::run(
        wb_kernel::SessionFile::create(&tmp, "D:/ws", "SESS_par"),
        agent_cfg(provider),
        emit,
        CancellationToken::new(),
        rx,
        "并行查两个".into(),
    )
    .await
    .expect("不应失败");
    let mut seen = started.lock().unwrap().clone();
    seen.sort();
    assert_eq!(seen, ["A", "B"], "两个调用都要执行");
    let results: Vec<String> = load_session(&tmp)
        .replay_messages()
        .iter()
        .filter_map(|m| match m {
            wb_kernel::Msg::ToolResult { content, .. } => Some(content.clone()),
            _ => None,
        })
        .collect();
    assert_eq!(results.len(), 2, "两个工具结果都要落盘，实际 {results:?}");
    let _ = std::fs::remove_dir_all(&tmp);
}

/// 并发分组：相邻只读合成一组，写工具自成一组且位置不变（两个写同时改一个文件会互相覆盖）。

#[test]
fn tool_batch_groups_merge_reads_and_isolate_writes() {
    struct ReadOnlyTool;
    #[async_trait]
    impl Tool for ReadOnlyTool {
        fn name(&self) -> &str {
            "ro"
        }
        fn label(&self) -> &str {
            "只读"
        }
        fn description(&self) -> &str {
            "只读工具"
        }
        fn parameters(&self) -> serde_json::Value {
            serde_json::json!({"type":"object"})
        }
        fn annotations(&self) -> wb_tools::ToolAnnotations {
            wb_tools::ToolAnnotations {
                read_only: true,
                destructive: false,
            }
        }
        async fn execute(
            &self,
            _args: serde_json::Value,
            _cancel: CancellationToken,
            _workdir: &Path,
        ) -> wb_pkg::Result<ToolOutput> {
            Ok(ToolOutput::plain("ro", false, 1))
        }
    }
    struct WriteTool;
    #[async_trait]
    impl Tool for WriteTool {
        fn name(&self) -> &str {
            "w"
        }
        fn label(&self) -> &str {
            "写"
        }
        fn description(&self) -> &str {
            "写工具"
        }
        fn parameters(&self) -> serde_json::Value {
            serde_json::json!({"type":"object"})
        }
        fn annotations(&self) -> wb_tools::ToolAnnotations {
            wb_tools::ToolAnnotations {
                read_only: false,
                destructive: true,
            }
        }
        async fn execute(
            &self,
            _args: serde_json::Value,
            _cancel: CancellationToken,
            _workdir: &Path,
        ) -> wb_pkg::Result<ToolOutput> {
            Ok(ToolOutput::plain("w", false, 1))
        }
    }

    let mut registry = ToolRegistry::new();
    registry.register(Arc::new(ReadOnlyTool));
    registry.register(Arc::new(WriteTool));
    registry.register(Arc::new(EchoTool));
    let calls: Vec<(String, String, String)> = [
        ("c0", "ro", "{}"),
        ("c1", "ro", "{}"),
        ("c2", "w", "{}"),
        ("c3", "ro", "{}"),
        ("c4", "echo", "{}"),
        ("c5", "ro", "{}"),
    ]
    .iter()
    .map(|(id, n, a)| (id.to_string(), n.to_string(), a.to_string()))
    .collect();

    let groups = wb_kernel::blocks::group_calls_for_concurrency(&registry, &calls);
    // echo 没有 read_only 注解，按最保守处理：单独串行。
    assert_eq!(
        groups,
        vec![vec![0, 1], vec![2], vec![3], vec![4], vec![5]],
        "只读相邻应并组，写与未标注工具各自串行"
    );
}

/// 进程死在工具执行中会留下没结果的 tool_call；回放必须补齐，否则发给模型是非法消息序。

#[test]
fn replay_backfills_tool_calls_that_never_got_a_result() {
    let dir = temp_dir("dangling");
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_DANGLING");
    sf.append(wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: "跑一下".into(),
            timestamp: 1,
        },
    })
    .unwrap();
    sf.append(wb_kernel::Entry::Message {
        message: wb_kernel::Msg::Assistant {
            content: vec![
                wb_kernel::Block::ToolCall {
                    tool_call_id: "TC_done".into(),
                    name: "read".into(),
                    arguments: "{}".into(),
                },
                wb_kernel::Block::ToolCall {
                    tool_call_id: "TC_lost".into(),
                    name: "ask_user_question".into(),
                    arguments: "{}".into(),
                },
                wb_kernel::Block::ToolCall {
                    tool_call_id: "TC_lost2".into(),
                    name: "bash".into(),
                    arguments: "{}".into(),
                },
            ],
            provider_id: "p".into(),
            model_id: "m".into(),
            message_id: "MSG_1".into(),
            usage: None,
            stop_reason: "tool_use".into(),
            timestamp: 2,
            duration_ms: 1,
        },
    })
    .unwrap();
    sf.append(wb_kernel::Entry::Message {
        message: wb_kernel::Msg::ToolResult {
            tool_call_id: "TC_done".into(),
            tool_name: "read".into(),
            content: "ok".into(),
            is_error: false,
            timestamp: 3,
            duration_ms: 1,
            file_path: None,
            diff: None,
        },
    })
    .unwrap();

    let msgs = sf.replay_messages();
    let results: Vec<&str> = msgs
        .iter()
        .filter_map(|m| match m {
            wb_kernel::Msg::ToolResult { tool_call_id, .. } => Some(tool_call_id.as_str()),
            _ => None,
        })
        .collect();
    let mut sorted = results.clone();
    sorted.sort_unstable();
    assert_eq!(
        sorted,
        vec!["TC_done", "TC_lost", "TC_lost2"],
        "三个调用都得有结果：实际 {results:?}"
    );
    let flagged = msgs
        .iter()
        .filter(|m| matches!(m, wb_kernel::Msg::ToolResult { is_error: true, .. }))
        .count();
    assert_eq!(flagged, 2, "补的结果要标 is_error");
    let aidx = msgs
        .iter()
        .position(|m| matches!(m, wb_kernel::Msg::Assistant { .. }))
        .unwrap();
    let right_after = &msgs[aidx + 1];
    assert!(
        matches!(
            right_after,
            wb_kernel::Msg::ToolResult { is_error: true, .. }
        ),
        "补的结果应紧跟在助手消息后，实际 {right_after:?}"
    );
    let _ = std::fs::remove_dir_all(&dir);
}
