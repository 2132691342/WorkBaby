//! bash：经 Git Bash 执行命令（PATH 解析失败时退回系统 shell）。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::Result;

use crate::exec;
use crate::registry::{Tool, ToolAnnotations, ToolOutput};
use crate::truncate;

/// bash 工具。
pub struct BashTool;

/// 定位 bash.exe：PATH → 常见 Git 安装位。
pub fn find_bash() -> Option<std::path::PathBuf> {
    if let Some(p) = which_bash() {
        return Some(p);
    }
    for dir in [
        r"C:\Program Files\Git\bin",
        r"C:\Program Files\Git\usr\bin",
        r"C:\Program Files (x86)\Git\bin",
    ] {
        let p = std::path::Path::new(dir).join("bash.exe");
        if p.exists() {
            return Some(p);
        }
    }
    None
}

fn which_bash() -> Option<std::path::PathBuf> {
    let path = std::env::var("PATH").ok()?;
    for dir in path.split(';') {
        let p = std::path::Path::new(dir).join("bash.exe");
        if p.is_file() {
            return Some(p);
        }
    }
    None
}

#[async_trait]
impl Tool for BashTool {
    fn name(&self) -> &str {
        "bash"
    }

    fn label(&self) -> &str {
        "执行命令"
    }

    fn description(&self) -> &str {
        "在 Git Bash 里执行命令。参数：command(必填)、timeout(秒，默认 120，上限 600)。\
         输出按 2000 行 / 50KB 截断。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "command": { "type": "string", "description": "要执行的命令" },
                "timeout": { "type": "integer", "description": "超时秒数（默认 120）" }
            },
            "required": ["command"]
        })
    }

    fn annotations(&self) -> ToolAnnotations {
        ToolAnnotations {
            read_only: false,
            destructive: false,
        }
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        workdir: &std::path::Path,
    ) -> Result<ToolOutput> {
        let command = args["command"]
            .as_str()
            .ok_or_else(|| wb_pkg::AppError::new("1001", "command 不能为空"))?;
        let timeout = args["timeout"].as_u64().unwrap_or(120).min(600);
        let started = std::time::Instant::now();
        let bash = find_bash().unwrap_or_else(|| std::path::PathBuf::from("bash"));
        let outcome = exec::run(
            &bash.display().to_string(),
            &["-c".to_string(), command.to_string()],
            workdir,
            timeout,
            &cancel,
        )
        .await?;
        let mut text = String::new();
        if !outcome.stdout.is_empty() {
            text.push_str(&outcome.stdout);
        }
        if !outcome.stderr.is_empty() {
            text.push_str("\n[stderr]\n");
            text.push_str(&outcome.stderr);
        }
        let mut is_error = false;
        if outcome.timed_out {
            is_error = true;
            text.push_str(&format!("\n[超时] 超过 {timeout}s 被终止"));
        } else if let Some(code) = outcome.code {
            if code != 0 {
                is_error = true;
                text.push_str(&format!("\n[退出码 {code}]"));
            }
        }
        Ok(ToolOutput {
            text: truncate::truncate(&text),
            is_error,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}
