//! python：用内置 Python 3.13 执行脚本或一段代码，工作区为当前目录。

use std::path::PathBuf;

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};

use crate::exec;
use crate::registry::{Tool, ToolAnnotations, ToolOutput};
use crate::truncate;

/// python 工具。
pub struct PythonTool;

/// 定位 python.exe：内置运行时 → 系统 PATH。
pub fn find_python() -> Option<PathBuf> {
    if let Some(data) = wb_domain::runtime::default_data_dir() {
        if let Some(p) = wb_domain::runtime::find_bundled_python(&data) {
            return Some(p);
        }
    }
    let path = std::env::var("PATH").ok()?;
    for dir in path.split(';') {
        let p = std::path::Path::new(dir).join("python.exe");
        if p.is_file() {
            return Some(p);
        }
    }
    None
}

#[async_trait]
impl Tool for PythonTool {
    fn name(&self) -> &str {
        "python"
    }

    fn label(&self) -> &str {
        "执行 Python"
    }

    fn description(&self) -> &str {
        "用内置 Python 3.13 执行代码。参数：code(必填，要执行的源码) 或 file(相对工作区的脚本路径)、\
         timeout(秒，默认 120，上限 600)。工作目录即工作区，需要第三方库时先用 bash 里的 pip 装到内置运行时。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "code": { "type": "string", "description": "要执行的 Python 源码（与 file 二选一）" },
                "file": { "type": "string", "description": "要执行的脚本路径（相对工作区）" },
                "timeout": { "type": "integer", "description": "超时秒数（默认 120）" }
            }
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
        let timeout = args["timeout"].as_u64().unwrap_or(120).min(600);
        let code = args["code"].as_str().unwrap_or("").trim();
        let file = args["file"].as_str().unwrap_or("").trim();
        if code.is_empty() && file.is_empty() {
            return Err(AppError::new("1001", "code 与 file 至少给一个"));
        }
        let started = std::time::Instant::now();
        let program = find_python().unwrap_or_else(|| PathBuf::from("python.exe"));
        // code 走 -c 直传；file 走相对工作区的路径，由 python 自己按 cwd 解析
        let argv: Vec<String> = if !code.is_empty() {
            vec!["-c".to_string(), code.to_string()]
        } else {
            vec![file.to_string()]
        };
        let outcome = exec::run(
            &program.display().to_string(),
            &argv,
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
        } else if let Some(code) = outcome.exit_code {
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
