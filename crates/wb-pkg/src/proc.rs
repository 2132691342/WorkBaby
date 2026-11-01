//! 子进程纪律：不闪控制台窗口、不在 PATH 探测上起进程。

use std::path::PathBuf;

/// Windows 子进程不闪控制台窗口；std 与 tokio 的 Command 共用同一常量。
pub const CREATE_NO_WINDOW: u32 = 0x0800_0000;

/// 给 std::process::Command 挂上不闪窗标志。
#[cfg(windows)]
pub fn hide_window(cmd: &mut std::process::Command) {
    use std::os::windows::process::CommandExt;
    cmd.creation_flags(CREATE_NO_WINDOW);
}

/// 非 Windows 无需处理。
#[cfg(not(windows))]
pub fn hide_window(_cmd: &mut std::process::Command) {}

/// 在 PATH 里定位可执行文件；查不到返回 None。
///
/// 不启子进程（`where` / `which` 会闪一次控制台），直接按 PATH 与 PATHEXT 拼路径。
pub fn resolve_command(command: &str) -> Option<PathBuf> {
    if command.contains(['/', '\\']) {
        return is_executable(&PathBuf::from(command));
    }
    let path = std::env::var_os("PATH")?;
    for dir in std::env::split_paths(&path) {
        let base = dir.join(command);
        if let Some(hit) = is_executable(&base) {
            return Some(hit);
        }
        for ext in executable_exts() {
            let candidate = dir.join(format!("{command}{ext}"));
            if let Some(hit) = is_executable(&candidate) {
                return Some(hit);
            }
        }
    }
    None
}

/// 可执行后缀候选；非 Windows 只有一个空后缀（即原样）。
///
/// 不用 PATHEXT：npm 装的 LSP（`typescript-language-server` 之类）靠 `.cmd` 启动，
/// 而用户在图形界面里不会带上后缀，必须由这里展开。
#[cfg(windows)]
fn executable_exts() -> Vec<String> {
    vec![
        String::new(),
        ".exe".into(),
        ".cmd".into(),
        ".bat".into(),
        ".com".into(),
    ]
}

/// 非 Windows：不加后缀。
#[cfg(not(windows))]
fn executable_exts() -> Vec<String> {
    vec![String::new()]
}

/// 是文件即视为可执行（Windows 无执行位；扩展名由调用方或 PATH 展开负责）。
fn is_executable(path: &std::path::Path) -> Option<PathBuf> {
    if path.is_file() {
        Some(path.to_path_buf())
    } else {
        None
    }
}
