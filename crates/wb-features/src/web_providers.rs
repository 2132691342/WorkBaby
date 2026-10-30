//! 搜索源：多服务商适配与结果归一。
//!
//! 每个搜索源返回自己的页面结构，适配器负责把页面转成统一的 `SearchResult` 列表。
//! 解析失败就少一个源，不编造结果。

use serde_json::Value;

use super::web_parse::{clean_text, SearchResult};
use crate::web::USER_AGENT;

/// 一个搜索源。
pub struct SearchProvider {
    /// 源码（模型可见）
    pub code: &'static str,
    /// 显示名
    pub label: &'static str,
    /// 构造请求
    pub build: fn(&str) -> reqwest::RequestBuilder,
    /// 解析结果页
    pub parse: fn(&str) -> Vec<SearchResult>,
}

/// 搜索源表：按可靠性排序，前面的先试。
pub const PROVIDERS: &[SearchProvider] = &[
    SearchProvider {
        code: "duckduckgo",
        label: "DuckDuckGo",
        build: |q| {
            reqwest::Client::new()
                .post("https://html.duckduckgo.com/html/")
                .form(&[("q", q)])
        },
        parse: parse_duckduckgo,
    },
    SearchProvider {
        code: "bing",
        label: "Bing",
        build: |q| {
            reqwest::Client::new()
                .get("https://www.bing.com/search")
                .query(&[("q", q)])
                .header("User-Agent", USER_AGENT)
        },
        parse: parse_bing,
    },
    SearchProvider {
        code: "wikipedia",
        label: "Wikipedia",
        build: |q| {
            reqwest::Client::new()
                .get("https://en.wikipedia.org/w/api.php")
                .query(&[
                    ("action", "query"),
                    ("list", "search"),
                    ("srsearch", q),
                    ("format", "json"),
                    ("srlimit", "10"),
                ])
                .header("User-Agent", USER_AGENT)
        },
        parse: parse_wikipedia,
    },
];

/// 按码取源；未知码回落到第一个。
pub fn by_code(code: &str) -> Option<&'static SearchProvider> {
    PROVIDERS.iter().find(|p| p.code == code)
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
            title,
            url: href,
            snippet,
        });
    }
    out
}

/// Wikipedia 检索 API：JSON 响应。
pub fn parse_wikipedia(body: &str) -> Vec<SearchResult> {
    let Ok(v) = serde_json::from_str::<Value>(body) else {
        return Vec::new();
    };
    let Some(items) = v.pointer("/query/search").and_then(|x| x.as_array()) else {
        return Vec::new();
    };
    items
        .iter()
        .filter_map(|it| {
            let title = it.get("title")?.as_str()?.to_string();
            let snippet = it
                .get("snippet")
                .and_then(|s| s.as_str())
                .map(clean_text)
                .unwrap_or_default();
            let url = format!(
                "https://en.wikipedia.org/wiki/{}",
                title.replace(' ', "_")
            );
            Some(SearchResult {
                title,
                url,
                snippet,
            })
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
