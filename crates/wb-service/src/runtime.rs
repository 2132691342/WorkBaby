//! 内置 MinGit 运行时：zip 编译期嵌入，启动时按指纹解压到数据目录（bash / git 工具的兜底运行时）。

use std::io::{Cursor, Read as _};
use std::path::Path;

use wb_pkg::{fsutil, AppError, Result};

/// MinGit 便携包（assets/runtime 下随二进制分发，安装包因此自带运行时）。
pub static MINGIT_ZIP: &[u8] = include_bytes!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/../../assets/runtime/MinGit-2.56.0.3-64-bit.zip"
));

/// 确保运行时就位：指纹（zip 字节数）不符即清空重解压；失败只告警不挡启动。
pub fn ensure_mingit(data_dir: &Path) {
    let root = wb_domain::runtime::mingit_root(data_dir);
    let stamp = MINGIT_ZIP.len().to_string();
    let marker = root.join(".pack_stamp");
    if root.is_dir() && fsutil::read_to_string(&marker).is_ok_and(|s| s == stamp) {
        return;
    }
    // 指纹不符先清空：旧版本残留与半成品目录一并清除
    let _ = std::fs::remove_dir_all(&root);
    if let Err(e) = extract(&root) {
        tracing::warn!("内置 MinGit 解压失败（bash/git 退回系统安装）：{e}");
        return;
    }
    if let Err(e) = fsutil::write_atomic(&marker, stamp.as_bytes()) {
        tracing::warn!("MinGit 指纹写入失败：{e}");
    }
}

/// 全量解压到 root；符号链接跳过（Windows 无需，且防路径逃逸）。
fn extract(root: &Path) -> Result<()> {
    fsutil::ensure_dir(root)?;
    let mut archive = zip::ZipArchive::new(Cursor::new(MINGIT_ZIP)).map_err(|e| {
        AppError::new(
            wb_domain::error::common::ARCHIVE_INVALID,
            format!("MinGit zip 解析失败：{e}"),
        )
    })?;
    for i in 0..archive.len() {
        let mut entry = archive.by_index(i).map_err(|e| {
            AppError::new(
                wb_domain::error::common::ARCHIVE_INVALID,
                format!("MinGit zip 条目读取失败：{e}"),
            )
        })?;
        let Some(rel) = entry.enclosed_name() else {
            continue;
        };
        let dest = root.join(rel);
        if entry.is_dir() {
            fsutil::ensure_dir(&dest)?;
            continue;
        }
        if entry.is_symlink() {
            continue;
        }
        let Some(parent) = dest.parent() else {
            continue;
        };
        fsutil::ensure_dir(parent)?;
        let mut bytes = Vec::with_capacity(entry.size() as usize);
        entry.read_to_end(&mut bytes).map_err(|e| {
            AppError::wrap(
                wb_domain::error::common::IO_FAILED,
                "MinGit 条目读出失败",
                e,
            )
        })?;
        fsutil::write_atomic(&dest, &bytes)?;
    }
    Ok(())
}
