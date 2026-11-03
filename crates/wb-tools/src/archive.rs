//! 输出归档：超长工具输出落盘并在上下文里留一个可再读的句柄。
//!
//! 直接截断是最省事的做法，代价是信息**永久丢失**——模型想知道后面还有什么
//! 就只能重跑一遍命令。这里改成：截断过的部分落盘，回一段带路径的引用，
//! 模型需要时用 `read` 再取。

use std::path::{Path, PathBuf};

use crate::truncate;

/// 归档目录：`{数据目录}\tool-output`。
pub fn archive_dir() -> Option<PathBuf> {
    wb_domain::runtime::default_data_dir().map(|d| d.join("tool-output"))
}

/// 把全文落盘并返回绝对路径；写失败返回 None（调用方退回纯截断）。
///
/// 文件名带毫秒时间戳与工具名：同一轮里两次超限调用不会互相覆盖，
/// 用户按文件名就能认出是哪次调用留下的。
pub fn archive(tool_name: &str, text: &str) -> Option<PathBuf> {
    let dir = archive_dir()?;
    wb_pkg::fsutil::ensure_dir(&dir).ok()?;
    let ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .ok()?
        .as_millis();
    let safe: String = tool_name
        .chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() || c == '_' || c == '-' {
                c
            } else {
                '_'
            }
        })
        .collect();
    let path = dir.join(format!("{ms}_{safe}.txt"));
    wb_pkg::fsutil::write_atomic(&path, text.as_bytes()).ok()?;
    Some(path)
}

/// 在截断提示行里补上归档位置（没有归档就只留原来的截断说明）。
pub fn with_archive_hint(truncated: String, archived: Option<&Path>) -> String {
    match archived {
        Some(p) => format!(
            "{truncated}\n[完整输出已归档：{}（共后续内容都在这里，需要时用 read 工具取）]",
            p.display()
        ),
        None => truncated,
    }
}

/// 是否会被 truncate 砍掉。必须与 truncate 的判据一致：
/// 只看字节会漏掉「行数超限但字节不多」的情况（4000 行短日志才 40KB），
/// 那份内容会被静默丢掉且无人归档。
pub fn worth_archiving(text: &str) -> bool {
    text.len() > truncate::MAX_BYTES || text.lines().count() > truncate::MAX_LINES
}
