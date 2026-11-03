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
            &wb_tools::ToolCtx::default(),
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
            &wb_tools::ToolCtx::default(),
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
            &wb_tools::ToolCtx::default(),
        )
        .await
        .unwrap();
    assert!(
        out.file_path.is_none() && out.diff.is_none(),
        "读文件不该报改动"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

/// 超长输出必须先落盘再截断：直接丢掉的字模型只能重跑命令才能拿回来。
/// 行数超限而字节不多的情形（4000 行短日志）同样要归档。
#[tokio::test]
async fn long_output_is_archived_and_handed_back_as_a_handle() {
    let dir = workdir("archive");
    let script = "for i in $(seq 1 4000); do echo LINE-$i; done";
    let out = wb_tools::builtin::bash::BashTool
        .execute(
            json!({ "command": script, "timeout": 60 }),
            CancellationToken::new(),
            &dir,
            &wb_tools::ToolCtx::default(),
        )
        .await
        .unwrap();
    assert!(!out.is_error, "命令本身应成功：got {}", out.text);
    assert!(
        out.text.contains("输出已截断"),
        "输出应被截断：got {} chars",
        out.text.len()
    );
    assert!(
        out.text.contains("完整输出已归档"),
        "截断时必须给出归档句柄，否则被砍掉的内容就永久丢了：got {}",
        &out.text[out.text.len().saturating_sub(400)..]
    );
    // 句柄指的文件要真的存在且是全文（不是截断后的那一份）
    let line = out
        .text
        .lines()
        .find(|l| l.starts_with("[完整输出已归档："))
        .expect("应有归档行");
    let path = line
        .trim_start_matches("[完整输出已归档：")
        .split('（')
        .next()
        .unwrap();
    let full = std::fs::read_to_string(path).expect("归档文件应可读");
    assert!(full.contains("LINE-4000"), "归档必须是全文，得包含最后一行");
    let _ = std::fs::remove_file(path);
    let _ = std::fs::remove_dir_all(&dir);
}

/// 读图必须把字节作为图片输出：否则视觉模型永远看不到用户截图，
/// 只会收到一句「二进制文件 N 字节」。
#[tokio::test]
async fn read_returns_image_parts_for_visual_models() {
    let dir = workdir("img");
    // 1x1 PNG：内容不重要，重要的是它被当成图片而不是二进制传给模型
    let png: [u8; 67] = [
        0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44,
        0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x06, 0x00, 0x00, 0x00, 0x1F,
        0x15, 0xC4, 0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00,
        0x01, 0x00, 0x00, 0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
        0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
    ];
    std::fs::write(dir.join("shot.png"), png).unwrap();
    let out = wb_tools::builtin::read::ReadTool
        .execute(
            json!({"path": "shot.png"}),
            CancellationToken::new(),
            &dir,
            &wb_tools::ToolCtx::default(),
        )
        .await
        .unwrap();
    assert_eq!(out.images.len(), 1, "读图应产出一张图片段");
    assert_eq!(out.images[0].mime, "image/png");
    assert!(
        out.images[0].base64.starts_with("iVBORw0KGgo"),
        "base64 应是 PNG 头：got {}",
        &out.images[0].base64[..16.min(out.images[0].base64.len())]
    );

    // 非图片的二进制仍旧只报字节数，不假装成图片交给模型
    std::fs::write(dir.join("blob.bin"), [0u8, 1, 2, 3]).unwrap();
    let out = wb_tools::builtin::read::ReadTool
        .execute(
            json!({"path": "blob.bin"}),
            CancellationToken::new(),
            &dir,
            &wb_tools::ToolCtx::default(),
        )
        .await
        .unwrap();
    assert!(out.images.is_empty(), "二进制文件不该被当成图片");
    assert!(out.text.contains("二进制文件"), "got {}", out.text);
    let _ = std::fs::remove_dir_all(&dir);
}
