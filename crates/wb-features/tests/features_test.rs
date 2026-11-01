//! wb-features 场景测试：依赖图、状态机、持久化往返、协议解析、边界拒绝。
//! 只留「改一处就可能坏一片」的场景；分节顺序与功能清单一致。

use std::path::{Path, PathBuf};
use std::sync::Arc;

use serde_json::json;
use tokio_util::sync::CancellationToken;
use wb_features::ask_user::AnswerBoard;
use wb_features::context_mode::store::KnowledgeStore;
use wb_features::todo::TodoStore;
use wb_features::FeatureCtx;
use wb_tools::Tool;

/// 临时目录里的功能上下文（唯一后缀防并行用例互踩）。
fn ctx_of(tag: &str) -> (PathBuf, FeatureCtx) {
    let dir = std::env::temp_dir().join(format!("wb_feat_{tag}_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    let ctx = FeatureCtx {
        data_dir: dir.clone(),
        workspace: dir.clone(),
        emit: Arc::new(|_, _, _| {}),
    };
    (dir, ctx)
}

/// 跑一次待办动作并取回文本输出。
async fn todo_op(
    tool: &wb_features::todo::TodoTool,
    args: serde_json::Value,
) -> wb_pkg::Result<String> {
    Ok(tool
        .execute(args, CancellationToken::new(), Path::new("."))
        .await?
        .text)
}

// ---- 待办：依赖图 ----

/// 依赖图的拒绝条件（悬空 / 自环 / 成环）、落盘与删除后的悬空清理。
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
    // 序号引用必须被归一成 ULID：存成序号会让成环检测与删除清理静默失效
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

    // 自环 / 悬空 / 成环都必须被拒
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
        "不存在的序号不应当成有效依赖"
    );
    // 第三→第二→第一 已成链，反向让第一依赖第三就是成环
    assert!(
        op(json!({"op":"update","todo_id":id1.clone(),"blocked_by":["3"]}))
            .await
            .is_err(),
        "用序号写法绕回环也应被拒"
    );

    // 删掉被依赖项后，剩下的悬空依赖被清理；合法依赖不受影响
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

/// 回答板三面：待答问题落盘，重启后仍能弹回（会话不因无人应答卡死）；
/// 一次性投递，重复回答 / 无人等待 / 接收端丢弃都安全拒绝；答完、取消、过期三种路径都清盘。
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

    // 重启模拟：新板子读同一文件，问题仍在
    let reborn = AnswerBoard::with_store(path.clone());
    let restored = reborn.load_pending();
    assert_eq!(restored.len(), 1, "重启后应能读回待答问题");
    assert_eq!(restored[0]["id"], "ASK_1");

    // 答完就从盘上消失：不该再弹一个已作废的问题
    assert!(board.answer("ASK_1", json!({ "value": "A" })));
    assert!(
        board.load_pending().is_empty(),
        "已答的问题应从待答名单消失"
    );

    // 取消 / 超时路径也要清理
    let _rx2 = board.register_with_card("ASK_2", json!({ "id": "ASK_2" }));
    assert_eq!(board.load_pending().len(), 1);
    board.forget("ASK_2");
    assert!(board.load_pending().is_empty(), "取消后不应留残留");

    // 过期的不再弹回
    let stale = vec![
        json!({ "ask_id": "ASK_OLD", "ask_card": { "id": "ASK_OLD" },
        "ask_created_at": 0 }),
    ];
    std::fs::write(&path, serde_json::to_string(&stale).unwrap()).unwrap();
    assert!(
        AnswerBoard::with_store(path.clone())
            .load_pending()
            .is_empty(),
        "超过等待上限的问题不该再弹"
    );
    let _ = std::fs::remove_dir_all(&dir);

    // 一次性投递语义
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

// ---- 记忆：FTS5 往返、会话检索、密钥扫描 ----

/// 记忆三面：保存 → 搜索 → 删除回路，同 kind 同文本去重（只刷新时间戳）；
/// 会话检索只命中消息正文行（吃了 JSONL 结构行结果就会混入整条 JSON）；
/// 密钥扫描真凭据必拦（明文落盘等于泄露）、相似无害文本放行。
#[test]
fn memory_roundtrip_session_search_and_secret_scan() {
    let (dir, ctx) = ctx_of("memory");
    let store = wb_features::memory_store::MemoryStore::open(&ctx).unwrap();
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

    // 中文检索：unicode61 不按词切分，整句会被当成一个 token，
    // 只走 MATCH 的话查「深色」永远为空——LIKE 逐词召回必须在。
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
    // 中英混排查询：一边的词也要能拉回结果
    assert_eq!(store.search("主题 ulid", 10).unwrap().len(), 1);

    // 会话检索只吃消息正文行
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
    let hits = wb_features::memory_store::search_sessions(&dir.join("sessions"), "compaction", 10)
        .unwrap();
    let mut line_nos: Vec<usize> = hits.iter().map(|h| h.line_no).collect();
    line_nos.sort();
    assert_eq!(line_nos, [2, 4, 5], "只应命中消息正文行");
    // 会话正文里的中文同样要能检索到
    std::fs::write(
        sessions.join("124_SESS_y.jsonl"),
        r#"{"type":"message","message":{"role":"user","content":"帮我看看压缩流程"}}"#,
    )
    .unwrap();
    let zh_hits =
        wb_features::memory_store::search_sessions(&dir.join("sessions"), "压缩流程", 10).unwrap();
    assert_eq!(zh_hits.len(), 1, "会话中文子串检索应命中");

    // 密钥扫描：真凭据拦下，相似无害文本放行
    assert!(wb_features::memory::scan_secret("key is sk-abc123XYZ4567890def").is_some());
    assert!(wb_features::memory::scan_secret("AKIAIOSFODNN7EXAMPLE").is_some());
    assert!(wb_features::memory::scan_secret("token ghp_0123456789abcdefghijklm").is_some());
    assert!(wb_features::memory::scan_secret("-----BEGIN RSA PRIVATE KEY-----").is_some());
    assert!(wb_features::memory::scan_secret("用户偏好深色主题，项目用 Rust 1.85").is_none());
    assert!(wb_features::memory::scan_secret("task-1234567890 是工单号").is_none());
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 知识库：FTS5 + RRF 双路召回 ----

/// 收录 → 检索往返：命中带来源；短语查询两路都能召回；空查询返回空。
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
    assert!(!hits.is_empty());
    assert!(hits[0].chunk_excerpt.contains("hello"));
    assert_eq!(hits[0].chunk_source, "file:///a.rs");
    assert_eq!(kb.doc_count().unwrap(), 1);

    kb.index("s", "t", "the quick brown fox jumps", 1000)
        .unwrap();
    assert!(!kb.search("quick brown", 5).unwrap().is_empty());
    assert!(kb.search("   ", 5).unwrap().is_empty());
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 联网：正文提取、搜索解析、视频字幕 ----

/// 网页解析三件套：正文提取剥结构噪音；三家搜索源转统一结果（跳转解开、标签剥掉）；
/// 字幕轨按语言偏好选轨且正文按时间轴分段。
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
    assert_eq!(b[0].result_url, "https://example.com/doc");

    let wiki = r#"{"query":{"search":[{"title":"Rust","snippet":"A <b>systems</b> language."}]}}"#;
    let w = wb_features::web_providers::parse_wikipedia(wiki);
    assert_eq!(w[0].result_url, "https://en.wikipedia.org/wiki/Rust");
    assert!(!w[0].result_snippet.contains("<b>"), "摘要应剥标签");

    // 四个 API 源的响应形状各异，各家解析器要认得自家的字段名
    let brave = r#"{"web":{"results":[{"title":"Brave A","url":"https://a.example","description":"desc a"}]}}"#;
    let br = wb_features::web_providers::parse_brave(brave);
    assert_eq!(br.len(), 1);
    assert_eq!(br[0].result_snippet, "desc a", "Brave 摘要取 description");
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
    // 没给 URL 的条目要丢掉：返一条点不开的假结果比不返更坑
    assert!(
        wb_features::web_providers::parse_brave(r#"{"web":{"results":[{"title":"x"}]}}"#)
            .is_empty(),
        "缺 url 不应当成一条有效结果"
    );

    // 排队：队里不该出现「需要密钥但没配」的源；免密钥公开源始终兜底
    let q = wb_features::web_providers::queue_for("");
    let codes: Vec<&str> = q.iter().map(|(p, _)| p.provider_code).collect();
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
    assert_eq!(
        wb_features::web_providers::by_code("brave")
            .unwrap()
            .provider_env_keys,
        ["BRAVE_API_KEY"],
        "实现里的变量名要和工具描述一致"
    );
    // 未知源码 → 空队（工具层据此报「未知搜索源」）
    assert!(wb_features::web_providers::queue_for("nope").is_empty());
    assert_eq!(
        wb_features::web_providers::codes().len(),
        wb_features::web_providers::PROVIDERS.len(),
        "工具参数的 enum 必须跟实际源表同步"
    );

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

// ---- 模糊搜索：产物目录跳过 ----

/// 遍历工作区时跳过 node_modules 与 target 这类产物目录。
#[test]
fn walker_skips_build_output_dirs() {
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
    let _ = std::fs::remove_dir_all(&root);
}

// ---- 语言服务：扩展名路由、产物目录跳过、文本编辑 ----

/// 扩展名决定语言服务器；未收录的没有服务器；诊断遍历跳过产物目录；
/// 文本编辑按区间替换且多行多处不能错位——区间算错会把修复写到错的行上。
#[test]
fn lsp_routes_dirs_and_edits() {
    use wb_features::lsp::adapters::{server_by_name, server_for_extension};
    assert_eq!(
        server_for_extension(".rs").unwrap().server_name,
        "rust-analyzer"
    );
    assert_eq!(server_for_extension("TS").unwrap().server_name, "biome");
    assert!(server_for_extension(".unknownext").is_none());

    let root = std::env::temp_dir().join(format!("wb_lsp_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&root);
    std::fs::create_dir_all(root.join("src")).unwrap();
    std::fs::create_dir_all(root.join("target")).unwrap();
    std::fs::write(root.join("src/a.rs"), "fn a() {}").unwrap();
    std::fs::write(root.join("target/b.rs"), "fn b() {}").unwrap();
    let files =
        wb_features::lsp::collect_files(server_by_name("rust-analyzer").unwrap(), &root, 10);
    assert_eq!(files.len(), 1);
    assert!(files[0].ends_with("a.rs"));

    let edit = |l0: u32, c0: u32, l1: u32, c1: u32, text: &str| wb_features::lsp::edits::TextEdit {
        edit_start_line: l0,
        edit_start_char: c0,
        edit_end_line: l1,
        edit_end_char: c1,
        edit_new_text: text.into(),
    };
    let edits = vec![edit(0, 0, 0, 5, "L1"), edit(2, 0, 2, 5, "L3")];
    assert_eq!(
        wb_features::lsp::edits::apply_edits("line1\nline2\nline3", &edits),
        "L1\nline2\nL3"
    );
    let _ = std::fs::remove_dir_all(&root);
}

// ---- 目标：状态机与续跑提示 ----

/// 续跑提示只在「活跃且有未完成步骤」时出现；阻塞 / 无目标 / 全完成都不给。
#[test]
fn goal_continuation_only_when_actionable() {
    use wb_features::goal::{continuation_prompt, GoalState, GoalStep, GoalStore};
    let (dir, ctx) = ctx_of("goal");
    let store = GoalStore::new(&ctx);
    let goal = |status: &str, steps: &[(&str, bool)]| GoalState {
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
        goal_created_at: 1,
        goal_updated_at: 1,
    };

    assert!(continuation_prompt(&ctx).is_none(), "没有目标时不该续跑");

    store
        .save(&goal("active", &[("写表单", true), ("接接口", false)]))
        .unwrap();
    let prompt = continuation_prompt(&ctx).expect("活跃目标应给出续跑提示");
    assert!(prompt.contains("把登录页做完"), "提示应带目标描述");
    assert!(prompt.contains("接接口"), "提示应列出待办步骤");
    assert!(!prompt.contains("写表单"), "已完成步骤不该再列");

    store.save(&goal("blocked", &[("接接口", false)])).unwrap();
    assert!(continuation_prompt(&ctx).is_none(), "阻塞目标不该自动续跑");

    store.save(&goal("active", &[("接接口", true)])).unwrap();
    assert!(continuation_prompt(&ctx).is_none(), "步骤全完成不该续跑");
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 记忆整合：语义判重 ----

/// 同一件事的两条合并且内容不丢、条目数守恒；不相关条目保持独立。
#[test]
fn consolidation_merges_same_facts_and_keeps_distinct_ones() {
    use wb_features::memory_consolidate::consolidate;
    use wb_features::memory_store::MemoryStore;
    let (dir, ctx) = ctx_of("memory_consolidate");
    let store = Arc::new(MemoryStore::open(&ctx).unwrap());

    // 同一件事两种说法（pnpm）+ 不相关的两条 → 只合并那对，内容并入更早一条
    store.save("project", "项目用 pnpm 管理依赖", 1).unwrap();
    store
        .save("project", "这个项目的依赖由 pnpm 管理", 2)
        .unwrap();
    store.save("user", "用户偏好深色主题", 3).unwrap();
    store
        .save("failure", "构建失败是因为 Node 版本太低", 4)
        .unwrap();
    let report = consolidate(&store);
    assert_eq!(report.scanned, 4);
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
    assert_eq!(
        store.list_by_kind("user", 10).unwrap().len(),
        1,
        "不相关条目不该被动"
    );

    // 说法相似但不是一回事的保持独立（部署 vs 本地起库）
    store.save("project", "部署用 docker compose", 5).unwrap();
    store
        .save("project", "本地开发用 docker compose 启动数据库", 6)
        .unwrap();
    let report = consolidate(&store);
    assert_eq!(
        report.remaining + report.merged,
        report.scanned,
        "条目数守恒"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 计划：标注按范围分别留存 ----

/// 计划标注：整条计划与某一步都能标，标注随计划一起呈现且步骤标注带序号。
/// 标注错位会让用户看到「这一步」被指向了别的步骤。
#[test]
fn plan_annotations_are_recorded_per_scope() {
    use wb_features::plan_types::{PlanAnnotation, PlanState, PlanStep, PlanStore};
    let (dir, ctx) = ctx_of("plan_annotate");
    let store = PlanStore::new(&ctx);
    store
        .mutate(|slot| {
            *slot = Some(PlanState {
                plan_id: "PLAN_1".into(),
                plan_title: "重构登录模块".into(),
                plan_status: "approved".into(),
                plan_reject_note: String::new(),
                plan_steps: vec![
                    PlanStep {
                        step_text: "拆出鉴权中间件".into(),
                        step_done: false,
                    },
                    PlanStep {
                        step_text: "迁移路由".into(),
                        step_done: false,
                    },
                ],
                plan_annotations: Vec::new(),
                plan_updated_at: 1,
            });
            Ok(())
        })
        .unwrap();

    let annotate = |scope: &str, step: i64, text: &str| {
        store
            .mutate(|slot| {
                let p = slot.as_mut().unwrap();
                p.plan_annotations.push(PlanAnnotation {
                    annotation_id: format!("ANN_{}", p.plan_annotations.len() + 1),
                    annotation_scope: scope.into(),
                    annotation_step: step,
                    annotation_text: text.into(),
                    annotation_author: "user".into(),
                    annotation_created_at: 2,
                });
                Ok(())
            })
            .unwrap();
    };
    annotate("plan", 0, "先补测试再动结构");
    annotate("step", 2, "这条要先确认影响范围");

    let p = store.load().expect("计划应可读回");
    assert_eq!(p.plan_annotations.len(), 2, "两条标注都该保留");
    assert!(p.plan_annotations[0]
        .render_line()
        .contains("先补测试再动结构"));
    assert!(
        p.plan_annotations[1].render_line().contains("步骤 2"),
        "步骤标注应带序号"
    );
    let _ = std::fs::remove_dir_all(&dir);
}
