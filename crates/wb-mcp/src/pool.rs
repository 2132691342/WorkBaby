//! 连接池：按配置连 MCP server，缓存工具清单；断线不自动重连（docs/MCP.md）。

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use rmcp::model::{CallToolRequestParams, CallToolResult};
use rmcp::service::{RoleClient, RunningService};
use rmcp::transport::streamable_http_client::StreamableHttpClientTransportConfig;
use rmcp::transport::{
    StreamableHttpClientTransport, TokioChildProcess, Transport as ClientTransport,
};
use rmcp::ServiceExt;
use tokio::process::Command;
use wb_domain::error::mcp as code;
use wb_domain::mcp::McpServerConfig;
use wb_pkg::{AppError, Result};

/// 一条连接的运行态。
pub enum Conn {
    /// 已连接：service 句柄 + 启动时抓到的工具清单
    Connected {
        /// rmcp 客户端句柄
        service: Arc<RunningService<RoleClient, ()>>,
        /// 工具名与描述（启动快照）
        tools: Vec<(String, String, Option<serde_json::Value>)>,
    },
    /// 连接失败（原因展示用）
    Failed(String),
}

/// 全部 server 的连接池。
#[derive(Default)]
pub struct Pool {
    /// server 名 → 运行态
    pub conns: Mutex<BTreeMap<String, Conn>>,
}

/// 连接单个 server（stdio / http 分流）；超时转 9102，失败不 panic。
pub async fn connect(
    name: &str,
    cfg: &McpServerConfig,
) -> Result<(
    Arc<RunningService<RoleClient, ()>>,
    Vec<(String, String, Option<serde_json::Value>)>,
)> {
    // 未配置按 10 秒起步：1 秒对多数 MCP server 的启动握手太紧
    let timeout = Duration::from_secs(if cfg.timeout > 0 { cfg.timeout } else { 10 });
    match cfg.transport.as_str() {
        "stdio" => {
            let mut cmd = Command::new(&cfg.command);
            cmd.args(&cfg.args).envs(&cfg.env);
            #[cfg(windows)]
            cmd.creation_flags(0x0800_0000); // CREATE_NO_WINDOW
            let transport = TokioChildProcess::new(cmd)
                .map_err(|e| AppError::new(code::START_FAILED, format!("{name}: 启动失败 {e}")))?;
            serve_with_timeout(name, transport, timeout).await
        }
        "http" => {
            let mut headers = std::collections::HashMap::new();
            for (k, v) in &cfg.headers {
                let name = http::HeaderName::from_bytes(k.as_bytes()).map_err(|e| {
                    AppError::new(
                        code::CONFIG_INVALID,
                        format!("{name}: 请求头名非法 {k}：{e}"),
                    )
                })?;
                let value = http::HeaderValue::from_str(v).map_err(|e| {
                    AppError::new(
                        code::CONFIG_INVALID,
                        format!("{name}: 请求头值非法 {k}：{e}"),
                    )
                })?;
                headers.insert(name, value);
            }
            let tcfg = StreamableHttpClientTransportConfig::with_uri(cfg.url.clone())
                .custom_headers(headers);
            let transport = StreamableHttpClientTransport::from_config(tcfg);
            serve_with_timeout(name, transport, timeout).await
        }
        other => Err(AppError::new(
            code::CONFIG_INVALID,
            format!("{name}: 不认识的 transport {other}"),
        )),
    }
}

/// 初始化握手 + 抓工具清单（限时）。
async fn serve_with_timeout<T: ClientTransport<RoleClient> + 'static>(
    name: &str,
    transport: T,
    timeout: Duration,
) -> Result<(
    Arc<RunningService<RoleClient, ()>>,
    Vec<(String, String, Option<serde_json::Value>)>,
)> {
    let service = tokio::time::timeout(timeout, ().serve(transport))
        .await
        .map_err(|_| AppError::new(code::START_FAILED, format!("{name}: 连接超时")))?
        .map_err(|e| AppError::new(code::START_FAILED, format!("{name}: 握手失败 {e}")))?;
    let listed = tokio::time::timeout(timeout, service.list_tools(None))
        .await
        .map_err(|_| AppError::new(code::START_FAILED, format!("{name}: 工具清单超时")))?
        .map_err(|e| AppError::new(code::START_FAILED, format!("{name}: 工具清单失败 {e}")))?;
    let tools = listed
        .tools
        .into_iter()
        .map(|t| {
            (
                t.name.into_owned(),
                t.description.map(|d| d.into_owned()).unwrap_or_default(),
                Some(serde_json::Value::Object((*t.input_schema).clone())),
            )
        })
        .collect();
    Ok((Arc::new(service), tools))
}

/// 调用工具：内容块拼成文本；is_error 转 9103。
pub async fn call(
    service: &RunningService<RoleClient, ()>,
    tool: &str,
    args: Option<serde_json::Map<String, serde_json::Value>>,
) -> Result<CallToolResult> {
    let params =
        CallToolRequestParams::new(tool.to_string()).with_arguments(args.unwrap_or_default());
    let result = service
        .call_tool(params)
        .await
        .map_err(|e| AppError::new(code::CALL_FAILED, format!("工具 {tool} 调用失败：{e}")))?;
    if result.is_error.unwrap_or(false) {
        let text = flatten_text(&result);
        return Err(AppError::new(code::CALL_FAILED, text));
    }
    Ok(result)
}

/// 结果内容块拼文本（text 块原文；其余块给占位说明）。
pub fn flatten_text(result: &CallToolResult) -> String {
    let mut out = String::new();
    for block in &result.content {
        match block {
            rmcp::model::ContentBlock::Text(t) => {
                out.push_str(&t.text);
                out.push('\n');
            }
            rmcp::model::ContentBlock::Image(i) => {
                out.push_str(&format!(
                    "[图片 {} 字节，mime {}]\n",
                    i.data.len(),
                    i.mime_type
                ));
            }
            rmcp::model::ContentBlock::Audio(a) => {
                out.push_str(&format!(
                    "[音频 {} 字节，mime {}]\n",
                    a.data.len(),
                    a.mime_type
                ));
            }
            rmcp::model::ContentBlock::Resource(_) => out.push_str("[嵌入资源]\n"),
            rmcp::model::ContentBlock::ResourceLink(r) => {
                out.push_str(&format!("[资源链接 {}]\n", r.name));
            }
            _ => out.push_str("[未知内容块]\n"),
        }
    }
    out.trim_end().to_string()
}

impl Pool {
    /// 空池。
    pub fn new() -> Arc<Self> {
        Arc::new(Self::default())
    }

    /// 记一条失败（连接编排里用）。
    pub fn mark_failed(&self, name: &str, err: String) {
        self.conns
            .lock()
            .unwrap()
            .insert(name.to_string(), Conn::Failed(err));
    }

    /// 记一条成功连接。
    pub fn mark_connected(
        &self,
        name: &str,
        service: Arc<RunningService<RoleClient, ()>>,
        tools: Vec<(String, String, Option<serde_json::Value>)>,
    ) {
        self.conns
            .lock()
            .unwrap()
            .insert(name.to_string(), Conn::Connected { service, tools });
    }

    /// 取一条连接的 service 句柄。
    pub fn service(&self, name: &str) -> Result<Arc<RunningService<RoleClient, ()>>> {
        match self.conns.lock().unwrap().get(name) {
            Some(Conn::Connected { service, .. }) => Ok(service.clone()),
            Some(Conn::Failed(e)) => Err(AppError::new(code::START_FAILED, e.clone())),
            None => Err(AppError::new(code::START_FAILED, format!("{name}: 未连接"))),
        }
    }

    /// 断开全部连接（reload 前调用）。
    pub fn shutdown_all(&self) {
        let mut conns = self.conns.lock().unwrap();
        let names: Vec<String> = conns.keys().cloned().collect();
        for name in names {
            if let Some(Conn::Connected { service, .. }) = conns.remove(&name) {
                if let Ok(svc) = Arc::try_unwrap(service) {
                    // 独占句柄：优雅取消；否则随最后一个引用 drop 时的清理退出
                    tokio::spawn(async move { svc.cancel().await.ok() });
                }
            }
        }
    }
}
