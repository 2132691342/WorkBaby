//! 内置运行时的路径契约与子进程 PATH：解压目标（wb-service）与消费方（wb-tools / wb-git）共用。

use std::path::{Path, PathBuf};

/// 数据目录名：wb-api 的 data_dir 与运行时探测共用，避免两处硬编码漂移。
pub const APP_DIR: &str = "WorkBaby";

/// 默认数据目录 `%APPDATA%\WorkBaby`；APPDATA 未设置时为 None。
pub fn default_data_dir() -> Option<PathBuf> {
    std::env::var_os("APPDATA").map(|base| PathBuf::from(base).join(APP_DIR))
}

/// 运行时根：`{data_dir}/runtime`，所有内置运行时解压到它下面。
pub fn runtime_root(data_dir: &Path) -> PathBuf {
    data_dir.join("runtime")
}

/// PortableGit 根：`{data_dir}/runtime/PortableGit`。
pub fn portablegit_root(data_dir: &Path) -> PathBuf {
    runtime_root(data_dir).join("PortableGit")
}

/// Python 根：`{data_dir}/runtime/python`。
pub fn python_root(data_dir: &Path) -> PathBuf {
    runtime_root(data_dir).join("python")
}

/// bash 候选：PortableGit 只带 `bin/bash.exe`，`usr/bin/sh.exe` 是兜底同源入口。
pub fn bash_candidates(root: &Path) -> [PathBuf; 2] {
    [
        root.join("bin").join("bash.exe"),
        root.join("usr").join("bin").join("sh.exe"),
    ]
}

/// git 候选：`cmd/git.exe` 是 PATH 安全入口，`mingw64/bin/git.exe` 是原生二进制兜底。
pub fn git_candidates(root: &Path) -> [PathBuf; 2] {
    [
        root.join("cmd").join("git.exe"),
        root.join("mingw64").join("bin").join("git.exe"),
    ]
}

/// Python 候选：主程序 + 窗口版（脚本里需要 GUI 时用后者）。
pub fn python_candidates(root: &Path) -> [PathBuf; 2] {
    [root.join("python.exe"), root.join("pythonw.exe")]
}

/// 内置运行时序目录，顺序即 PATH 优先级：git → bash → GNU 工具 → python。
pub fn bundled_path_dirs(data_dir: &Path) -> Vec<PathBuf> {
    let git = portablegit_root(data_dir);
    let py = python_root(data_dir);
    vec![
        git.join("cmd"),
        git.join("bin"),
        git.join("mingw64").join("bin"),
        git.join("usr").join("bin"),
        py.clone(),
        py.join("Scripts"),
    ]
}

/// 构造子进程 PATH：内置运行时在前、现有 PATH 在后，用户环境既不被破坏也仍可访问。
pub fn child_path(data_dir: &Path) -> String {
    let mut parts: Vec<String> = bundled_path_dirs(data_dir)
        .iter()
        .map(|p| p.display().to_string())
        .collect();
    if let Some(existing) = std::env::var_os("PATH") {
        parts.push(existing.to_string_lossy().into_owned());
    }
    parts.join(";")
}

/// 在候选表里挑第一个存在的可执行文件。
pub fn first_existing(candidates: &[PathBuf]) -> Option<PathBuf> {
    candidates.iter().find(|p| p.is_file()).cloned()
}

/// 定位内置 bash；未解压或缺失时返回 None。
pub fn find_bundled_bash(data_dir: &Path) -> Option<PathBuf> {
    first_existing(&bash_candidates(&portablegit_root(data_dir)))
}

/// 定位内置 git；未解压或缺失时返回 None。
pub fn find_bundled_git(data_dir: &Path) -> Option<PathBuf> {
    first_existing(&git_candidates(&portablegit_root(data_dir)))
}

/// 定位内置 python；未解压或缺失时返回 None。
pub fn find_bundled_python(data_dir: &Path) -> Option<PathBuf> {
    first_existing(&python_candidates(&python_root(data_dir)))
}
