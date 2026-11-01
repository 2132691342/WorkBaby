//! 会话路由：列表、详情、删除、重命名、模型与思考档切换、导出。

use axum::extract::{Path, State};
use axum::response::Response;
use axum::routing::{get, post};
use axum::Router;

use wb_pkg::AppError;

use crate::routes::{err_json, ok_json, AppState};

/// 会话路由子表（不带 state；仅 GET/POST）。
pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/sessions", get(sessions_list))
        .route("/api/v1/sessions/{id}", get(session_detail))
        .route("/api/v1/sessions/delete/{id}", post(session_delete))
        .route("/api/v1/sessions/rename/{id}", post(session_rename))
        .route("/api/v1/sessions/model/{id}", post(session_model))
        .route("/api/v1/sessions/thinking/{id}", post(session_thinking))
        .route("/api/v1/sessions/export/{id}", post(session_export))
        .route("/api/v1/sessions/branch/{id}", post(session_branch))
        .route("/api/v1/sessions/leaf/{id}", post(session_leaf))
}

/// 从某条用户消息之后重开（旧分支保留）。
async fn session_branch(
    State(st): State<AppState>,
    Path(id): Path<String>,
    axum::Json(req): axum::Json<wb_domain::session::SessionBranchREQ>,
) -> Response {
    match wb_service::store::branch(&st.sv, &id, &req.branch_entry_id) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

/// 切回某个旧分支的叶子。
async fn session_leaf(
    State(st): State<AppState>,
    Path(id): Path<String>,
    axum::Json(req): axum::Json<wb_domain::session::SessionLeafREQ>,
) -> Response {
    match wb_service::store::switch_leaf(&st.sv, &id, &req.leaf_entry_id) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn sessions_list(State(st): State<AppState>) -> Response {
    match wb_service::store::list(&st.sv) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn session_detail(State(st): State<AppState>, Path(id): Path<String>) -> Response {
    match wb_service::store::detail(&st.sv, &id) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn session_delete(State(st): State<AppState>, Path(id): Path<String>) -> Response {
    if st.sv.chat.lock().unwrap().is_running(&id) {
        return err_json(&AppError::new(
            wb_domain::error::session::FILE_BUSY,
            "对话正在运行，先停止再删除",
        ));
    }
    match wb_service::store::find_session_file(&st.sv, &id) {
        Ok(p) => {
            let r =
                std::fs::remove_file(&p).map_err(|e| AppError::wrap("1002", "删除会话文件失败", e));
            let _ = st.sv.repo.sessions_delete(&id);
            match r {
                Ok(_) => ok_json(serde_json::Value::Null),
                Err(e) => err_json(&e),
            }
        }
        Err(e) => err_json(&e),
    }
}

async fn session_rename(
    State(st): State<AppState>,
    Path(id): Path<String>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let name = body["session_name"].as_str().unwrap_or("").to_string();
    match wb_service::store::find_session_file(&st.sv, &id) {
        Ok(p) => match wb_kernel::SessionFile::load(&p) {
            Ok(mut sf) => {
                let r = sf.append(wb_kernel::Entry::SessionInfo {
                    session_name: Some(name),
                });
                match r {
                    Ok(_) => ok_json(serde_json::Value::Null),
                    Err(e) => err_json(&e),
                }
            }
            Err(e) => err_json(&e),
        },
        Err(e) => err_json(&e),
    }
}

/// 会话切模型（写 ModelChange 条目）。
async fn session_model(
    State(st): State<AppState>,
    Path(id): Path<String>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let pid = body["session_provider"].as_str().unwrap_or("").to_string();
    let mid = body["session_model"].as_str().unwrap_or("").to_string();
    if pid.is_empty() || mid.is_empty() {
        return err_json(&AppError::new(
            wb_domain::error::common::PARAM_INVALID,
            "provider 与 model 不能为空",
        ));
    }
    match wb_service::store::find_session_file(&st.sv, &id) {
        Ok(p) => match wb_kernel::SessionFile::load(&p) {
            Ok(mut sf) => {
                let r = sf.append(wb_kernel::Entry::ModelChange {
                    provider_id: pid,
                    model_id: mid,
                });
                match r {
                    Ok(_) => ok_json(serde_json::Value::Null),
                    Err(e) => err_json(&e),
                }
            }
            Err(e) => err_json(&e),
        },
        Err(e) => err_json(&e),
    }
}

/// 会话切思考档位（写 ThinkingChange 条目）。
async fn session_thinking(
    State(st): State<AppState>,
    Path(id): Path<String>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let level = body["session_thinking_level"]
        .as_str()
        .unwrap_or("")
        .to_string();
    if level.is_empty() {
        return err_json(&AppError::new(
            wb_domain::error::common::PARAM_INVALID,
            "thinking_level 不能为空",
        ));
    }
    match wb_service::store::find_session_file(&st.sv, &id) {
        Ok(p) => match wb_kernel::SessionFile::load(&p) {
            Ok(mut sf) => {
                let r = sf.append(wb_kernel::Entry::ThinkingChange {
                    thinking_level: level,
                });
                match r {
                    Ok(_) => ok_json(serde_json::Value::Null),
                    Err(e) => err_json(&e),
                }
            }
            Err(e) => err_json(&e),
        },
        Err(e) => err_json(&e),
    }
}

/// 导出会话 HTML。
async fn session_export(
    State(st): State<AppState>,
    Path(id): Path<String>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let path = body["session_export_path"]
        .as_str()
        .or(body["path"].as_str())
        .unwrap_or("")
        .to_string();
    match wb_service::store::export_html(&st.sv, &id, &path) {
        Ok(p) => ok_json(p),
        Err(e) => err_json(&e),
    }
}

// ---- 对话 ----
