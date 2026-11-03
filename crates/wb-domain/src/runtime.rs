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

/// Windows 系统目录：cmd / powershell / where 等原生工具都在这里。
/// 用户 PATH 被删改过（常见于精简装机与安全软件）时，缺了它 `cmd` 与 `where` 就不可达。
#[cfg(windows)]
const SYSTEM_DIRS: [&str; 4] = [
    r"C:\Windows\System32",
    r"C:\Windows",
    r"C:\Windows\System32\Wbem",
    r"C:\Windows\System32\WindowsPowerShell\v1.0",
];

/// 非 Windows 无系统目录补位（本项目只发行 Windows，保留空表让类型一致）。
#[cfg(not(windows))]
const SYSTEM_DIRS: [&str; 0] = [];

/// 内置运行时目录，顺序即 PATH 优先级：git → bash → GNU 工具 → python。
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

/// 系统目录路径（去重后追加到内置运行时之后，用户 PATH 之前）。
pub fn system_path_dirs() -> Vec<PathBuf> {
    SYSTEM_DIRS
        .iter()
        .map(PathBuf::from)
        .filter(|p| p.is_dir())
        .collect()
}

/// 构造子进程 PATH：内置运行时 → 系统目录 → 现有 PATH。
/// 内置运行时优先保证「模型先用内置的」，系统目录补位保证 cmd / powershell 可达，
/// 用户 PATH 兜底保证用户已装的工具仍能用——三层顺序即优先级，用户环境不被改写。
pub fn child_path(data_dir: &Path) -> String {
    let mut parts: Vec<String> = bundled_path_dirs(data_dir)
        .iter()
        .chain(system_path_dirs().iter())
        .map(|p| p.display().to_string())
        .collect();
    if let Some(existing) = std::env::var_os("PATH") {
        parts.push(existing.to_string_lossy().into_owned());
    }
    parts.join(";")
}

/// 定位 PowerShell：用户装的 pwsh 7 → 系统 PowerShell 5.1。
/// 5.1 是 Windows 自带组件，永远存在，所以这条工具链不会因缺运行时而不可用。
#[cfg(windows)]
pub fn find_powershell(_data_dir: &Path) -> Option<PathBuf> {
    let candidates = [
        first_existing(&[
            PathBuf::from(r"C:\Program Files\PowerShell\7\pwsh.exe"),
            PathBuf::from(r"C:\Program Files\PowerShell\7-preview\pwsh.exe"),
        ]),
        Some(PathBuf::from(
            r"C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe",
        )),
    ];
    if let Some(hit) = candidates.into_iter().flatten().find(|p| p.is_file()) {
        return Some(hit);
    }
    let path = std::env::var("PATH").ok()?;
    for dir in path.split(';') {
        for exe in ["pwsh.exe", "powershell.exe"] {
            let p = Path::new(dir).join(exe);
            if p.is_file() {
                return Some(p);
            }
        }
    }
    None
}

/// 非 Windows 占位：本项目不分支运行，仍给出可编译实现。
#[cfg(not(windows))]
pub fn find_powershell(_data_dir: &Path) -> Option<PathBuf> {
    None
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
