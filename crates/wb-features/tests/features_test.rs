//! features 域场景测试：待办依赖图、提问回答板、记忆检索、联网解析、模糊搜索、
//! 知识库往返与 LSP 路由。
//! 全部是「改一处就可能坏一片」的场景，不放平凡断言。

use std::path::PathBuf;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use serde_json::json;
use wb_features::ask_user::AnswerBoard;
use wb_features::context_mode::store::KnowledgeStore;
use wb_features::todo::{TodoItem, TodoStore};
use wb_features::FeatureCtx;

/// 临时目录里的功能上下文。
fn ctx_of(tag: &str) -> (PathBuf, FeatureCtx) {
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let dir = std::env::temp_dir().join(format!("wb_feat_{tag}_{}_{}", std::process::id(), nanos));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    let ctx = FeatureCtx {
        data_dir: dir.clone(),
        workspace: dir.clone(),
        emit: Arc::new(|_, _, _| {}),
    };
    (dir, ctx)
}

// ---- 待办：依赖图与悬空清理 ----

/// add → status → remove 回路；删掉被依赖项后，依赖方的 blocked_by 不留悬空引用。
#[test]
fn todo_dependencies_survive_deletion() {
    let (dir, ctx) = ctx_of("todo");
    let store = TodoStore::new(&ctx);
    let item = |id: &str, blocked: Vec<&str>| TodoItem {
        todo_id: id.into(),
        todo_text: format!("步骤 {id}"),
        todo_status: "pending".into(),
        todo_blocked_by: blocked.into_iter().map(String::from).collect(),
        todo_created_at: 1,
        todo_updated_at: 1,
    };
    store
        .mutate(|items| {
            items.push(item("TODO_1", vec![]));
            items.push(item("TODO_2", vec!["TODO_1"]));
            Ok(())
        })
        .unwrap();
    assert_eq!(store.load().len(), 2, "两条待办都应落盘");
    assert_eq!(store.load()[1].todo_blocked_by, vec!["TODO_1"]);

    store
        .mutate(|items| {
            items.retain(|t| t.todo_id != "TODO_1");
            for t in items.iter_mut() {
                t.todo_blocked_by.retain(|d| d != "TODO_1");
            }
            Ok(())
        })
        .unwrap();
    assert!(
        store.load()[0].todo_blocked_by.is_empty(),
        "删掉 TODO_1 后 TODO_2 不应留悬空依赖"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 提问回答板 ----

/// register → answer 回填；重复回答与无人等待者都被拒。
#[test]
fn answer_board_delivers_once_and_rejects_others() {
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
}

/// 等待者被丢弃后再回答：不 panic，只报失败。
#[test]
fn answer_board_tolerates_dropped_waiter() {
    let board = AnswerBoard::new();
    let rx = board.register("ASK_2");
    drop(rx);
    assert!(
        !board.answer("ASK_2", json!({})),
        "接收端已丢弃，发送失败但不 panic"
    );
}

// ---- 记忆：FTS5 往返与去重 ----

/// 保存 → 搜索 → 删除的完整回路；同 kind 同文本只刷新时间戳不重复插入。
#[test]
fn memory_roundtrip_searches_and_dedups() {
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
    assert!(hits[0].memory_summary.contains("ulid"));
    assert_eq!(hits[0].memory_updated_at, 2000, "命中应带最新时间戳");
    assert!(store.remove(&id1).unwrap());
    assert!(!store.remove(&id1).unwrap());
    assert!(store.search("ulid", 10).unwrap().is_empty());
    assert_eq!(store.search("theme", 10).unwrap().len(), 1);
    let _ = std::fs::remove_dir_all(&dir);
}

/// 历史会话检索只命中消息正文行，不吃 JSONL 的结构行与坏行。
#[test]
fn session_search_hits_message_lines_only() {
    let (dir, _ctx) = ctx_of("sessions");
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
        r#"{"type":"message","message":{"role":"assistant","message":{"content":"compaction nested wrapper form"}}}"#,
        "\n",
        r#"{"type":"message","message":{"role":"assistant","content":[{"type":"tool_use","text":"ignored"}]}}"#,
        "\n",
    );
    std::fs::write(sessions.join("123_SESS_x.jsonl"), content).unwrap();
    let hits = wb_features::memory_store::search_sessions(&dir.join("sessions"), "compaction", 10)
        .unwrap();
    let mut line_nos: Vec<usize> = hits.iter().map(|h| h.line_no).collect();
    line_nos.sort();
    assert_eq!(line_nos, vec![2, 4, 5], "只应命中消息正文行");
    assert!(hits.iter().all(|h| h.line_summary.contains("compaction")));
    assert!(
        wb_features::memory_store::search_sessions(&dir.join("sessions"), "zzznotfound", 10)
            .unwrap()
            .is_empty()
    );
    let _ = std::fs::remove_dir_all(&dir);
}

/// 密钥扫描：真凭据被拦，相似但无害的文本放行。
#[test]
fn secret_scan_separates_credentials_from_prose() {
    assert!(wb_features::memory::scan_secret("key is sk-abc123XYZ4567890def").is_some());
    assert!(wb_features::memory::scan_secret("AKIAIOSFODNN7EXAMPLE").is_some());
    assert!(wb_features::memory::scan_secret("token ghp_0123456789abcdefghijklm").is_some());
    assert!(wb_features::memory::scan_secret("-----BEGIN RSA PRIVATE KEY-----").is_some());
    assert!(wb_features::memory::scan_secret("用户偏好深色主题，项目用 Rust 1.85").is_none());
    assert!(wb_features::memory::scan_secret("task-1234567890 是工单号").is_none());
}

// ---- 联网：正文提取与搜索结果解析 ----

/// HTML 正文提取要剥掉脚本、样式与导航/页脚等结构噪音，保留标题与段落。
#[test]
fn readable_extraction_strips_structure_noise() {
    let html = r#"<html><head><title>文档标题</title><style>.a{color:red}</style></head>
<body><nav>菜单导航</nav><h1>正文标题</h1><p>第一段 &amp; 说明</p>
<script>var x = "&lt;script&gt;";</script><footer>版权信息</footer></body></html>"#;
    let text = wb_features::web_parse::readable_text(html);
    assert!(text.contains("文档标题"));
    assert!(text.contains("正文标题"));
    assert!(text.contains("第一段 & 说明"));
    assert!(!text.contains("var x"));
    assert!(!text.contains("菜单导航"));
    assert!(!text.contains("版权信息"));
    assert!(!text.contains("<p>"));
}

/// 搜索结果页解析：取标题、真实链接与摘要，并解开跳转包装的 URL。
#[test]
fn search_parse_extracts_and_unwraps_results() {
    let html = r##"<div class="result results_links">
<a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fwww.rust-lang.org%2F&amp;rut=abc">The Rust Programming Language</a>
<a class="result__snippet" href="#">A language empowering everyone &amp; building reliable software.</a>
</div>
<div class="result">
<a rel="nofollow" class="result__a" href="https://docs.rs/">docs.rs</a>
<a class="result__snippet" href="#">Documentation for Rust crates</a>
</div>"##;
    let results = wb_features::web_parse::parse_results(html);
    assert_eq!(results.len(), 2);
    assert_eq!(results[0].title, "The Rust Programming Language");
    assert_eq!(
        results[0].url, "https://www.rust-lang.org/",
        "跳转包装应解开"
    );
    assert_eq!(
        results[0].snippet,
        "A language empowering everyone & building reliable software."
    );
    assert_eq!(results[1].url, "https://docs.rs/");
}

/// 内容类型决定渲染路径：JSON 直通，HTML 走提取，二进制不解析。
#[test]
fn content_type_routes_to_renderer() {
    assert_eq!(
        wb_features::web::classify("application/json; charset=utf-8", "{}"),
        "passthrough"
    );
    assert_eq!(
        wb_features::web::classify("text/html; charset=utf-8", ""),
        "html"
    );
    assert_eq!(
        wb_features::web::classify("text/plain", "hi"),
        "passthrough"
    );
    assert_eq!(
        wb_features::web::classify("", "<!DOCTYPE html><body>x</body>"),
        "html"
    );
    assert_eq!(
        wb_features::web::classify("application/octet-stream", "\x00\x01"),
        "other"
    );
}

// ---- 模糊搜索：目录跳过 ----

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
    wb_features::fff::walk_files(
        &root,
        &tokio_util::sync::CancellationToken::new(),
        |_, rel| seen.push(rel.to_string()),
    );
    assert!(seen.contains(&"src/main.rs".to_string()), "got {seen:?}");
    assert!(!seen.iter().any(|p| p.contains("node_modules")));
    assert!(!seen.iter().any(|p| p.contains("target")));
    let _ = std::fs::remove_dir_all(&root);
}

// ---- 知识库：FTS5 + RRF 双路召回 ----

/// 收录 → 检索的往返；命中片段要带来源与文档句柄。
#[test]
fn knowledge_index_then_search_returns_source() {
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
    let _ = std::fs::remove_dir_all(&dir);
}

/// 短语查询两条召回路都能命中；空查询返回空。
#[test]
fn knowledge_search_handles_phrase_and_empty_query() {
    let (dir, _ctx) = ctx_of("kb2");
    let kb = KnowledgeStore::open(&dir).unwrap();
    kb.index("s", "t", "the quick brown fox jumps", 1000)
        .unwrap();
    assert!(!kb.search("quick brown", 5).unwrap().is_empty());
    assert!(kb.search("   ", 5).unwrap().is_empty());
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- LSP：扩展名路由与文本编辑 ----

/// 扩展名决定语言服务器；未收录的扩展名没有服务器。
#[test]
fn lsp_routes_by_extension() {
    assert_eq!(
        wb_features::lsp::adapters::server_for_extension(".rs")
            .unwrap()
            .server_name,
        "rust-analyzer"
    );
    assert_eq!(
        wb_features::lsp::adapters::server_for_extension("TS")
            .unwrap()
            .server_name,
        "biome"
    );
    assert!(wb_features::lsp::adapters::server_for_extension(".unknownext").is_none());
}

/// 诊断遍历跳过产物目录，只留下真正该检查的源文件。
#[test]
fn lsp_collect_files_skips_build_dirs() {
    let root = std::env::temp_dir().join(format!("wb_lsp_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&root);
    std::fs::create_dir_all(root.join("src")).unwrap();
    std::fs::create_dir_all(root.join("target")).unwrap();
    std::fs::write(root.join("src/a.rs"), "fn a() {}").unwrap();
    std::fs::write(root.join("target/b.rs"), "fn b() {}").unwrap();
    let spec = wb_features::lsp::adapters::server_by_name("rust-analyzer").unwrap();
    let files = wb_features::lsp::collect_files(spec, &root, 10);
    assert_eq!(files.len(), 1);
    assert!(files[0].ends_with("a.rs"));
    let _ = std::fs::remove_dir_all(&root);
}

/// 文本编辑按区间替换；多行与多处编辑不能互相错位。
#[test]
fn lsp_text_edits_apply_by_range() {
    use wb_features::lsp::edits::{apply_edits, TextEdit};
    let text = "line1\nline2\nline3";
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
    assert_eq!(apply_edits(text, &edits), "L1\nline2\nL3");
}
