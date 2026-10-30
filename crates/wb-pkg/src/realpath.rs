//! realpath：短名（RUNNER~1）与符号链接归一，路径比较与越界判定的前置。

use std::path::{Component, Path, PathBuf};

use crate::error::{AppError, Result};

/// 归一绝对路径：解开 `.`/`..`、短名与符号链接；失败时回退到词法清理。
pub fn real_path(path: &Path) -> Result<PathBuf> {
    let cleaned = lexical_clean(path);
    #[cfg(windows)]
    {
        match dunce::canonicalize(&cleaned) {
            Ok(p) => Ok(p),
            // 不存在的路径按词法清理结果返回（创建前的目标路径也要能比较）
            Err(_) => Ok(cleaned),
        }
    }
    #[cfg(not(windows))]
    {
        match std::fs::canonicalize(&cleaned) {
            Ok(p) => Ok(p),
            Err(_) => Ok(cleaned),
        }
    }
}

/// 判定 `child` 是否落在 `root` 内（两侧都先归一）。
pub fn is_within(root: &Path, child: &Path) -> Result<bool> {
    let root = real_path(root)?;
    let child = real_path(child)?;
    Ok(child.starts_with(&root))
}

/// 取 `child` 相对 `root` 的路径；越界返回错误 1003。
pub fn relative_to(root: &Path, child: &Path) -> Result<PathBuf> {
    let root = real_path(root)?;
    let child = real_path(child)?;
    child
        .strip_prefix(&root)
        .map(Path::to_path_buf)
        .map_err(|_| {
            AppError::new("1003", "路径越界：目标不在当前工作区内")
                .with_details(child.display().to_string())
        })
}

/// 词法清理：解析 `.` 与 `..`，保留绝对性。
pub fn lexical_clean(path: &Path) -> PathBuf {
    let mut out = PathBuf::new();
    for comp in path.components() {
        match comp {
            Component::CurDir => {}
            Component::ParentDir => {
                out.pop();
            }
            other => out.push(other.as_os_str()),
        }
    }
    out
}
