//! 联网解析：HTML 正文提取（readable）与搜索结果页解析。

use std::sync::OnceLock;

use regex::Regex;

/// 需整块剥除的元素（脚本样式承载代码，结构块不是正文）。
const STRIP_TAGS: &[&str] = &["script", "style", "nav", "header", "footer", "aside"];

/// 静态剥块正则：每个标签一条 <tag …>…</tag>。
fn strip_rules() -> &'static Vec<Regex> {
    static RULES: OnceLock<Vec<Regex>> = OnceLock::new();
    RULES.get_or_init(|| {
        STRIP_TAGS
            .iter()
            .map(|tag| {
                Regex::new(&format!(r"(?is)<{tag}\b[^>]*>.*?</{tag}>")).expect("静态正则合法")
            })
            .collect()
    })
}

/// 去全部标签。
fn tag_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| Regex::new(r"(?s)<[^>]*>").expect("静态正则合法"))
}

/// 去注释。
fn comment_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| Regex::new(r"(?s)<!--.*?-->").expect("静态正则合法"))
}

/// 压缩空白串。
fn ws_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| Regex::new(r"\s+").expect("静态正则合法"))
}

/// 解常见 HTML 实体；&amp; 最后处理避免二次解。
fn html_unescape(s: &str) -> String {
    s.replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&quot;", "\"")
        .replace("&#39;", "'")
        .replace("&nbsp;", " ")
        .replace("&amp;", "&")
}

/// 去标签 + 解实体 + 压空白，得到纯文本。
fn clean_html_text(s: &str) -> String {
    let no_tags = tag_re().replace_all(s, "");
    ws_re()
        .replace_all(html_unescape(&no_tags).trim(), " ")
        .into_owned()
}

/// readable 提取：剥脚本样式与结构块、去标签、解实体、压空白。
pub fn readable_text(html: &str) -> String {
    let mut s = comment_re().replace_all(html, "").into_owned();
    for rule in strip_rules() {
        s = rule.replace_all(&s, "").into_owned();
    }
    clean_html_text(&s)
}

/// 一条搜索结果。
pub struct SearchResult {
    /// 标题（已去标签解实体）
    pub title: String,
    /// 目标 URL（已从跳转链接还原）
    pub url: String,
    /// 摘要（已去标签解实体，可能为空）
    pub snippet: String,
}

/// 结果锚点：标题在 inner，href 另行提取（不依赖属性顺序）。
fn anchor_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| {
        Regex::new(r#"(?is)<a\b[^>]*class="[^"]*result__a[^"]*"[^>]*>(.*?)</a>"#)
            .expect("静态正则合法")
    })
}

/// 摘要锚点。
fn snippet_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| {
        Regex::new(r#"(?is)<a\b[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>"#)
            .expect("静态正则合法")
    })
}

/// 从锚点开始标签里抽 href。
fn href_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    RE.get_or_init(|| Regex::new(r#"href="([^"]+)""#).expect("静态正则合法"))
}

/// 解析搜索结果页；结构对不上返回空（调用方如实告知，不编造）。
pub fn parse_results(html: &str) -> Vec<SearchResult> {
    let items: Vec<(String, String)> = anchor_re()
        .captures_iter(html)
        .filter_map(|cap| {
            let anchor = cap.get(0)?.as_str();
            let open = anchor.split_once('>')?.0;
            let href = href_re().captures(open)?.get(1)?.as_str();
            Some((unwrap_redirect_href(href), clean_html_text(&cap[1])))
        })
        .collect();
    let snippets: Vec<String> = snippet_re()
        .captures_iter(html)
        .map(|cap| clean_html_text(&cap[1]))
        .collect();
    items
        .into_iter()
        .enumerate()
        .map(|(i, (url, title))| SearchResult {
            title,
            url,
            snippet: snippets.get(i).cloned().unwrap_or_default(),
        })
        .collect()
}

/// 结果 href 常是跳转链接（//域名/l/?uddg=编码URL&rut=…）；还原真实地址。
fn unwrap_redirect_href(href: &str) -> String {
    let trimmed = href.trim();
    let target = trimmed
        .strip_prefix("//")
        .map(|rest| format!("https://{rest}"))
        .unwrap_or_else(|| trimmed.to_string());
    if let Some(pos) = target.find("uddg=") {
        let encoded = &target[pos + "uddg=".len()..];
        let end = encoded.find('&').unwrap_or(encoded.len());
        let decoded = percent_decode(&encoded[..end]);
        if decoded.starts_with("http") {
            return decoded;
        }
    }
    target
}

/// 最小百分号解码；不完整转义原样保留（不翻译 +，跳转参数是严格百分号编码）。
fn percent_decode(s: &str) -> String {
    let bytes = s.as_bytes();
    let mut out: Vec<u8> = Vec::with_capacity(bytes.len());
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i] == b'%' && i + 2 < bytes.len() {
            let hex = std::str::from_utf8(&bytes[i + 1..i + 3]).unwrap_or("");
            if let Ok(v) = u8::from_str_radix(hex, 16) {
                out.push(v);
                i += 3;
                continue;
            }
        }
        out.push(bytes[i]);
        i += 1;
    }
    String::from_utf8_lossy(&out).into_owned()
}
