//! 路由与 handler：薄转发到 service，统一 Resp 信封。

use std::sync::Arc;

use axum::extract::{Path, Query, State};
use axum::http::StatusCode;
use axum::response::sse::{Event as SseEvent, KeepAlive, Sse};
use axum::response::{IntoResponse, Response};
use axum::routing::{get, post};
use axum::Router;
use futures::stream::Stream;

use wb_domain::settings::SettingsSaveREQ;
use wb_domain::tool::ToolToggleREQ;
use wb_pkg::resp::Resp;
use wb_pkg::AppError;
use wb_service::Services;

use crate::hub::Hub;

/// 共享状态。
#[derive(Clone)]
pub struct AppState {
    /// 服务容器
    pub sv: Arc<Services>,
    /// SSE Hub
    pub hub: Arc<Hub>,
}

/// 组装路由表（与 docs/API.md 一一对应；仅 GET/POST）。
pub fn router(state: AppState) -> Router {
    Router::new()
        .merge(crate::chat_routes::routes())
        .merge(crate::git_routes::routes())
        .merge(crate::mcp_routes::routes())
        .merge(crate::session_routes::routes())
        .merge(crate::system_routes::routes())
        .route("/api/v1/bootstrap", get(bootstrap))
        .route("/api/v1/warmup", get(warmup))
        .route("/api/v1/settings", get(settings_get).post(settings_save))
        .route("/api/v1/providers", get(providers_list))
        .route("/api/v1/skills", get(skills_list))
        .route("/api/v1/skills/toggle/{id}", post(skill_toggle))
        .route("/api/v1/tools", get(tools_list))
        .route("/api/v1/tools/toggle", post(tool_toggle))
        .route("/api/v1/features", get(features_list))
        .route("/api/v1/features/toggle", post(feature_toggle))
        .route(
            "/api/v1/features/{code}/config",
            get(feature_config_get).post(feature_config_save),
        )
        .route("/api/v1/loadout", get(loadout_list))
        .route("/api/v1/loadout/save", post(loadout_save))
        .route("/api/v1/loadout/apply", post(loadout_apply))
        .route("/api/v1/loadout/delete", post(loadout_delete))
        .route("/api/v1/inspect", get(inspect))
        .route("/api/v1/stats", get(stats))
        .route("/api/v1/kernel/status", get(kernel_status))
        .route("/api/v1/events", get(events))
        .with_state(state)
        .layer(crate::cors::cors_layer())
}

// ---- 响应助手 ----

pub(crate) fn ok_json<T: serde::Serialize>(data: T) -> Response {
    (StatusCode::OK, axum::Json(Resp::ok(data))).into_response()
}

pub(crate) fn err_json(e: &AppError) -> Response {
    (
        StatusCode::OK,
        axum::Json(wb_pkg::resp::Resp::<serde_json::Value>::err(e)),
    )
        .into_response()
}

// ---- 引导 ----

async fn bootstrap(State(st): State<AppState>) -> Response {
    let ws = st.sv.workspace.lock().unwrap().clone();
    let (ready, reason) = match wb_service::chat::default_model_state(&st.sv) {
        Ok(_) => (true, String::new()),
        Err(e) => (false, e.message),
    };
    let dp = st
        .sv
        .repo
        .settings_get("default_provider")
        .ok()
        .flatten()
        .unwrap_or_default();
    let dm = st
        .sv
        .repo
        .settings_get("default_model")
        .ok()
        .flatten()
        .unwrap_or_default();
    let settings: std::collections::BTreeMap<String, String> = st
        .sv
        .repo
        .settings_all()
        .unwrap_or_default()
        .into_iter()
        .map(|kv| (kv.setting_key, kv.setting_value))
        .collect();
    ok_json(wb_domain::settings::BootVO {
        boot_version: env!("CARGO_PKG_VERSION").into(),
        boot_data_dir: st.sv.data_dir.display().to_string(),
        boot_workspace: ws,
        boot_ready: ready,
        boot_block_reason: reason,
        boot_default_provider: dp,
        boot_default_model: dm,
        boot_settings: settings,
    })
}

async fn warmup(State(st): State<AppState>) -> Response {
    let providers = wb_service::providers::list_vo(&st.sv)
        .map(|v| v.0)
        .unwrap_or_default();
    // 上次进程死在提问途中：把还没答的问题重新弹出去，
    // 否则会话卡在一个永远等不到回答的工具调用上，用户只能自己发现。
    let pending_asks = st.sv.asks.load_pending();
    for card in &pending_asks {
        st.sv.emit_event("", "chat:ask", card.clone());
    }
    let skills = st
        .sv
        .skills
        .read()
        .unwrap()
        .list_vo(|n| st.sv.repo.skill_enabled(n).unwrap_or(true));
    let tools = wb_service::resources::tools_all_vo(&st.sv);
    let features = wb_service::features::list_vo(&st.sv);
    ok_json(serde_json::json!({
        "providers": providers,
        "skills": skills,
        "tools": tools,
        "features": features,
        "pending_asks": pending_asks
    }))
}

// ---- 设置 ----

async fn settings_get(State(st): State<AppState>) -> Response {
    match st.sv.repo.settings_all() {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn settings_save(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<SettingsSaveREQ>,
) -> Response {
    let mut last = Ok(());
    for kv in &req.req_values {
        last = st.sv.repo.settings_set(&kv.setting_key, &kv.setting_value);
    }
    match last {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

// ---- 会话 ----

async fn providers_list(State(st): State<AppState>) -> Response {
    match wb_service::providers::list_vo(&st.sv) {
        Ok((providers, dp, dm, path)) => ok_json(serde_json::json!({
            "resp_providers": providers, "resp_default_provider": dp, "resp_default_model": dm, "resp_config_path": path,
        })),
        Err(e) => err_json(&e),
    }
}

async fn tools_list(State(st): State<AppState>) -> Response {
    ok_json(wb_service::resources::tools_all_vo(&st.sv))
}

async fn tool_toggle(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<ToolToggleREQ>,
) -> Response {
    match st
        .sv
        .repo
        .tool_set_enabled(&req.req_tool_name, req.req_enabled)
    {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn skills_list(State(st): State<AppState>) -> Response {
    let skills = st.sv.skills.read().unwrap();
    ok_json(skills.list_vo(|n| st.sv.repo.skill_enabled(n).unwrap_or(true)))
}

async fn skill_toggle(
    State(st): State<AppState>,
    Path(id): Path<String>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let on = body["skill_enabled"].as_bool().unwrap_or(true);
    match st.sv.repo.skill_set_enabled(&id, on) {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

/// 功能清单与开关（清单本体下沉 service，供 /inspect 复用）。
async fn features_list(State(st): State<AppState>) -> Response {
    ok_json(wb_service::features::list_vo(&st.sv))
}

/// 功能级配置读取（KV `feature_config_{code}`）。
async fn feature_config_get(State(st): State<AppState>, Path(code): Path<String>) -> Response {
    let safe = code.replace(['/', '\\', '.'], "");
    match st.sv.repo.settings_get(&format!("feature_config_{safe}")) {
        Ok(v) => ok_json(
            serde_json::json!({ "resp_feature_code": safe, "resp_config": v.unwrap_or_default() }),
        ),
        Err(e) => err_json(&e),
    }
}

/// 功能级配置写入。
async fn feature_config_save(
    State(st): State<AppState>,
    Path(code): Path<String>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let safe = code.replace(['/', '\\', '.'], "");
    let raw = body["feature_config"].as_str().unwrap_or("").to_string();
    match st
        .sv
        .repo
        .settings_set(&format!("feature_config_{safe}"), &raw)
    {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

// ---- 装载配置 / 内省 ----

async fn loadout_list(State(st): State<AppState>) -> Response {
    ok_json(wb_service::loadout::list(&st.sv))
}

async fn loadout_save(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<wb_domain::feature::LoadoutREQ>,
) -> Response {
    match wb_service::loadout::save(&st.sv, &req.loadout_name).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn loadout_apply(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<wb_domain::feature::LoadoutREQ>,
) -> Response {
    match wb_service::loadout::apply(&st.sv, &req.loadout_name).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn loadout_delete(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<wb_domain::feature::LoadoutREQ>,
) -> Response {
    match wb_service::loadout::delete(&st.sv, &req).await {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

#[derive(serde::Deserialize)]
struct InspectQuery {
    /// 会话 id（可选：附带会话元信息）
    session_id: Option<String>,
}

async fn inspect(State(st): State<AppState>, Query(q): Query<InspectQuery>) -> Response {
    ok_json(wb_service::features::inspect_snapshot(
        &st.sv,
        q.session_id.as_deref().unwrap_or(""),
    ))
}

async fn feature_toggle(
    State(st): State<AppState>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let code = body["feature_code"].as_str().unwrap_or("");
    let on = body["feature_enabled"].as_bool().unwrap_or(false);
    match st.sv.repo.feature_set_enabled(code, on) {
        // 开关变了注册表就得重建：停用即整体撤出工具
        Ok(_) => {
            wb_service::features::refresh_registry(&st.sv);
            ok_json(serde_json::Value::Null)
        }
        Err(e) => err_json(&e),
    }
}

// ---- 统计 / 内核状态 / SSE ----

async fn stats(
    State(st): State<AppState>,
    Query(q): Query<std::collections::HashMap<String, String>>,
) -> Response {
    let span = q.get("span").map(|s| s.as_str()).unwrap_or("day30");
    let days = match span {
        "day7" => 7,
        "all" => 3650,
        _ => 30,
    };
    match st.sv.repo.usage_stats(days) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn kernel_status(State(st): State<AppState>) -> Response {
    let ready = wb_service::chat::default_model_state(&st.sv).is_ok();
    ok_json(serde_json::json!({ "ready": ready, "data_dir": st.sv.data_dir.display().to_string() }))
}

#[derive(serde::Deserialize)]
struct EventsQuery {
    /// 订阅的会话；空 = 全局通道
    session_id: Option<String>,
    /// 断点序号：前端手动重连带查询参数，EventSource 原生重连带请求头
    last_event_id: Option<i64>,
}

/// SSE 端点：Last-Event-ID 重放 + 心跳。
async fn events(
    State(st): State<AppState>,
    Query(q): Query<EventsQuery>,
    headers: axum::http::HeaderMap,
) -> Sse<impl Stream<Item = Result<SseEvent, std::convert::Infallible>>> {
    let sid = q.session_id.unwrap_or_default();
    let (tx, rx) = st.hub.subscribe(&sid);
    // 重放：断点之后的帧先推（查询参数与请求头任一命中即可）
    let last = q.last_event_id.or_else(|| {
        headers
            .get("last-event-id")
            .and_then(|v| v.to_str().ok())
            .and_then(|v| v.parse::<i64>().ok())
    });
    if let Some(last) = last {
        for f in st.hub.replay_after(&sid, last) {
            let _ = tx.send(f);
        }
    }
    let hub = st.hub.clone();
    let sid2 = sid.clone();
    Sse::new(futures::stream::unfold(
        (rx, tx, hub, sid2),
        |(mut rx, tx, hub, sid)| async move {
            match rx.recv().await {
                Some(f) => Some((
                    Ok(SseEvent::default()
                        .id(f.seq.to_string())
                        .event(f.event)
                        .data(f.data.to_string())),
                    (rx, tx, hub, sid),
                )),
                None => {
                    hub.unsubscribe(&sid, &tx);
                    None
                }
            }
        },
    ))
    .keep_alive(KeepAlive::default())
}
