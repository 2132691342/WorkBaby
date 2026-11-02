//! write：整文件写入（自动建父目录，原子写）。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::{fsutil, AppError, Result};

use crate::builtin::rel_display;
use crate::registry::{Tool, ToolAnnotations, ToolOutput};

/// write 工具。
pub struct WriteTool;

#[async_trait]
impl Tool for WriteTool {
    fn name(&self) -> &str {
        "write"
    }

    fn label(&self) -> &str {
        "写入文件"
    }

    fn description(&self) -> &str {
        "写入文件（整文件覆盖，自动创建父目录）。参数：path(必填)、content(必填)。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "path": { "type": "string", "description": "文件路径（绝对或相对工作区）" },
                "content": { "type": "string", "description": "完整文件内容" }
            },
            "required": ["path", "content"]
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
        let content = args["content"]
            .as_str()
            .ok_or_else(|| AppError::new("1001", "content 不能为空"))?;
        // 原文件存在就留一份用来算 diff：整文件覆盖是最容易改坏内容的方式，
        // 用户需要在不打开编辑器的情况下看见「到底删掉了什么」。
        let before = fsutil::read_to_string(&path).unwrap_or_default();
        fsutil::write_atomic(&path, content.as_bytes())?;
        let diff = crate::difftext::line_diff(&before, content);
        Ok(ToolOutput::plain(
            format!("已写入 {}（{} 字节）", path.display(), content.len()),
            false,
            0,
        )
        .with_changes(rel_display(workdir, &path), diff))
    }
}
