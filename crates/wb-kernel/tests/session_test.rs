//! wb-kernel 会话树测试：JSONL 往返、头行与叶指针、悬空调用补齐、分支编辑。

mod common;

use common::*;

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
            images: Vec::new(),
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
/// 重载后再 append 不能把会话头再写一遍。
/// 多写一次就是往文件中间插一段 header，之后每次回放那一段都被当成坏行跳过，
/// 会话被静默截断且没有任何错误浮到界面上。
#[test]
fn reloaded_session_never_rewrites_the_header_line() {
    let dir = temp_dir("header");
    let user = |t: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: t.into(),
            timestamp: ts,
            skill: None,
            images: Vec::new(),
        },
    };
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_HDR");
    sf.append(user("一", 1)).unwrap();
    let path = sf.path.clone();
    drop(sf);

    // 每次重载后追加一条：三次加载 → 文件里只能有一个头行
    for i in 0..3 {
        let mut reloaded = wb_kernel::SessionFile::load(&path).unwrap();
        reloaded.append(user("再", 10 + i)).unwrap();
    }

    let text = std::fs::read_to_string(&path).unwrap();
    let headers = text
        .lines()
        .filter(|l| l.contains("\"type\":\"session\""))
        .count();
    assert_eq!(headers, 1, "头行必须只有 1 个，实际 {headers}：\n{text}");

    // 每条非头行都应该是可解析条目（一条都不该冒出坏行）
    let sf = wb_kernel::SessionFile::load(&path).unwrap();
    let total = text.lines().count();
    assert_eq!(
        sf.entries.len(),
        total - 1,
        "除了头行以外全部应是合法条目：解析到 {} / 应为 {}：\n{text}",
        sf.entries.len(),
        total - 1
    );
    assert_eq!(sf.replay_messages().len(), 4, "四条用户消息一条不能丢");
    let _ = std::fs::remove_dir_all(&dir);
}

/// 分叉过之后的消息必须能在重载后回来。
/// 叶指针只在 fork_at 时落盘的话，后续 append 推出的叶子不会写回，
/// 下次加载又回到分叉前的旧位置——分叉之后的对话被静默丢弃。
#[test]
fn leaf_pointer_survives_reload_after_fork() {
    let dir = temp_dir("leafptr");
    let user = |t: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: t.into(),
            timestamp: ts,
            skill: None,
            images: Vec::new(),
        },
    };
    let mut sf = wb_kernel::SessionFile::create(&dir, "D:\\t", "SESS_LEAF");
    sf.append(user("一", 1)).unwrap();
    sf.append(user("二", 2)).unwrap();
    let first = sf.path_to_leaf()[0].entry_id.clone();
    sf.fork_at(&first).unwrap();
    sf.append(user("三", 3)).unwrap();
    let path = sf.path.clone();
    drop(sf);

    let reloaded = wb_kernel::SessionFile::load(&path).unwrap();
    let texts: Vec<String> = reloaded
        .replay_messages()
        .iter()
        .filter_map(|m| match m {
            wb_kernel::Msg::User { content, .. } => Some(content.clone()),
            _ => None,
        })
        .collect();
    assert_eq!(
        texts,
        vec!["一", "三"],
        "重载后应停在新分支的末端，而不是回到分叉前；实际 {texts:?}"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

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
    // 回溯前先把被放弃的分支抽成摘要（旧叶子到分叉点之间的那一段）
    let old_leaf = sf.leaf.clone().unwrap();
    let abandoned = wb_kernel::branch_summary::readable_turns(
        &wb_kernel::branch_summary::abandoned_entries(&sf, &old_leaf, &first_user),
    );
    sf.fork_at(&first_user).unwrap();
    assert_eq!(sf.path_to_leaf().len(), 1, "回到第一问处应只剩一条");
    // 被放弃的那段必须能被读出来，否则回溯后模型不知道自己之前做过什么
    let texts: Vec<&str> = abandoned.iter().map(|t| t.text.as_str()).collect();
    assert_eq!(
        texts,
        vec!["第一答", "第二问", "第二答"],
        "放弃段应从分叉点之后开始，按时间序收集；实际 {texts:?}"
    );
    wb_kernel::branch_summary::inject(&mut sf, "## Tried\n- 试过重写登录表单").unwrap();
    let replayed = sf.replay_messages();
    assert!(
        replayed.iter().any(|m| matches!(
            m,
            wb_kernel::Msg::User { content, .. } if content.contains("已离开的分支摘要")
        )),
        "摘要应作为一条 user 消息接在新分支开头"
    );
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
    assert_eq!(
        sf.entries.len(),
        7,
        "应是原 4 条 + 分支摘要 1 条 + 新分支 2 条，一条不删"
    );

    sf.fork_at(&alt[0]).unwrap();
    let back: Vec<String> = sf
        .replay_messages()
        .iter()
        .filter_map(|m| match m {
            wb_kernel::Msg::User { content, .. } => Some(content.clone()),
            _ => None,
        })
        // 切回旧分支时摘要属于新分支，不在这一条路径上
        .filter(|c| !c.contains("已离开的分支摘要"))
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

// ---- 会话树 ----

/// 树的形状：主干在一条链上，分支挂在分叉点下；当前路径上的节点带 on_path 标记。
/// 画错形状用户就会以为分支是另一条线，切回去切错地方。
#[test]
fn session_tree_marks_path_and_branches() {
    let user = |text: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::User {
            content: text.into(),
            timestamp: ts,
            skill: None,
            images: Vec::new(),
        },
    };
    let asst = |text: &str, ts: i64| wb_kernel::Entry::Message {
        message: wb_kernel::Msg::Assistant {
            content: vec![wb_kernel::Block::Text { text: text.into() }],
            provider_id: "p".into(),
            model_id: "m".into(),
            message_id: format!("MSG_{ts}"),
            usage: None,
            stop_reason: "stop".into(),
            duration_ms: 0,
            timestamp: ts,
        },
    };
    let dir = std::env::temp_dir().join(format!("wb_tree_{}", std::process::id()));
    std::fs::create_dir_all(&dir).unwrap();
    let mut sf = wb_kernel::session::SessionFile::create(&dir, "D:/ws", "SESS_T");

    sf.append(user("第一问", 1)).unwrap();
    sf.append(asst("第一答", 2)).unwrap();
    sf.append(user("第二问", 3)).unwrap();
    let second = sf.leaf.clone().unwrap();
    sf.append(asst("第二答", 4)).unwrap();

    // 从第二问分叉，走另一条路
    sf.fork_at(&second).unwrap();
    sf.append(user("岔路问", 5)).unwrap();
    sf.append(asst("岔路答", 6)).unwrap();

    let tree = wb_kernel::session_tree::build_tree(&sf);
    // 单一根时树根就是那条主干的第一个节点；多根时树根是虚拟根
    let root = if tree.node_preview.is_empty() && !tree.node_children.is_empty() {
        &tree.node_children[0]
    } else {
        &tree
    };
    assert_eq!(root.node_preview, "第一问");
    assert!(root.node_on_path, "第一问在当前路径上");

    // 节点是所有消息：助手回复也在树里，否则树的形状不对应会话的真实结构
    let first_ans = root.node_children[0].clone();
    assert!(first_ans.node_preview.is_empty(), "助手节点没有正文摘要");

    // 主干：第二问是「第一答」的子节点，岔路挂在第二问下
    let second_node = first_ans
        .node_children
        .iter()
        .find(|n| n.node_entry_id == second)
        .expect("第二问应在第一答下");
    assert_eq!(second_node.node_preview, "第二问");
    assert!(second_node.node_is_user, "第二问是用户消息");
    // 第二问下面挂着两条：旧分支的「第二答」与新分出去的「岔路问」
    assert_eq!(
        second_node.node_children.len(),
        2,
        "旧分支与新分支都在第二问下"
    );
    // 分叉后当前路径走的是新那条：岔路问 → 岔路答
    let fork = second_node
        .node_children
        .iter()
        .find(|n| n.node_preview == "岔路问")
        .expect("岔路问应是第二问的子节点");
    assert!(fork.node_on_path, "分叉后当前路径就是新分支");
    let old = second_node
        .node_children
        .iter()
        .find(|n| n.node_preview.is_empty())
        .expect("旧分支的第二答也在第二问下");
    assert!(!old.node_on_path, "旧分支不在当前路径上，但可切回去");

    let _ = std::fs::remove_dir_all(&dir);
}
