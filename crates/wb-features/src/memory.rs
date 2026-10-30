//! 持久记忆：保存 / 检索 / 删除 + 历史会话检索四个工具，存储见 memory_store。

use std::path::Path;
use std::sync::{Arc, OnceLock};
use std::time::{Instant, SystemTime, UNIX_EPOCH};

use async_trait::async_trait;
use regex::Regex;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolAnnotations, ToolOutput};

use crate::memory_store::{search_sessions, MemoryStore};
use crate::FeatureCtx;

/// 取字符串参数（缺省空串）。
fn arg_str(args: &Value, key: &str) -> String {
    args.get(key)
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string()
}

/// 取数值参数（缺省 default）。
fn arg_usize(args: &Value, key: &str, default: usize) -> usize {
    args.get(key)
        .and_then(|v| v.as_u64())
        .map(|n| n as usize)
        .unwrap_or(default)
}

/// 当前毫秒时间戳。
fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

/// 疑似密钥形态规则（说明文案, 正则）；命中即拒绝入库。
const SECRET_RULES: &[(&str, &str)] = &[
    ("OpenAI 风格 API 密钥（sk-…）", r"\bsk-[A-Za-z0-9_-]{10,}"),
    ("AWS 访问密钥（AKIA…）", r"\bAKIA[0-9A-Z]{16}"),
    (
        "GitHub 令牌（ghp_/gho_/github_pat_…）",
        r"\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})",
    ),
    ("Slack 令牌（xox…）", r"\bxox[bpars]-[A-Za-z0-9-]{10,}"),
    ("GitLab 令牌（glpat-…）", r"\bglpat-[A-Za-z0-9_-]{15,}"),
    ("Google API 密钥（AIza…）", r"\bAIza[0-9A-Za-z_-]{30,}"),
    ("PEM 私钥块", r"-----BEGIN[A-Z ]*PRIVATE KEY-----"),
];

/// 扫描疑似密钥；命中返回形态说明（不回显密钥本体）。
pub fn scan_secret(text: &str) -> Option<&'static str> {
    static RULES: OnceLock<Vec<(&'static str, Regex)>> = OnceLock::new();
    let rules = RULES.get_or_init(|| {
        SECRET_RULES
            .iter()
            .map(|(label, pattern)| (*label, Regex::new(pattern).expect("静态正则必须合法")))
            .collect()
    });
    rules
        .iter()
        .find(|(_, re)| re.is_match(text))
        .map(|(label, _)| *label)
}

/// memory_save 工具：写入持久记忆（写前密钥扫描，同文本去重）。
pub struct MemorySaveTool {
    /// 共享记忆库
    store: Arc<MemoryStore>,
}

impl MemorySaveTool {
    /// 装配。
    pub fn new(store: Arc<MemoryStore>) -> Self {
        Self { store }
    }
}

#[async_trait]
impl Tool for MemorySaveTool {
    fn name(&self) -> &str {
        "memory_save"
    }

    fn label(&self) -> &str {
        "记忆保存"
    }

    fn description(&self) -> &str {
        "把值得跨会话记住的信息写入持久记忆：用户偏好（kind=user）、项目约定（project）、失败教训（failure）、一般事实（memory）。当用户说\"记住…\"或你发现某信息后续会反复用到时主动使用；同 kind 同文本重复保存只会刷新时间戳。正文含疑似密钥会被拒绝。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "kind": { "type": "string", "enum": ["memory", "user", "project", "failure"], "description": "记忆类别，默认 memory" },
                "text": { "type": "string", "description": "记忆正文，一段完整自足的陈述" }
            },
            "required": ["text"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let text = arg_str(&args, "text");
        if text.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "text 必填"));
        }
        let kind = {
            let k = arg_str(&args, "kind");
            if k.is_empty() {
                "memory".to_string()
            } else {
                k
            }
        };
        if !matches!(kind.as_str(), "memory" | "user" | "project" | "failure") {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                "kind 只能是 memory / user / project / failure",
            ));
        }
        if let Some(label) = scan_secret(&text) {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                format!("疑似密钥（{label}），请脱敏后重试"),
            ));
        }
        let store = self.store.clone();
        let kind_display = kind.clone();
        let (memory_id, fresh) =
            tokio::task::spawn_blocking(move || store.save(&kind, &text, now_ms()))
                .await
                .map_err(|e| AppError::new("7001", e.to_string()))??;
        let verb = if fresh {
            "已保存"
        } else {
            "已更新既有"
        };
        Ok(ToolOutput {
            text: format!("{verb}记忆 {memory_id}（{kind_display}）"),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// memory_search 工具：FTS5 全文检索持久记忆。
pub struct MemorySearchTool {
    /// 共享记忆库
    store: Arc<MemoryStore>,
}

impl MemorySearchTool {
    /// 装配。
    pub fn new(store: Arc<MemoryStore>) -> Self {
        Self { store }
    }
}

#[async_trait]
impl Tool for MemorySearchTool {
    fn name(&self) -> &str {
        "memory_search"
    }

    fn label(&self) -> &str {
        "记忆检索"
    }

    fn description(&self) -> &str {
        "全文检索持久记忆库。回答涉及用户偏好、项目背景、历史教训的问题前先查这里，避免重复追问用户；也用于确认一条记忆是否已存在。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "query": { "type": "string", "description": "检索词，空格分词，词间为与关系" },
                "limit": { "type": "number", "description": "返回条数，默认 10" }
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
        let query = arg_str(&args, "query");
        if query.trim().is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "query 必填"));
        }
        let limit = arg_usize(&args, "limit", 10).clamp(1, 50);
        let store = self.store.clone();
        let hits = tokio::task::spawn_blocking(move || store.search(&query, limit))
            .await
            .map_err(|e| AppError::new("7001", e.to_string()))??;
        let mut out = String::new();
        for (i, h) in hits.iter().enumerate() {
            out.push_str(&format!(
                "{}. {} [{}] 更新 {}\n   {}\n",
                i + 1,
                h.memory_id,
                h.memory_kind,
                h.memory_updated_at,
                h.memory_summary
            ));
        }
        if out.is_empty() {
            out = "没有相关记忆".into();
        }
        Ok(ToolOutput {
            text: out,
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// memory_remove 工具：按 id 删除一条过时记忆。
pub struct MemoryRemoveTool {
    /// 共享记忆库
    store: Arc<MemoryStore>,
}

impl MemoryRemoveTool {
    /// 装配。
    pub fn new(store: Arc<MemoryStore>) -> Self {
        Self { store }
    }
}

#[async_trait]
impl Tool for MemoryRemoveTool {
    fn name(&self) -> &str {
        "memory_remove"
    }

    fn label(&self) -> &str {
        "记忆删除"
    }

    fn description(&self) -> &str {
        "按 memory_id 删除一条过时或错误的持久记忆。发现记忆与现实冲突时先删旧再重存；memory_id 来自 memory_search 的结果。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "memory_id": { "type": "string", "description": "要删除的记忆 id（MEM_ 前缀）" }
            },
            "required": ["memory_id"]
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
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let memory_id = arg_str(&args, "memory_id");
        if memory_id.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "memory_id 必填"));
        }
        let store = self.store.clone();
        let id_for_msg = memory_id.clone();
        let removed = tokio::task::spawn_blocking(move || store.remove(&memory_id))
            .await
            .map_err(|e| AppError::new("7001", e.to_string()))??;
        if !removed {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                format!("记忆 {id_for_msg} 不存在"),
            ));
        }
        Ok(ToolOutput {
            text: format!("已删除 {id_for_msg}"),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// session_search 工具：FTS5 检索历史会话消息正文。
pub struct SessionSearchTool {
    /// 运行上下文（定位会话目录）
    ctx: FeatureCtx,
}

impl SessionSearchTool {
    /// 装配。
    pub fn new(ctx: FeatureCtx) -> Self {
        Self { ctx }
    }
}

#[async_trait]
impl Tool for SessionSearchTool {
    fn name(&self) -> &str {
        "session_search"
    }

    fn label(&self) -> &str {
        "历史会话检索"
    }

    fn description(&self) -> &str {
        "全文检索历史会话记录（消息正文），用于找回\"上次聊过什么 / 之前试过什么方案\"这类上下文。返回会话文件名、行号与摘要，行号可在用户要求时用于人工定位。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "query": { "type": "string", "description": "检索词，空格分词，词间为与关系" },
                "limit": { "type": "number", "description": "返回条数，默认 10" }
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
        _cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let query = arg_str(&args, "query");
        if query.trim().is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "query 必填"));
        }
        let limit = arg_usize(&args, "limit", 10).clamp(1, 50);
        let dir = self.ctx.data_dir.join("sessions");
        let hits = tokio::task::spawn_blocking(move || search_sessions(&dir, &query, limit))
            .await
            .map_err(|e| AppError::new("7001", e.to_string()))??;
        let mut out = String::new();
        for (i, h) in hits.iter().enumerate() {
            out.push_str(&format!(
                "{}. {}:{}\n   {}\n",
                i + 1,
                h.session_file,
                h.line_no,
                h.line_summary
            ));
        }
        if out.is_empty() {
            out = "历史会话中没有匹配内容".into();
        }
        Ok(ToolOutput {
            text: out,
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}
