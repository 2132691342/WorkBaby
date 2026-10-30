//! 文件编排：工作区目录树与单文件字节读取（文档预览的数据源）。

use std::path::{Path, PathBuf};
use std::sync::Arc;

use wb_domain::error::common;
use wb_domain::resource::{FileBytesRESP, FileEntryVO, FileListRESP};
use wb_pkg::{AppError, Result};

use super::Services;

/// 工作区目录清单（两级以内常用；一次一层）。
pub fn files_list(sv: &Arc<Services>, rel: &str) -> Result<FileListRESP> {
    let ws = sv.workspace.lock().unwrap().clone();
    let root = PathBuf::from(&ws);
    let target = if rel.is_empty() {
        root.clone()
    } else {
        let p = wb_pkg::realpath::relative_to(&root, Path::new(rel))?;
        root.join(p)
    };
    if !target.is_dir() {
        return Err(AppError::new(
            common::PATH_MISSING,
            format!("目录不存在：{rel}"),
        ));
    }
    let mut dirs = Vec::new();
    let mut files = Vec::new();
    for p in wb_pkg::fsutil::list_dir(&target) {
        let name = p
            .file_name()
            .map(|n| n.to_string_lossy().into_owned())
            .unwrap_or_default();
        if name.starts_with('.') {
            continue;
        }
        let meta = std::fs::metadata(&p).ok();
        let rel_path = if rel.is_empty() {
            name.clone()
        } else {
            format!("{}/{}", rel.trim_end_matches('/'), name)
        };
        let entry = FileEntryVO {
            entry_name: name,
            entry_path: rel_path.replace('\\', "/"),
            entry_is_dir: p.is_dir(),
            entry_size: meta.as_ref().map(|m| m.len() as i64).unwrap_or(0),
            entry_mtime: meta
                .and_then(|m| m.modified().ok())
                .map(|t| {
                    t.duration_since(std::time::UNIX_EPOCH)
                        .unwrap_or_default()
                        .as_millis() as i64
                })
                .unwrap_or(0),
        };
        if entry.entry_is_dir {
            dirs.push(entry);
        } else {
            files.push(entry);
        }
    }
    dirs.sort_by(|a, b| a.entry_name.cmp(&b.entry_name));
    files.sort_by(|a, b| a.entry_name.cmp(&b.entry_name));
    dirs.extend(files);
    Ok(FileListRESP {
        resp_path: rel.trim_end_matches('/').to_string(),
        resp_entries: dirs,
    })
}

// ---- 文档 / 命令 ----

/// 预览单文件大小上限（字节）：PDF 与表格超过这个值就不再回吐。
const PREVIEW_MAX_BYTES: u64 = 32 * 1024 * 1024;

/// 读工作区内文件的字节（文档预览用；路径越界直接拒）。
pub fn file_bytes(sv: &Arc<Services>, rel: &str) -> Result<FileBytesRESP> {
    let ws = sv.workspace.lock().unwrap().clone();
    let root = PathBuf::from(&ws);
    let p = wb_pkg::realpath::relative_to(&root, Path::new(rel))?;
    let target = root.join(p);
    if !target.is_file() {
        return Err(AppError::new(
            common::PATH_MISSING,
            format!("文件不存在：{rel}"),
        ));
    }
    let meta = std::fs::metadata(&target)
        .map_err(|e| AppError::wrap(common::IO_FAILED, "读文件元信息失败", e))?;
    if meta.len() > PREVIEW_MAX_BYTES {
        return Err(AppError::new(
            common::IO_FAILED,
            format!("文件太大（{} MB），不提供预览", meta.len() / 1024 / 1024),
        ));
    }
    let bytes =
        std::fs::read(&target).map_err(|e| AppError::wrap(common::IO_FAILED, "读文件失败", e))?;
    let name = target
        .file_name()
        .map(|n| n.to_string_lossy().into_owned())
        .unwrap_or_default();
    Ok(FileBytesRESP {
        resp_file_name: name.clone(),
        resp_mime: mime_of(&name),
        resp_content_base64: base64_encode(&bytes),
        resp_size: bytes.len() as i64,
    })
}

/// 按扩展名给 MIME（前端也按扩展名选渲染器，两边保持同一套判断）。
fn mime_of(name: &str) -> String {
    let lower = name.to_ascii_lowercase();
    let ext = lower.rsplit('.').next().unwrap_or("");
    match ext {
        "pdf" => "application/pdf",
        "docx" => "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        "doc" => "application/msword",
        "xlsx" => "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        "xls" => "application/vnd.ms-excel",
        "csv" => "text/csv",
        "md" => "text/markdown",
        "json" => "application/json",
        "txt" => "text/plain",
        "html" | "htm" => "text/html",
        "png" => "image/png",
        "jpg" | "jpeg" => "image/jpeg",
        "webp" => "image/webp",
        "gif" => "image/gif",
        "svg" => "image/svg+xml",
        _ => "application/octet-stream",
    }
    .to_string()
}

/// 标准 base64 编码（预览数据走 JSON，必须能安全嵌入）。
fn base64_encode(data: &[u8]) -> String {
    const TABLE: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::with_capacity(data.len().div_ceil(3) * 4);
    for chunk in data.chunks(3) {
        let b0 = chunk[0] as u32;
        let b1 = *chunk.get(1).unwrap_or(&0) as u32;
        let b2 = *chunk.get(2).unwrap_or(&0) as u32;
        let n = (b0 << 16) | (b1 << 8) | b2;
        out.push(TABLE[((n >> 18) & 63) as usize] as char);
        out.push(TABLE[((n >> 12) & 63) as usize] as char);
        out.push(if chunk.len() > 1 {
            TABLE[((n >> 6) & 63) as usize] as char
        } else {
            '='
        });
        out.push(if chunk.len() > 2 {
            TABLE[(n & 63) as usize] as char
        } else {
            '='
        });
    }
    out
}
