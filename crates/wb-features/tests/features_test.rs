//! wb-features 场景测试：依赖图 / 回答板 / 记忆 / 知识库 / 目标 / 联网解析 / 模糊搜索 / LSP。
//! 守长链路与边界：成环清理、一次性投递、FTS5 往返与密钥扫描、多搜索源解析、遍历纪律。

use std::path::{Path, PathBuf};
use std::sync::Arc;

use serde_json::json;
use tokio_util::sync::CancellationToken;

use wb_features::ask_user::AnswerBoard;
use wb_features::context_mode::store::KnowledgeStore;
use wb_features::memory_consolidate::ConsolidateOpts;
use wb_features::todo::TodoStore;
use wb_features::FeatureCtx;
use wb_tools::Tool;

/// 临时数据目录与对应的 FeatureCtx；调用方负责保持该目录存活到测试结束。
fn ctx_of(tag: &str) -> (PathBuf, FeatureCtx) {
    let dir = std::env::temp_dir().join(format!("wb_feat_{tag}_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    let ctx = FeatureCtx {
        data_dir: dir.clone(),
        workspace: dir.clone(),
        emit: Arc::new(|_, _, _| {}),
        configs: Default::default(),
    };
    (dir, ctx)
}

async fn todo_op(
    tool: &wb_features::todo::TodoTool,
    args: serde_json::Value,
) -> wb_pkg::Result<String> {
    Ok(tool
        .execute(
            args,
            CancellationToken::new(),
            Path::new("."),
            &wb_tools::ToolCtx::new("SESS_test", "RUN_test"),
        )
        .await?
        .text)
}

// ---- 待办：依赖图 ----

/// 依赖必须归一成 ULID 并拦下悬空 / 自环 / 成环；删除后不留悬空依赖。
/// 存成序号会让成环检测与删除清理静默失效。
#[tokio::test]
async fn todo_dependency_graph_rejects_and_cleans_up() {
    let (dir, ctx) = ctx_of("todo");
    let tool = wb_features::todo::TodoTool::new(Arc::new(TodoStore::new(&ctx)), ctx.clone());
    let op = |v: serde_json::Value| todo_op(&tool, v);

    op(json!({"op":"add","text":"第一步"})).await.unwrap();
    op(json!({"op":"add","text":"第二步","blocked_by":["1"]}))
        .await
        .unwrap();
    op(json!({"op":"add","text":"第三步","blocked_by":["2"]}))
        .await
        .unwrap();

    let store = TodoStore::new(&ctx);
    let all = store.load();
    assert_eq!(all.len(), 3, "三条待办都应落盘");
    let id1 = all[0].todo_id.clone();
    assert_eq!(
        all[1].todo_blocked_by,
        [id1.as_str()],
        "依赖应存成真实 id 而不是序号"
    );
    assert!(
        wb_pkg::ulid::is_valid(&all[1].todo_blocked_by[0]),
        "入库的依赖要是合法 ULID"
    );

    assert!(
        op(json!({"op":"update","todo_id":"3","blocked_by":["3"]}))
            .await
            .is_err(),
        "自己依赖自己应被拒"
    );
    assert!(
        op(json!({"op":"add","text":"悬空","blocked_by":["TODO_999"]}))
            .await
            .is_err()
    );
    assert!(
        op(json!({"op":"add","text":"越界","blocked_by":["4"]}))
            .await
            .is_err(),
        "不存在的序号不应当有效依赖"
    );
    assert!(
        op(json!({"op":"update","todo_id":id1.clone(),"blocked_by":["3"]}))
            .await
            .is_err(),
        "成环也应被拒"
    );

    op(json!({"op":"remove","todo_id":"1"})).await.unwrap();
    let todos = store.load();
    assert_eq!(todos.len(), 2, "只该剩两条");
    let ids: Vec<&str> = todos.iter().map(|t| t.todo_id.as_str()).collect();
    assert!(
        todos
            .iter()
            .all(|t| t.todo_blocked_by.iter().all(|b| ids.contains(&b.as_str()))),
        "不应留悬空依赖"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 提问回答板 ----

/// 一次性投递与持久化：待答落盘、重启仍能读回；重复回答 / 无人等待 / 接收端丢弃
/// 都安全；答完、取消、过期都清盘。
#[test]
fn answer_board_persists_delivers_once_and_expires() {
    let dir = std::env::temp_dir().join(format!("wb_ask_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    let path = dir.join("pending_asks.json");

    let board = AnswerBoard::with_store(path.clone());
    let card = json!({ "id": "ASK_1", "method": "select", "message": "选哪个？" });
    let _rx = board.register_with_card("ASK_1", card.clone());
    assert!(path.exists(), "登记时应落盘");
    assert_eq!(board.load_pending().len(), 1);
    assert_eq!(board.load_pending()[0]["message"], "选哪个？", "要点不丢");

    let reborn = AnswerBoard::with_store(path.clone());
    assert_eq!(reborn.load_pending().len(), 1, "重启后应能读回待答问题");
    assert_eq!(reborn.load_pending()[0]["id"], "ASK_1");

    assert!(board.answer("ASK_1", json!({ "value": "A" })));
    assert!(
        board.load_pending().is_empty(),
        "已答的问题应从待答名单消失"
    );

    let _rx2 = board.register_with_card("ASK_2", json!({ "id": "ASK_2" }));
    assert_eq!(board.load_pending().len(), 1);
    board.forget("ASK_2");
    assert!(board.load_pending().is_empty(), "取消后不应留残留");

    let stale =
        vec![json!({ "ask_id": "ASK_OLD", "ask_card": { "id": "ASK_OLD" }, "ask_created_at": 0 })];
    std::fs::write(&path, serde_json::to_string(&stale).unwrap()).unwrap();
    assert!(
        AnswerBoard::with_store(path.clone())
            .load_pending()
            .is_empty(),
        "超过等待上限的问题不该再弹"
    );
    let _ = std::fs::remove_dir_all(&dir);

    let board = AnswerBoard::new();
    let rx = board.register("ASK_1");
    assert!(
        board.answer("ASK_1", json!({ "value": "A" })),
        "第一次回答应送达"
    );
    assert_eq!(
        rx.blocking_recv()
            .unwrap()
            .get("value")
            .and_then(|v| v.as_str()),
        Some("A")
    );
    assert!(
        !board.answer("ASK_1", json!({})),
        "同一 ask_id 不能回答两次"
    );
    assert!(
        !board.answer("ASK_404", json!({})),
        "没有等待者的回答应被拒绝"
    );
    let rx2 = board.register("ASK_2");
    drop(rx2);
    assert!(
        !board.answer("ASK_2", json!({})),
        "接收端已丢弃，发送失败但不 panic"
    );
}

// ---- 记忆：FTS5 往返、会话检索、密钥扫描、整合 ----

/// 记忆保存 → 搜索 → 删除回路，同 kind 同文本去重；中文靠逐词召回；
/// 会话检索只命中消息正文行；密钥扫描真凭据必拦、无害文本放行；同一事实的两条合并。
#[test]
fn memory_roundtrip_search_secret_scan_and_consolidation() {
    let (dir, ctx) = ctx_of("memory");
    let store = Arc::new(wb_features::memory_store::MemoryStore::open(&ctx).unwrap());
    let (id1, fresh) = store
        .save("project", "WorkBaby uses ulid keys", 1000)
        .unwrap();
    assert!(fresh && id1.starts_with("MEM_"));
    let (id2, fresh2) = store
        .save("project", "WorkBaby uses ulid keys", 2000)
        .unwrap();
    assert_eq!(id1, id2, "同 kind 同文本应复用同一条");
    assert!(!fresh2);
    store.save("user", "prefers dark theme", 3000).unwrap();

    let hits = store.search("ulid", 10).unwrap();
    assert_eq!(hits.len(), 1);
    assert_eq!(hits[0].memory_id, id1);
    assert_eq!(hits[0].memory_updated_at, 2000, "命中应带最新时间戳");
    assert!(store.remove(&id1).unwrap());
    assert!(!store.remove(&id1).unwrap());
    assert!(store.search("ulid", 10).unwrap().is_empty());
    assert_eq!(store.search("theme", 10).unwrap().len(), 1);

    let (zh_id, _) = store
        .save("user", "用户偏好深色主题与紧凑布局", 4000)
        .unwrap();
    let zh = store.search("深色", 10).unwrap();
    assert_eq!(zh.len(), 1, "中文子串应命中");
    assert_eq!(zh[0].memory_id, zh_id);
    assert_eq!(
        store.search("节表", 10).unwrap().len(),
        0,
        "不存在的词不应误返"
    );

    // 会话检索只吃消息正文行（吃了 JSONL 结构行结果会混入整条 JSON）。
    let sessions = dir.join("sessions").join("D--demo");
    std::fs::create_dir_all(&sessions).unwrap();
    let content = concat!(
        r#"{"type":"session","session_id":"SESS_x"}"#,
        "\n",
        r#"{"type":"message","entry_id":"ENT_1","message":{"role":"user","content":"explain compaction please"}}"#,
        "\n",
        "not a json line\n",
        r#"{"payload":{"message":{"role":"assistant","content":[{"type":"text","text":"compaction runs in four steps"}]}}}"#,
        "\n",
        r#"{"type":"message","message":{"role":"assistant","content":"compaction nested wrapper form"}}"#,
        "\n",
        r#"{"type":"message","message":{"role":"assistant","content":[{"type":"tool_use","text":"ignored"}]}}"#,
        "\n",
    );
    std::fs::write(sessions.join("123_SESS_x.jsonl"), content).unwrap();
    let mut line_nos: Vec<usize> =
        wb_features::memory_store::search_sessions(&dir.join("sessions"), "compaction", 10)
            .unwrap()
            .iter()
            .map(|h| h.line_no)
            .collect();
    line_nos.sort();
    assert_eq!(line_nos, [2, 4, 5], "只应命中消息正文行");

    // 密钥扫描：真凭据必拦，无害文本放行。
    assert!(wb_features::memory::scan_secret("key is sk-abc123XYZ4567890def").is_some());
    assert!(wb_features::memory::scan_secret("AKIAIOSFODNN7EXAMPLE").is_some());
    assert!(wb_features::memory::scan_secret("token ghp_0123456789abcdefghijklm").is_some());
    assert!(wb_features::memory::scan_secret("-----BEGIN RSA PRIVATE KEY-----").is_some());
    assert!(wb_features::memory::scan_secret("用户偏好深色主题，项目用 Rust 1.85").is_none());
    assert!(wb_features::memory::scan_secret("task-1234567890 是工单号").is_none());

    // 语义判重：同一件事的两条合并且内容不丢、条目数守恒；不相关条目保持独立。
    store.save("project", "项目用 pnpm 管理依赖", 1).unwrap();
    store
        .save("project", "这个项目的依赖由 pnpm 管理", 2)
        .unwrap();
    let report = wb_features::memory_consolidate::consolidate(&store, ConsolidateOpts::default());
    // 此前的 store 已含 2 条用户记忆（去重/删除后），加上 2 条 pnpm 共 4 条。
    assert_eq!(report.scanned, 4, "含上方两条 pnpm 记忆");
    assert_eq!(report.merged, 1, "同一件事的两条该合并成一条");
    assert_eq!(
        report.remaining + report.merged,
        report.scanned,
        "条目数守恒"
    );
    let hits = store.search("pnpm", 10).unwrap();
    assert_eq!(hits.len(), 1, "重复项应只剩一条");
    let body = &store.list_by_kind("project", 10).unwrap()[0].1;
    assert!(
        body.contains("pnpm") && body.len() > "项目用 pnpm 管理依赖".len(),
        "合并后内容应更全"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 知识库：FTS5 检索 ----

/// 收录 → 检索往返：命中带来源；短语查询可召回；空查询返回空。
#[test]
fn knowledge_roundtrip_phrase_and_empty_query() {
    let (dir, _ctx) = ctx_of("kb");
    let kb = KnowledgeStore::open(&dir).unwrap();
    kb.index(
        "file:///a.rs",
        "a.rs",
        "fn main() { println!(\"hello world\"); }",
        1000,
    )
    .unwrap();
    let hits = kb.search("hello", 5).unwrap();
    assert!(hits[0].chunk_excerpt.contains("hello"));
    assert_eq!(hits[0].chunk_source, "file:///a.rs");
    assert_eq!(kb.doc_count().unwrap(), 1);

    kb.index("s", "t", "the quick brown fox jumps", 1000)
        .unwrap();
    assert!(!kb.search("quick brown", 5).unwrap().is_empty());
    assert!(kb.search("   ", 5).unwrap().is_empty());
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 模糊搜索与语言服务：共用同一份“跳过产物目录”的遍历纪律 ----

/// 遍历工作区时跳过 node_modules / target，扩展名决定语言服务器；文本编辑按区间替换不错位。
#[test]
fn workspace_scans_skip_build_dirs_and_route_by_extension() {
    use wb_features::lsp::adapters::{server_by_name, server_for_extension};
    use wb_features::lsp::edits::TextEdit;
    assert_eq!(
        server_for_extension(".rs").unwrap().server_name,
        "rust-analyzer"
    );
    assert_eq!(server_for_extension("TS").unwrap().server_name, "biome");
    assert!(server_for_extension(".unknownext").is_none());

    let root = std::env::temp_dir().join(format!("wb_fff_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&root);
    for (dir, file) in [
        ("node_modules", "x.js"),
        ("target", "a.rs"),
        ("src", "main.rs"),
    ] {
        std::fs::create_dir_all(root.join(dir)).unwrap();
        std::fs::write(root.join(dir).join(file), "m").unwrap();
    }

    let mut seen: Vec<String> = Vec::new();
    wb_features::fff::walk_files(&root, &CancellationToken::new(), |_, rel| {
        seen.push(rel.to_string())
    });
    assert!(seen.contains(&"src/main.rs".to_string()), "got {seen:?}");
    assert!(!seen
        .iter()
        .any(|p| p.contains("node_modules") || p.contains("target")));

    let files =
        wb_features::lsp::collect_files(server_by_name("rust-analyzer").unwrap(), &root, 10);
    assert_eq!(files.len(), 1, "诊断文件清单同样跳过产物目录");
    assert!(files[0].ends_with("main.rs"));

    // 多区间替换不能互相错位。
    let edits = vec![
        TextEdit {
            edit_start_line: 0,
            edit_start_char: 0,
            edit_end_line: 0,
            edit_end_char: 5,
            edit_new_text: "L1".into(),
        },
        TextEdit {
            edit_start_line: 2,
            edit_start_char: 0,
            edit_end_line: 2,
            edit_end_char: 5,
            edit_new_text: "L3".into(),
        },
    ];
    assert_eq!(
        wb_features::lsp::edits::apply_edits("line1\nline2\nline3", &edits),
        "L1\nline2\nL3"
    );

    let _ = std::fs::remove_dir_all(&root);
}

// ---- 目标：状态机与续跑提示 ----

/// 续跑提示只在「活跃、归属本会话、且有未完成步骤」时出现；
/// 阻塞 / 无目标 / 全完成 / 属于别的会话都不给。
#[test]
fn goal_continuation_only_when_actionable() {
    use wb_features::goal::{continuation_prompt, GoalState, GoalStep, GoalStore};
    let (dir, ctx) = ctx_of("goal");
    let store = GoalStore::new(&ctx);
    let goal = |status: &str, steps: &[(&str, bool)], owner: &str| GoalState {
        goal_id: "GOAL_1".into(),
        goal_text: "把登录页做完".into(),
        goal_status: status.into(),
        goal_note: String::new(),
        goal_steps: steps
            .iter()
            .map(|(t, d)| GoalStep {
                step_text: (*t).into(),
                step_done: *d,
            })
            .collect(),
        goal_session_id: owner.into(),
        goal_created_at: 1,
        goal_updated_at: 1,
    };

    assert!(
        continuation_prompt(&ctx, "SESS_A").is_none(),
        "没有目标时不该续跑"
    );
    store
        .save(&goal(
            "active",
            &[("写表单", true), ("接接口", false)],
            "SESS_A",
        ))
        .unwrap();
    let prompt = continuation_prompt(&ctx, "SESS_A").expect("活跃目标应给出续跑提示");
    assert!(prompt.contains("把登录页做完"), "提示应带目标描述");
    assert!(prompt.contains("接接口"), "提示应列出待办步骤");
    assert!(!prompt.contains("写表单"), "已完成步骤不该再列");
    assert!(
        continuation_prompt(&ctx, "SESS_B").is_none(),
        "属于别的会话的目标不该驱动本会话续跑"
    );
    store
        .save(&goal("blocked", &[("接接口", false)], "SESS_A"))
        .unwrap();
    assert!(
        continuation_prompt(&ctx, "SESS_A").is_none(),
        "阻塞目标不该自动续跑"
    );
    store
        .save(&goal("active", &[("接接口", true)], "SESS_A"))
        .unwrap();
    assert!(
        continuation_prompt(&ctx, "SESS_A").is_none(),
        "步骤全完成不该续跑"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 联网：正文提取、搜索源解析、排队、视频字幕 ----

/// 网页解析三件套：正文提取剥结构噪音；多家搜索源转统一结果（跳转解开、标签剥掉、
/// 各家字段名各异）；排队的门禁（未配密钥的 API 源不进队、公开源始终兜底）；
/// 字幕轨按语言偏好选轨。
#[test]
fn web_parsing_extracts_text_results_and_captions() {
    let html = r#"<html><head><title>文档标题</title><style>.a{color:red}</style></head>
<body><nav>菜单导航</nav><h1>正文标题</h1><p>第一段 &amp; 说明</p>
<script>var x = "&lt;script&gt;";</script><footer>版权信息</footer></body></html>"#;
    let text = wb_features::web_parse::readable_text(html);
    assert!(text.contains("正文标题") && text.contains("第一段 & 说明"));
    assert!(!text.contains("var x") && !text.contains("菜单导航") && !text.contains("<p>"));

    let ddg = r##"<div class="result results_links">
<a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fwww.rust-lang.org%2F&amp;rut=abc">The Rust Programming Language</a>
<a class="result__snippet" href="#">A language empowering everyone &amp; building reliable software.</a>
</div>"##;
    let dd = wb_features::web_parse::parse_results(ddg);
    assert_eq!(
        dd[0].result_url, "https://www.rust-lang.org/",
        "跳转包装应解开"
    );
    assert_eq!(
        dd[0].result_snippet,
        "A language empowering everyone & building reliable software."
    );

    let bing = r#"<li class="b_algo"><h2><a href="https://example.com/doc">Example Doc</a></h2><p>Some snippet here.</p></li>"#;
    let b = wb_features::web_providers::parse_bing(bing);
    assert_eq!(b[0].result_title, "Example Doc");

    let wiki = r#"{"query":{"search":[{"title":"Rust","snippet":"A <b>systems</b> language."}]}}"#;
    let w = wb_features::web_providers::parse_wikipedia(wiki);
    assert_eq!(w[0].result_url, "https://en.wikipedia.org/wiki/Rust");
    assert!(!w[0].result_snippet.contains("<b>"), "摘要应剥标签");

    // 各家 API 响应形状各异，解析器要认得自家字段名。
    let brave = r#"{"web":{"results":[{"title":"Brave A","url":"https://a.example","description":"desc a"}]}}"#;
    assert_eq!(
        wb_features::web_providers::parse_brave(brave)[0].result_snippet,
        "desc a",
        "Brave 摘要取 description"
    );
    let tavily = r#"{"results":[{"title":"Tav B","url":"https://b.example","content":"body b"}]}"#;
    assert_eq!(
        wb_features::web_providers::parse_tavily(tavily)[0].result_snippet,
        "body b"
    );
    let serper = r#"{"organic":[{"title":"Ser C","link":"https://c.example","snippet":"snip c"}]}"#;
    assert_eq!(
        wb_features::web_providers::parse_serper(serper)[0].result_url,
        "https://c.example",
        "Serper 的链接字段是 link"
    );
    let exa = r#"{"results":[{"title":"Exa D","url":"https://d.example","text":"text d"}]}"#;
    assert_eq!(
        wb_features::web_providers::parse_exa(exa)[0].result_title,
        "Exa D"
    );
    assert!(
        wb_features::web_providers::parse_brave(r#"{"web":{"results":[{"title":"x"}]}}"#)
            .is_empty(),
        "缺 url 不应当成一条有效结果"
    );

    // 排队：未配密钥的 API 源不进队，免密钥公开源始终兜底。
    let codes: Vec<&str> = wb_features::web_providers::queue_for("")
        .iter()
        .map(|(p, _)| p.provider_code)
        .collect();
    for c in &codes {
        let p = wb_features::web_providers::by_code(c).unwrap();
        assert!(
            p.provider_env_keys.is_empty() || wb_features::web_providers::key_for(p).is_some(),
            "未配密钥的 API 源不该进队：{c}"
        );
    }
    for open in ["duckduckgo", "bing", "wikipedia"] {
        assert!(codes.contains(&open), "公开源应一直在队：{open}");
    }
    assert!(
        wb_features::web_providers::queue_for("nope").is_empty(),
        "未知源码 → 空队"
    );

    // 界面里填的密钥优先于环境变量：用户不必先学会改环境变量才能联网。
    wb_features::config::set_web_keys(wb_features::web_providers::parse_keys(
        "brave=cfg-key\n\n tavily = t-key \nbad-line",
    ));
    let brave = wb_features::web_providers::by_code("brave").unwrap();
    assert_eq!(
        wb_features::web_providers::key_for(brave).as_deref(),
        Some("cfg-key"),
        "配置里的密钥应优先"
    );
    assert!(
        wb_features::web_providers::queue_for("")
            .iter()
            .any(|(p, k)| p.provider_code == "brave" && k.is_some()),
        "配了密钥的 API 源应进队"
    );
    wb_features::config::set_web_keys(Vec::new());

    use wb_features::web_video::{caption_tracks, parse_caption, pick_track};
    let yt = r#"{"playerResponse":{"captions":{"playerCaptionsTracklistRenderer":{"captionTracks":[
      {"baseUrl":"https://yt.example/api/timedtext?v=1&lang=en","name":{"simpleText":"English"}},
      {"baseUrl":"https://yt.example/api/timedtext?v=1&lang=zh","name":{"simpleText":"Chinese"}}
    ]}}}}"#;
    let tracks = caption_tracks(yt);
    assert_eq!(tracks.len(), 2, "两条字幕轨都该被认出");
    assert_eq!(pick_track(&tracks, "zh").unwrap().label, "Chinese");
    let xml = r#"<transcript><text start="0.0" dur="1.2">Hello &amp; welcome</text>
<text start="1.2" dur="2.0">Second line</text></transcript>"#;
    assert_eq!(parse_caption(xml), "Hello & welcome\nSecond line");
}
