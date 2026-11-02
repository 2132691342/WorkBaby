//! 首次启动的默认工作目录：不能拿进程 cwd 当默认值。
//! 从开始菜单 / 任务栏启动时 cwd 往往落在 System32 或安装目录，
//! 助手会在那里读写文件——这是「首次使用就不对劲」的典型成因。

use std::path::PathBuf;

/// 建议的默认工作区：用户主目录下的文档目录，退而求其次用主目录。
pub fn suggested_workspace() -> PathBuf {
    let home = std::env::var_os("USERPROFILE")
        .map(PathBuf::from)
        .filter(|p| p.is_dir());
    let Some(home) = home else {
        return std::env::current_dir().unwrap_or_else(|_| PathBuf::from("."));
    };
    let docs = home.join("Documents");
    if docs.is_dir() {
        return docs;
    }
    home
}

/// 该路径是否适合当工作区：系统目录与盘根都拒，避免助手在系统位置乱写。
pub fn is_sane_workspace(dir: &str) -> bool {
    let trimmed = dir.trim();
    if trimmed.is_empty() {
        return false;
    }
    let path = std::path::Path::new(trimmed);
    if !path.is_dir() {
        return false;
    }
    let lowered = trimmed.replace('/', "\\").to_lowercase();
    const BAD: [&str; 4] = [
        "\\windows\\system32",
        "\\windows\\syswow64",
        "\\windows",
        "\\program files",
    ];
    if BAD.iter().any(|b| lowered.contains(b)) {
        return false;
    }
    // 盘根（C:\ 这种）不是工作区：选它等于把整个盘暴露给助手
    path.parent().is_some()
}
