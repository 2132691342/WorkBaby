//! wb-kernel 会话树场景测试：持久化往返、分支、压缩与子代理隔离。断言守一条契约——回放一字不差。

mod common;

use common::*;

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
