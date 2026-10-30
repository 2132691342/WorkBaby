//! wb-features 场景测试：记忆、待办、提问板、知识库。
//! 全是「改一处就可能坏一片」的场景：状态机、依赖图、持久化往返、边界拒绝。

use std::path::PathBuf;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use serde_json::json;
use tokio_util::sync::CancellationToken;
use wb_features::ask_user::AnswerBoard;
use wb_features::context_mode::store::KnowledgeStore;
use wb_features::todo::TodoStore;
use wb_features::FeatureCtx;
use wb_tools::Tool;

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

/// 造一个待办工具（依赖校验与悬空清理都在工具层生效）。
fn todo_tool(ctx: &FeatureCtx) -> wb_features::todo::TodoTool {
    wb_features::todo::TodoTool::new(Arc::new(TodoStore::new(ctx)), ctx.clone())
}

/// 跑一次待办动作。
async fn todo_op(
    tool: &wb_features::todo::TodoTool,
    args: serde_json::Value,
) -> wb_pkg::Result<String> {
    let out = tool
        .execute(args, CancellationToken::new(), std::path::Path::new("."))
        .await?;
    Ok(out.text)
}

/// 依赖图的三个拒绝条件：悬空 id、自环、成环。
/// 依赖图一旦成环，「被依赖项没完成前不能开始」的语义就失效了。
#[tokio::test]
async fn todo_dependency_graph_rejects_cycles_and_dangling_refs() {
    let (dir, ctx) = ctx_of("todo_graph");
    let tool = todo_tool(&ctx);

    todo_op(&tool, json!({"op":"add","text":"第一步"}))
        .await
        .unwrap();
    todo_op(
        &tool,
        json!({"op":"add","text":"第二步","blocked_by":["TODO_1"]}),
    )
    .await
    .unwrap();
    todo_op(
        &tool,
        json!({"op":"add","text":"第三步","blocked_by":["TODO_2"]}),
    )
    .await
    .unwrap();

    let store = TodoStore::new(&ctx);
    assert_eq!(store.load().len(), 3, "三条待办都应落盘");
    assert_eq!(store.load()[1].todo_blocked_by, vec!["TODO_1"]);

    // 自环
    let self_loop = todo_op(
        &tool,
        json!({"op":"add","text":"自环","blocked_by":["TODO_4"]}),
    )
    .await;
    assert!(self_loop.is_err(), "依赖自己必须被拒");

    // 悬空引用
    let dangling = todo_op(
        &tool,
        json!({"op":"add","text":"悬空","blocked_by":["TODO_999"]}),
    )
    .await;
    assert!(dangling.is_err(), "依赖不存在的条目必须被拒");

    // 成环：TODO_1 依赖 TODO_3，而 TODO_3 已依赖 TODO_2 → TODO_1
    let cycle = todo_op(
        &tool,
        json!({"op":"update","todo_id":"TODO_1","blocked_by":["TODO_3"]}),
    )
    .await;
    assert!(cycle.is_err(), "成环必须被拒");

    let _ = std::fs::remove_dir_all(&dir);
}

/// 删掉被依赖项后，依赖方的 blocked_by 不留悬空引用。
#[tokio::test]
async fn removing_todo_clears_dangling_dependencies() {
    let (dir, ctx) = ctx_of("todo_remove");
    let tool = todo_tool(&ctx);

    todo_op(&tool, json!({"op":"add","text":"第一步"}))
        .await
        .unwrap();
    todo_op(
        &tool,
        json!({"op":"add","text":"第二步","blocked_by":["TODO_1"]}),
    )
    .await
    .unwrap();

    todo_op(&tool, json!({"op":"remove","todo_id":"TODO_1"}))
        .await
        .unwrap();

    let store = TodoStore::new(&ctx);
    assert_eq!(store.load().len(), 1, "只该剩一条");
    assert!(
        store.load()[0].todo_blocked_by.is_empty(),
        "删掉 TODO_1 后 TODO_2 不应留悬空依赖"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 提问回答板 ----

/// register → answer 回填；重复回答与无人等待者都被拒。
/// 一次性投递是提问卡的语义基础——重复投递会让界面收到两套答案。
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
/// 命中错行会让检索结果混进整条 JSON，模型看到的是噪声。
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
/// 记忆库明文落盘，密钥写进去等于泄露——这是硬门槛。
#[test]
fn secret_scan_separates_credentials_from_prose() {
    assert!(wb_features::memory::scan_secret("key is sk-abc123XYZ4567890def").is_some());
    assert!(wb_features::memory::scan_secret("AKIAIOSFODNN7EXAMPLE").is_some());
    assert!(wb_features::memory::scan_secret("token ghp_0123456789abcdefghijklm").is_some());
    assert!(wb_features::memory::scan_secret("-----BEGIN RSA PRIVATE KEY-----").is_some());
    assert!(wb_features::memory::scan_secret("用户偏好深色主题，项目用 Rust 1.85").is_none());
    assert!(wb_features::memory::scan_secret("task-1234567890 是工单号").is_none());
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
