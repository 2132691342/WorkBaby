//! 功能活状态路由：待办 / 目标 / 计划 / 记忆 的读取与记忆删除。
//! 界面读结构化 VO，不再解析 widget 文本。

use axum::extract::State;
use axum::response::Response;
use axum::routing::{get, post};
use axum::Router;

use wb_domain::feature_state::MemoryDeleteREQ;

use crate::routes::{err_json, ok_json, AppState};

/// 功能状态子路由（仅 GET/POST）。
pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/todo", get(todo_list))
        .route("/api/v1/goal", get(goal_get))
        .route("/api/v1/plan", get(plan_get))
        .route("/api/v1/memory", get(memory_list))
        .route("/api/v1/memory/delete", post(memory_delete))
}

async fn todo_list(State(st): State<AppState>) -> Response {
    match wb_service::feature_state::todo_list(&st.sv) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn goal_get(State(st): State<AppState>) -> Response {
    match wb_service::feature_state::goal_get(&st.sv) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn plan_get(State(st): State<AppState>) -> Response {
    match wb_service::feature_state::plan_get(&st.sv) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn memory_list(State(st): State<AppState>) -> Response {
    match wb_service::feature_state::memory_list(&st.sv) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn memory_delete(
    State(st): State<AppState>,
    axum::Json(body): axum::Json<MemoryDeleteREQ>,
) -> Response {
    match wb_service::feature_state::memory_delete(&st.sv, &body.memory_id) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}
