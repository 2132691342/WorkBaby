//! wb-tools 场景测试：写文件类工具必须回报改动文件与差异行——那是界面就地着色的唯一来源。

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

fn changes(out: &wb_tools::ToolOutput) -> Vec<(String, String)> {
    out.diff
        .as_ref()
        .expect("改动必须带差异行")
        .iter()
        .filter(|d| d.line_kind != "ctx")
        .map(|d| (d.line_kind.clone(), d.line_text.clone()))
        .collect()
}

#[test]
fn line_diff_marks_added_and_removed_lines() {
    let rows = wb_tools::difftext::line_diff("a\nb\nc\nd\ne\n", "a\nb\nX\nd\ne\n");
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
    assert!(
        rows.iter()
            .filter(|r| r.line_kind == "ctx")
            .any(|r| r.line_text == "b"),
        "公共前后缀应保留为上下文行：{rows:?}"
    );
}

/// 写入与编辑都必须成对给出删除行与新增行；只读工具不报改动。
#[tokio::test]
async fn write_and_edit_report_paired_diff_lines_while_read_reports_none() {
    let dir = workdir("write");
    std::fs::write(dir.join("n.txt"), "one\ntwo\n").unwrap();

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
    assert_eq!(
        changes(&out),
        vec![
            ("del".to_string(), "two".to_string()),
            ("add".to_string(), "TWO".to_string())
        ],
        "整文件覆盖也要成对给出改动行"
    );

    std::fs::write(dir.join("e.rs"), "fn a() {}\nfn b() {}\n").unwrap();
    let out = wb_tools::builtin::edit::EditTool
        .execute(
            json!({"path": "e.rs", "edits": [{"old_text": "fn b() {}", "new_text": "fn c() {}"}]}),
            CancellationToken::new(),
            &dir,
        )
        .await
        .unwrap();
    assert_eq!(
        changes(&out),
        vec![
            ("del".to_string(), "fn b() {}".to_string()),
            ("add".to_string(), "fn c() {}".to_string())
        ],
        "edit 的删除行与新增行都要在"
    );

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
