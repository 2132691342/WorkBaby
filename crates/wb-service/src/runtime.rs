//! 内置运行时：PortableGit 与 Python 的 zip 编译期嵌入，启动时按指纹解压到数据目录。

use std::io::{Cursor, Read as _};
use std::path::Path;

use wb_pkg::{fsutil, AppError, Result};

/// PortableGit 归档（bash / git / GNU 工具，随安装包分发）。
pub static PORTABLEGIT_ZIP: &[u8] = include_bytes!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/../../assets/runtime/PortableGit.zip"
));

/// Python 3.13 归档（agent 跑数据脚本与文档转换的解释器）。
pub static PYTHON_ZIP: &[u8] = include_bytes!(concat!(
    env!("CARGO_MANIFEST_DIR"),
    "/../../assets/runtime/Python313.zip"
));

/// 全部内置运行时就位；单件失败只告警不挡启动。
pub fn ensure_all(data_dir: &Path) {
    ensure_archive(
        &wb_domain::runtime::portablegit_root(data_dir),
        PORTABLEGIT_ZIP,
        "PortableGit",
        "PortableGit",
    );
    ensure_archive(
        &wb_domain::runtime::python_root(data_dir),
        PYTHON_ZIP,
        "python",
        "Python",
    );
}

/// 单个运行时解压：指纹（zip 字节数）不符即清空重解压；归档内顶层目录为 `entry_root`。
fn ensure_archive(target: &Path, zip_bytes: &'static [u8], entry_root: &str, label: &str) {
    let stamp = zip_bytes.len().to_string();
    let marker = target.join(".pack_stamp");
    if target.is_dir() && fsutil::read_to_string(&marker).is_ok_and(|s| s == stamp) {
        return;
    }
    // 指纹不符先清空：旧版本残留与半成品目录一并清除
    let _ = std::fs::remove_dir_all(target);
    if let Err(e) = extract(target, zip_bytes, entry_root) {
        tracing::warn!("内置 {label} 解压失败（对应工具退回系统安装）：{e}");
        return;
    }
    if let Err(e) = fsutil::write_atomic(&marker, stamp.as_bytes()) {
        tracing::warn!("{label} 指纹写入失败：{e}");
    }
}

/// 全量解压到 target；符号链接跳过（Windows 无需，且防路径逃逸）。
fn extract(target: &Path, zip_bytes: &'static [u8], entry_root: &str) -> Result<()> {
    fsutil::ensure_dir(target)?;
    let mut archive = zip::ZipArchive::new(Cursor::new(zip_bytes)).map_err(|e| {
        AppError::new(
            wb_domain::error::common::ARCHIVE_INVALID,
            format!("{entry_root} zip 解析失败：{e}"),
        )
    })?;
    for i in 0..archive.len() {
        let mut entry = archive.by_index(i).map_err(|e| {
            AppError::new(
                wb_domain::error::common::ARCHIVE_INVALID,
                format!("{entry_root} zip 条目读取失败：{e}"),
            )
        })?;
        let Some(rel) = entry.enclosed_name() else {
            continue;
        };
        // 归档顶层目录名已被 target 取代，剥掉一层才能让根路径契约（runtime/PortableGit 等）成立
        let Some(rel) = rel.strip_prefix(entry_root).ok() else {
            continue;
        };
        if rel.as_os_str().is_empty() {
            continue;
        }
        let dest = target.join(rel);
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
                format!("{entry_root} 条目读出失败"),
                e,
            )
        })?;
        fsutil::write_atomic(&dest, &bytes)?;
    }
    Ok(())
}
