//! ls：列目录（一层条目，目录加 / 后缀）。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::Result;

use crate::registry::{Tool, ToolAnnotations, ToolOutput};

/// ls 工具。
pub struct LsTool;

#[async_trait]
impl Tool for LsTool {
    fn name(&self) -> &str {
        "ls"
    }

    fn label(&self) -> &str {
        "列目录"
    }

    fn description(&self) -> &str {
        "列出一层目录内容。参数：path(默认工作区)、limit(默认 500)。目录条目带 / 后缀。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "path": { "type": "string", "description": "目录路径（默认工作区）" },
                "limit": { "type": "integer", "description": "最大条数（默认 500）" }
            }
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
        _cancel: CancellationToken,
        workdir: &std::path::Path,
    ) -> Result<ToolOutput> {
        let path = match args["path"].as_str() {
            Some(p) if !p.is_empty() => crate::builtin::resolve_in_workdir(workdir, p)?,
            _ => workdir.to_path_buf(),
        };
        let limit = args["limit"].as_u64().unwrap_or(500) as usize;
        let mut entries = std::fs::read_dir(&path)
            .map_err(|e| wb_pkg::AppError::wrap("1004", "目录不存在", e))?
            .filter_map(|e| e.ok())
            .map(|e| {
                let name = e.file_name().to_string_lossy().into_owned();
                let is_dir = e.file_type().map(|t| t.is_dir()).unwrap_or(false);
                (name, is_dir)
            })
            .collect::<Vec<_>>();
        entries.sort();
        let mut out = String::new();
        let mut count = 0usize;
        for (name, is_dir) in entries {
            if count >= limit {
                out.push_str(&format!("[已达 {limit} 条上限]"));
                break;
            }
            if is_dir {
                out.push_str(&format!("{name}/\n"));
            } else {
                out.push_str(&name);
                out.push('\n');
            }
            count += 1;
        }
        if count == 0 {
            out.push_str("[空目录]");
        }
        Ok(ToolOutput {
            text: out,
            is_error: false,
            duration_ms: 0,
        })
    }
}
