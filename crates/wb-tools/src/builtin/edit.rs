//! edit：多处精确替换（old_text 必须唯一且不重叠）。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::{fsutil, AppError, Result};

use crate::registry::{Tool, ToolAnnotations, ToolOutput};

/// edit 工具。
pub struct EditTool;

#[async_trait]
impl Tool for EditTool {
    fn name(&self) -> &str {
        "edit"
    }

    fn label(&self) -> &str {
        "编辑文件"
    }

    fn description(&self) -> &str {
        "精确替换文件内容。参数：path(必填)、edits(必填，数组 [{old_text, new_text}])。\
         每个 old_text 必须在文件中恰好出现一次，多处编辑按顺序应用。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "path": { "type": "string", "description": "文件路径" },
                "edits": {
                    "type": "array",
                    "items": {
                        "type": "object",
                        "properties": {
                            "old_text": { "type": "string", "description": "要替换的原文（必须唯一）" },
                            "new_text": { "type": "string", "description": "替换后的文本" }
                        },
                        "required": ["old_text", "new_text"]
                    }
                }
            },
            "required": ["path", "edits"]
        })
    }

    fn annotations(&self) -> ToolAnnotations {
        ToolAnnotations {
            read_only: false,
            destructive: true,
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
        let edits = args["edits"]
            .as_array()
            .ok_or_else(|| AppError::new("1001", "edits 必须是数组"))?;
        let mut content = fsutil::read_to_string(&path)?;
        for (i, e) in edits.iter().enumerate() {
            let old = e["old_text"]
                .as_str()
                .ok_or_else(|| AppError::new("1001", format!("edits[{i}].old_text 缺失")))?;
            let new = e["new_text"]
                .as_str()
                .ok_or_else(|| AppError::new("1001", format!("edits[{i}].new_text 缺失")))?;
            let count = content.matches(old).count();
            match count {
                0 => {
                    return Err(AppError::new(
                        "1001",
                        format!("edits[{i}]: old_text 未在文件中找到"),
                    ))
                }
                1 => content = content.replacen(old, new, 1),
                _ => {
                    return Err(AppError::new(
                        "1001",
                        format!("edits[{i}]: old_text 出现 {count} 次，必须唯一"),
                    ))
                }
            }
        }
        fsutil::write_atomic(&path, content.as_bytes())?;
        Ok(ToolOutput {
            text: format!("已编辑 {}（{} 处）", path.display(), edits.len()),
            is_error: false,
            duration_ms: 0,
        })
    }
}
