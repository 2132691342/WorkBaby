//! 共享命令执行器：子进程纪律的唯一入口（CREATE_NO_WINDOW / 超时 / 输出截断）。

use std::path::Path;
use std::process::Stdio;

use tokio::io::AsyncReadExt;
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};

/// CREATE_NO_WINDOW：子进程不闪控制台窗口（Windows 纪律）。
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

/// 执行结果。
pub struct ExecOutcome {
    /// stdout（超限时截断）
    pub stdout: String,
    /// stderr
    pub stderr: String,
    /// 退出码（被信号杀 = None）
    pub exit_code: Option<i32>,
    /// 是否因超时终止
    pub timed_out: bool,
}

/// 单条输出上限（内存保护；最终截断仍走 truncate）。
const OUTPUT_CAP: usize = 1024 * 1024;

/// 跑一条命令：argv 数组直传（永不进 shell 解释）；超时或取消都以终止进程收场。
pub async fn run(
    program: &str,
    args: &[String],
    workdir: &Path,
    timeout_secs: u64,
    cancel: &CancellationToken,
) -> Result<ExecOutcome> {
    run_with_limits(program, args, workdir, timeout_secs, cancel, None).await
}

/// 同 run，但可套 Windows 作业对象（内存 / 进程数上限，关闭即杀光残留）。
pub async fn run_with_limits(
    program: &str,
    args: &[String],
    workdir: &Path,
    timeout_secs: u64,
    cancel: &CancellationToken,
    limits: Option<crate::job::JobLimits>,
) -> Result<ExecOutcome> {
    let mut job_opt = limits.and_then(crate::job::Job::create);
    let mut cmd = tokio::process::Command::new(program);
    cmd.args(args)
        .current_dir(workdir)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true);
    apply_runtime_env(&mut cmd);
    #[cfg(windows)]
    cmd.creation_flags(CREATE_NO_WINDOW);

    let mut child = cmd
        .spawn()
        .map_err(|e| AppError::wrap("7003", format!("无法启动 {program}"), e))?;
    // 越早入作业越好：进程已起但还没跑起来，收编失败的窗口极小
    if let (Some(job), Some(pid)) = (job_opt.as_ref(), child.id()) {
        job.assign(pid);
    }
    // 作业必须活到进程结束（Drop 即杀光），但它跨 await 必须先摘出来，
    // 裸 HANDLE 不是 Send：放到阻塞段结束前由 drop 收尸。
    let job = job_opt.take();
    let mut out_pipe: tokio::process::ChildStdout =
        child.stdout.take().expect("stdout 已声明 piped");
    let mut err_pipe: tokio::process::ChildStderr =
        child.stderr.take().expect("stderr 已声明 piped");

    let deadline = tokio::time::timeout(
        std::time::Duration::from_secs(timeout_secs.max(1)),
        collect(&mut out_pipe, &mut err_pipe, &mut child, cancel),
    );
    let result = match deadline.await {
        Ok(Ok((stdout, stderr, code))) => Ok(ExecOutcome {
            stdout,
            stderr,
            exit_code: code,
            timed_out: false,
        }),
        Ok(Err(e)) => Err(e),
        Err(_) => {
            child.kill().await.ok();
            Ok(ExecOutcome {
                stdout: String::new(),
                stderr: String::new(),
                exit_code: None,
                timed_out: true,
            })
        }
    };
    drop(job);
    result
}

/// 收集输出并等待退出；取消令牌触发即杀进程。
async fn collect(
    out: &mut tokio::process::ChildStdout,
    err: &mut tokio::process::ChildStderr,
    child: &mut tokio::process::Child,
    cancel: &CancellationToken,
) -> Result<(String, String, Option<i32>)> {
    let mut stdout = Vec::new();
    let mut stderr = Vec::new();
    let read_out = async { out.read_to_end(&mut stdout).await };
    tokio::select! {
        _ = cancel.cancelled() => {
            child.kill().await.ok();
            return Err(AppError::new(wb_domain::error::tool::EXEC_CANCELLED, "命令被取消"));
        }
        _ = read_out => {}
    }
    // 剩余 stderr 与退出码并行等（wait 借 child 可变，须在输出读完后独占）
    let r_err = err.read_to_end(&mut stderr).await;
    r_err.map_err(|e| AppError::wrap("7003", "读取命令输出失败", e))?;
    let status = child
        .wait()
        .await
        .map_err(|e| AppError::wrap("7003", "等待命令退出失败", e))?;
    Ok((cap(stdout), cap(stderr), status.code()))
}

/// 子进程环境：内置运行时置于 PATH 首位，并锁定 Python / Git 的交互行为。
fn apply_runtime_env(cmd: &mut tokio::process::Command) {
    if let Some(data) = wb_domain::runtime::default_data_dir() {
        cmd.env("PATH", wb_domain::runtime::child_path(&data));
    }
    // 解释器与输出一律 UTF-8：中文路径 / 输出不按系统 ANSI 码页走
    cmd.env("PYTHONUTF8", "1")
        .env("PYTHONIOENCODING", "utf-8")
        .env("PYTHONUNBUFFERED", "1");
    // git 永不弹凭据提示（无 TTY 会永久挂起）
    cmd.env("GIT_TERMINAL_PROMPT", "0")
        .env("GIT_PAGER", "cat")
        .env("LC_ALL", "C.UTF-8");
}

fn cap(mut v: Vec<u8>) -> String {
    if v.len() > OUTPUT_CAP {
        v.truncate(OUTPUT_CAP);
    }
    String::from_utf8_lossy(&v).into_owned()
}
