//! wb-tools 场景测试：写文件类工具必须回报改动文件与差异行（界面就地着色的唯一来源）。

use std::path::{Path, PathBuf};

use serde_json::json;
use tokio_util::sync::CancellationToken;

use wb_tools::Tool;

/// 造一个临时工作区目录。
fn workdir(tag: &str) -> PathBuf {
    let dir = std::env::temp_dir().join(format!("wb_tools_{tag}_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    dir
}

// ---- 差异行：裁剪公共前后缀，增删行分类正确 ----

#[test]
fn line_diff_marks_added_and_removed_lines() {
    let before = "a\nb\nc\nd\ne\n";
    let after = "a\nb\nX\nd\ne\n";
    let rows = wb_tools::difftext::line_diff(before, after);
    let adds: Vec<&str> = rows
        .iter()
        .filter(|r| r.line_kind == "add")
        .map(|r| r.line_text.as_str())
        .collect();
    let dels: Vec<&str> = rows
        .iter()
        .filter(|r| r.line_kind == "del")
        .map(|r| r.line_text.as_str())
        .collect();
    assert_eq!(adds, vec!["X"], "新增行只有 X");
    assert_eq!(dels, vec!["c"], "删除行只有 c");
    let ctx: Vec<&str> = rows
        .iter()
        .filter(|r| r.line_kind == "ctx")
        .map(|r| r.line_text.as_str())
        .collect();
    assert!(ctx.contains(&"b") && ctx.contains(&"d"), "保留上下文行");
}

// ---- write：整文件覆盖也要带出改动（否则用户看不见删掉了什么） ----

#[tokio::test]
async fn write_tool_reports_file_and_diff() {
    let dir = workdir("write");
    let file = dir.join("n.txt");
    std::fs::write(&file, "one\ntwo\n").unwrap();

    let out = wb_tools::builtin::write::WriteTool
        .execute(
            json!({"path": "n.txt", "content": "one\nTWO\n"}),
            CancellationToken::new(),
            &dir,
        )
        .await
        .unwrap();

    assert_eq!(
        out.file_path.as_deref(),
        Some("n.txt"),
        "改动文件用工作区相对路径"
    );
    let diff = out.diff.expect("改动必须带差异行");
    let dels: Vec<&str> = diff
        .iter()
        .filter(|d| d.line_kind == "del")
        .map(|d| d.line_text.as_str())
        .collect();
    assert_eq!(dels, vec!["two"], "被替换的旧行应标为删除");
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- edit：多处替换的差异行成对出现 ----

#[tokio::test]
async fn edit_tool_reports_paired_diff_lines() {
    let dir = workdir("edit");
    std::fs::write(dir.join("e.rs"), "fn a() {}\nfn b() {}\n").unwrap();
    let out = wb_tools::builtin::edit::EditTool
        .execute(
            json!({"path": "e.rs", "edits": [{"old_text": "fn b() {}", "new_text": "fn c() {}"}]}),
            CancellationToken::new(),
            &dir,
        )
        .await
        .unwrap();
    let diff = out.diff.expect("edit 必须带差异行");
    assert!(
        diff.iter()
            .any(|d| d.line_kind == "del" && d.line_text == "fn b() {}")
            && diff
                .iter()
                .any(|d| d.line_kind == "add" && d.line_text == "fn c() {}"),
        "删除行与新增行都要在：{diff:?}"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- read：只读工具不产生改动 ----

#[tokio::test]
async fn read_tool_reports_no_changes() {
    let dir = workdir("read");
    std::fs::write(dir.join("r.txt"), "hello\n").unwrap();
    let out = wb_tools::builtin::read::ReadTool
        .execute(
            json!({"path": "r.txt"}),
            CancellationToken::new(),
            Path::new(&dir),
        )
        .await
        .unwrap();
    assert!(
        out.file_path.is_none() && out.diff.is_none(),
        "读文件不该报改动"
    );
    let _ = std::fs::remove_dir_all(&dir);
}
