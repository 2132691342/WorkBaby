//! wb-kernel 场景测试：对话循环、工具批、会话树、压缩、子代理。
//! 守两条契约——事件顺序与回放一字不差。

mod common;

use std::path::Path;

use common::*;
use std::sync::{Arc, Mutex};

/// 一轮 run 的完整链路：采样 → 工具 → 回灌 → 定稿，事件顺序、落盘回放、并行工具批。
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
        wb_kernel::UserInput::text("请调用 echo 工具"),
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
        wb_kernel::UserInput::text("并行查两个"),
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

/// 并发分组：相邻只读合成一组，写工具与未标注工具各自串行且位置不变
/// （两个写同时改一个文件会互相覆盖）。
#[test]
fn tool_batch_groups_merge_reads_and_isolate_writes() {
    let mut registry = ToolRegistry::new();
    registry.register(Arc::new(ProbeTool {
        name: "ro",
        read_only: Some(true),
    }));
    registry.register(Arc::new(ProbeTool {
        name: "w",
        read_only: Some(false),
    }));
    registry.register(Arc::new(ProbeTool {
        name: "plain",
        read_only: None,
    }));
    let calls: Vec<(String, String, String)> = ["ro", "ro", "w", "ro", "plain", "ro"]
        .iter()
        .enumerate()
        .map(|(i, n)| (format!("c{i}"), n.to_string(), "{}".to_string()))
        .collect();

    let groups = wb_kernel::blocks::group_calls_for_concurrency(&registry, &calls);
    // 未标注的工具按最保守处理：单独串行。
    assert_eq!(
        groups,
        vec![vec![0, 1], vec![2], vec![3], vec![4], vec![5]],
        "只读相邻应并组，写与未标注工具各自串行"
    );
}

/// 会话文件语义：追加 → 重载 → 回放一字不差；进程死在工具执行中留下的悬空调用
/// 必须补齐（否则发给模型是非法消息序）；压缩以摘要替代早期历史；ContextEdit 摘条目。
#[test]
fn session_file_roundtrip_backfills_dangling_calls_compacts_and_edits() {
    let user = |text: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: text.into(),
            timestamp: ts,
            skill: None,
            images: Vec::new(),
        },
    };
    let assistant = |calls: &[(&str, &str)], ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::Assistant {
            content: calls
                .iter()
                .map(|(id, name)| wb_kernel::Block::ToolCall {
                    tool_call_id: (*id).into(),
                    name: (*name).into(),
                    arguments: "{}".into(),
                })
                .collect(),
            provider_id: "p".into(),
            model_id: "m".into(),
            message_id: format!("MSG_{ts}"),
            usage: None,
            stop_reason: "tool_use".into(),
            timestamp: ts,
            duration_ms: 1,
        },
    };
    let result = |id: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::ToolResult {
            tool_call_id: id.into(),
            tool_name: "read".into(),
            content: "ok".into(),
            is_error: false,
            timestamp: ts,
            duration_ms: 1,
            file_path: None,
            diff: None,
        },
    };

    // ---- 追加 → 重载 → 悬空调用补齐 ----
    let dir = temp_dir("session");
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_TEST");
    sf.append(user("hi", 1)).unwrap();
    sf.append(assistant(
        &[
            ("TC_done", "read"),
            ("TC_lost", "ask_user"),
            ("TC_lost2", "bash"),
        ],
        2,
    ))
    .unwrap();
    sf.append(result("TC_done", 3)).unwrap();

    let path = sf.path.clone();
    drop(sf);
    let mut reloaded = wb_kernel::SessionFile::load(&path).unwrap();
    let msgs = reloaded.replay_messages();
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
    assert!(
        matches!(
            &msgs[aidx + 1],
            wb_kernel::Msg::ToolResult { is_error: true, .. }
        ),
        "补的结果应紧跟在助手消息后，实际 {:?}",
        msgs[aidx + 1]
    );

    // ---- 压缩：早期消息被一条摘要替代 ----
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

    // ---- ContextEdit：目标条目被摘出回放 ----
    let bad = reloaded.append(user("bad", 4)).unwrap().entry_id.clone();
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

/// 分支编辑：从任一用户节点分叉、旧分支可切回、重载后分支不变、一条不删。
#[test]
fn session_fork_keeps_old_branch_and_switches_back() {
    let dir = temp_dir("fork");
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_FORK");
    let user = |t: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: t.into(),
            timestamp: ts,
            skill: None,
            images: Vec::new(),
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

    sf.fork_at(&alt[0]).unwrap();
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
    let again: Vec<String> = wb_kernel::SessionFile::load(&path)
        .unwrap()
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
    let mut sc = sub_cfg(&tmp, &["echo", "task"]);
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

/// 压缩把旧消息整段丢掉，reinforce 闭包必须把仍在生效的活状态钉回上下文。
#[tokio::test]
async fn compaction_reinjects_live_state_after_dropping_history() {
    let fill = |dir: &Path, id: &str| {
        let mut sf = wb_kernel::SessionFile::create(dir, "D:\\t", id);
        for i in 0..30 {
            sf.append(wb_kernel::Entry::Message {
                message: wb_kernel::Msg::User {
                    content: format!("{i} {}", "填充上下文".repeat(80)),
                    timestamp: i,
                    skill: None,
                    images: Vec::new(),
                },
            })
            .unwrap();
        }
        sf
    };

    let dir = temp_dir("reinforce");
    let mut sf = fill(&dir, "SESS_REINFORCE");
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
    let mut sf2 = fill(&dir2, "SESS_REINFORCE_OFF");
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

/// 组请求时的展开：点名技能才插 `<skill>` 块且不吞掉人话，图片必须变成 Image 片段
/// （只落到会话文件而没进请求等于图白传了）。
#[tokio::test]
async fn request_assembly_expands_skill_blocks_and_image_parts() {
    let tmp = temp_dir("request");
    let provider = FakeProvider::new(vec![vec![delta("好的"), done(1, StopReason::Stop)]]);
    let mut cfg = agent_cfg(provider.clone());
    cfg.skill_loader = Some(Arc::new(|name: &str| {
        (name == "deep-research").then(|| "先列假设，再逐个验证。".to_string())
    }));
    let (_tx, rx) = tokio::sync::mpsc::channel(4);
    let sink: wb_kernel::EmitFn = Arc::new(|_| {});

    wb_kernel::run(
        wb_kernel::SessionFile::create(&tmp, "D:/ws", "SESS_skill"),
        cfg,
        sink,
        CancellationToken::new(),
        rx,
        wb_kernel::UserInput {
            text: "研究一下这个仓库".into(),
            skill: Some("deep-research".into()),
            images: vec![wb_kernel::ImagePart {
                mime: "image/png".into(),
                base64: "aGVsbG8=".into(),
            }],
        },
    )
    .await
    .expect("不应失败");

    let user = {
        let seen = provider.seen.lock().unwrap();
        seen[0]
            .messages
            .iter()
            .find(|m| m.role == Role::User)
            .map(|m| m.parts.clone())
            .expect("应有用户消息")
    };
    assert_eq!(user.len(), 3, "技能正文 + 人话 + 图片三段");
    let ContentPart::Text(skill) = &user[0] else {
        panic!("第一段应是技能块")
    };
    assert!(
        skill.starts_with("<skill name=\"deep-research\">"),
        "技能块开头应是标签，实际 {skill}"
    );
    assert!(
        skill.contains("先列假设"),
        "技能正文应原样进请求，实际 {skill}"
    );
    assert!(
        matches!(&user[1], ContentPart::Text(t) if t == "研究一下这个仓库"),
        "人话不能被技能块吞掉，实际 {:?}",
        user[1]
    );
    assert!(
        matches!(&user[2], ContentPart::Image { mime, base64 } if mime == "image/png" && base64 == "aGVsbG8="),
        "图片必须进请求的 Image 片段，实际 {:?}",
        user[2]
    );
    let _ = std::fs::remove_dir_all(&tmp);
}
