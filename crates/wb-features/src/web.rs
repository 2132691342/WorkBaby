//! 联网抓取与搜索两个工具；HTML 解析与正文提取见 web_parse。

use std::future::Future;
use std::path::Path;
use std::time::{Duration, Instant};

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolAnnotations, ToolOutput};

use crate::web_parse::readable_text;
use crate::web_providers;

/// 统一 UA，便于目标方识别流量来源。
pub const USER_AGENT: &str = "WorkBaby/2.0";
/// 单请求超时。
const HTTP_TIMEOUT: Duration = Duration::from_secs(10);
/// 最多跟随的重定向次数。
const MAX_REDIRECTS: usize = 3;

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

/// 复用的 HTTP 客户端（超时 / UA / 重定向上限统一在此）。
fn http_client() -> reqwest::Client {
    reqwest::Client::builder()
        .user_agent(USER_AGENT)
        .timeout(HTTP_TIMEOUT)
        .redirect(reqwest::redirect::Policy::limited(MAX_REDIRECTS))
        .build()
        .expect("HTTP client 构建失败")
}

/// 等待响应（可取消），校验 2xx，返回 (正文, content-type)。
async fn finish(
    send: impl Future<Output = reqwest::Result<reqwest::Response>>,
    cancel: &CancellationToken,
) -> Result<(String, String)> {
    let mut send = Box::pin(send);
    let resp = tokio::select! {
        r = send.as_mut() => r,
        _ = cancel.cancelled() => {
            return Err(AppError::new(toolcode::EXEC_CANCELLED, "请求已随会话取消"));
        }
    }
    .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("请求失败：{e}")))?;
    let status = resp.status();
    if !status.is_success() {
        return Err(AppError::new(
            toolcode::ARG_INVALID,
            format!("目标返回非 2xx 状态：{}", status.as_u16()),
        ));
    }
    let content_type = resp
        .headers()
        .get(reqwest::header::CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
        .to_string();
    let body = resp
        .text()
        .await
        .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("读取响应失败：{e}")))?;
    Ok((body, content_type))
}

/// 按 content-type（缺省时按内容嗅探）分类：passthrough 原样返回，html 走正文提取。
pub fn classify(content_type: &str, body: &str) -> &'static str {
    let ct = content_type.to_lowercase();
    if ct.contains("json") || ct.contains("javascript") {
        return "passthrough";
    }
    if ct.contains("html") {
        return "html";
    }
    if ct.starts_with("text/") {
        return "passthrough";
    }
    let head = body.trim_start().to_lowercase();
    if head.starts_with("<!doctype html") || head.starts_with("<html") {
        return "html";
    }
    if ct.is_empty() {
        return "passthrough";
    }
    "other"
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

/// fetch_content 工具：抓取网页并按模式返回文本。
pub struct FetchTool {
    /// 复用的 HTTP 客户端
    http: reqwest::Client,
}

impl FetchTool {
    /// 装配。
    pub fn new() -> Self {
        Self {
            http: http_client(),
        }
    }
}

impl Default for FetchTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl Tool for FetchTool {
    fn name(&self) -> &str {
        "fetch_content"
    }

    fn label(&self) -> &str {
        "网页抓取"
    }

    fn description(&self) -> &str {
        "抓取一个 http/https 地址并返回文本内容。读文档页面、API 响应、文章正文时使用；mode=readable 提取 HTML 正文（默认，脚本/导航等干扰已剥除），raw 返回原始内容，answer 等价 readable（由你基于正文作答）。max_chars 限制返回长度。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "url": { "type": "string", "description": "http/https 地址，其它 scheme 会被拒绝" },
                "mode": { "type": "string", "enum": ["readable", "raw", "answer"], "description": "提取模式，默认 readable" },
                "max_chars": { "type": "number", "description": "返回的最大字符数，默认 8000" }
            },
            "required": ["url"]
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
        let url = arg_str(&args, "url");
        if !(url.to_lowercase().starts_with("http://")
            || url.to_lowercase().starts_with("https://"))
        {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                "只允许 http/https 地址",
            ));
        }
        let mode = {
            let m = arg_str(&args, "mode");
            if m.is_empty() {
                "readable".to_string()
            } else {
                m
            }
        };
        if !matches!(mode.as_str(), "readable" | "raw" | "answer") {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                "mode 只能是 readable / raw / answer",
            ));
        }
        let max_chars = arg_usize(&args, "max_chars", 8000).clamp(200, 200_000);
        let (body, content_type) = finish(self.http.get(&url).send(), &cancel).await?;
        let text = match classify(&content_type, &body) {
            "passthrough" => body,
            "html" => {
                if mode == "raw" {
                    body
                } else {
                    readable_text(&body)
                }
            }
            _ => {
                return Err(AppError::new(
                    toolcode::ARG_INVALID,
                    format!("不支持的 content-type：{content_type}"),
                ))
            }
        };
        Ok(ToolOutput {
            text: truncate_chars(text.trim(), max_chars),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// web_search 工具：多搜索源检索。
pub struct SearchTool;

impl SearchTool {
    /// 装配。
    pub fn new() -> Self {
        Self
    }
}

impl Default for SearchTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl Tool for SearchTool {
    fn name(&self) -> &str {
        "web_search"
    }

    fn label(&self) -> &str {
        "联网搜索"
    }

    fn description(&self) -> &str {
        "联网搜索并返回编号的结果清单（标题 / URL / 摘要）。需要最新信息、外部文档或事实核查而本地上下文不足时使用；拿到结果后通常再用 fetch_content 抓取目标页面。\
         默认按优先级尝试：配了密钥的 API 源（brave / tavily / serper / exa）优先，\
         再回落到公开抓取源（duckduckgo / bing / wikipedia），第一个解析出结果即返回；\
         API 源靠环境变量供密钥（BRAVE_API_KEY / TAVILY_API_KEY / SERPER_API_KEY / EXA_API_KEY），未配则自动跳过。\
         可用 provider 指定单一搜索源。搜索源解析失败时会如实说明。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "query": { "type": "string", "description": "搜索词" },
                "limit": { "type": "number", "description": "返回条数，默认 8" },
                "provider": {
                    "type": "string",
                    "enum": web_providers::codes(),
                    "description": "指定搜索源；不填则按优先级依次尝试"
                }
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
        let query = arg_str(&args, "query");
        if query.trim().is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "query 必填"));
        }
        let limit = arg_usize(&args, "limit", 8).clamp(1, 20);
        let wanted = arg_str(&args, "provider");
        if !wanted.is_empty() && web_providers::by_code(&wanted).is_none() {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                format!(
                    "未知搜索源 {wanted}；可用：{}",
                    web_providers::codes().join(" / ")
                ),
            ));
        }
        // 排队顺序由 providers 表给出：有密钥的 API 源在前，公开抓取源兜底。
        // 显式指定了要密钥的源却没配：直接说清缺哪个环境变量，
        // 不要绕到「所有搜索源都无法解析」那种误导向的结论。
        let candidates = web_providers::queue_for(&wanted);
        if let Some((p, None)) = candidates.first() {
            if !wanted.is_empty() {
                return Err(AppError::new(
                    toolcode::ARG_INVALID,
                    format!(
                        "搜索源 {} 需要密钥：设置环境变量 {} 后重试",
                        p.provider_label,
                        p.provider_env_keys.join(" 或 ")
                    ),
                ));
            }
        }

        let mut failures: Vec<String> = Vec::new();
        for (p, key) in candidates {
            let body = finish((p.build_fn)(&query, key.as_deref()).send(), &cancel).await;
            let (body, _) = match body {
                Ok(v) => v,
                Err(e) => {
                    failures.push(format!("{}: {}", p.provider_label, e.message));
                    continue;
                }
            };
            let results = (p.parse_fn)(&body);
            if results.is_empty() {
                failures.push(format!("{}: 结果页无法解析", p.provider_label));
                continue;
            }
            let mut out = format!("（搜索源：{}）\n", p.provider_label);
            for (i, r) in results.iter().take(limit).enumerate() {
                out.push_str(&format!(
                    "{}. {}\n  {}\n  {}\n",
                    i + 1,
                    r.result_title,
                    r.result_url,
                    r.result_snippet
                ));
            }
            return Ok(ToolOutput {
                text: out,
                is_error: false,
                duration_ms: started.elapsed().as_millis() as i64,
            });
        }
        Ok(ToolOutput {
            text: format!(
                "所有搜索源都没有返回可解析的结果：\n{}",
                failures.join("\n")
            ),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}
