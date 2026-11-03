//! read：读文本文件（行/字节双限截断）；图片文件按 image 段回给模型。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::Result;

use crate::registry::{Tool, ToolAnnotations, ToolImage, ToolOutput};
use crate::truncate;

/// 单张图片的大小上限：base64 后体积是原文件的 4/3，超过这个值模型侧基本会拒。
const MAX_IMAGE_BYTES: usize = 5 * 1024 * 1024;

/// 按扩展名判 MIME；不是图片返回 None。
fn image_mime(path: &std::path::Path) -> Option<&'static str> {
    let ext = path
        .extension()
        .and_then(|e| e.to_str())
        .unwrap_or("")
        .to_ascii_lowercase();
    match ext.as_str() {
        "png" => Some("image/png"),
        "jpg" | "jpeg" => Some("image/jpeg"),
        "gif" => Some("image/gif"),
        "webp" => Some("image/webp"),
        "bmp" => Some("image/bmp"),
        _ => None,
    }
}

/// 标准 base64 编码（不引额外依赖）。
fn base64_encode(bytes: &[u8]) -> String {
    const TABLE: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::with_capacity(bytes.len().div_ceil(3) * 4);
    for chunk in bytes.chunks(3) {
        let b = [
            chunk[0],
            *chunk.get(1).unwrap_or(&0),
            *chunk.get(2).unwrap_or(&0),
        ];
        let n = ((b[0] as u32) << 16) | ((b[1] as u32) << 8) | b[2] as u32;
        out.push(TABLE[(n >> 18) as usize & 63] as char);
        out.push(TABLE[(n >> 12) as usize & 63] as char);
        out.push(if chunk.len() > 1 {
            TABLE[(n >> 6) as usize & 63] as char
        } else {
            '='
        });
        out.push(if chunk.len() > 2 {
            TABLE[n as usize & 63] as char
        } else {
            '='
        });
    }
    out
}

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
        "读取文件内容。参数：path(必填)、offset(起始行，1 起)、limit(行数)。文本按 2000 行 / 50KB 截断；\
         图片（png/jpg/gif/webp/bmp，≤5MB）直接作为图片交给视觉模型看，不要再拿 bash 去猜内容。"
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
        _ctx: &crate::ToolCtx,
    ) -> Result<ToolOutput> {
        let path =
            crate::builtin::resolve_in_workdir(workdir, args["path"].as_str().unwrap_or(""))?;
        let meta = std::fs::metadata(&path)
            .map_err(|e| wb_pkg::AppError::wrap("1004", "路径不存在", e))?;
        if meta.is_dir() {
            return Ok(ToolOutput::plain(
                format!("[目录] {}（用 ls 列出内容）", path.display()),
                false,
                0,
            ));
        }
        // 图片先按扩展名分流：二进制嗅探会把所有图片都判成「二进制文件」，
        // 视觉模型就永远看不到用户的截图与 UI 图。
        if let Some(mime) = image_mime(&path) {
            if meta.len() as usize > MAX_IMAGE_BYTES {
                return Ok(ToolOutput::plain(
                    format!(
                        "[图片过大] {}（{} 字节，上限 {} MB）",
                        path.display(),
                        meta.len(),
                        MAX_IMAGE_BYTES / 1024 / 1024
                    ),
                    true,
                    0,
                ));
            }
            let bytes = std::fs::read(&path)
                .map_err(|e| wb_pkg::AppError::wrap("1002", "读图片失败", e))?;
            let b64 = base64_encode(&bytes);
            return Ok(ToolOutput::plain(
                format!(
                    "[图片] {}（{}，{} 字节）已随结果作为图片交给模型，不必再读一次。",
                    path.display(),
                    mime,
                    bytes.len()
                ),
                false,
                0,
            )
            .with_images(vec![ToolImage {
                mime: mime.to_string(),
                base64: b64,
            }]));
        }
        let bytes =
            std::fs::read(&path).map_err(|e| wb_pkg::AppError::wrap("1002", "读文件失败", e))?;
        // 二进制嗅探：含 NUL 视为二进制
        if bytes.contains(&0) {
            return Ok(ToolOutput::plain(
                format!("[二进制文件] {}，{} 字节", path.display(), bytes.len()),
                false,
                0,
            ));
        }
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
        Ok(ToolOutput::plain(truncate::truncate(&out), false, 0))
    }
}
