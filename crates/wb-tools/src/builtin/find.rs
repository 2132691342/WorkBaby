//! find：glob 文件名搜索（尊重 .gitignore）。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};

use crate::registry::{Tool, ToolAnnotations, ToolOutput};

/// find 工具。
pub struct FindTool;

#[async_trait]
impl Tool for FindTool {
    fn name(&self) -> &str {
        "find"
    }

    fn label(&self) -> &str {
        "文件搜索"
    }

    fn description(&self) -> &str {
        "按 glob 模式找文件。参数：pattern(必填，如 \"*.rs\" 或 \"src/**\")、path(子目录)、limit(默认 1000)。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "pattern": { "type": "string", "description": "glob 模式" },
                "path": { "type": "string", "description": "搜索根（默认工作区）" },
                "limit": { "type": "integer", "description": "最大条数（默认 1000）" }
            },
            "required": ["pattern"]
        })
    }

    fn annotations(&self) -> ToolAnnotations {
        ToolAnnotations {
            read_only: true,
            destructive: false,
        }
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        workdir: &std::path::Path,
    ) -> Result<ToolOutput> {
        let pattern = args["pattern"]
            .as_str()
            .ok_or_else(|| AppError::new("1001", "pattern 不能为空"))?;
        let root = match args["path"].as_str() {
            Some(p) if !p.is_empty() => crate::builtin::resolve_in_workdir(workdir, p)?,
            _ => workdir.to_path_buf(),
        };
        let limit = args["limit"].as_u64().unwrap_or(1000) as usize;
        let matcher = globset::Glob::new(pattern)
            .map_err(|e| AppError::wrap("1001", "glob 非法", e))?
            .compile_matcher();
        let mut out = String::new();
        let mut count = 0usize;
        let walker = ignore::WalkBuilder::new(&root).hidden(false).build();
        for entry in walker.flatten() {
            if cancel.is_cancelled() {
                break;
            }
            let p = entry.path();
            if !p.is_file() {
                continue;
            }
            let rel = p.strip_prefix(workdir).unwrap_or(p);
            if matcher.is_match(rel) || matcher.is_match(p.file_name().unwrap_or_default()) {
                out.push_str(&rel.display().to_string());
                out.push('\n');
                count += 1;
                if count >= limit {
                    out.push_str(&format!("[已达 {limit} 条上限]"));
                    break;
                }
            }
        }
        if count == 0 {
            out.push_str("[无命中]");
        }
        Ok(ToolOutput {
            text: out,
            is_error: false,
            duration_ms: 0,
        })
    }
}
