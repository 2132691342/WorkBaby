//! 审查 PR：拿分支与基线之间的 diff，让模型逐文件给审查意见。
//! 审查的是本地分支差异——PR 在本地就是「基线分支 vs 特性分支」的 diff，
//! 所以不需要连 GitHub API，也不需要令牌。

use std::path::Path;
use std::time::Instant;

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_pkg::Result;
use wb_tools::{Tool, ToolAnnotations, ToolOutput};

use super::Services;

/// 单个文件的 diff 文本上限。
const MAX_FILE_DIFF: usize = 20_000;
/// 整次审查的 diff 文本上限。
const MAX_TOTAL_DIFF: usize = 120_000;

/// review_diff 工具：审查一个分支相对基线的差异。
pub struct ReviewDiffTool {
    sv: std::sync::Arc<Services>,
}

impl ReviewDiffTool {
    /// 装配。
    pub fn new(sv: std::sync::Arc<Services>) -> Self {
        Self { sv }
    }
}

#[async_trait]
impl Tool for ReviewDiffTool {
    fn name(&self) -> &str {
        "review_diff"
    }

    fn label(&self) -> &str {
        "审查差异"
    }

    fn description(&self) -> &str {
        "审查当前仓库里一个分支相对基线分支的差异（PR 的本地等价物）。\
         base 缺省是与当前分支的共同祖先（merge-base），否则用 origin/main。\
         返回逐文件的 diff 与变更统计，你据此找出缺陷、不一致与遗漏的用例并给出审查意见。\
         改动面大时先看统计再挑重点文件。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "base": { "type": "string", "description": "基线分支或提交（缺省取 merge-base）" },
                "head": { "type": "string", "description": "待审查的分支（缺省当前分支）" },
                "path": { "type": "string", "description": "只看某个路径前缀下的文件（可选）" },
                "max_chars": { "type": "number", "description": "返回的最大字符数，默认 60000" }
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
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let ws = self.sv.workspace.lock().unwrap().clone();
        let ws = Path::new(&ws);
        let max_chars = args
            .get("max_chars")
            .and_then(|v| v.as_u64())
            .unwrap_or(60_000)
            .clamp(2_000, 400_000) as usize;

        let head = args
            .get("head")
            .and_then(|v| v.as_str())
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty())
            .unwrap_or_else(|| current_branch(ws));
        let base = args
            .get("base")
            .and_then(|v| v.as_str())
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty());
        let path_filter = args
            .get("path")
            .and_then(|v| v.as_str())
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty());

        let diff = build_diff(ws, base.as_deref(), &head, path_filter.as_deref())?;
        let mut out = format!("变更统计（{} → {}）：\n", base_label(base.as_deref()), head);
        out.push_str(&diff.stat);
        out.push_str("\n\n逐文件 diff：\n");
        out.push_str(&diff.body);
        Ok(ToolOutput {
            text: truncate_chars(out.trim(), max_chars),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// 基线显示名。
fn base_label(base: Option<&str>) -> String {
    base.map(|b| b.to_string())
        .unwrap_or_else(|| "merge-base".to_string())
}

/// 组装 diff：统计 + 逐文件内容。
struct DiffBundle {
    /// 变更统计（numstat 汇总）
    stat: String,
    /// 逐文件 diff 正文
    body: String,
}

/// 取当前分支名。
fn current_branch(ws: &Path) -> String {
    wb_git::exec::ok(ws, &["rev-parse", "--abbrev-ref", "HEAD"])
        .map(|s| s.trim().to_string())
        .unwrap_or_else(|_| "HEAD".to_string())
}

/// 组装差异内容。
fn build_diff(
    ws: &Path,
    base: Option<&str>,
    head: &str,
    path_filter: Option<&str>,
) -> Result<DiffBundle> {
    // 基线：显式指定优先，否则 merge-base，再否则 origin/main。
    let base_ref = match base {
        Some(b) => b.to_string(),
        None => {
            let merged = wb_git::exec::run(ws, &["merge-base", "HEAD", "origin/main"]);
            if merged.ok() {
                merged.stdout.trim().to_string()
            } else {
                "origin/main".to_string()
            }
        }
    };
    let base_ref = if base_ref.is_empty() {
        "HEAD~1".to_string()
    } else {
        base_ref
    };

    // 统计：numstat 逐文件增删行。
    let mut stat_args = vec!["diff", "--numstat", &base_ref, head];
    if let Some(p) = path_filter {
        stat_args.push("--");
        stat_args.push(p);
    }
    let stat_out = wb_git::exec::ok(ws, &stat_args)?;
    let mut files = 0usize;
    let mut added = 0i64;
    let mut removed = 0i64;
    let mut stat = String::new();
    for line in stat_out.lines().filter(|l| !l.trim().is_empty()) {
        let cols: Vec<&str> = line.split('\t').collect();
        if cols.len() < 3 {
            continue;
        }
        let a: i64 = cols[0].parse().unwrap_or(0);
        let d: i64 = cols[1].parse().unwrap_or(0);
        files += 1;
        added += a;
        removed += d;
        stat.push_str(&format!("  +{a} -{d}  {}\n", cols[2]));
    }
    if files == 0 {
        stat.push_str("  （无文件变更）\n");
    }
    stat.push_str(&format!("\n合计：{files} 个文件，+{added} -{removed}"));

    // 正文：逐文件 diff，超长的文件截断。
    let mut body_args = vec!["diff", "--unified=3", &base_ref, head];
    if let Some(p) = path_filter {
        body_args.push("--");
        body_args.push(p);
    }
    let body_out = wb_git::exec::ok(ws, &body_args)?;
    let mut body = String::new();
    for block in body_out.split("diff --git ") {
        if block.trim().is_empty() {
            continue;
        }
        if body.len() >= MAX_TOTAL_DIFF {
            body.push_str("\n[差异过长，已截断]\n");
            break;
        }
        let chunk = format!("diff --git {block}");
        let piece = if chunk.len() > MAX_FILE_DIFF {
            format!("{}\n[本文件 diff 过长，已截断]\n", &chunk[..MAX_FILE_DIFF])
        } else {
            chunk
        };
        body.push_str(&piece);
        body.push('\n');
    }
    Ok(DiffBundle { stat, body })
}

/// 按字符截断（不切 UTF-8），超长加省略说明。
fn truncate_chars(s: &str, max: usize) -> String {
    let count = s.chars().count();
    if count <= max {
        return s.to_string();
    }
    let head: String = s.chars().take(max).collect();
    format!("{head}…（已截断，原文 {count} 字符）")
}
