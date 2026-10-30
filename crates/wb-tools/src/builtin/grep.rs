//! grep：内容搜索（正则或字面量，尊重 .gitignore）。

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};

use crate::registry::{Tool, ToolAnnotations, ToolOutput};
use crate::truncate;

/// grep 工具。
pub struct GrepTool;

#[async_trait]
impl Tool for GrepTool {
    fn name(&self) -> &str {
        "grep"
    }

    fn label(&self) -> &str {
        "内容搜索"
    }

    fn description(&self) -> &str {
        "在工作区内搜索文件内容。参数：pattern(必填)、path(子目录)、glob(文件名过滤，如 *.rs)、\
         ignore_case、literal(字面量搜索，默认 false)、context(前后各 N 行)、limit(默认 100 条)。"
    }

    fn parameters(&self) -> Value {
        serde_json::json!({
            "type": "object",
            "properties": {
                "pattern": { "type": "string", "description": "搜索串（正则或字面量）" },
                "path": { "type": "string", "description": "搜索根（默认工作区）" },
                "glob": { "type": "string", "description": "文件名通配" },
                "ignore_case": { "type": "boolean" },
                "literal": { "type": "boolean", "description": "按字面量搜索（不做正则解释）" },
                "context": { "type": "integer", "description": "命中行的前后各 N 行" },
                "limit": { "type": "integer", "description": "最大命中数（默认 100）" }
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
        let ignore_case = args["ignore_case"].as_bool().unwrap_or(false);
        let literal = args["literal"].as_bool().unwrap_or(false);
        let context = args["context"].as_u64().unwrap_or(0) as usize;
        let limit = args["limit"].as_u64().unwrap_or(100) as usize;

        let pattern_expr: String = if literal {
            regex::escape(pattern)
        } else {
            pattern.to_string()
        };
        let mut builder = regex::RegexBuilder::new(&pattern_expr);
        builder.case_insensitive(ignore_case);
        let re = builder
            .build()
            .map_err(|e| AppError::wrap("1001", "正则非法", e))?;

        let glob = args["glob"]
            .as_str()
            .and_then(|g| globset::Glob::new(g).ok().map(|g| g.compile_matcher()));
        let mut out = String::new();
        let mut hits = 0usize;
        let mut visited: Vec<std::path::PathBuf> = Vec::new();
        let walker = ignore::WalkBuilder::new(&root).hidden(false).build();
        for entry in walker.flatten() {
            if cancel.is_cancelled() {
                break;
            }
            let p = entry.path();
            if !p.is_file() {
                continue;
            }
            if let Some(g) = &glob {
                let rel = p.strip_prefix(&root).unwrap_or(p);
                if !g.is_match(rel) && !g.is_match(p.file_name().unwrap_or_default()) {
                    continue;
                }
            }
            let Ok(bytes) = std::fs::read(p) else {
                continue;
            };
            if bytes.contains(&0) {
                continue; // 二进制跳过
            }
            let text = String::from_utf8_lossy(&bytes);
            let lines: Vec<&str> = text.lines().collect();
            let mut shown_ctx: Vec<usize> = Vec::new();
            for (i, line) in lines.iter().enumerate() {
                if !re.is_match(line) {
                    continue;
                }
                hits += 1;
                let rel = p.strip_prefix(workdir).unwrap_or(p).display().to_string();
                out.push_str(&format!(
                    "{}:{}: {}\n",
                    rel,
                    i + 1,
                    truncate::clip_line(line)
                ));
                if context > 0 {
                    let lo = i.saturating_sub(context);
                    let hi = (i + context + 1).min(lines.len());
                    for (j, ctx_line) in lines.iter().enumerate().take(hi).skip(lo) {
                        if j != i && !shown_ctx.contains(&j) {
                            out.push_str(&format!(
                                "{}:{}- {}\n",
                                rel,
                                j + 1,
                                truncate::clip_line(ctx_line)
                            ));
                        }
                    }
                    shown_ctx.clear();
                    shown_ctx.extend(lo..hi);
                }
                if hits >= limit {
                    break;
                }
            }
            visited.push(p.to_path_buf());
            if hits >= limit {
                out.push_str(&format!("[已达 {limit} 条上限，停止]"));
                break;
            }
        }
        if hits == 0 {
            out.push_str("[无命中]");
        }
        let _ = visited.len();
        Ok(ToolOutput {
            text: out,
            is_error: false,
            duration_ms: 0,
        })
    }
}
