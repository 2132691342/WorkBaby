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

/// 全部内置运行时就位；单件失败只告警不挡启动，失败原因汇总返回给调用方。
pub fn ensure_all(data_dir: &Path) -> Vec<String> {
    let mut failures = Vec::new();
    if let Err(e) = ensure_archive(
        &wb_domain::runtime::portablegit_root(data_dir),
        PORTABLEGIT_ZIP,
        "PortableGit",
        "PortableGit",
    ) {
        failures.push(format!("PortableGit：{e}"));
    }
    if let Err(e) = ensure_archive(
        &wb_domain::runtime::python_root(data_dir),
        PYTHON_ZIP,
        "python",
        "Python",
    ) {
        failures.push(format!("Python：{e}"));
    }
    failures
}

/// 单个运行时解压：指纹（zip 字节数）不符即清空重解压；归档内顶层目录为 `entry_root`。
fn ensure_archive(
    target: &Path,
    zip_bytes: &'static [u8],
    entry_root: &str,
    label: &str,
) -> Result<()> {
    let stamp = zip_bytes.len().to_string();
    let marker = target.join(".pack_stamp");
    if target.is_dir() && fsutil::read_to_string(&marker).is_ok_and(|s| s == stamp) {
        return Ok(());
    }
    // 指纹不符先清空：旧版本残留与半成品目录一并清除
    let _ = std::fs::remove_dir_all(target);
    extract(target, zip_bytes, entry_root)?;
    if let Err(e) = fsutil::write_atomic(&marker, stamp.as_bytes()) {
        tracing::warn!("{label} 指纹写入失败：{e}");
    }
    Ok(())
}

/// 当前运行时状态：以磁盘上的可执行文件为准，不依赖启动时的解压结果。
pub fn status(
    data_dir: &Path,
    failures: &[String],
) -> wb_domain::runtime_status::RuntimeStatusRESP {
    use wb_domain::runtime_status::{RuntimeBinaryVO, RuntimeStatusRESP};
    let probe = [
        ("bash", wb_domain::runtime::find_bundled_bash(data_dir)),
        ("git", wb_domain::runtime::find_bundled_git(data_dir)),
        ("python", wb_domain::runtime::find_bundled_python(data_dir)),
    ];
    let binaries: Vec<RuntimeBinaryVO> = probe
        .into_iter()
        .map(|(binary_name, hit)| RuntimeBinaryVO {
            binary_name: binary_name.to_string(),
            binary_ready: hit.is_some(),
            binary_path: hit.map(|p| p.display().to_string()).unwrap_or_default(),
        })
        .collect();
    RuntimeStatusRESP {
        resp_root: wb_domain::runtime::runtime_root(data_dir)
            .display()
            .to_string(),
        resp_ready: binaries.iter().all(|b| b.binary_ready),
        resp_binaries: binaries,
        resp_failure: failures.join("; "),
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
