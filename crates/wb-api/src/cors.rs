//! CORS：仅放行本机前端（dev 各端口）与 Tauri webview 的跨域请求。

use std::time::Duration;

use axum::http::{header, HeaderName, HeaderValue, Method};
use tower_http::cors::{AllowOrigin, CorsLayer};

/// 允许的来源：localhost / 127.0.0.1（任意端口，覆盖 dev 端口漂移）与 tauri.localhost。
/// 服务只绑 127.0.0.1 且无凭据，外部站点 origin 不匹配即读不到响应。
fn origin_allowed(origin: &HeaderValue) -> bool {
    let Some(s) = origin.to_str().ok() else {
        return false;
    };
    s == "tauri://localhost"
        || ["http", "https"].iter().any(|scheme| {
            ["localhost", "127.0.0.1", "tauri.localhost"]
                .iter()
                .any(|host| s.starts_with(&format!("{scheme}://{host}")))
        })
}

/// 组装 CORS 层：方法限 GET/POST，头放行 JSON 与 SSE 断线续传。
pub fn cors_layer() -> CorsLayer {
    CorsLayer::new()
        .allow_origin(AllowOrigin::predicate(|origin, _| origin_allowed(origin)))
        .allow_methods([Method::GET, Method::POST])
        .allow_headers([
            header::CONTENT_TYPE,
            HeaderName::from_static("last-event-id"),
        ])
        .max_age(Duration::from_secs(86400))
}
