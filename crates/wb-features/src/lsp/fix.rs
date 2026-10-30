//! lsp_fix 工具：让语言服务器给快速修复并按需写入。

use std::path::{Path, PathBuf};
use std::time::Instant;

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::common;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolAnnotations, ToolOutput};

use super::adapters::{self, ServerSpec};
use super::client::LspClient;
use super::edits::apply_edits;
use super::{codes, command_exists, DEFAULT_TIMEOUT_MS};
use crate::FeatureCtx;

/// lsp_fix 工具：让语言服务器给快速修复并按需写入。
pub struct LspFixTool {
    /// 工作区根（相对路径以此为基准）
    root: PathBuf,
}

impl LspFixTool {
    /// 装配。
    pub fn new(ctx: FeatureCtx) -> Self {
        Self {
            root: ctx.workspace,
        }
    }
}

#[async_trait]
impl Tool for LspFixTool {
    fn name(&self) -> &str {
        "lsp_fix"
    }

    fn label(&self) -> &str {
        "语言快速修复"
    }

    fn description(&self) -> &str {
        "对指定文件请求语言服务器的快速修复（默认 source.fixAll，如格式化与导入整理）。\
         默认只预览改动内容不写盘；确认无误后传 write=true 才落盘。"
    }

    fn annotations(&self) -> ToolAnnotations {
        ToolAnnotations {
            read_only: false,
            destructive: false,
        }
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "path": { "type": "string", "description": "目标文件（相对工作区）" },
                "kind": {
                    "type": "string",
                    "description": "修复类型（默认 source.fixAll）"
                },
                "write": {
                    "type": "boolean",
                    "description": "是否把修复写入文件（默认 false，仅预览）"
                }
            },
            "required": ["path"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let rel = args
            .get("path")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        if rel.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "path 不能为空"));
        }
        let kind = args
            .get("kind")
            .and_then(|v| v.as_str())
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty())
            .unwrap_or_else(|| "source.fixAll".to_string());
        let write = args.get("write").and_then(|v| v.as_bool()).unwrap_or(false);
        let file = if self.root.is_dir() {
            self.root.join(&rel)
        } else {
            workdir.join(&rel)
        };
        if !file.is_file() {
            return Err(AppError::new(
                common::PATH_MISSING,
                format!("文件不存在：{rel}"),
            ));
        }
        let ext = file
            .extension()
            .and_then(|e| e.to_str())
            .unwrap_or("")
            .to_string();
        let spec: &ServerSpec = adapters::server_for_extension(&ext).ok_or_else(|| {
            AppError::new(codes::NO_MATCH, format!("没有语言服务器支持 {ext} 文件"))
        })?;
        if !command_exists(spec.server_command) {
            return Err(AppError::new(
                codes::NOT_INSTALLED,
                format!(
                    "语言服务器 {} 未安装（命令 {} 不在 PATH）",
                    spec.server_name, spec.server_command
                ),
            ));
        }
        let text =
            std::fs::read_to_string(&file).map_err(|e| AppError::wrap("1002", "读文件失败", e))?;
        let lang = adapters::language_id(&file);
        let mut lsp = LspClient::start(
            spec.server_command,
            spec.server_args,
            workdir,
            DEFAULT_TIMEOUT_MS,
        )
        .await?;
        let edits = lsp
            .code_actions(&file, &text, lang, &kind, DEFAULT_TIMEOUT_MS)
            .await?;
        lsp.shutdown().await;
        if edits.is_empty() {
            return Ok(ToolOutput {
                text: format!("{} 没有可用的 {} 修复。", rel, kind),
                is_error: false,
                duration_ms: started.elapsed().as_millis() as i64,
            });
        }
        let fixed = apply_edits(&text, &edits);
        if write {
            std::fs::write(&file, &fixed).map_err(|e| AppError::wrap("1002", "写文件失败", e))?;
            Ok(ToolOutput {
                text: format!(
                    "已对 {} 应用 {} 修复（{} 处编辑）。",
                    rel,
                    kind,
                    edits.len()
                ),
                is_error: false,
                duration_ms: started.elapsed().as_millis() as i64,
            })
        } else {
            let preview = diff_preview(&text, &fixed);
            Ok(ToolOutput {
                text: format!(
                    "{} 的 {} 修复预览（{} 处编辑，未写入）：\n{}",
                    rel,
                    kind,
                    edits.len(),
                    preview
                ),
                is_error: false,
                duration_ms: started.elapsed().as_millis() as i64,
            })
        }
    }
}

/// 极简预览：按行对比，给出删除行与新增行。
fn diff_preview(old: &str, new: &str) -> String {
    let old_lines: Vec<&str> = old.lines().collect();
    let new_lines: Vec<&str> = new.lines().collect();
    let mut out = Vec::new();
    for l in &old_lines {
        if !new_lines.contains(l) {
            out.push(format!("- {}", l.trim_end()));
        }
    }
    for l in &new_lines {
        if !old_lines.contains(l) {
            out.push(format!("+ {}", l.trim_end()));
        }
    }
    if out.is_empty() {
        out.push("（无可见差异）".into());
    }
    out.join("\n")
}
