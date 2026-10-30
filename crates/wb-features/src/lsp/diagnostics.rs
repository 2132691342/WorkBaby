//! lsp_diagnostics 工具：全工作区静态诊断。

use std::path::{Path, PathBuf};
use std::time::Instant;

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_pkg::Result;
use wb_tools::{Tool, ToolOutput};

use super::adapters::{self, ServerSpec};
use super::client::{self, LspClient};
use super::{collect_files, command_exists, DEFAULT_FILE_LIMIT, DEFAULT_TIMEOUT_MS};
use crate::FeatureCtx;

/// 启动 server 并跑一次诊断（server 生命周期与本次调用一致）。
async fn run_diagnostics(
    spec: &ServerSpec,
    root: &Path,
    files: &[PathBuf],
    timeout_ms: u64,
) -> Result<(Vec<client::Diagnostic>, String)> {
    let mut lsp = LspClient::start(spec.server_command, spec.server_args, root, timeout_ms).await?;
    let command_text = lsp.command_text().to_string();
    let mut all = Vec::new();
    for file in files {
        let Ok(text) = std::fs::read_to_string(file) else {
            continue;
        };
        let lang = adapters::language_id(file);
        match lsp.diagnostics(file, &text, lang, timeout_ms).await {
            Ok(diags) => all.extend(diags),
            Err(e) => {
                lsp.shutdown().await;
                return Err(e);
            }
        }
    }
    lsp.shutdown().await;
    Ok((all, command_text))
}

/// lsp_diagnostics 工具：全工作区诊断。
pub struct LspDiagnosticsTool {
    /// 工作区根（诊断命令在此目录下执行）
    root: PathBuf,
}

impl LspDiagnosticsTool {
    /// 装配。
    pub fn new(ctx: FeatureCtx) -> Self {
        Self {
            root: ctx.workspace,
        }
    }
}

#[async_trait]
impl Tool for LspDiagnosticsTool {
    fn name(&self) -> &str {
        "lsp_diagnostics"
    }

    fn label(&self) -> &str {
        "语言诊断"
    }

    fn description(&self) -> &str {
        "用语言服务器对工作区做静态诊断，返回错误与警告（含文件、行、列、说明）。\
         改完代码后先跑它确认没引入问题，比自己逐个文件读一遍快。\
         需要本机已装对应语言服务器（如 rust-analyzer、gopls、biome），未装的语言会被跳过并列出。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "paths": {
                    "type": "array",
                    "items": { "type": "string" },
                    "description": "限定检查的文件或目录（相对工作区）；省略则全工作区"
                },
                "limit": {
                    "type": "integer",
                    "description": "最多检查多少个文件（1..200，默认 50）",
                    "minimum": 1,
                    "maximum": 200
                }
            }
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let root = if self.root.is_dir() {
            self.root.clone()
        } else {
            workdir.to_path_buf()
        };
        let limit = args
            .get("limit")
            .and_then(|v| v.as_u64())
            .unwrap_or(DEFAULT_FILE_LIMIT as u64)
            .clamp(1, 200) as usize;
        let paths: Vec<String> = args
            .get("paths")
            .and_then(|v| v.as_array())
            .map(|a| {
                a.iter()
                    .filter_map(|x| x.as_str().map(|s| s.to_string()))
                    .collect()
            })
            .unwrap_or_default();
        // 收集目标文件：指定 paths 时只看它们，否则全工作区
        let mut targets: Vec<PathBuf> = Vec::new();
        if paths.is_empty() {
            for spec in adapters::SERVERS {
                if !command_exists(spec.server_command) {
                    continue;
                }
                for f in collect_files(spec, &root, limit) {
                    if !targets.contains(&f) {
                        targets.push(f);
                    }
                }
            }
        } else {
            for p in &paths {
                let abs = root.join(p);
                if abs.is_dir() {
                    for spec in adapters::SERVERS {
                        if !command_exists(spec.server_command) {
                            continue;
                        }
                        for f in collect_files(spec, &abs, limit) {
                            if !targets.contains(&f) {
                                targets.push(f);
                            }
                        }
                    }
                } else if abs.is_file() {
                    targets.push(abs);
                }
            }
        }
        if targets.is_empty() {
            return Ok(ToolOutput {
                text: "没有可诊断的文件（工作区里没有受支持的语言，或语言服务器未安装）。".into(),
                is_error: false,
                duration_ms: started.elapsed().as_millis() as i64,
            });
        }
        // 按服务器分组执行
        let mut all: Vec<client::Diagnostic> = Vec::new();
        let mut skipped: Vec<String> = Vec::new();
        let mut servers: Vec<String> = Vec::new();
        for spec in adapters::SERVERS {
            let files: Vec<PathBuf> = targets
                .iter()
                .filter(|f| adapters::accepts(spec, f))
                .cloned()
                .collect();
            if files.is_empty() {
                continue;
            }
            if !command_exists(spec.server_command) {
                skipped.push(format!(
                    "{}（未安装 {}）",
                    spec.server_name, spec.server_command
                ));
                continue;
            }
            match run_diagnostics(spec, &root, &files, DEFAULT_TIMEOUT_MS).await {
                Ok((diags, cmd_text)) => {
                    servers.push(cmd_text);
                    all.extend(diags);
                }
                Err(e) => skipped.push(format!("{}（{}）", spec.server_name, e.message)),
            }
        }
        if all.is_empty() {
            let mut text = "没有诊断问题。".to_string();
            if !skipped.is_empty() {
                text.push_str(&format!("\n跳过的服务器：{}", skipped.join("、")));
            }
            return Ok(ToolOutput {
                text,
                is_error: false,
                duration_ms: started.elapsed().as_millis() as i64,
            });
        }
        let mut lines = Vec::with_capacity(all.len());
        for d in &all {
            let rel = d
                .diag_path
                .strip_prefix(root.to_string_lossy().as_ref())
                .unwrap_or(&d.diag_path)
                .to_string();
            lines.push(format!(
                "[{}] {}:{}:{} {}",
                d.diag_severity, rel, d.diag_line, d.diag_column, d.diag_message
            ));
        }
        let mut text = format!("共 {} 条诊断：\n{}", all.len(), lines.join("\n"));
        if !servers.is_empty() {
            text.push_str(&format!("\n使用的服务器：{}", servers.join("、")));
        }
        if !skipped.is_empty() {
            text.push_str(&format!("\n跳过的服务器：{}", skipped.join("、")));
        }
        Ok(ToolOutput {
            text,
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}
