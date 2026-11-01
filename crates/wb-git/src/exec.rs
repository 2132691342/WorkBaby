//! git 子进程执行：参数数组直传，用户输入永不进 shell；Windows 加 CREATE_NO_WINDOW。

use std::path::Path;
use std::process::Command;

/// 一次 git 调用的原始结果。
pub struct GitOut {
    /// 退出码
    pub git_exit_code: i32,
    /// 标准输出（UTF-8 丢失处理）
    pub stdout: String,
    /// 标准错误
    pub stderr: String,
}

impl GitOut {
    /// 是否成功。
    pub fn ok(&self) -> bool {
        self.git_exit_code == 0
    }
}

/// 定位 git.exe：内置 PortableGit 优先，其次系统 PATH。
fn git_program() -> String {
    if let Some(data) = wb_domain::runtime::default_data_dir() {
        if let Some(p) = wb_domain::runtime::find_bundled_git(&data) {
            return p.display().to_string();
        }
    }
    "git".to_string()
}

/// 探测 git；返回版本串（如 "2.45.1"），未装返回 None。
pub fn version() -> Option<String> {
    let out = Command::new(git_program()).arg("--version").output().ok()?;
    if !out.status.success() {
        return None;
    }
    let text = String::from_utf8_lossy(&out.stdout);
    text.split_whitespace().nth(2).map(|s| s.trim().to_string())
}

/// 在 `dir` 下跑 git（失败不 panic，退出码带出）。
pub fn run(dir: &Path, args: &[&str]) -> GitOut {
    let mut cmd = Command::new(git_program());
    // quotepath=false：numstat 等输出里非 ASCII 路径保持原样，才能与 porcelain 路径对上
    cmd.args(["-c", "core.quotepath=false"])
        .args(args)
        .current_dir(dir)
        .env("GIT_OPTIONAL_LOCKS", "0")
        .env("GIT_PAGER", "cat")
        .env("LC_ALL", "C.UTF-8");
    // 内置运行时进 PATH：git 调 ssh / 凭据助手时也能自给自足
    if let Some(data) = wb_domain::runtime::default_data_dir() {
        cmd.env("PATH", wb_domain::runtime::child_path(&data));
    }
    hide_window(&mut cmd);
    match cmd.output() {
        Ok(out) => GitOut {
            git_exit_code: out.status.code().unwrap_or(-1),
            stdout: String::from_utf8_lossy(&out.stdout).into_owned(),
            stderr: String::from_utf8_lossy(&out.stderr).into_owned(),
        },
        // git 不存在或不可执行
        Err(_) => GitOut {
            git_exit_code: -1,
            stdout: String::new(),
            stderr: "git not found".into(),
        },
    }
}

/// 跑 git 并要求成功；失败转 1017（带 stderr 摘要）。
pub fn ok(dir: &Path, args: &[&str]) -> wb_pkg::Result<String> {
    let out = run(dir, args);
    if out.ok() {
        Ok(out.stdout)
    } else {
        Err(wb_pkg::AppError::new(
            wb_domain::error::common::GIT_FAILED,
            git_fail_text(args, &out.stderr),
        ))
    }
}

/// stderr 摘要：取最后一个非空行（git 的结论一般在末尾）。
fn git_fail_text(args: &[&str], stderr: &str) -> String {
    let head = args.first().copied().unwrap_or("");
    let tail = stderr
        .lines()
        .rev()
        .find(|l| !l.trim().is_empty())
        .unwrap_or("git 命令失败");
    format!("git {head} 失败：{tail}")
}

#[cfg(windows)]
fn hide_window(cmd: &mut Command) {
    use std::os::windows::process::CommandExt;
    cmd.creation_flags(0x0800_0000); // CREATE_NO_WINDOW
}

#[cfg(not(windows))]
fn hide_window(_cmd: &mut Command) {}
