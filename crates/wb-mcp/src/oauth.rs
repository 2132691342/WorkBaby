//! MCP 远端服务器的 OAuth 登录：授权码 + PKCE + 本地回调端口，凭据落盘进 `mcp.json` 的 headers。

use std::collections::BTreeMap;
use std::sync::Arc;

use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
use tokio::net::TcpListener;

use wb_pkg::{AppError, Result};

use crate::oauth_crypto::{parse_query, urlencode};

/// OAuth 端点发现结果。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct AuthServerMeta {
    /// 授权端点
    pub authorization_endpoint: String,
    /// 令牌端点
    pub token_endpoint: String,
    /// 注册端点（动态客户端注册）
    pub registration_endpoint: String,
    /// 是否支持 PKCE
    pub code_challenge_methods_supported: Vec<String>,
}

impl AuthServerMeta {
    /// 是否支持 PKCE（不支持就不走授权码流）。
    pub fn supports_pkce(&self) -> bool {
        self.code_challenge_methods_supported
            .iter()
            .any(|m| m.eq_ignore_ascii_case("S256"))
    }
}

/// 登录结果。
#[derive(Debug, Clone, Serialize)]
pub struct OAuthOutcome {
    /// 拿到的 access token
    pub access_token: String,
    /// 令牌类型（Bearer 等）
    pub token_type: String,
    /// 过期秒数（0 = 未知）
    pub expires_in: i64,
}

/// 授权元数据探测：先试标准路径，再试 MCP 规范里的路径。
pub async fn discover(http: &reqwest::Client, server_url: &str) -> Result<AuthServerMeta> {
    let base = server_url.trim_end_matches('/');
    let mut urls = vec![
        format!("{base}/.well-known/oauth-authorization-server"),
        format!("{base}/.well-known/oauth-protected-resource"),
    ];
    // 有些网关把 MCP 挂在子路径上，路径要带上 base 之外的部分
    if let Some(idx) = base.find("://") {
        if let Some(slash) = base[idx..].find('/') {
            let host = &base[..idx + slash];
            let path = &base[idx + slash..];
            urls.push(format!(
                "{host}/.well-known/oauth-authorization-server{path}"
            ));
        }
    }
    let mut last_err = None;
    for url in urls {
        match http.get(&url).send().await {
            Ok(resp) if resp.status().is_success() => {
                let text = resp.text().await.unwrap_or_default();
                match serde_json::from_str::<Value>(&text) {
                    Ok(v) => return Ok(parse_meta(&v)),
                    Err(e) => last_err = Some(format!("{url} 解析失败：{e}")),
                }
            }
            Ok(resp) => {
                last_err = Some(format!("{url} 返回 {}", resp.status()));
            }
            Err(e) => last_err = Some(format!("{url} 请求失败：{e}")),
        }
    }
    Err(AppError::new(
        wb_domain::error::mcp::CONFIG_INVALID,
        format!(
            "找不到 OAuth 授权端点（{}）。可以在配置里用 headers 直接传凭据。",
            last_err.unwrap_or_default()
        ),
    ))
}

/// 从元数据 JSON 里取端点。
fn parse_meta(v: &Value) -> AuthServerMeta {
    AuthServerMeta {
        authorization_endpoint: v["authorization_endpoint"]
            .as_str()
            .unwrap_or("")
            .to_string(),
        token_endpoint: v["token_endpoint"].as_str().unwrap_or("").to_string(),
        registration_endpoint: v["registration_endpoint"]
            .as_str()
            .unwrap_or("")
            .to_string(),
        code_challenge_methods_supported: v["code_challenge_methods_supported"]
            .as_array()
            .map(|a| {
                a.iter()
                    .filter_map(|x| x.as_str().map(String::from))
                    .collect()
            })
            .unwrap_or_default(),
    }
}

/// 动态客户端注册：拿不到 client_id 时现场注册一个。
pub async fn register_client(
    http: &reqwest::Client,
    meta: &AuthServerMeta,
    server_name: &str,
) -> Result<String> {
    if meta.registration_endpoint.is_empty() {
        return Err(AppError::new(
            wb_domain::error::mcp::CONFIG_INVALID,
            "该 server 不支持动态注册，也没有给 client_id：请在配置里直接写 headers",
        ));
    }
    let body = json!({
        "client_name": format!("WorkBaby ({server_name})"),
        "redirect_uris": [format!("http://127.0.0.1:{CALLBACK_PORT}/callback")],
        "grant_types": ["authorization_code", "refresh_token"],
        "response_types": ["code"],
        "token_endpoint_auth_method": "none",
    });
    let resp = http
        .post(&meta.registration_endpoint)
        .json(&body)
        .send()
        .await
        .map_err(|e| AppError::wrap("9103", "客户端注册请求失败", e))?;
    if !resp.status().is_success() {
        let text = resp.text().await.unwrap_or_default();
        return Err(AppError::new(
            wb_domain::error::mcp::CONFIG_INVALID,
            format!("客户端注册失败：{text}"),
        ));
    }
    let v: Value = resp.json().await.unwrap_or(Value::Null);
    v["client_id"].as_str().map(String::from).ok_or_else(|| {
        AppError::new(
            wb_domain::error::mcp::CONFIG_INVALID,
            "注册响应里没有 client_id",
        )
    })
}

/// 登录主流程：起本地回调 → 让用户去浏览器授权 → 换 token。
pub async fn login(
    http: &reqwest::Client,
    server_name: &str,
    server_url: &str,
    client_id: Option<&str>,
    open_browser: &(dyn Fn(&str) + Send + Sync),
) -> Result<OAuthOutcome> {
    let meta = discover(http, server_url).await?;
    if meta.authorization_endpoint.is_empty() || meta.token_endpoint.is_empty() {
        return Err(AppError::new(
            wb_domain::error::mcp::CONFIG_INVALID,
            "授权端点信息不全，请在配置里用 headers 直接传凭据",
        ));
    }
    if !meta.supports_pkce() {
        return Err(AppError::new(
            wb_domain::error::mcp::CONFIG_INVALID,
            "该 server 不支持 PKCE，请在配置里用 headers 直接传凭据",
        ));
    }
    let client_id = match client_id {
        Some(c) if !c.is_empty() => c.to_string(),
        _ => register_client(http, &meta, server_name).await?,
    };
    let (verifier, challenge) = crate::oauth_crypto::pkce_pair();
    let state = crate::oauth_crypto::random_state();
    let redirect_uri = format!("http://127.0.0.1:{CALLBACK_PORT}/callback");

    // 先把回调服务起起来：用户点授权可能很快，起晚了就收不到 code
    let listener = TcpListener::bind(("127.0.0.1", CALLBACK_PORT))
        .await
        .map_err(|e| AppError::wrap("9103", "起本地回调端口失败", e))?;
    let auth_url = format!(
        "{}?response_type=code&client_id={}&redirect_uri={}&code_challenge={}&code_challenge_method=S256&state={}",
        meta.authorization_endpoint,
        urlencode(&client_id),
        urlencode(&redirect_uri),
        urlencode(&challenge),
        urlencode(&state),
    );
    open_browser(&auth_url);

    let code = receive_code(listener, &state).await?;
    exchange_code(http, &meta, &client_id, &code, &verifier, &redirect_uri).await
}

/// 本地回调端口：固定值便于用户在 server 侧登记 redirect_uri。
const CALLBACK_PORT: u16 = 15232;

/// 收回调：等浏览器把 code 送回来。
async fn receive_code(listener: TcpListener, expect_state: &str) -> Result<String> {
    // 最多等 5 分钟：用户可能要先去找 token
    let accept = tokio::time::timeout(std::time::Duration::from_secs(300), listener.accept())
        .await
        .map_err(|_| AppError::new(wb_domain::error::mcp::CONFIG_INVALID, "等授权回调超时"))?;
    let (mut stream, _) = accept.map_err(|e| AppError::wrap("9103", "接收授权回调失败", e))?;
    let mut reader = BufReader::new(&mut stream);
    let mut request_line = String::new();
    reader
        .read_line(&mut request_line)
        .await
        .map_err(|e| AppError::wrap("9103", "读授权回调失败", e))?;
    // 把剩下的头读完：不等对方发完，浏览器会等响应形成死锁
    loop {
        let mut line = String::new();
        let n = reader
            .read_line(&mut line)
            .await
            .map_err(|e| AppError::wrap("9103", "读授权回调失败", e))?;
        if n == 0 || line.trim().is_empty() {
            break;
        }
    }
    let query = request_line
        .split_whitespace()
        .nth(1)
        .and_then(|p| p.split('?').nth(1))
        .unwrap_or("");
    let params = parse_query(query);
    let page: String = match (params.get("code"), params.get("state")) {
        (Some(_code), Some(st)) if st == expect_state => {
            "<!doctype html><meta charset=\"utf-8\"><body style=\"font-family:system-ui;padding:40px\"><h2>授权成功</h2><p>可以关闭这个标签页了。</p></body>"
                .to_string()
        }
        (Some(_), _) => "<!doctype html><meta charset=\"utf-8\"><body style=\"font-family:system-ui;padding:40px\"><h2>授权失败</h2><p>state 校验没过，请重试。</p></body>".to_string(),
        _ => "<!doctype html><meta charset=\"utf-8\"><body style=\"font-family:system-ui;padding:40px\"><h2>授权失败</h2><p>没有拿到授权码。</p></body>".to_string(),
    };
    let resp = format!(
        "HTTP/1.1 200 OK\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
        page.len(),
        page
    );
    let _ = stream.write_all(resp.as_bytes()).await;
    let _ = stream.flush().await;
    params
        .get("code")
        .cloned()
        .ok_or_else(|| AppError::new(wb_domain::error::mcp::CONFIG_INVALID, "回调里没有授权码"))
}

/// 用 code 换 token。
async fn exchange_code(
    http: &reqwest::Client,
    meta: &AuthServerMeta,
    client_id: &str,
    code: &str,
    verifier: &str,
    redirect_uri: &str,
) -> Result<OAuthOutcome> {
    let body = json!({
        "grant_type": "authorization_code",
        "code": code,
        "client_id": client_id,
        "code_verifier": verifier,
        "redirect_uri": redirect_uri,
    });
    let resp = http
        .post(&meta.token_endpoint)
        .header("Content-Type", "application/x-www-form-urlencoded")
        .form(&[
            ("grant_type", "authorization_code"),
            ("code", code),
            ("client_id", client_id),
            ("code_verifier", verifier),
            ("redirect_uri", redirect_uri),
        ])
        .send()
        .await
        .map_err(|e| AppError::wrap("9103", "换 token 请求失败", e))?;
    let status = resp.status();
    let text = resp.text().await.unwrap_or_default();
    if !status.is_success() {
        return Err(AppError::new(
            wb_domain::error::mcp::CONFIG_INVALID,
            format!("换 token 失败（{status}）：{text}"),
        ));
    }
    let v: Value = serde_json::from_str(&text).unwrap_or(Value::Null);
    let access_token = v["access_token"].as_str().unwrap_or("").to_string();
    if access_token.is_empty() {
        return Err(AppError::new(
            wb_domain::error::mcp::CONFIG_INVALID,
            "token 响应里没有 access_token",
        ));
    }
    let _ = body;
    Ok(OAuthOutcome {
        access_token,
        token_type: v["token_type"].as_str().unwrap_or("Bearer").to_string(),
        expires_in: v["expires_in"].as_i64().unwrap_or(0),
    })
}

/// 把 token 写进 server 配置的 headers。
pub fn apply_token(cfg: &mut wb_domain::mcp::McpServerConfig, outcome: &OAuthOutcome) {
    let mut headers = BTreeMap::new();
    headers.insert(
        "Authorization".to_string(),
        format!("{} {}", outcome.token_type, outcome.access_token),
    );
    cfg.headers.extend(headers);
}

/// 供上层复用的 HTTP 客户端。
pub fn http_client() -> Arc<reqwest::Client> {
    Arc::new(reqwest::Client::new())
}
