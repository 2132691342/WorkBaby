//! MCP 编排：连接、工具并入注册表（mcp__server__tool）、配置读写与 reload。

use std::path::Path;
use std::sync::Arc;
use std::time::{Duration, Instant};

use async_trait::async_trait;
use serde_json::Value;
use tokio_util::sync::CancellationToken;
use wb_domain::error::{mcp as mcpcode, tool as toolcode};
use wb_domain::mcp::{McpListRESP, McpNameREQ, McpSaveREQ, McpServerVO};
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

use super::Services;

/// 单次工具调用超时。
const CALL_TIMEOUT: Duration = Duration::from_secs(60);

/// MCP 工具桥：远端工具挂进本仓 Tool 契约，内核只见普通工具。
struct McpBridge {
    pool: Arc<wb_mcp::Pool>,
    /// 全名（mcp__<server>__<tool>），即注册表身份
    full_name: String,
    server_name: String,
    tool_name: String,
    tool_label: String,
    tool_description: String,
    tool_schema: Value,
}

#[async_trait]
impl Tool for McpBridge {
    fn name(&self) -> &str {
        // 全名即身份：mcp__<server>__<tool>
        &self.full_name
    }

    fn label(&self) -> &str {
        &self.tool_label
    }

    fn description(&self) -> &str {
        &self.tool_description
    }

    fn parameters(&self) -> Value {
        self.tool_schema.clone()
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let service = self.pool.service(&self.server_name)?;
        let args = args.as_object().cloned();
        let call = wb_mcp::pool::call(&service, &self.tool_name, args);
        tokio::pin!(call);
        let result = tokio::select! {
            r = &mut call => match r {
                Ok(v) => v,
                Err(e) => return Err(e),
            },
            _ = tokio::time::sleep(CALL_TIMEOUT) => {
                return Err(AppError::new(toolcode::EXEC_TIMEOUT, format!("工具 {} 超过 60s", self.full_name)));
            }
            _ = cancel.cancelled() => {
                return Err(AppError::new(toolcode::EXEC_CANCELLED, "工具调用已取消"));
            }
        };
        Ok(ToolOutput {
            text: wb_mcp::pool::flatten_text(&result),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

/// 把已连接 server 的工具并入注册表（注册表组装统一在 features::build_registry）。
pub(crate) fn install_mcp_tools(sv: &Arc<Services>, reg: &mut wb_tools::ToolRegistry) {
    let conns = sv.mcp.conns.lock().unwrap();
    for (server, conn) in conns.iter() {
        if let wb_mcp::pool::Conn::Connected { tools, .. } = conn {
            for (tname, desc, schema) in tools {
                reg.register(Arc::new(McpBridge {
                    pool: sv.mcp.clone(),
                    full_name: format!("mcp__{server}__{tname}"),
                    server_name: server.clone(),
                    tool_name: tname.clone(),
                    tool_label: tname.clone(),
                    tool_description: desc.clone(),
                    tool_schema: schema
                        .clone()
                        .unwrap_or(serde_json::Value::Object(Default::default())),
                }));
            }
        }
    }
}

/// 启动装配：后台并发连接 enabled 且非 hidden 的 server。
pub fn install(sv: &Arc<Services>) {
    let sv = Arc::clone(sv);
    tokio::spawn(async move {
        let _ = reload(&sv).await;
    });
}

/// 重载：断开全部 → 重新连接 → 重建注册表。
pub async fn reload(sv: &Arc<Services>) -> Result<()> {
    sv.mcp.shutdown_all();
    let ws = sv.workspace.lock().unwrap().clone();
    let cfg = wb_mcp::McpConfig::load(&sv.data_dir, Path::new(&ws));
    let targets: Vec<_> = cfg
        .merged()
        .into_iter()
        .filter(|(_, c, _)| c.enabled && c.exposure != "hidden")
        .collect();
    let mut jobs = Vec::new();
    for (name, scfg, _) in &targets {
        let pool = sv.mcp.clone();
        let name = name.clone();
        let scfg = scfg.clone();
        jobs.push(tokio::spawn(async move {
            match wb_mcp::pool::connect(&name, &scfg).await {
                Ok((service, tools)) => {
                    pool.mark_connected(&name, service, tools);
                    Ok(name)
                }
                Err(e) => {
                    pool.mark_failed(&name, e.message.clone());
                    Err((name, e.message))
                }
            }
        }));
    }
    let mut first_err = None;
    for job in jobs {
        if let Ok(Err((_, msg))) = job.await {
            first_err.get_or_insert(msg);
        }
    }
    super::features::refresh_registry(sv);
    match first_err {
        Some(msg) => Err(AppError::new(mcpcode::START_FAILED, msg)),
        None => Ok(()),
    }
}

/// 清单视图（面板）。
pub fn list_vo(sv: &Arc<Services>) -> McpListRESP {
    let ws = sv.workspace.lock().unwrap().clone();
    let cfg = wb_mcp::McpConfig::load(&sv.data_dir, Path::new(&ws));
    let conns = sv.mcp.conns.lock().unwrap();
    let mut servers = Vec::new();
    for (name, scfg, scope) in cfg.merged() {
        let (state, mcp_error, mcp_tools) = match conns.get(&name) {
            Some(wb_mcp::pool::Conn::Connected { tools, .. }) => (
                "connected",
                String::new(),
                tools
                    .iter()
                    .map(|(n, _, _)| format!("mcp__{name}__{n}"))
                    .collect::<Vec<_>>(),
            ),
            Some(wb_mcp::pool::Conn::Failed(e)) => ("error", e.clone(), Vec::new()),
            None => ("unknown", String::new(), Vec::new()),
        };
        let source = match scope {
            wb_mcp::Scope::Global => cfg.global_path.display().to_string(),
            wb_mcp::Scope::Workspace => cfg.workspace_path.display().to_string(),
        };
        let target = if scfg.transport == "http" {
            scfg.url.clone()
        } else {
            let mut cmd = scfg.command.clone();
            for a in &scfg.args {
                cmd.push(' ');
                cmd.push_str(a);
            }
            cmd
        };
        servers.push(McpServerVO {
            mcp_name: name,
            mcp_scope: scope.as_str().to_string(),
            mcp_source: source,
            mcp_enabled: scfg.enabled,
            mcp_exposure: scfg.exposure.clone(),
            mcp_transport: scfg.transport.clone(),
            mcp_target: target,
            mcp_state: state.to_string(),
            mcp_error,
            mcp_tools,
        });
    }
    McpListRESP {
        resp_servers: servers,
        resp_global_path: cfg.global_path.display().to_string(),
        resp_workspace_path: cfg.workspace_path.display().to_string(),
        resp_global_raw: cfg.raw(wb_mcp::Scope::Global),
        resp_workspace_raw: cfg.raw(wb_mcp::Scope::Workspace),
    }
}

/// 保存配置并重载。
pub async fn save(sv: &Arc<Services>, req: &McpSaveREQ) -> Result<()> {
    let ws = sv.workspace.lock().unwrap().clone();
    let cfg = wb_mcp::McpConfig::load(&sv.data_dir, Path::new(&ws));
    let path = match req.mcp_scope.as_str() {
        "workspace" => cfg.workspace_path.clone(),
        _ => cfg.global_path.clone(),
    };
    wb_mcp::config::save_raw(&path, &req.mcp_raw)?;
    reload(sv).await
}

/// 移除 server 并重载。
pub async fn remove(sv: &Arc<Services>, req: &McpNameREQ) -> Result<()> {
    let ws = sv.workspace.lock().unwrap().clone();
    let cfg = wb_mcp::McpConfig::load(&sv.data_dir, Path::new(&ws));
    let path = match req.mcp_scope.as_str() {
        "workspace" => cfg.workspace_path.clone(),
        _ => cfg.global_path.clone(),
    };
    wb_mcp::config::remove_server(&path, &req.mcp_name)?;
    reload(sv).await
}

/// OAuth 登录：v1 不支持（远端鉴权 server 用 header / 环境变量凭据）。
pub async fn login(_sv: &Arc<Services>, _req: &McpNameREQ) -> Result<()> {
    Err(AppError::new(
        mcpcode::CONFIG_INVALID,
        "OAuth 登录在 v1 暂不支持，请在配置里用 headers 传凭据",
    ))
}
