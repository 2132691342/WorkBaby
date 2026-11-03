//! edit：多处精确替换（old_text 必须唯一且不重叠）。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::{fsutil, AppError, Result};

use crate::builtin::rel_display;
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
         每个 old_text 必须在文件中恰好出现一次，多处编辑按顺序应用。\
         匹配忽略行尾差异（LF / CRLF 均可），写回时保留文件原有行尾。"
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
        _ctx: &crate::ToolCtx,
    ) -> Result<ToolOutput> {
        let path =
            crate::builtin::resolve_in_workdir(workdir, args["path"].as_str().unwrap_or(""))?;
        let edits = args["edits"]
            .as_array()
            .ok_or_else(|| AppError::new("1001", "edits 必须是数组"))?;
        let raw = fsutil::read_to_string(&path)?;
        // 改前快照：界面要在工具卡里就地看见增删行
        let before = raw.clone();
        // 匹配在归一后的正文上做：模型从 read 工具看到的是按行切出的文本，
        // 拿它构造的 old_text 不带 \r，直接拿原文匹配必然落空。
        let (bom, body) = wb_pkg::text::strip_bom(&raw);
        let ending = wb_pkg::text::detect_line_ending(body);
        let mut content = wb_pkg::text::normalize_to_lf(body);
        for (i, e) in edits.iter().enumerate() {
            let old = e["old_text"]
                .as_str()
                .ok_or_else(|| AppError::new("1001", format!("edits[{i}].old_text 缺失")))?;
            let new = e["new_text"]
                .as_str()
                .ok_or_else(|| AppError::new("1001", format!("edits[{i}].new_text 缺失")))?;
            // old_text / new_text 也归一：new_text 里带 \r 会把归一后的正文再污染回去
            let old = wb_pkg::text::normalize_to_lf(old);
            let new = wb_pkg::text::normalize_to_lf(new);
            if old.is_empty() {
                return Err(AppError::new(
                    "1001",
                    format!("edits[{i}]: old_text 不能为空"),
                ));
            }
            let count = content.matches(&old).count();
            match count {
                0 => {
                    return Err(AppError::new(
                        "1001",
                        format!("edits[{i}]: old_text 未在文件中找到"),
                    ))
                }
                1 => content = content.replacen(&old, &new, 1),
                _ => {
                    return Err(AppError::new(
                        "1001",
                        format!("edits[{i}]: old_text 出现 {count} 次，必须唯一"),
                    ))
                }
            }
        }
        // 写回时恢复原行尾与 BOM：归一只是匹配口径，不是要把文件改成 LF
        let restored = wb_pkg::text::apply_line_ending(&content, ending);
        fsutil::write_atomic(&path, format!("{bom}{restored}").as_bytes())?;
        let diff = crate::difftext::line_diff(&before, &restored);
        Ok(ToolOutput::plain(
            format!("已编辑 {}（{} 处）", path.display(), edits.len()),
            false,
            0,
        )
        .with_changes(rel_display(workdir, &path), diff))
    }
}
