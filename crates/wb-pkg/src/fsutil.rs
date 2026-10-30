//! 文件系统助手：原子写、目录确保、体积与遍历辅助。

use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};

use crate::error::{AppError, Result};

/// 原子写：先写同目录临时文件再 rename，避免半截文件。
pub fn write_atomic(path: &Path, data: &[u8]) -> Result<()> {
    if let Some(parent) = path.parent() {
        ensure_dir(parent)?;
    }
    let tmp = path.with_extension("tmp-wb");
    {
        let mut f = fs::File::create(&tmp).map_err(|e| {
            AppError::wrap("1002", "写文件失败", e).with_details(path.display().to_string())
        })?;
        f.write_all(data)
            .map_err(|e| AppError::wrap("1002", "写文件失败", e))?;
        f.sync_all().ok();
    }
    fs::rename(&tmp, path).map_err(|e| {
        AppError::wrap("1002", "替换文件失败", e).with_details(path.display().to_string())
    })?;
    Ok(())
}

/// 递归建目录；已存在视为成功。
pub fn ensure_dir(path: &Path) -> Result<()> {
    fs::create_dir_all(path).map_err(|e| {
        AppError::wrap("1002", "创建目录失败", e).with_details(path.display().to_string())
    })
}

/// 读文本文件，带路径定位的报错。
pub fn read_to_string(path: &Path) -> Result<String> {
    fs::read_to_string(path).map_err(|e| {
        AppError::wrap("1002", "读文件失败", e).with_details(path.display().to_string())
    })
}

/// 文件修改时间（毫秒）；不存在返回 None。
pub fn mtime_ms(path: &Path) -> Option<i64> {
    let meta = fs::metadata(path).ok()?;
    let t = meta.modified().ok()?;
    Some(t.duration_since(std::time::UNIX_EPOCH).ok()?.as_millis() as i64)
}

/// 列出目录下的一层条目（不保证排序）；目录不存在返回空。
pub fn list_dir(path: &Path) -> Vec<PathBuf> {
    let Ok(rd) = fs::read_dir(path) else {
        return Vec::new();
    };
    rd.filter_map(|e| e.ok()).map(|e| e.path()).collect()
}
