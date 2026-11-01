//! 内核场景测试：一轮对话的循环与工具批、并发分组、回放补齐、
//! 会话树持久化与分支、压缩、子代理隔离与续跑回退。
//!
//! 断言集中在「事件顺序」与「写进去读回来一字不差」两条契约上——前者是界面时序的契约，
//! 后者是会话文件作为唯一真相的契约，任一被破坏都是行为变了。

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
            Ok(ToolOutput {
                text: "ro".into(),
                is_error: false,
                duration_ms: 1,
            })
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
            Ok(ToolOutput {
                text: "w".into(),
                is_error: false,
                duration_ms: 1,
            })
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

// ---- 会话树：持久化、分支、压缩与子代理隔离 ----

/// 会话文件语义：追加 → 重载 → 回放一字不差；压缩以摘要替代早期历史；ContextEdit 把目标条目从回放摘掉。
#[test]
fn session_file_survives_reload_compaction_and_context_edit() {
    let dir = temp_dir("session");
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_TEST");
    let user = |text: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: text.into(),
            timestamp: ts,
        },
    };
    sf.append(user("hi", 1)).unwrap();
    sf.append(wb_kernel::Entry::Message {
        message: wb_kernel::Msg::Assistant {
            content: vec![wb_kernel::Block::Text {
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

    let path = sf.path.clone();
    drop(sf);
    let mut reloaded = wb_kernel::SessionFile::load(&path).unwrap();
    assert_eq!(reloaded.replay_messages().len(), 2, "重载后消息数不该变");

    // 压缩记录落在树上：回放时早期消息被摘要替代。
    reloaded
        .append(wb_kernel::Entry::Compaction {
            summary: "之前聊了问候".into(),
            first_kept_entry_id: String::new(),
            tokens_before: 100,
        })
        .unwrap();
    let after = reloaded.replay_messages();
    assert_eq!(after.len(), 1, "压缩后只剩摘要一条");
    match &after[0] {
        wb_kernel::Msg::User { content, .. } => assert!(
            content.contains("之前聊了问候"),
            "摘要应进入上下文：{content}"
        ),
        other => panic!("压缩后应是摘要形态的 user 消息，得到 {other:?}"),
    }

    // ContextEdit：目标条目被摘出回放。
    let bad = reloaded.append(user("bad", 3)).unwrap().entry_id.clone();
    reloaded
        .append(wb_kernel::Entry::ContextEdit {
            target_entry_id: bad,
            replacement: None,
        })
        .unwrap();
    assert_eq!(
        reloaded.replay_messages().len(),
        1,
        "被编辑的条目不应出现在回放里"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

/// 子代理隔离子上下文、收窄工具白名单、轮次用尽回退最后正文。
#[tokio::test]
async fn subagent_isolates_context_narrows_tools_and_falls_back() {
    let tmp = temp_dir("subagent");
    let provider = FakeProvider::new(vec![
        vec![delta("共找到 3 个匹配文件"), done(30, StopReason::Stop)],
        vec![
            tool_end(1, "c1", r#"{"text":"扫描结果"}"#),
            done(30, StopReason::ToolUse),
        ],
    ]);

    let mut cfg = agent_cfg(provider.clone());
    cfg.system_prompt = "父助手提示词\n<cwd>D:/ws</cwd>\n<skills>\n- demo\n</skills>".into();
    let mut sc = sub_cfg(&tmp, &["echo"]);
    sc.role_prompt = "你是探查员".into();
    sc.task = "扫描工作区并汇总".into();

    let outcome = wb_kernel::run_subagent(&cfg, sc, CancellationToken::new())
        .await
        .expect("子代理不应失败");
    assert_eq!(outcome.conclusion, "共找到 3 个匹配文件", "结论应原样带回");
    assert!(
        outcome.session_id.starts_with("SUB_"),
        "会话 id 应带 SUB_ 前缀"
    );
    assert_eq!(outcome.usage.input, 60, "两轮用量应累加");

    let msgs = load_session(&tmp).replay_messages();
    assert_eq!(
        role_names(&msgs),
        ["user", "assistant", "tool", "assistant"]
    );
    let wb_kernel::Msg::ToolResult { content, .. } = &msgs[2] else {
        panic!("第三条应是工具结果")
    };
    assert_eq!(content, "扫描结果", "工具结果应留在子会话里");

    let sys = provider.first_system_text();
    assert!(sys.contains("你是探查员"), "子提示词应含角色职责");
    assert!(sys.contains("D:/ws"), "子提示词应继承工作区");
    assert!(
        !sys.contains("父助手提示词"),
        "不应把父提示词正文整段灌进子代理"
    );

    // 工具白名单收窄——task 被剔除，子代理不能递归派生。
    let tmp = temp_dir("subagent-tools");
    let provider = FakeProvider::new(vec![vec![delta("结论"), done(0, StopReason::Stop)]]);
    let mut cfg = agent_cfg(provider.clone());
    cfg.enabled_tools = vec!["echo".into(), "task".into()];
    let mut sc = sub_cfg(&tmp, &["echo"]);
    sc.tool_names = vec!["echo".into(), "task".into()];
    wb_kernel::run_subagent(&cfg, sc, CancellationToken::new())
        .await
        .expect("子代理不应失败");
    let names: Vec<String> = provider.seen.lock().unwrap()[0]
        .tools
        .iter()
        .map(|t| t.name.clone())
        .collect();
    assert_eq!(names, ["echo"], "task 应被剔除，避免子代理再派子代理");

    // 轮次用尽回退到最后正文。
    let tmp = temp_dir("subagent-limit");
    let provider = FakeProvider::new(vec![
        vec![tool_end(2, "c2", r#"{"text":"y"}"#), tool_use()],
        vec![
            delta("我先看看文件"),
            tool_end(1, "c1", r#"{"text":"x"}"#),
            tool_use(),
        ],
    ]);
    let mut sc = sub_cfg(&tmp, &["echo"]);
    sc.max_turns = 2;
    let outcome = wb_kernel::run_subagent(&agent_cfg(provider), sc, CancellationToken::new())
        .await
        .expect("子代理不应报错");
    assert!(
        outcome.conclusion.contains("我先看看文件"),
        "轮次用尽应回退到最后一段正文，实际：{}",
        outcome.conclusion
    );
    let _ = std::fs::remove_dir_all(&tmp);
}

/// 分支编辑：从任一用户节点分叉、旧分支可切回、重载后分支不变、一条不删。

#[test]
fn session_fork_keeps_old_branch_and_switches_back() {
    let dir = temp_dir("fork");
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_FORK");
    let user = |t: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: t.into(),
            timestamp: ts,
        },
    };
    let asst = |t: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::Assistant {
            content: vec![wb_kernel::Block::Text { text: t.into() }],
            provider_id: "p".into(),
            model_id: "m".into(),
            message_id: format!("MSG_{ts}"),
            usage: None,
            stop_reason: "stop".into(),
            timestamp: ts,
            duration_ms: 1,
        },
    };
    sf.append(user("第一问", 1)).unwrap();
    sf.append(asst("第一答", 2)).unwrap();
    let second_user = sf.append(user("第二问", 3)).unwrap().entry_id.clone();
    sf.append(asst("第二答", 4)).unwrap();
    assert_eq!(sf.path_to_leaf().len(), 4);

    let forks = wb_kernel::session::user_entry_ids(&sf);
    assert_eq!(forks.len(), 2, "两条用户消息都应可分支");
    assert!(forks.contains(&second_user));

    let first_user = forks[0].clone();
    sf.fork_at(&first_user).unwrap();
    assert_eq!(sf.path_to_leaf().len(), 1, "回到第一问处应只剩一条");
    let alt = wb_kernel::session::alternate_leaves(&sf);
    assert_eq!(alt.len(), 1, "旧分支末端应该能切回去，实际：{alt:?}");

    sf.append(user("另起一问", 5)).unwrap();
    sf.append(asst("另起一答", 6)).unwrap();
    let new_path: Vec<String> = sf
        .replay_messages()
        .iter()
        .map(|m| match m {
            wb_kernel::Msg::User { content, .. } => content.clone(),
            wb_kernel::Msg::Assistant { content, .. } => format!("a:{content:?}"),
            _ => String::new(),
        })
        .collect();
    assert!(
        new_path.iter().any(|s| s.contains("另起一问")),
        "新分支应包含新提问"
    );
    assert!(
        !new_path.iter().any(|s| s.contains("第二问")),
        "旧分支的消息不该混进新分支"
    );
    assert_eq!(sf.entries.len(), 6, "应是原 4 条 + 新分支 2 条，一条不删");

    let old_leaf = alt[0].clone();
    sf.fork_at(&old_leaf).unwrap();
    let back: Vec<String> = sf
        .replay_messages()
        .iter()
        .filter_map(|m| match m {
            wb_kernel::Msg::User { content, .. } => Some(content.clone()),
            _ => None,
        })
        .collect();
    assert_eq!(back, vec!["第一问", "第二问"], "切回后应恢复原分支");

    let path = sf.path.clone();
    drop(sf);
    let reloaded = wb_kernel::SessionFile::load(&path).unwrap();
    let again: Vec<String> = reloaded
        .replay_messages()
        .iter()
        .filter_map(|m| match m {
            wb_kernel::Msg::User { content, .. } => Some(content.clone()),
            _ => None,
        })
        .collect();
    assert_eq!(again, vec!["第一问", "第二问"], "重载后分支不该变");
    let _ = std::fs::remove_dir_all(&dir);
}

/// 压缩把旧消息整段丢掉，reinforce 闭包必须把仍在生效的活状态钉回上下文。

#[tokio::test]
async fn compaction_reinjects_live_state_after_dropping_history() {
    let dir = temp_dir("reinforce");
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_REINFORCE");
    for i in 0..30 {
        sf.append(wb_kernel::Entry::Message {
            message: wb_kernel::Msg::User {
                content: format!("{i} {}", "填充上下文".repeat(80)),
                timestamp: i,
            },
        })
        .unwrap();
    }
    let before = sf.replay_messages().len();
    let provider = FakeProvider::new(vec![vec![
        delta("已摘要前三十条"),
        done(0, StopReason::Stop),
    ]]);
    let reinforce: wb_kernel::compaction::ReinforceFn =
        Box::new(|| Some("待办清单：\n1. [in_progress] 修复沙箱".into()));

    let (tokens_before, summary) = wb_kernel::compaction::compact(
        &mut sf,
        provider.as_ref(),
        "m",
        1024,
        Some(&reinforce),
        CancellationToken::new(),
    )
    .await
    .expect("压缩不应报错");
    assert!(tokens_before > 0, "应估出压缩前 token 数");
    assert!(summary.contains("已摘要"), "摘要应来自模型返回");

    let msgs = sf.replay_messages();
    assert!(msgs.len() < before, "压缩应减少回放条数");
    let last_user = msgs
        .iter()
        .rev()
        .find_map(|m| match m {
            wb_kernel::Msg::User { content, .. } => Some(content.clone()),
            _ => None,
        })
        .expect("压缩后应有一条活状态消息");
    assert!(
        last_user.contains("修复沙箱"),
        "活状态应被钉回上下文，实际：{last_user}"
    );

    // 对照组：同样内容不开活状态注入，回放应正好少掉那条 user 消息（差量比才分得清折叠与钉回）。
    let dir2 = temp_dir("reinforce-off");
    let mut sf2 = wb_kernel::SessionFile::create(&dir2, "D:\\t", "SESS_REINFORCE_OFF");
    for i in 0..30 {
        sf2.append(wb_kernel::Entry::Message {
            message: wb_kernel::Msg::User {
                content: format!("{i} {}", "填充上下文".repeat(80)),
                timestamp: i,
            },
        })
        .unwrap();
    }
    let empty: wb_kernel::compaction::ReinforceFn = Box::new(|| None);
    wb_kernel::compaction::compact(
        &mut sf2,
        FakeProvider::new(vec![vec![
            delta("已摘要前三十条"),
            done(0, StopReason::Stop),
        ]])
        .as_ref(),
        "m",
        1024,
        Some(&empty),
        CancellationToken::new(),
    )
    .await
    .unwrap();
    assert_eq!(
        msgs.len(),
        sf2.replay_messages().len() + 1,
        "reinforce 开启时只应多一条活状态消息"
    );
    assert!(
        sf2.replay_messages().iter().all(
            |m| !matches!(m, wb_kernel::Msg::User { content, .. } if content.contains("修复沙箱"))
        ),
        "闭包返回 None 时不应凭空插消息"
    );
    let _ = std::fs::remove_dir_all(&dir);
}
