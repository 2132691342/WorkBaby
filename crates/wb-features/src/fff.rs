//! FFF：模糊文件名（fffind）与模糊内容（ffgrep）搜索工具，frecency 使用频率加权排序。

mod store;

use std::path::Path;
use std::sync::Arc;
use std::time::Instant;

use async_trait::async_trait;
use nucleo::pattern::{CaseMatching, Normalization, Pattern};
use nucleo::{Config, Matcher, Utf32Str};
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolAnnotations, ToolOutput};

use self::store::{
    best_lines, bonus_from, clip, glob_regex, now_ms, LINE_CLIP_CHARS, MAX_FILE_BYTES,
};
pub use self::store::{frecency_score, walk_files, FrecencyStore};
use crate::FeatureCtx;

/// 单文件的内容命中：按最高行分参与排序。
struct FileHits {
    /// 文件最高行分
    best: u32,
    /// 相对路径（正斜杠）
    rel: String,
    /// 命中行（行号从 1 起，按行号序）
    lines: Vec<(usize, String)>,
}

/// 从参数取非空 query；缺失报 7004。
fn need_query(args: &Value) -> Result<String> {
    match args["query"].as_str() {
        Some(q) if !q.is_empty() => Ok(q.to_string()),
        _ => Err(AppError::new(toolcode::ARG_INVALID, "query 不能为空")),
    }
}

/// 重活在阻塞线程池跑（文件遍历不能卡 runtime），join 失败转 7001。
async fn run_blocking<T: Send + 'static>(
    f: impl FnOnce() -> Result<T> + Send + 'static,
) -> Result<T> {
    tokio::task::spawn_blocking(f)
        .await
        .map_err(|e| AppError::new("7001", e.to_string()))?
}

/// 组装带耗时的成功输出。
fn out(text: String, started: Instant) -> ToolOutput {
    ToolOutput {
        text,
        is_error: false,
        duration_ms: started.elapsed().as_millis() as i64,
    }
}

/// ffind 同步体：遍历打分 + frecency 加权 + 落库；返回最终输出文本。
fn fffind_sync(
    store: &FrecencyStore,
    root: &Path,
    query: &str,
    limit: usize,
    cancel: &CancellationToken,
) -> Result<String> {
    let pattern = Pattern::parse(query, CaseMatching::Smart, Normalization::Smart);
    let mut matcher = Matcher::new(Config::DEFAULT);
    let mut buf: Vec<char> = Vec::new();
    let mut scored: Vec<(u32, String)> = Vec::new();
    let cancelled = walk_files(root, cancel, |_, rel| {
        if let Some(s) = pattern.score(Utf32Str::new(rel, &mut buf), &mut matcher) {
            if s > 0 {
                scored.push((s, rel.to_string()));
            }
        }
    });
    if cancelled {
        return Err(AppError::new(
            toolcode::EXEC_CANCELLED,
            "fffind 已随会话取消",
        ));
    }
    let now = now_ms();
    let frec = store.snapshot();
    let mut weighted: Vec<(f64, String)> = scored
        .into_iter()
        .map(|(s, rel)| (s as f64 * (1.0 + bonus_from(&frec, &rel, now)), rel))
        .collect();
    weighted.sort_by(|a, b| b.0.total_cmp(&a.0).then_with(|| a.1.cmp(&b.1)));
    // 写库失败只降级统计，不影响本次搜索结果
    for (_, rel) in weighted.iter().take(limit) {
        let _ = store.record_hit(rel, now);
    }
    let _ = store.record_query(query, now);
    let hits: Vec<String> = weighted
        .into_iter()
        .take(limit)
        .map(|(_, rel)| rel)
        .collect();
    Ok(if hits.is_empty() {
        "[无命中]".into()
    } else {
        hits.join("\n")
    })
}

/// ffgrep 同步体：遍历读文件逐行打分 + 落库；返回最终输出文本。
fn ffgrep_sync(
    store: &FrecencyStore,
    root: &Path,
    query: &str,
    limit: usize,
    glob: Option<regex::Regex>,
    cancel: &CancellationToken,
) -> Result<String> {
    let pattern = Pattern::parse(query, CaseMatching::Smart, Normalization::Smart);
    let mut matcher = Matcher::new(Config::DEFAULT);
    let mut files: Vec<FileHits> = Vec::new();
    let cancelled = walk_files(root, cancel, |p, rel| {
        if let Some(re) = &glob {
            let name = p
                .file_name()
                .map(|n| n.to_string_lossy().into_owned())
                .unwrap_or_default();
            if !re.is_match(&name) && !re.is_match(rel) {
                return;
            }
        }
        let Ok(bytes) = std::fs::read(p) else {
            return;
        };
        // 大文件与二进制（含 \0）跳过：逐行模糊打分只对文本有意义
        if bytes.len() > MAX_FILE_BYTES || bytes.contains(&0) {
            return;
        }
        let text = String::from_utf8_lossy(&bytes);
        if let Some((best, lines)) = best_lines(&text, &pattern, &mut matcher) {
            files.push(FileHits {
                best,
                rel: rel.to_string(),
                lines,
            });
        }
    });
    if cancelled {
        return Err(AppError::new(
            toolcode::EXEC_CANCELLED,
            "ffgrep 已随会话取消",
        ));
    }
    let now = now_ms();
    files.sort_by(|a, b| b.best.cmp(&a.best).then_with(|| a.rel.cmp(&b.rel)));
    files.truncate(limit);
    let mut out_lines: Vec<String> = Vec::new();
    for f in &files {
        let _ = store.record_hit(&f.rel, now);
        for (no, text) in &f.lines {
            out_lines.push(format!("{}:{no}: {}", f.rel, clip(text, LINE_CLIP_CHARS)));
        }
    }
    let _ = store.record_query(query, now);
    if out_lines.is_empty() {
        out_lines.push("[无命中]".into());
    }
    Ok(out_lines.join("\n"))
}

/// fffind 工具：模糊文件名搜索。
pub struct FffindTool {
    /// 共享 frecency 存储
    store: Arc<FrecencyStore>,
    /// 功能上下文（工作区根）
    ctx: FeatureCtx,
}

impl FffindTool {
    /// 装配。
    pub fn new(store: Arc<FrecencyStore>, ctx: FeatureCtx) -> Self {
        Self { store, ctx }
    }
}

#[async_trait]
impl Tool for FffindTool {
    fn name(&self) -> &str {
        "fffind"
    }

    fn label(&self) -> &str {
        "模糊找文件"
    }

    fn description(&self) -> &str {
        "按模糊关键字找文件路径：容忍拼写误差与大小写，支持路径片段（如 \"fffeat\" 命中 src/fff/feat.rs，\
         \"todo store\" 匹配两段）。大概记得文件名、想列出候选文件时用它；精确 glob 用 find，搜文件内容用 ffgrep。\
         常用文件按使用频率置顶。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "query": { "type": "string", "description": "模糊关键字（文件名或路径片段）" },
                "limit": { "type": "integer", "description": "最大条数（默认 20）" }
            },
            "required": ["query"]
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
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let query = need_query(&args)?;
        let limit = args["limit"].as_u64().unwrap_or(20).clamp(1, 200) as usize;
        let store = self.store.clone();
        let root = self.ctx.workspace.clone();
        let lines =
            run_blocking(move || fffind_sync(&store, &root, &query, limit, &cancel)).await?;
        Ok(out(lines, started))
    }
}

/// ffgrep 工具：模糊内容搜索。
pub struct FfgrepTool {
    /// 共享 frecency 存储
    store: Arc<FrecencyStore>,
    /// 功能上下文（工作区根）
    ctx: FeatureCtx,
}

impl FfgrepTool {
    /// 装配。
    pub fn new(store: Arc<FrecencyStore>, ctx: FeatureCtx) -> Self {
        Self { store, ctx }
    }
}

#[async_trait]
impl Tool for FfgrepTool {
    fn name(&self) -> &str {
        "ffgrep"
    }

    fn label(&self) -> &str {
        "模糊搜内容"
    }

    fn description(&self) -> &str {
        "在工作区文本文件里做模糊内容搜索：逐行匹配、容忍拼写误差、不解释正则，每文件最多返回 3 行。\
         大概记得函数名、报错文案或注释片段而背不出全文时用它；正则搜索用 grep，只找文件名用 fffind。\
         返回 文件:行号: 行内容。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "query": { "type": "string", "description": "模糊关键字" },
                "limit": { "type": "integer", "description": "最大文件数（默认 30）" },
                "glob": { "type": "string", "description": "文件名过滤，如 *.rs（可选）" }
            },
            "required": ["query"]
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
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let query = need_query(&args)?;
        let limit = args["limit"].as_u64().unwrap_or(30).clamp(1, 200) as usize;
        let glob = match args["glob"].as_str() {
            Some(g) if !g.is_empty() => Some(glob_regex(g)?),
            _ => None,
        };
        let store = self.store.clone();
        let root = self.ctx.workspace.clone();
        let lines =
            run_blocking(move || ffgrep_sync(&store, &root, &query, limit, glob, &cancel)).await?;
        Ok(out(lines, started))
    }
}
