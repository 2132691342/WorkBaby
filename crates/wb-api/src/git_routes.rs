//! git 路由：读 6 端点 + 白名单写 16 端点，全部薄转发 wb_service::git。

use std::sync::Arc;

use axum::extract::{Query, State};
use axum::response::Response;
use axum::routing::{get, post};
use axum::Router;

use wb_domain::git::{
    GitCommitREQ, GitInitREQ, GitMergeREQ, GitNameREQ, GitPathsREQ, GitPullREQ, GitPushREQ,
    GitRemoteREQ, GitStateREQ,
};
use wb_service::Services;

use crate::routes::{err_json, ok_json, AppState};

/// git 子路由（不带 state，由主路由统一 with_state；仅 GET/POST）。
pub fn routes() -> Router<AppState> {
    Router::new()
        .route("/api/v1/git/status", get(git_status))
        .route("/api/v1/git/diff", get(git_diff))
        .route("/api/v1/git/history", get(git_history))
        .route("/api/v1/git/commit", get(git_commit))
        .route("/api/v1/git/file-history", get(git_file_history))
        .route("/api/v1/git/branches", get(git_branches))
        .route("/api/v1/git/stage", post(git_stage))
        .route("/api/v1/git/unstage", post(git_unstage))
        .route("/api/v1/git/discard", post(git_discard))
        .route("/api/v1/git/resolve", post(git_resolve))
        .route("/api/v1/git/commit", post(git_commit_post))
        .route("/api/v1/git/branch", post(git_branch))
        .route("/api/v1/git/branch/delete", post(git_branch_delete))
        .route("/api/v1/git/switch", post(git_switch))
        .route("/api/v1/git/merge", post(git_merge))
        .route("/api/v1/git/state", post(git_state))
        .route("/api/v1/git/pull", post(git_pull))
        .route("/api/v1/git/fetch", post(git_fetch))
        .route("/api/v1/git/push", post(git_push))
        .route("/api/v1/git/init", post(git_init))
        .route("/api/v1/git/remote", post(git_remote))
        .route("/api/v1/git/message", post(git_message))
}

/// 会话 id 查询参数（当前一律用工作区，解析后忽略）。
#[derive(serde::Deserialize)]
struct GitQuery {
    /// 会话 id（当前一律用工作区，解析后忽略）
    #[allow(dead_code)]
    session_id: Option<String>,
    /// 文件相对路径
    path: Option<String>,
    /// 提交 hash
    hash: Option<String>,
    /// 条数上限
    limit: Option<i64>,
}

fn sv(State(st): State<AppState>) -> Arc<Services> {
    st.sv.clone()
}

async fn git_status(st: State<AppState>) -> Response {
    ok_json(wb_service::git::status(&sv(st)))
}

async fn git_diff(st: State<AppState>, Query(q): Query<GitQuery>) -> Response {
    match wb_service::git::diff(&sv(st), q.path.as_deref().unwrap_or("")) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_history(st: State<AppState>, Query(q): Query<GitQuery>) -> Response {
    match wb_service::git::history(&sv(st), q.limit.unwrap_or(50)) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_commit(st: State<AppState>, Query(q): Query<GitQuery>) -> Response {
    match wb_service::git::commit_detail(
        &sv(st),
        q.hash.as_deref().unwrap_or(""),
        q.path.as_deref(),
    ) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_file_history(st: State<AppState>, Query(q): Query<GitQuery>) -> Response {
    match wb_service::git::file_history(
        &sv(st),
        q.path.as_deref().unwrap_or(""),
        q.limit.unwrap_or(50),
    ) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_branches(st: State<AppState>) -> Response {
    match wb_service::git::branches(&sv(st)) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_stage(st: State<AppState>, axum::Json(req): axum::Json<GitPathsREQ>) -> Response {
    match wb_service::git::stage(&sv(st), &req.git_paths) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_unstage(st: State<AppState>, axum::Json(req): axum::Json<GitPathsREQ>) -> Response {
    match wb_service::git::unstage(&sv(st), &req.git_paths) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_discard(st: State<AppState>, axum::Json(req): axum::Json<GitPathsREQ>) -> Response {
    match wb_service::git::discard(&sv(st), &req.git_paths) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_resolve(st: State<AppState>, axum::Json(req): axum::Json<GitPathsREQ>) -> Response {
    match wb_service::git::resolve(&sv(st), &req.git_paths) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_commit_post(
    st: State<AppState>,
    axum::Json(req): axum::Json<GitCommitREQ>,
) -> Response {
    match wb_service::git::commit(&sv(st), &req.git_message, req.git_amend) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_branch(st: State<AppState>, axum::Json(req): axum::Json<GitNameREQ>) -> Response {
    match wb_service::git::branch_create(&sv(st), &req.git_name) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_branch_delete(
    st: State<AppState>,
    axum::Json(req): axum::Json<GitNameREQ>,
) -> Response {
    match wb_service::git::branch_delete(&sv(st), &req.git_name) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_switch(st: State<AppState>, axum::Json(req): axum::Json<GitNameREQ>) -> Response {
    match wb_service::git::switch_branch(&sv(st), &req.git_name) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_merge(st: State<AppState>, axum::Json(req): axum::Json<GitMergeREQ>) -> Response {
    match wb_service::git::merge(&sv(st), &req.git_name, req.git_rebase) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_state(st: State<AppState>, axum::Json(req): axum::Json<GitStateREQ>) -> Response {
    match wb_service::git::state_action(&sv(st), &req.git_action) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_pull(st: State<AppState>, axum::Json(req): axum::Json<GitPullREQ>) -> Response {
    match wb_service::git::pull(&sv(st), &req.git_remote, &req.git_branch, req.git_rebase) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_fetch(
    st: State<AppState>,
    axum::Json(req): axum::Json<serde_json::Value>,
) -> Response {
    let remote = req["git_remote"].as_str().unwrap_or("").to_string();
    match wb_service::git::fetch(&sv(st), &remote) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_push(st: State<AppState>, axum::Json(req): axum::Json<GitPushREQ>) -> Response {
    match wb_service::git::push(&sv(st), &req) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_init(st: State<AppState>, axum::Json(req): axum::Json<GitInitREQ>) -> Response {
    match wb_service::git::init(&sv(st), &req.git_branch) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_remote(st: State<AppState>, axum::Json(req): axum::Json<GitRemoteREQ>) -> Response {
    match wb_service::git::remote_add(&sv(st), &req.git_name, &req.git_url) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}

async fn git_message(st: State<AppState>, axum::Json(req): axum::Json<GitPathsREQ>) -> Response {
    match wb_service::git::draft_message(&sv(st), &req.git_paths) {
        Ok(v) => ok_json(v),
        Err(e) => err_json(&e),
    }
}
