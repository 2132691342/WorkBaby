//! read：读文本文件（行/字节双限截断），图片回 image 提示。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::Result;

use crate::registry::{Tool, ToolAnnotations, ToolOutput};
use crate::truncate;

/// read 工具。
pub struct ReadTool;

#[async_trait]
impl Tool for ReadTool {
    fn name(&self) -> &str {
        "read"
    }

    fn label(&self) -> &str {
        "读取文件"
    }

    fn description(&self) -> &str {
        "读取文件内容。参数：path(必填)、offset(起始行，1 起)、limit(行数)。文本按 2000 行 / 50KB 截断。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "path": { "type": "string", "description": "文件路径（绝对或相对工作区）" },
                "offset": { "type": "integer", "description": "起始行（1 起）" },
                "limit": { "type": "integer", "description": "最多读取行数" }
            },
            "required": ["path"]
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
        let path =
            crate::builtin::resolve_in_workdir(workdir, args["path"].as_str().unwrap_or(""))?;
        let meta = std::fs::metadata(&path)
            .map_err(|e| wb_pkg::AppError::wrap("1004", "路径不存在", e))?;
        if meta.is_dir() {
            return Ok(ToolOutput {
                text: format!("[目录] {}（用 ls 列出内容）", path.display()),
                is_error: false,
                duration_ms: 0,
            });
        }
        let bytes =
            std::fs::read(&path).map_err(|e| wb_pkg::AppError::wrap("1002", "读文件失败", e))?;
        // 二进制嗅探：含 NUL 视为二进制
        if bytes.contains(&0) {
            return Ok(ToolOutput {
                text: format!("[二进制文件] {}，{} 字节", path.display(), bytes.len()),
                is_error: false,
                duration_ms: 0,
            });
        }
        let ext = path
            .extension()
            .and_then(|e| e.to_str())
            .unwrap_or("")
            .to_ascii_lowercase();
        let all = String::from_utf8_lossy(&bytes);
        let lines: Vec<&str> = all.lines().collect();
        let offset = args["offset"].as_u64().unwrap_or(1).max(1) as usize;
        let limit = args["limit"].as_u64().unwrap_or(truncate::MAX_LINES as u64) as usize;
        let start = (offset - 1).min(lines.len());
        let end = (start + limit).min(lines.len());
        let mut out = String::new();
        for (i, line) in lines[start..end].iter().enumerate() {
            out.push_str(&format!(
                "{}: {}\n",
                start + i + 1,
                truncate::clip_line(line)
            ));
        }
        if offset > 1 || end < lines.len() {
            out.push_str(&format!(
                "\n[共 {} 行，显示 {}-{}]",
                lines.len(),
                start + 1,
                end
            ));
        }
        let _ = ext;
        Ok(ToolOutput {
            text: truncate::truncate(&out),
            is_error: false,
            duration_ms: 0,
        })
    }
}
