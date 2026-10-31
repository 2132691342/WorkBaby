//! 内置 MinGit 运行时的路径契约：解压目标（wb-service）与消费探测（wb-tools / wb-git）共用。

use std::path::{Path, PathBuf};

/// 数据目录名：wb-api 的 data_dir 与运行时探测共用，避免两处硬编码漂移。
pub const APP_DIR: &str = "WorkBaby";

/// MinGit 运行时根：`{data_dir}/runtime/MinGit`。
pub fn mingit_root(data_dir: &Path) -> PathBuf {
    data_dir.join("runtime").join("MinGit")
}

/// bash 候选（顺序即优先级）：MinGit 变体只带 sh.exe，它与 bash.exe 是同一二进制。
pub fn mingit_bash(root: &Path) -> [PathBuf; 2] {
    [
        root.join("usr").join("bin").join("bash.exe"),
        root.join("usr").join("bin").join("sh.exe"),
    ]
}

/// git 候选：ucrt64/bin 的原生 git 优先，cmd 包装脚本兜底。
pub fn mingit_git(root: &Path) -> [PathBuf; 2] {
    [
        root.join("ucrt64").join("bin").join("git.exe"),
        root.join("cmd").join("git.exe"),
    ]
}

/// 默认数据目录 `%APPDATA%\WorkBaby`；APPDATA 未设置时为 None。
pub fn default_data_dir() -> Option<PathBuf> {
    std::env::var_os("APPDATA").map(|base| PathBuf::from(base).join(APP_DIR))
}
