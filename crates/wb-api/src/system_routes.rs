//! 系统资源路由：providers / skills 管理 / files / docs / commands / settings 附属 / context。

use std::sync::Arc;

use axum::extract::{Path, Query, State};
use axum::response::{IntoResponse, Response};
use axum::routing::{get, post};
use axum::Router;

use wb_domain::provider::UpsertProviderREQ;
use wb_domain::resource::BackgroundSaveREQ;
use wb_domain::skill::{SkillCreateREQ, SkillImportZipREQ};
use wb_service::Services;

use crate::routes::{err_json, ok_json, AppState};

/// 系统资源子路由（不带 state；仅 GET/POST）。
pub fn routes() -> Router<AppState> {
    Router::new()
        // Provider
        .route("/api/v1/providers/upsert", post(providers_upsert))
        .route("/api/v1/providers/delete", post(providers_delete))
        .route("/api/v1/providers/test", post(providers_test))
        .route("/api/v1/providers/reveal", post(providers_reveal))
        .route("/api/v1/providers/default", post(providers_default))
        .route(
            "/api/v1/providers/models/fetch",
            post(providers_models_fetch),
        )
        .route("/api/v1/models/available", get(models_available))
        // 技能管理
        .route("/api/v1/skills/create", post(skills_create))
        .route("/api/v1/skills/import", post(skills_import))
        .route("/api/v1/skills/import-zip", post(skills_import_zip))
        .route("/api/v1/skills/delete/{id}", post(skills_delete))
        .route("/api/v1/skills/content/{id}", get(skills_content))
        // 文件树 / 文档 / 命令
        .route("/api/v1/files", get(files_list))
        .route("/api/v1/files/bytes", get(files_bytes))
        .route("/api/v1/docs", get(docs_list))
        .route("/api/v1/docs/{name}", get(docs_content))
        .route("/api/v1/commands", get(commands_list))
        // 设置附属
        .route(
            "/api/v1/settings/background",
            get(background_get).post(background_save),
        )
        .route("/api/v1/assets/background", get(background_bytes))
        // 上下文分解
        .route("/api/v1/context/breakdown", get(context_breakdown))
        // 子代理：角色预设 / 活跃清单 / 过程正文
        .route("/api/v1/subagents/roles", get(subagent_roles))
        .route("/api/v1/subagents", get(subagents_list))
        .route(
            "/api/v1/subagents/transcript/{id}",
            get(subagent_transcript),
        )
}

fn sv(State(st): State<AppState>) -> Arc<Services> {
    st.sv.clone()
}

// ---- 子代理 ----

/// 子代理角色预设清单。
async fn subagent_roles(_st: State<AppState>) -> Response {
    ok_json(wb_domain::subagent::SubagentListRESP {
        resp_roles: wb_service::task::roles_vo(),
    })
}

/// 正在跑的子代理清单；带 session_id 时只列该父会话下的。
async fn subagents_list(st: State<AppState>, Query(q): Query<AgentListQuery>) -> Response {
    let all = st.sv.agents.list();
    let items = match q.session_id.filter(|s| !s.is_empty()) {
        Some(parent) => all
            .into_iter()
            .filter(|a| a.run_parent_session_id == parent)
            .collect(),
        None => all,
    };
    ok_json(wb_domain::subagent::SubagentActiveRESP { resp_agents: items })
}

/// 一个子代理已累积的过程正文（结束后回看用）。
async fn subagent_transcript(st: State<AppState>, Path(id): Path<String>) -> Response {
    ok_json(st.sv.agents.transcript(&id))
}

/// 活跃子代理查询参数。
#[derive(Debug, serde::Deserialize)]
pub struct AgentListQuery {
    /// 父会话 id；空/缺省即全工作区的活跃子代理
    pub session_id: Option<String>,
}

// ---- Provider ----

async fn providers_upsert(
    st: State<AppState>,
    axum::Json(req): axum::Json<UpsertProviderREQ>,
) -> Response {
    match wb_service::providers::upsert(&sv(st), &req).await {
        Ok(pid) => ok_json(serde_json::json!({ "resp_provider_id": pid })),
        Err(e) => err_json(&e),
    }
}

async fn providers_delete(
    st: State<AppState>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let pid = body["req_provider_id"]
        .as_str()
        .or(body["provider_id"].as_str())
        .unwrap_or("");
    match wb_service::providers::delete(&sv(st), pid).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn providers_test(
    st: State<AppState>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let pid = body["req_provider_id"]
        .as_str()
        .or(body["provider_id"].as_str())
        .unwrap_or("");
    match wb_service::providers::test(&sv(st), pid).await {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn providers_reveal(
    st: State<AppState>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let pid = body["req_provider_id"]
        .as_str()
        .or(body["provider_id"].as_str())
        .unwrap_or("");
    match wb_service::providers::reveal(&sv(st), pid).await {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn providers_default(
    st: State<AppState>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let pid = body["req_provider"]
        .as_str()
        .or(body["provider"].as_str())
        .unwrap_or("");
    let mid = body["req_model"]
        .as_str()
        .or(body["model"].as_str())
        .unwrap_or("");
    match wb_service::providers::set_default(&sv(st), pid, mid).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn providers_models_fetch(
    st: State<AppState>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    match wb_service::providers::fetch_models(
        &sv(st),
        body["id"].as_str(),
        body["api"].as_str(),
        body["base_url"].as_str(),
        body["api_key"].as_str(),
    )
    .await
    {
        Ok(models) => ok_json(serde_json::json!({ "resp_models": models })),
        Err(e) => err_json(&e),
    }
}

async fn models_available(st: State<AppState>) -> Response {
    match wb_service::providers::available(&sv(st)).await {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

// ---- 技能管理 ----

async fn skills_create(
    st: State<AppState>,
    axum::Json(req): axum::Json<SkillCreateREQ>,
) -> Response {
    match wb_service::resources::skill_create(
        &sv(st),
        &req.req_skill_name,
        &req.req_skill_description,
        &req.req_skill_body,
    )
    .await
    {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn skills_import(
    st: State<AppState>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let paths: Vec<String> = body["req_paths"]
        .as_array()
        .or(body["paths"].as_array())
        .map(|a| {
            a.iter()
                .filter_map(|v| v.as_str().map(|s| s.to_string()))
                .collect()
        })
        .unwrap_or_default();
    match wb_service::resources::skill_import(&sv(st), &paths).await {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn skills_import_zip(
    st: State<AppState>,
    axum::Json(req): axum::Json<SkillImportZipREQ>,
) -> Response {
    match wb_service::resources::skill_import_zip(&sv(st), &req.req_skill_data).await {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn skills_delete(st: State<AppState>, Path(id): Path<String>) -> Response {
    match wb_service::resources::skill_delete(&sv(st), &id).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn skills_content(st: State<AppState>, Path(id): Path<String>) -> Response {
    match wb_service::resources::skill_content(&sv(st), &id) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

// ---- 文件树 / 文档 / 命令 ----

#[derive(serde::Deserialize)]
struct FilesQuery {
    /// 相对工作区路径；空 = 根目录
    path: Option<String>,
}

async fn files_list(st: State<AppState>, Query(q): Query<FilesQuery>) -> Response {
    match wb_service::files::files_list(&sv(st), q.path.as_deref().unwrap_or("")) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

#[derive(serde::Deserialize)]
struct FileBytesQuery {
    /// 相对工作区路径
    path: String,
}

/// 单文件字节（文档预览：PDF / Word / Excel）。
async fn files_bytes(st: State<AppState>, Query(q): Query<FileBytesQuery>) -> Response {
    match wb_service::files::file_bytes(&sv(st), &q.path) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn docs_list() -> Response {
    ok_json(wb_service::misc::docs_list())
}

async fn docs_content(Path(name): Path<String>) -> Response {
    match wb_service::misc::doc_content(&name) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn commands_list(st: State<AppState>) -> Response {
    ok_json(wb_service::misc::commands_list(&sv(st)))
}

// ---- 设置附属 ----

async fn background_get(st: State<AppState>) -> Response {
    ok_json(wb_service::misc::background_get(&sv(st)))
}

async fn background_save(
    st: State<AppState>,
    axum::Json(req): axum::Json<BackgroundSaveREQ>,
) -> Response {
    match wb_service::misc::background_save(
        &sv(st),
        &req.req_background_path,
        req.req_background_opacity,
        req.req_background_blur,
    )
    .await
    {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

/// 背景图本体（前端 backgroundUrl 直引）。
async fn background_bytes(State(st): State<AppState>) -> Response {
    let file = st
        .sv
        .repo
        .settings_get("background_file")
        .ok()
        .flatten()
        .unwrap_or_default();
    if file.is_empty() {
        return (axum::http::StatusCode::NOT_FOUND, "no background").into_response();
    }
    match std::fs::read(st.sv.data_dir.join(&file)) {
        Ok(bytes) => {
            let mime = if file.ends_with(".png") {
                "image/png"
            } else if file.ends_with(".jpg") || file.ends_with(".jpeg") {
                "image/jpeg"
            } else if file.ends_with(".webp") {
                "image/webp"
            } else {
                "application/octet-stream"
            };
            ([(axum::http::header::CONTENT_TYPE, mime)], bytes).into_response()
        }
        Err(_) => (axum::http::StatusCode::NOT_FOUND, "no background").into_response(),
    }
}

// ---- 上下文分解 ----

#[derive(serde::Deserialize)]
struct ContextQuery {
    /// 会话 id；空 = 只算系统段
    session_id: Option<String>,
}

async fn context_breakdown(st: State<AppState>, Query(q): Query<ContextQuery>) -> Response {
    match wb_service::context::breakdown(&sv(st), q.session_id.as_deref().unwrap_or("")) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}
