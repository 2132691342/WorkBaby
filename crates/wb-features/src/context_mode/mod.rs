//! context_mode 工具：沙箱执行 + 知识库索引与检索（省上下文的核心手段）。

pub mod sandbox;
pub mod store;

use std::path::Path;
use std::time::{Instant, SystemTime, UNIX_EPOCH};

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

use crate::FeatureCtx;
use sandbox::Runtime;
use store::KnowledgeStore;

/// 沙箱目录：feature_dir\sandbox。
fn sandbox_dir(ctx: &FeatureCtx) -> std::path::PathBuf {
    ctx.feature_dir().join("sandbox")
}

/// 知识库：feature_dir\knowledge.db。
fn knowledge_path(ctx: &FeatureCtx) -> std::path::PathBuf {
    ctx.feature_dir().join("knowledge.db")
}

/// 当前毫秒时间戳。
fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

/// ctx_execute 工具：在沙箱里跑一段代码，只有 stdout 进上下文。
pub struct CtxExecuteTool {
    ctx: FeatureCtx,
}

impl CtxExecuteTool {
    /// 装配。
    pub fn new(ctx: FeatureCtx) -> Self {
        Self { ctx }
    }
}

#[async_trait]
impl Tool for CtxExecuteTool {
    fn name(&self) -> &str {
        "ctx_execute"
    }

    fn label(&self) -> &str {
        "沙箱执行"
    }

    fn description(&self) -> &str {
        "在沙箱里执行一段代码并只取回 stdout（stderr 不进上下文，省 token）。\
         适合：统计与聚合、大批量文本/文件的过滤与规整、把冗长原始输出压成几行结论。\
         语言可选 javascript / typescript / python / shell / powershell / ruby / php / perl / go / dotnet / java / lua。\
         不要用它改工作区文件——只读的探查与分析才用它。\
         需要把大量资料留作后查时，先用 ctx_index 收进知识库，再用 ctx_search 检索。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "language": {
                    "type": "string",
                    "description": "执行语言",
                    "enum": sandbox::supported_languages()
                },
                "code": { "type": "string", "description": "要执行的代码（脚本正文）" },
                "timeout": {
                    "type": "integer",
                    "description": "超时秒数（1..120，默认 30）",
                    "minimum": 1,
                    "maximum": sandbox::MAX_TIMEOUT_SECS
                }
            },
            "required": ["language", "code"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let (language, code, timeout) =
            sandbox::parse_args(&args).map_err(|e| AppError::new(toolcode::ARG_INVALID, e))?;
        let runtime: &Runtime = sandbox::find_runtime(&language).ok_or_else(|| {
            AppError::new(
                toolcode::ARG_INVALID,
                format!(
                    "不支持的语言 {language}；可用：{}",
                    sandbox::supported_languages().join(" / ")
                ),
            )
        })?;
        let dir = sandbox_dir(&self.ctx);
        let file = sandbox::write_script(&dir, runtime.lang_ext, &code)
            .map_err(|e| AppError::new(toolcode::ARG_INVALID, e))?;
        let program = sandbox::resolve_program(runtime);
        let argv = sandbox::build_args(runtime, &file);
        // 子进程 cwd 落在沙箱目录而不是真实工作区：脚本改文件改不到项目，
        // 只有显式传绝对路径才够得着（配合作业对象的资源上限）。
        if std::fs::create_dir_all(&dir).is_err() {
            return Err(AppError::new(toolcode::ARG_INVALID, "建沙箱目录失败"));
        }
        let outcome = wb_tools::exec::run_with_limits(
            &program,
            &argv,
            &dir,
            timeout,
            &cancel,
            Some(wb_tools::job::JobLimits::default()),
        )
        .await?;
        // 清理脚本：沙箱不留下模型产生的垃圾
        let _ = std::fs::remove_file(&file);
        let mut text = outcome.stdout.trim().to_string();
        if text.is_empty() {
            text = "(无 stdout 输出)".into();
        }
        if !outcome.stderr.trim().is_empty() {
            text.push_str("\n\n[stderr]\n");
            text.push_str(outcome.stderr.trim());
        }
        if outcome.timed_out {
            text.push_str(&format!("\n\n[超时：{timeout}s 被终止]"));
        } else if let Some(code) = outcome.exit_code {
            if code != 0 {
                text.push_str(&format!("\n\n[退出码 {code}]"));
            }
        }
        Ok(ToolOutput {
            text,
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// ctx_index 工具：把一份内容收进知识库（只留检索句柄，不占上下文）。
pub struct CtxIndexTool {
    ctx: FeatureCtx,
}

impl CtxIndexTool {
    /// 装配。
    pub fn new(ctx: FeatureCtx) -> Self {
        Self { ctx }
    }
}

#[async_trait]
impl Tool for CtxIndexTool {
    fn name(&self) -> &str {
        "ctx_index"
    }

    fn label(&self) -> &str {
        "收录知识"
    }

    fn description(&self) -> &str {
        "把一段长内容（网页正文、文档、源码、日志）收进本地知识库，只返回文档句柄而不回吐正文。\
         之后用 ctx_search 检索片段。这样长资料不必整段留在上下文里。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "content": { "type": "string", "description": "要收录的正文" },
                "source": { "type": "string", "description": "来源标识（URL 或路径，便于回溯）" },
                "title": { "type": "string", "description": "标题（可选）" }
            },
            "required": ["content"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let content = store::text_arg(&args, "content");
        if content.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "content 不能为空"));
        }
        let source = {
            let s = store::text_arg(&args, "source");
            if s.is_empty() {
                "inline".to_string()
            } else {
                s
            }
        };
        let title = {
            let t = store::text_arg(&args, "title");
            if t.is_empty() {
                source.clone()
            } else {
                t
            }
        };
        let path = knowledge_path(&self.ctx);
        let source_text = source.clone();
        let doc_id = tokio::task::spawn_blocking(move || {
            let kb = KnowledgeStore::open(path.parent().unwrap_or(Path::new(".")))?;
            kb.index(&source, &title, &content, now_ms())
        })
        .await
        .map_err(|e| AppError::new("7001", format!("知识库任务失败：{e}")))??;
        Ok(ToolOutput {
            text: format!("已收录：{doc_id}（来源 {source_text}）"),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// ctx_search 工具：在知识库里检索片段。
pub struct CtxSearchTool {
    ctx: FeatureCtx,
}

impl CtxSearchTool {
    /// 装配。
    pub fn new(ctx: FeatureCtx) -> Self {
        Self { ctx }
    }
}

#[async_trait]
impl Tool for CtxSearchTool {
    fn name(&self) -> &str {
        "ctx_search"
    }

    fn label(&self) -> &str {
        "检索知识"
    }

    fn description(&self) -> &str {
        "在已收录的知识库里检索片段，返回命中的段落摘要与来源。\
         先 ctx_index 收资料，再用它查；比把原文整段读进上下文省得多。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "query": { "type": "string", "description": "检索词或短语" },
                "limit": { "type": "integer", "description": "返回条数（1..20，默认 5）", "minimum": 1, "maximum": 20 }
            },
            "required": ["query"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let query = store::text_arg(&args, "query");
        if query.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "query 不能为空"));
        }
        let limit = args
            .get("limit")
            .and_then(|v| v.as_u64())
            .unwrap_or(5)
            .clamp(1, 20) as usize;
        let path = knowledge_path(&self.ctx);
        let hits = tokio::task::spawn_blocking(move || {
            let kb = KnowledgeStore::open(path.parent().unwrap_or(Path::new(".")))?;
            kb.search(&query, limit)
        })
        .await
        .map_err(|e| AppError::new("7001", format!("知识库任务失败：{e}")))??;
        if hits.is_empty() {
            return Ok(ToolOutput {
                text: "知识库里没有命中。".into(),
                is_error: false,
                duration_ms: started.elapsed().as_millis() as i64,
            });
        }
        let mut lines = Vec::with_capacity(hits.len());
        for h in &hits {
            lines.push(format!(
                "[{}] {} (段 {})\n{}",
                h.chunk_doc_id, h.chunk_source, h.chunk_ordinal, h.chunk_excerpt
            ));
        }
        Ok(ToolOutput {
            text: lines.join("\n\n"),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// ctx_batch_execute 工具：一次跑多条沙箱命令，只回汇总输出。
pub struct CtxBatchExecuteTool {
    ctx: FeatureCtx,
}

impl CtxBatchExecuteTool {
    /// 装配。
    pub fn new(ctx: FeatureCtx) -> Self {
        Self { ctx }
    }
}

#[async_trait]
impl Tool for CtxBatchExecuteTool {
    fn name(&self) -> &str {
        "ctx_batch_execute"
    }

    fn label(&self) -> &str {
        "批量沙箱执行"
    }

    fn description(&self) -> &str {
        "在一次调用里跑多条沙箱命令（同一语言），只返回每条的输出摘要。\
         需要对同一份数据跑多段处理时用它，比多次 ctx_execute 少一轮往返。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "language": {
                    "type": "string",
                    "description": "执行语言",
                    "enum": sandbox::supported_languages()
                },
                "commands": {
                    "type": "array",
                    "items": { "type": "string" },
                    "description": "多条代码片段，按顺序执行"
                },
                "timeout": {
                    "type": "integer",
                    "description": "每条的超时秒数（1..120，默认 30）",
                    "minimum": 1,
                    "maximum": sandbox::MAX_TIMEOUT_SECS
                }
            },
            "required": ["language", "commands"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let language = store::text_arg(&args, "language");
        let runtime: &Runtime = sandbox::find_runtime(&language).ok_or_else(|| {
            AppError::new(
                toolcode::ARG_INVALID,
                format!(
                    "不支持的语言 {language}；可用：{}",
                    sandbox::supported_languages().join(" / ")
                ),
            )
        })?;
        let commands: Vec<String> = args
            .get("commands")
            .and_then(|v| v.as_array())
            .map(|a| {
                a.iter()
                    .filter_map(|x| x.as_str().map(|s| s.to_string()))
                    .collect()
            })
            .unwrap_or_default();
        if commands.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "commands 不能为空"));
        }
        let timeout = args
            .get("timeout")
            .and_then(|v| v.as_u64())
            .unwrap_or(sandbox::DEFAULT_TIMEOUT_SECS)
            .clamp(1, sandbox::MAX_TIMEOUT_SECS);
        let dir = sandbox_dir(&self.ctx);
        std::fs::create_dir_all(&dir)
            .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("建沙箱目录失败：{e}")))?;
        let program = sandbox::resolve_program(runtime);
        let mut parts: Vec<String> = Vec::with_capacity(commands.len());
        for (i, code) in commands.iter().enumerate() {
            let file = sandbox::write_script(&dir, runtime.lang_ext, code)
                .map_err(|e| AppError::new(toolcode::ARG_INVALID, e))?;
            let argv = sandbox::build_args(runtime, &file);
            // 与 ctx_execute 同一条纪律：cwd 落沙箱、套作业对象上限
            let outcome = wb_tools::exec::run_with_limits(
                &program,
                &argv,
                &dir,
                timeout,
                &cancel,
                Some(wb_tools::job::JobLimits::default()),
            )
            .await?;
            let _ = std::fs::remove_file(&file);
            let mut part = format!("--- #{} ---\n{}", i + 1, outcome.stdout.trim());
            if outcome.timed_out {
                part.push_str(&format!("\n[超时 {timeout}s]"));
            } else if let Some(c) = outcome.exit_code {
                if c != 0 {
                    part.push_str(&format!("\n[退出码 {c}]"));
                }
            }
            if !outcome.stderr.trim().is_empty() {
                part.push_str(&format!("\n[stderr] {}", outcome.stderr.trim()));
            }
            parts.push(part);
        }
        Ok(ToolOutput {
            text: parts.join("\n\n"),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}
