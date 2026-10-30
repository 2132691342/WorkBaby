//! LSP JSON-RPC 客户端：stdio 传输，按 Content-Length 分帧。
//! 生命周期与工具调用一致——一次调用内启停，不留常驻进程。

use std::path::Path;
use std::process::Stdio;

use serde_json::{json, Value};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::process::{Child, ChildStdin, ChildStdout};

use wb_pkg::{AppError, Result};

use super::edits::TextEdit;

/// CREATE_NO_WINDOW：子进程不闪控制台窗口（Windows 纪律）。
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

/// 一次诊断条目。
pub struct Diagnostic {
    /// 文件路径
    pub diag_path: String,
    /// 行号（1 起）
    pub diag_line: u32,
    /// 列号（1 起）
    pub diag_column: u32,
    /// 严重级（error / warning / info / hint）
    pub diag_severity: String,
    /// 诊断信息
    pub diag_message: String,
    /// 诊断来源（如 rust-analyzer）
    pub diag_source: String,
}

/// LSP 客户端。
pub struct LspClient {
    /// 子进程
    child: Option<Child>,
    /// stdin 写端
    stdin: Option<ChildStdin>,
    /// stdout 读端
    stdout: Option<BufReader<ChildStdout>>,
    /// 请求 id 计数
    next_id: i64,
    /// 服务器能力
    capabilities: Value,
    /// 启动命令（诊断信息里回显）
    command_text: String,
}

impl LspClient {
    /// 启动 server 并完成 initialize 握手。
    pub async fn start(command: &str, args: &[&str], root: &Path, timeout_ms: u64) -> Result<Self> {
        let mut cmd = tokio::process::Command::new(command);
        cmd.args(args)
            .current_dir(root)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::null())
            .kill_on_drop(true);
        #[cfg(windows)]
        cmd.creation_flags(CREATE_NO_WINDOW);
        let mut child = cmd.spawn().map_err(|e| {
            AppError::wrap(
                wb_domain::error::tool::LSP_START_FAILED,
                format!("无法启动语言服务器 {command}（未安装或不在 PATH）"),
                e,
            )
        })?;
        let stdin = child.stdin.take();
        let stdout = child.stdout.take().map(BufReader::new);
        let mut client = Self {
            child: Some(child),
            stdin,
            stdout,
            next_id: 1,
            capabilities: Value::Null,
            command_text: format!("{command} {}", args.join(" ")).trim().to_string(),
        };
        let root_uri = super::adapters::file_uri(root);
        let init = json!({
            "processId": std::process::id(),
            "rootUri": root_uri,
            "rootPath": root.display().to_string(),
            "capabilities": {
                "textDocument": {
                    "publishDiagnostics": { "relatedInformation": true },
                    "codeAction": {
                        "codeActionLiteralSupport": {
                            "codeActionKind": { "valueSet": ["source.fixAll"] }
                        }
                    }
                },
                "workspace": { "applyEdit": true }
            },
            "clientInfo": { "name": "WorkBaby", "version": env!("CARGO_PKG_VERSION") }
        });
        let resp = client.request("initialize", init, timeout_ms).await?;
        client.capabilities = resp.get("capabilities").cloned().unwrap_or(Value::Null);
        client.notify("initialized", json!({})).await?;
        Ok(client)
    }

    /// 发请求并等响应；超时或进程退出即报错。
    async fn request(&mut self, method: &str, params: Value, timeout_ms: u64) -> Result<Value> {
        let id = self.next_id;
        self.next_id += 1;
        let body = json!({"jsonrpc": "2.0", "id": id, "method": method, "params": params});
        self.send(&body).await?;
        let deadline = tokio::time::timeout(
            std::time::Duration::from_millis(timeout_ms.max(500)),
            self.read_response(id),
        );
        match deadline.await {
            Ok(Ok(v)) => Ok(v),
            Ok(Err(e)) => Err(e),
            Err(_) => Err(AppError::new(
                wb_domain::error::tool::EXEC_TIMEOUT,
                format!("语言服务器响应超时（{method}）"),
            )),
        }
    }

    /// 发通知（无响应）。
    async fn notify(&mut self, method: &str, params: Value) -> Result<()> {
        let body = json!({"jsonrpc": "2.0", "method": method, "params": params});
        self.send(&body).await
    }

    /// 写一帧 JSON-RPC 消息。
    async fn send(&mut self, body: &Value) -> Result<()> {
        let text = serde_json::to_string(body)
            .map_err(|e| AppError::new("7003", format!("序列化失败：{e}")))?;
        let stdin = self
            .stdin
            .as_mut()
            .ok_or_else(|| AppError::new("7003", "语言服务器 stdin 已关闭"))?;
        let frame = format!("Content-Length: {}\r\n\r\n{text}", text.len());
        stdin
            .write_all(frame.as_bytes())
            .await
            .map_err(|e| AppError::wrap("7003", "写语言服务器失败", e))?;
        stdin
            .flush()
            .await
            .map_err(|e| AppError::wrap("7003", "刷新语言服务器失败", e))
    }

    /// 读消息直到拿到目标 id 的响应（跳过服务端请求与通知）。
    async fn read_response(&mut self, want_id: i64) -> Result<Value> {
        loop {
            let msg = self.read_message().await?;
            let Some(obj) = msg.as_object() else {
                continue;
            };
            if obj.get("method").is_some() {
                // 服务端请求：回一个空结果，避免它等我们
                if let Some(id) = obj.get("id") {
                    let reply = json!({"jsonrpc": "2.0", "id": id, "result": Value::Null});
                    self.send(&reply).await.ok();
                }
                continue;
            }
            if obj.get("id").and_then(|v| v.as_i64()) == Some(want_id) {
                if let Some(err) = obj.get("error") {
                    let msg = err
                        .get("message")
                        .and_then(|v| v.as_str())
                        .unwrap_or("未知错误");
                    return Err(AppError::new(
                        super::codes::SERVER_ERROR,
                        format!("语言服务器报错：{msg}"),
                    ));
                }
                return Ok(obj.get("result").cloned().unwrap_or(Value::Null));
            }
        }
    }

    /// 读一帧消息（解析 Content-Length 头）。
    async fn read_message(&mut self) -> Result<Value> {
        let stdout = self
            .stdout
            .as_mut()
            .ok_or_else(|| AppError::new("7003", "语言服务器 stdout 已关闭"))?;
        let mut length = 0usize;
        loop {
            let mut line = String::new();
            let n = stdout
                .read_line(&mut line)
                .await
                .map_err(|e| AppError::wrap("7003", "读语言服务器失败", e))?;
            if n == 0 {
                return Err(AppError::new("7003", "语言服务器已退出"));
            }
            let trimmed = line.trim_end();
            if trimmed.is_empty() {
                break;
            }
            if let Some(v) = trimmed.strip_prefix("Content-Length:") {
                length = v.trim().parse().unwrap_or(0);
            }
        }
        if length == 0 {
            return Ok(Value::Null);
        }
        let mut buf = vec![0u8; length];
        stdout
            .read_exact(&mut buf)
            .await
            .map_err(|e| AppError::wrap("7003", "读语言服务器消息体失败", e))?;
        serde_json::from_slice(&buf)
            .map_err(|e| AppError::new("7003", format!("解析消息失败：{e}")))
    }

    /// 打开文档并拉取诊断；结束后关闭文档。
    pub async fn diagnostics(
        &mut self,
        path: &Path,
        text: &str,
        language: &str,
        timeout_ms: u64,
    ) -> Result<Vec<Diagnostic>> {
        let uri = super::adapters::file_uri(path);
        self.notify(
            "textDocument/didOpen",
            json!({
                "textDocument": { "uri": uri, "languageId": language, "version": 1, "text": text }
            }),
        )
        .await?;
        let resp = self
            .request(
                "textDocument/diagnostic",
                json!({ "textDocument": { "uri": uri } }),
                timeout_ms,
            )
            .await
            .unwrap_or(Value::Null);
        let mut out = Vec::new();
        if let Some(items) = resp.get("items").and_then(|v| v.as_array()) {
            for item in items {
                out.push(parse_diagnostic(path, item));
            }
        }
        // 关闭文档：server 收到 didClose 才会释放该文件的诊断缓存
        self.notify(
            "textDocument/didClose",
            json!({ "textDocument": { "uri": uri } }),
        )
        .await
        .ok();
        Ok(out)
    }

    /// 请求 code action 并收集文本编辑。
    pub async fn code_actions(
        &mut self,
        path: &Path,
        text: &str,
        language: &str,
        kind: &str,
        timeout_ms: u64,
    ) -> Result<Vec<TextEdit>> {
        let uri = super::adapters::file_uri(path);
        self.notify(
            "textDocument/didOpen",
            json!({
                "textDocument": { "uri": uri, "languageId": language, "version": 1, "text": text }
            }),
        )
        .await?;
        let resp = self
            .request(
                "textDocument/codeAction",
                json!({
                    "textDocument": { "uri": uri },
                    "range": {
                        "start": { "line": 0, "character": 0 },
                        "end": { "line": 0, "character": 0 }
                    },
                    "context": { "diagnostics": [], "only": [kind] }
                }),
                timeout_ms,
            )
            .await
            .unwrap_or(Value::Null);
        let mut edits = Vec::new();
        if let Some(actions) = resp.as_array() {
            for action in actions {
                if let Some(edit) = action.get("edit") {
                    super::edits::collect_edits(edit, &mut edits);
                }
            }
        }
        self.notify(
            "textDocument/didClose",
            json!({ "textDocument": { "uri": uri } }),
        )
        .await
        .ok();
        Ok(edits)
    }

    /// 服务器是否声明了某项能力。
    pub fn supports(&self, path: &str) -> bool {
        self.capabilities
            .pointer(path)
            .map(|v| !v.is_null() && v != &Value::Bool(false))
            .unwrap_or(false)
    }

    /// 启动命令文本（诊断信息里回显）。
    pub fn command_text(&self) -> &str {
        &self.command_text
    }

    /// 关闭 server。
    pub async fn shutdown(&mut self) {
        self.notify("shutdown", Value::Null).await.ok();
        self.notify("exit", Value::Null).await.ok();
        if let Some(mut child) = self.child.take() {
            child.kill().await.ok();
        }
        self.stdin = None;
        self.stdout = None;
    }
}

/// 解析一条诊断。
fn parse_diagnostic(path: &Path, item: &Value) -> Diagnostic {
    let line = item
        .pointer("/range/start/line")
        .and_then(|v| v.as_u64())
        .unwrap_or(0) as u32
        + 1;
    let column = item
        .pointer("/range/start/character")
        .and_then(|v| v.as_u64())
        .unwrap_or(0) as u32
        + 1;
    let severity = match item.get("severity").and_then(|v| v.as_u64()) {
        Some(1) => "error",
        Some(2) => "warning",
        Some(3) => "info",
        Some(4) => "hint",
        _ => "unknown",
    };
    Diagnostic {
        diag_path: path.display().to_string(),
        diag_line: line,
        diag_column: column,
        diag_severity: severity.to_string(),
        diag_message: item
            .get("message")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string(),
        diag_source: item
            .get("source")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string(),
    }
}
