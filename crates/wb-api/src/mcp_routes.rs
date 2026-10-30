//! MCP 路由：清单 / 保存 / 移除 / 重载（薄转发 wb_service::mcp）。

use std::sync::Arc;

use axum::extract::State;
use axum::response::Response;
use axum::routing::{get, post};
use axum::Router;

use wb_domain::mcp::{McpNameREQ, McpSaveREQ};
use wb_service::Services;

use crate::routes::{err_json, ok_json, AppState};

/// MCP 子路由（不带 state，由主路由统一 with_state；仅 GET/POST）。
pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/mcp", get(mcp_list))
        .route("/api/v1/mcp/save", post(mcp_save))
        .route("/api/v1/mcp/remove", post(mcp_remove))
        .route("/api/v1/mcp/login", post(mcp_login))
        .route("/api/v1/mcp/logout", post(mcp_logout))
        .route("/api/v1/mcp/reload", post(mcp_reload))
}

fn sv(State(st): State<AppState>) -> Arc<Services> {
    st.sv.clone()
}

async fn mcp_list(st: State<AppState>) -> Response {
    ok_json(wb_service::mcp::list_vo(&sv(st)))
}

async fn mcp_save(st: State<AppState>, axum::Json(req): axum::Json<McpSaveREQ>) -> Response {
    match wb_service::mcp::save(&sv(st), &req).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn mcp_remove(st: State<AppState>, axum::Json(req): axum::Json<McpNameREQ>) -> Response {
    match wb_service::mcp::remove(&sv(st), &req).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn mcp_login(st: State<AppState>, axum::Json(req): axum::Json<McpNameREQ>) -> Response {
    match wb_service::mcp::login(&sv(st), &req).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn mcp_logout(st: State<AppState>, axum::Json(req): axum::Json<McpNameREQ>) -> Response {
    // 与 login 同因：无 OAuth，登出无意义
    match wb_service::mcp::login(&sv(st), &req).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn mcp_reload(st: State<AppState>) -> Response {
    match wb_service::mcp::reload(&sv(st)).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}
