//! powershell：用系统自带 Windows PowerShell 5.1 执行命令（bash 不适用的原生命令走这里）。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};

use crate::exec;
use crate::registry::{Tool, ToolAnnotations, ToolOutput};
use crate::truncate;

/// powershell 工具。
pub struct PowerShellTool;

#[async_trait]
impl Tool for PowerShellTool {
    fn name(&self) -> &str {
        "powershell"
    }

    fn label(&self) -> &str {
        "执行 PowerShell"
    }

    fn description(&self) -> &str {
        "用 PowerShell 执行命令（优先用用户装的 pwsh 7，没有则用系统自带的 5.1）。\
         大多数事情走 bash 即可；只有 Windows 原生操作\
         （注册表、服务、WMI/CIM、注册 COM、组策略）或用户明确要求时才用它。\
         参数：command(必填)、timeout(秒，默认 120，上限 600)。输出按 2000 行 / 50KB 截断。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "command": { "type": "string", "description": "要执行的 PowerShell 命令" },
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
        ctx: &crate::ToolCtx,
    ) -> Result<ToolOutput> {
        let command = args["command"]
            .as_str()
            .ok_or_else(|| AppError::new("1001", "command 不能为空"))?;
        let timeout = args["timeout"].as_u64().unwrap_or(120).min(600);
        let started = std::time::Instant::now();
        let Some(shell) = wb_domain::runtime::default_data_dir()
            .and_then(|d| wb_domain::runtime::find_powershell(&d))
        else {
            return Err(AppError::new(
                wb_domain::error::tool::NOT_AVAILABLE,
                "找不到 PowerShell：系统自带的 5.1 也不可达，请检查系统安装是否损坏",
            ));
        };
        // -NoProfile 跳过用户配置（可能是交互式提示或慢脚本，会让命令挂住）；
        // -NonInteractive 下 Read-Host 直接失败而不是等输入；$OutputEncoding 与
        // [Console]::OutputEncoding 一起锁 UTF-8，中文输出不按系统码页变成乱码。
        let prelude = "[Console]::OutputEncoding=[Text.Encoding]::UTF8; \
                       $OutputEncoding=[Text.Encoding]::UTF8; ";
        let script = format!("{prelude}{command}");
        let argv = vec![
            "-NoProfile".to_string(),
            "-NonInteractive".to_string(),
            "-ExecutionPolicy".to_string(),
            "Bypass".to_string(),
            "-Command".to_string(),
            script,
        ];
        let outcome = exec::run_streaming(
            &shell.display().to_string(),
            &argv,
            workdir,
            timeout,
            &cancel,
            ctx.progress.as_ref().map(std::sync::Arc::clone),
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
        let archived = if crate::archive::worth_archiving(&text) {
            crate::archive::archive("powershell", &text)
        } else {
            None
        };
        Ok(ToolOutput::plain(
            crate::archive::with_archive_hint(truncate::truncate(&text), archived.as_deref()),
            is_error,
            started.elapsed().as_millis() as i64,
        ))
    }
}
