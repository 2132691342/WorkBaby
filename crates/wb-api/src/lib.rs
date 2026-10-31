//! 组合根：装配 Services + Hub + axum，返回端口。壳层与独立调试 bin 共用。

pub mod chat_routes;
pub mod cors;
pub mod git_routes;
pub mod hub;
pub mod mcp_routes;
pub mod routes;
pub mod session_routes;
pub mod system_routes;

use std::path::PathBuf;
use std::sync::Arc;

use wb_pkg::Result;

pub use hub::{Frame, Hub};
pub use routes::AppState;

/// 启动结果：HTTP 端口 + 服务状态。
pub struct Startup {
    /// 监听端口（127.0.0.1 随机）
    pub port: u16,
    /// 服务容器（壳层要再引用）
    pub sv: Arc<wb_service::Services>,
    /// Hub（SSE 出口）
    pub hub: Arc<Hub>,
}

/// 数据目录：`%APPDATA%\WorkBaby`。
pub fn data_dir() -> PathBuf {
    wb_domain::runtime::default_data_dir().unwrap_or_else(|| PathBuf::from("."))
}

/// 装配并起 HTTP 服务（tokio runtime 已由调用方提供）。
pub async fn startup(data_dir: PathBuf, workspace: String) -> Result<Startup> {
    wb_pkg::fsutil::ensure_dir(&data_dir)?;
    let repo = Arc::new(wb_repo::Repo::open(&data_dir.join("settings.db"))?);
    // 工作区以设置 KV 为准：界面上选过的目录重启后继续生效
    let workspace = resolve_workspace(&repo, workspace);
    let sv = wb_service::Services::new(data_dir, workspace, repo);
    let hub = Hub::new();
    sv.set_emitter(hub.clone());
    // MCP server 后台连接并并入工具注册表；失败不阻塞启动
    wb_service::mcp::install(&sv);
    // 功能工具（todo / ask_user 等）按开关并入注册表
    wb_service::features::install(&sv);
    // 历史会话用量一次性回填（幂等）
    wb_service::usage::backfill(&sv);
    let state = AppState {
        sv: sv.clone(),
        hub: hub.clone(),
    };
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0")
        .await
        .map_err(|e| wb_pkg::AppError::wrap("2004", "本地端口监听失败", e))?;
    let port = listener
        .local_addr()
        .map_err(|e| wb_pkg::AppError::wrap("2004", "读取端口失败", e))?
        .port();
    tokio::spawn(async move {
        axum::serve(listener, routes::router(state)).await.ok();
    });
    Ok(Startup { port, sv, hub })
}

/// 工作区解析：设置 KV 里存过且目录仍存在则用之；否则用传入目录并写回 KV 作初始值。
fn resolve_workspace(repo: &Arc<wb_repo::Repo>, fallback: String) -> String {
    if let Ok(Some(dir)) = repo.settings_get("workspace") {
        if !dir.trim().is_empty() && std::path::Path::new(&dir).is_dir() {
            return dir;
        }
    }
    let _ = repo.settings_set("workspace", &fallback);
    fallback
}
