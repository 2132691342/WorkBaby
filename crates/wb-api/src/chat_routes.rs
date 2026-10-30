//! 对话路由：发送、停止、插话、追问、应答、重试取消与压缩。

use axum::extract::State;
use axum::response::Response;
use axum::routing::post;
use axum::Router;

use wb_domain::chat::{ChatAnswerREQ, ChatCompactREQ, ChatFollowupREQ, ChatSendREQ, ChatSteerREQ};
use wb_pkg::AppError;

use crate::routes::{err_json, ok_json, AppState};

/// 对话路由子表（不带 state；仅 POST）。
pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/chat/send", post(chat_send))
        .route("/api/v1/chat/stop", post(chat_stop))
        .route("/api/v1/chat/steer", post(chat_steer))
        .route("/api/v1/chat/answer", post(chat_answer))
        .route("/api/v1/chat/followup", post(chat_followup))
        .route("/api/v1/chat/retry-cancel", post(chat_retry_cancel))
        .route("/api/v1/chat/compact", post(chat_compact))
}

async fn chat_followup(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<ChatFollowupREQ>,
) -> Response {
    match wb_service::chat::followup(st.sv.clone(), &req.chat_session_id, req.chat_text).await {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn chat_retry_cancel(
    State(st): State<AppState>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let id = body["chat_session_id"].as_str().unwrap_or("");
    ok_json(serde_json::json!({ "cancelled": wb_service::chat::retry_cancel(&st.sv, id) }))
}

async fn chat_compact(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<ChatCompactREQ>,
) -> Response {
    match wb_service::chat::compact(st.sv.clone(), &req.chat_session_id, req.chat_instructions)
        .await
    {
        Ok((tokens_before, summary)) => ok_json(
            serde_json::json!({ "resp_tokens_before": tokens_before, "resp_summary": summary }),
        ),
        Err(e) => err_json(&e),
    }
}

async fn chat_send(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<ChatSendREQ>,
) -> Response {
    match wb_service::chat::send(st.sv.clone(), req).await {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn chat_stop(
    State(st): State<AppState>,
    axum::Json(body): axum::Json<serde_json::Value>,
) -> Response {
    let id = body["chat_session_id"].as_str().unwrap_or("");
    ok_json(serde_json::json!({ "stopped": wb_service::chat::stop(&st.sv, id) }))
}

async fn chat_steer(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<ChatSteerREQ>,
) -> Response {
    match wb_service::chat::steer(&st.sv, &req.chat_session_id, req.chat_text) {
        Ok(_) => ok_json(serde_json::Value::Null),
        Err(e) => err_json(&e),
    }
}

async fn chat_answer(
    State(st): State<AppState>,
    axum::Json(req): axum::Json<ChatAnswerREQ>,
) -> Response {
    let value = serde_json::json!({
        "value": req.answer_value,
        "confirmed": req.answer_confirmed,
        "cancelled": req.answer_cancelled,
    });
    if st.sv.asks.answer(&req.answer_ask_id, value) {
        ok_json(serde_json::Value::Null)
    } else {
        err_json(&AppError::new(
            wb_domain::error::session::ASK_EXPIRED,
            "没有等待中的提问",
        ))
    }
}

// ---- Provider / 技能 / 工具 / 功能 ----
