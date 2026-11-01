//! 搜索源：多服务商适配与结果归一。
//! 配了密钥的 API 源排在前面（结果稳定、不怕页面改版），
//! 没配密钥时回落到公开页面抓取。解析失败就少一个源，不编造结果。

use serde_json::Value;

use super::web_parse::{clean_text, SearchResult};
use crate::web::USER_AGENT;

/// 一个搜索源。
pub struct SearchProvider {
    /// 源码（模型可见）
    pub provider_code: &'static str,
    /// 展示名
    pub provider_label: &'static str,
    /// 取密钥的环境变量名；空表 = 免密钥公开源
    pub provider_env_keys: &'static [&'static str],
    /// 构造请求（第二参是已解析的 API key，公开源忽略）
    pub build_fn: fn(&str, Option<&str>) -> reqwest::RequestBuilder,
    /// 解析响应正文
    pub parse_fn: fn(&str) -> Vec<SearchResult>,
}

/// 搜索源表：API 源在前，公开抓取源兜底。
pub const PROVIDERS: &[SearchProvider] = &[
    SearchProvider {
        provider_code: "brave",
        provider_label: "Brave Search",
        provider_env_keys: &["BRAVE_API_KEY"],
        build_fn: build_brave,
        parse_fn: parse_brave,
    },
    SearchProvider {
        provider_code: "tavily",
        provider_label: "Tavily",
        provider_env_keys: &["TAVILY_API_KEY"],
        build_fn: build_tavily,
        parse_fn: parse_tavily,
    },
    SearchProvider {
        provider_code: "serper",
        provider_label: "Serper",
        provider_env_keys: &["SERPER_API_KEY"],
        build_fn: build_serper,
        parse_fn: parse_serper,
    },
    SearchProvider {
        provider_code: "exa",
        provider_label: "Exa",
        provider_env_keys: &["EXA_API_KEY"],
        build_fn: build_exa,
        parse_fn: parse_exa,
    },
    SearchProvider {
        provider_code: "duckduckgo",
        provider_label: "DuckDuckGo",
        provider_env_keys: &[],
        build_fn: build_duckduckgo,
        parse_fn: parse_duckduckgo,
    },
    SearchProvider {
        provider_code: "bing",
        provider_label: "Bing",
        provider_env_keys: &[],
        build_fn: build_bing,
        parse_fn: parse_bing,
    },
    SearchProvider {
        provider_code: "wikipedia",
        provider_label: "Wikipedia",
        provider_env_keys: &[],
        build_fn: build_wikipedia,
        parse_fn: parse_wikipedia,
    },
];

/// 已配置的密钥：按变量名顺序取第一个非空值。
pub fn key_for(provider: &'static SearchProvider) -> Option<String> {
    provider
        .provider_env_keys
        .iter()
        .find_map(|name| std::env::var(name).ok())
        .map(|v| v.trim().to_string())
        .filter(|v| !v.is_empty())
}

/// 该源是否必须有密钥才能用。
pub fn needs_key(provider: &'static SearchProvider) -> bool {
    !provider.provider_env_keys.is_empty()
}

/// 本轮可尝试的源（按优先级）：有密钥的 API 源 + 全部免密钥公开源。
///  wanted 非空时只返回该源，调用方自己校验密钥是否齐备。
pub fn queue_for(wanted: &str) -> Vec<(&'static SearchProvider, Option<String>)> {
    if !wanted.is_empty() {
        return by_code(wanted)
            .map(|p| vec![(p, key_for(p))])
            .unwrap_or_default();
    }
    let mut out: Vec<(&'static SearchProvider, Option<String>)> = PROVIDERS
        .iter()
        .filter(|p| needs_key(p))
        .filter_map(|p| key_for(p).map(|k| (p, Some(k))))
        .collect();
    out.extend(
        PROVIDERS
            .iter()
            .filter(|p| !needs_key(p))
            .map(|p| (p, None)),
    );
    out
}

/// 按码取源；未知码返回 None。
pub fn by_code(code: &str) -> Option<&'static SearchProvider> {
    PROVIDERS.iter().find(|p| p.provider_code == code)
}

/// 全部源码（工具参数的 enum 列表，避免手写清单与实际源脱节）。
pub fn codes() -> Vec<&'static str> {
    PROVIDERS.iter().map(|p| p.provider_code).collect()
}

/// 需要密钥却没配的源码列表（界面提示用户用）。
pub fn missing_key_codes() -> Vec<&'static str> {
    PROVIDERS
        .iter()
        .filter(|p| needs_key(p) && key_for(p).is_none())
        .map(|p| p.provider_code)
        .collect()
}

// ---------- 请求构造 ----------

/// Brave：REST + X-Subscription-Token。
fn build_brave(query: &str, key: Option<&str>) -> reqwest::RequestBuilder {
    reqwest::Client::new()
        .get("https://api.search.brave.com/res/v1/web/search")
        .query(&[("q", query), ("count", "10")])
        .header("X-Subscription-Token", key.unwrap_or(""))
        .header("Accept", "application/json")
        .header("User-Agent", USER_AGENT)
}

/// Tavily：POST JSON，密钥放在请求体里。
fn build_tavily(query: &str, key: Option<&str>) -> reqwest::RequestBuilder {
    reqwest::Client::new()
        .post("https://api.tavily.com/search")
        .json(&serde_json::json!({
            "api_key": key.unwrap_or(""),
            "query": query,
            "max_results": 10
        }))
        .header("User-Agent", USER_AGENT)
}

/// Serper：Google SERP API，POST JSON + X-API-KEY。
fn build_serper(query: &str, key: Option<&str>) -> reqwest::RequestBuilder {
    reqwest::Client::new()
        .post("https://google.serper.dev/search")
        .header("X-API-KEY", key.unwrap_or(""))
        .json(&serde_json::json!({ "q": query, "num": 10 }))
        .header("User-Agent", USER_AGENT)
}

/// Exa：语义检索，带正文片段。
fn build_exa(query: &str, key: Option<&str>) -> reqwest::RequestBuilder {
    reqwest::Client::new()
        .post("https://api.exa.ai/search")
        .header("x-api-key", key.unwrap_or(""))
        .json(&serde_json::json!({
            "query": query,
            "numResults": 10,
            "text": true
        }))
        .header("User-Agent", USER_AGENT)
}

/// DuckDuckGo 的 HTML 版：无需密钥。
fn build_duckduckgo(query: &str, _key: Option<&str>) -> reqwest::RequestBuilder {
    reqwest::Client::new()
        .post("https://html.duckduckgo.com/html/")
        .form(&[("q", query)])
}

/// Bing：公开搜索页。
fn build_bing(query: &str, _key: Option<&str>) -> reqwest::RequestBuilder {
    reqwest::Client::new()
        .get("https://www.bing.com/search")
        .query(&[("q", query)])
        .header("User-Agent", USER_AGENT)
}

/// Wikipedia 检索 API：公开 JSON。
fn build_wikipedia(query: &str, _key: Option<&str>) -> reqwest::RequestBuilder {
    reqwest::Client::new()
        .get("https://en.wikipedia.org/w/api.php")
        .query(&[
            ("action", "query"),
            ("list", "search"),
            ("srsearch", query),
            ("format", "json"),
            ("srlimit", "10"),
        ])
        .header("User-Agent", USER_AGENT)
}

// ---------- 响应解析 ----------

/// 按 JSON 指针取数组，逐条抽 标题 / 链接 / 摘要 三字段。
/// 各源字段名不同，所以键名交给调用方。
fn json_rows(
    body: &str,
    pointer: &str,
    title_key: &str,
    url_key: &str,
    snippet_key: &str,
) -> Vec<SearchResult> {
    let Ok(v) = serde_json::from_str::<Value>(body) else {
        return Vec::new();
    };
    let Some(items) = v.pointer(pointer).and_then(|x| x.as_array()) else {
        return Vec::new();
    };
    items
        .iter()
        .filter_map(|it| {
            let title = clean_text(it.get(title_key)?.as_str()?);
            let url = it.get(url_key)?.as_str()?;
            if title.is_empty() || url.is_empty() {
                return None;
            }
            let snippet = it
                .get(snippet_key)
                .and_then(|s| s.as_str())
                .map(clean_text)
                .unwrap_or_default();
            Some(SearchResult {
                result_title: title,
                result_url: url.to_string(),
                result_snippet: snippet,
            })
        })
        .collect()
}

/// Brave：web.results[]。
pub fn parse_brave(body: &str) -> Vec<SearchResult> {
    json_rows(body, "/web/results", "title", "url", "description")
}

/// Tavily：results[]，正文摘要在 content。
pub fn parse_tavily(body: &str) -> Vec<SearchResult> {
    json_rows(body, "/results", "title", "url", "content")
}

/// Serper：organic[]，链接字段是 link。
pub fn parse_serper(body: &str) -> Vec<SearchResult> {
    json_rows(body, "/organic", "title", "link", "snippet")
}

/// Exa：results[]，摘要用 text（正文片段）。
pub fn parse_exa(body: &str) -> Vec<SearchResult> {
    json_rows(body, "/results", "title", "url", "text")
}

/// DuckDuckGo HTML 版：结果在 class 含 result__a 的锚点里。
pub fn parse_duckduckgo(html: &str) -> Vec<SearchResult> {
    super::web_parse::parse_results(html)
}

/// Bing：结果块是 <li class="b_algo">，标题在 <h2><a>。
pub fn parse_bing(html: &str) -> Vec<SearchResult> {
    let blocks = split_blocks(html, "b_algo");
    let mut out = Vec::new();
    for b in blocks {
        let Some(href) = attr_in(&b, "href") else {
            continue;
        };
        let title = clean_text(&tag_inner(&b, "h2"));
        if title.is_empty() {
            continue;
        }
        let snippet = clean_text(&tag_inner(&b, "p"));
        out.push(SearchResult {
            result_title: title,
            result_url: href,
            result_snippet: snippet,
        });
    }
    out
}

/// Wikipedia 检索 API：标题拼 URL，摘要去标签。
pub fn parse_wikipedia(body: &str) -> Vec<SearchResult> {
    json_rows(body, "/query/search", "title", "title", "snippet")
        .into_iter()
        .map(|mut r| {
            r.result_url = format!(
                "https://en.wikipedia.org/wiki/{}",
                r.result_title.replace(' ', "_")
            );
            r
        })
        .collect()
}

/// 按 class 名切块：找 class 含目标串的标签起点，向下取到配对的闭合标签。
fn split_blocks(html: &str, class: &str) -> Vec<String> {
    let mut out = Vec::new();
    let mut rest = html;
    while let Some(pos) = rest.find(class) {
        // 回退到该 class 所在的标签起点。
        let start = rest[..pos].rfind('<').unwrap_or(0);
        let Some(tag_end) = rest[start..].find('>').map(|i| start + i) else {
            break;
        };
        let Some(close) = rest[tag_end..].find("</li>").map(|i| tag_end + i) else {
            break;
        };
        out.push(rest[start..close + 5].to_string());
        rest = &rest[close + 5..];
    }
    out
}

/// 取标签内的文本（取第一个匹配标签）。
fn tag_inner(html: &str, tag: &str) -> String {
    let open = format!("<{tag}");
    let Some(start) = html.find(&open) else {
        return String::new();
    };
    let Some(tag_end) = html[start..].find('>').map(|i| start + i) else {
        return String::new();
    };
    let close = format!("</{tag}>");
    let Some(end) = html[tag_end..].find(&close).map(|i| tag_end + i) else {
        return String::new();
    };
    html[tag_end + 1..end].to_string()
}

/// 取标签上的属性值。
fn attr_in(html: &str, attr: &str) -> Option<String> {
    let pat = format!("{attr}=\"");
    let start = html.find(&pat)? + pat.len();
    let end = html[start..].find('"')? + start;
    Some(html[start..end].to_string())
}
