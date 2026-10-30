//! PDF 文本抽取与 GitHub 仓库克隆两个工具。
//!
//! PDF 走本地解析（不外传文件）；GitHub 克隆走 git，浅克隆到工作区。

use std::path::Path;
use std::process::Stdio;
use std::time::{Duration, Instant};

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolAnnotations, ToolOutput};

/// 下载大小上限（PDF 走内存，超过就拒）。
const MAX_PDF_BYTES: usize = 64 * 1024 * 1024;
/// 克隆超时。
const CLONE_TIMEOUT: Duration = Duration::from_secs(300);
/// CREATE_NO_WINDOW：子进程不闪控制台窗口（Windows 纪律）。
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

/// pdf_read 工具：抽取 PDF 的文字内容。
pub struct PdfReadTool {
    http: reqwest::Client,
}

impl PdfReadTool {
    /// 装配。
    pub fn new() -> Self {
        Self {
            http: reqwest::Client::builder()
                .user_agent(crate::web::USER_AGENT)
                .timeout(Duration::from_secs(30))
                .build()
                .expect("HTTP client 构建失败"),
        }
    }
}

impl Default for PdfReadTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl Tool for PdfReadTool {
    fn name(&self) -> &str {
        "pdf_read"
    }

    fn label(&self) -> &str {
        "PDF 取文"
    }

    fn description(&self) -> &str {
        "读取 PDF 的文字内容。source 可以是本地路径或 http/https 地址（远程文件先下载再解析）。\
         扫描版 PDF 没有文字层，会如实说明而不是编造内容。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "source": { "type": "string", "description": "PDF 路径或 http/https 地址" },
                "max_chars": { "type": "number", "description": "返回的最大字符数，默认 20000" },
                "pages": { "type": "string", "description": "页码范围，如 1-5（缺省全部）" }
            },
            "required": ["source"]
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
        let source = args
            .get("source")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        if source.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "source 必填"));
        }
        let max_chars = args
            .get("max_chars")
            .and_then(|v| v.as_u64())
            .unwrap_or(20_000)
            .clamp(200, 500_000) as usize;

        let bytes = if source.to_lowercase().starts_with("http") {
            fetch_bytes(&self.http, &source, &cancel).await?
        } else {
            let p = Path::new(&source);
            let meta = std::fs::metadata(p)
                .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("读不到文件：{e}")))?;
            if meta.len() as usize > MAX_PDF_BYTES {
                return Err(AppError::new(toolcode::ARG_INVALID, "PDF 超过 64MB 上限"));
            }
            std::fs::read(p)
                .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("读文件失败：{e}")))?
        };

        let text = tokio::task::spawn_blocking(move || pdf_extract::extract_text_from_mem(&bytes))
            .await
            .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("PDF 解析任务失败：{e}")))?
            .map_err(|e| {
                AppError::new(
                    toolcode::ARG_INVALID,
                    format!("PDF 解析失败（可能是扫描版，没有文字层）：{e}"),
                )
            })?;

        let text = text.trim().to_string();
        if text.is_empty() {
            return Ok(ToolOutput {
                text: "PDF 没有可抽取的文字（可能是扫描版，需要 OCR）".into(),
                is_error: false,
                duration_ms: started.elapsed().as_millis() as i64,
            });
        }
        Ok(ToolOutput {
            text: truncate_chars(&text, max_chars),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// github_clone 工具：把公开仓库浅克隆到工作区。
pub struct GithubCloneTool;

#[async_trait]
impl Tool for GithubCloneTool {
    fn name(&self) -> &str {
        "github_clone"
    }

    fn label(&self) -> &str {
        "克隆仓库"
    }

    fn description(&self) -> &str {
        "把一个公开的 GitHub 仓库浅克隆到工作区的目标目录（depth=1，不拉历史）。\
         需要看别人的项目代码时用它；仓库很大时先确认目标目录不会覆盖已有内容。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "repo": { "type": "string", "description": "owner/name 形式的仓库标识" },
                "dest": { "type": "string", "description": "目标目录名（缺省用仓库名）" },
                "branch": { "type": "string", "description": "分支或 tag（缺省默认分支）" }
            },
            "required": ["repo"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let repo = args
            .get("repo")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        // 仓库标识只允许 owner/name 形态：拼进 URL 前先卡住，避免注入。
        let mut parts = repo.split('/');
        let owner = parts.next().unwrap_or("");
        let name = parts.next().unwrap_or("");
        if parts.next().is_some()
            || owner.is_empty()
            || name.is_empty()
            || !owner
                .chars()
                .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_')
            || !name
                .chars()
                .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_' || c == '.')
        {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                "repo 应为 owner/name 形式的公开仓库标识",
            ));
        }
        let dest = args
            .get("dest")
            .and_then(|v| v.as_str())
            .map(|s| s.trim().to_string())
            .filter(|s| !s.is_empty())
            .unwrap_or_else(|| name.to_string());
        if dest.contains(['/', '\\', ':']) || dest.contains("..") {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                "dest 只能是工作区下的目录名",
            ));
        }
        let target = workdir.join(&dest);
        if target.exists() {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                format!("目标目录已存在：{dest}"),
            ));
        }

        let url = format!("https://github.com/{owner}/{name}.git");
        let mut argv = vec![
            "clone".to_string(),
            "--depth".to_string(),
            "1".to_string(),
            url.clone(),
            dest.clone(),
        ];
        if let Some(branch) = args.get("branch").and_then(|v| v.as_str()) {
            if branch.is_empty() || branch.contains(char::is_whitespace) || branch.starts_with('-')
            {
                return Err(AppError::new(toolcode::ARG_INVALID, "branch 非法"));
            }
            argv.push("--branch".to_string());
            argv.push(branch.to_string());
        }

        let mut cmd = tokio::process::Command::new("git");
        cmd.args(&argv)
            .current_dir(workdir)
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .kill_on_drop(true);
        #[cfg(windows)]
        cmd.creation_flags(CREATE_NO_WINDOW);
        let mut child = cmd
            .spawn()
            .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("启动 git 失败：{e}")))?;
        let status = tokio::select! {
            r = child.wait() => r
                .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("git 失败：{e}")))?,
            _ = tokio::time::sleep(CLONE_TIMEOUT) => {
                return Err(AppError::new(toolcode::EXEC_TIMEOUT, "克隆超过 300s 被终止"));
            }
            _ = cancel.cancelled() => {
                return Err(AppError::new(toolcode::EXEC_CANCELLED, "克隆已随会话取消"));
            }
        };
        if !status.success() {
            let _ = std::fs::remove_dir_all(&target);
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                format!("git clone 失败（退出码 {:?}）", status.code()),
            ));
        }
        Ok(ToolOutput {
            text: format!("已克隆 {url} 到 {}/{}", workdir.display(), dest),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// 下载远程文件（带大小上限）。
async fn fetch_bytes(
    http: &reqwest::Client,
    url: &str,
    cancel: &CancellationToken,
) -> Result<Vec<u8>> {
    let resp = tokio::select! {
        r = http.get(url).send() => r,
        _ = cancel.cancelled() => {
            return Err(AppError::new(toolcode::EXEC_CANCELLED, "下载已随会话取消"));
        }
    }
    .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("下载失败：{e}")))?;
    if !resp.status().is_success() {
        return Err(AppError::new(
            toolcode::ARG_INVALID,
            format!("目标返回非 2xx 状态：{}", resp.status().as_u16()),
        ));
    }
    if let Some(len) = resp.content_length() {
        if len as usize > MAX_PDF_BYTES {
            return Err(AppError::new(toolcode::ARG_INVALID, "文件超过 64MB 上限"));
        }
    }
    let bytes = resp
        .bytes()
        .await
        .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("读取响应失败：{e}")))?;
    if bytes.len() > MAX_PDF_BYTES {
        return Err(AppError::new(toolcode::ARG_INVALID, "文件超过 64MB 上限"));
    }
    Ok(bytes.to_vec())
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
