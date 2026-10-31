//! 视频理解：取视频页面的元数据与字幕轨。
//! 字幕是理解视频的主要入口——有字幕就抽字幕，没有就取页面上的标题与描述，
//! 都没有就如实说明，不编造视频内容。

use std::path::Path;
use std::time::{Duration, Instant};

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolAnnotations, ToolOutput};

use crate::web_parse::readable_text;

/// 单请求超时。
const HTTP_TIMEOUT: Duration = Duration::from_secs(20);
/// 字幕轨最多取几条。
const MAX_TRACKS: usize = 5;

/// video_understand 工具：抽取视频元数据与字幕。
pub struct VideoTool {
    http: reqwest::Client,
}

impl VideoTool {
    /// 装配。
    pub fn new() -> Self {
        Self {
            http: reqwest::Client::builder()
                .user_agent(crate::web::USER_AGENT)
                .timeout(HTTP_TIMEOUT)
                .build()
                .expect("HTTP client 构建失败"),
        }
    }
}

impl Default for VideoTool {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl Tool for VideoTool {
    fn name(&self) -> &str {
        "video_understand"
    }

    fn label(&self) -> &str {
        "视频理解"
    }

    fn description(&self) -> &str {
        "理解一个视频页面：取标题、时长、描述与字幕轨的文本。\
         YouTube 有字幕轨专门解析（按时间轴分段返回全文）；其余站点（含 bilibili）\
         解析页面元信息，能挖到字幕就用，挖不到就如实说明并回退元信息。\
         url 也可以是任意带 <video> 的页面。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "url": { "type": "string", "description": "视频页面地址" },
                "lang": { "type": "string", "description": "字幕语言偏好，如 en / zh；缺省取任意可用轨" },
                "max_chars": { "type": "number", "description": "返回的最大字符数，默认 20000" }
            },
            "required": ["url"]
        })
    }

    fn annotations(&self) -> ToolAnnotations {
        ToolAnnotations {
            read_only: true,
            destructive: false,
        }
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let url = args
            .get("url")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        if !(url.to_lowercase().starts_with("http://")
            || url.to_lowercase().starts_with("https://"))
        {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                "只允许 http/https 地址",
            ));
        }
        let max_chars = args
            .get("max_chars")
            .and_then(|v| v.as_u64())
            .unwrap_or(20_000)
            .clamp(200, 500_000) as usize;
        let lang = args
            .get("lang")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();

        let resp = tokio::select! {
            r = self.http.get(&url).send() => r,
            _ = cancel.cancelled() => {
                return Err(AppError::new(toolcode::EXEC_CANCELLED, "请求已随会话取消"));
            }
        }
        .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("请求失败：{e}")))?;
        if !resp.status().is_success() {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                format!("目标返回非 2xx 状态：{}", resp.status().as_u16()),
            ));
        }
        let html = resp
            .text()
            .await
            .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("读取响应失败：{e}")))?;

        let mut out = String::new();
        let title = meta_content(&html, "og:title").or_else(|| page_title(&html));
        let desc =
            meta_content(&html, "og:description").or_else(|| meta_content(&html, "description"));
        if let Some(t) = title {
            out.push_str(&format!("标题：{t}\n"));
        }
        if let Some(d) = desc {
            out.push_str(&format!("描述：{d}\n"));
        }

        // 字幕轨：YouTube 的 playerResponse 里带 captionTracks。
        let tracks = caption_tracks(&html);
        if tracks.is_empty() {
            out.push_str("\n（这个页面没有暴露字幕轨，只能读到上面的元信息）\n");
        } else {
            out.push_str(&format!("\n字幕轨：{} 条\n", tracks.len()));
            let picked = pick_track(&tracks, &lang);
            if let Some(t) = picked {
                out.push_str(&format!("（选用 {}）\n\n", t.label));
                match self.fetch_caption(&t.url, &cancel).await {
                    Ok(text) => out.push_str(&text),
                    Err(e) => out.push_str(&format!("（字幕拉取失败：{}）", e.message)),
                }
            }
        }

        // 页面正文兜底：很多站点把简介放在正文里。
        if out.trim().len() < 80 {
            let body = readable_text(&html);
            if body.len() > 80 {
                out.push_str("\n页面正文：\n");
                out.push_str(&body);
            }
        }

        Ok(ToolOutput {
            text: truncate_chars(out.trim(), max_chars),
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}

impl VideoTool {
    /// 拉字幕轨内容：优先 XML 格式（带时间轴），退化到纯文本。
    async fn fetch_caption(&self, url: &str, cancel: &CancellationToken) -> Result<String> {
        let resp = tokio::select! {
            r = self.http.get(url).send() => r,
            _ = cancel.cancelled() => {
                return Err(AppError::new(toolcode::EXEC_CANCELLED, "请求已随会话取消"));
            }
        }
        .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("拉字幕失败：{e}")))?;
        if !resp.status().is_success() {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                format!("字幕返回非 2xx：{}", resp.status().as_u16()),
            ));
        }
        let body = resp
            .text()
            .await
            .map_err(|e| AppError::new(toolcode::ARG_INVALID, format!("读字幕失败：{e}")))?;
        Ok(parse_caption(&body))
    }
}

/// 一条字幕轨。
#[derive(Clone)]
pub struct CaptionTrack {
    /// 轨道标签（语言名）
    pub label: String,
    /// 字幕地址
    pub url: String,
}

/// 取页面上的 og/meta 属性。
fn meta_content(html: &str, prop: &str) -> Option<String> {
    let needle = format!("\"{prop}\"");
    let pos = html.find(&needle)? + needle.len();
    let rest = &html[pos..];
    let content = rest.find("content=\"")? + "content=\"".len();
    let value = &rest[content..];
    let end = value.find('"')?;
    let v = value[..end].trim();
    if v.is_empty() {
        None
    } else {
        Some(v.to_string())
    }
}

/// 取 <title> 文本。
fn page_title(html: &str) -> Option<String> {
    let start = html.find("<title>")? + "<title>".len();
    let end = html[start..].find("</title>")? + start;
    let t = html[start..end].trim();
    if t.is_empty() {
        None
    } else {
        Some(t.to_string())
    }
}

/// 从页面里抠出字幕轨列表（playerResponse 的 captionTracks）。
pub fn caption_tracks(html: &str) -> Vec<CaptionTrack> {
    let Some(start) = html.find("\"captionTracks\":") else {
        return Vec::new();
    };
    let rest = &html[start..];
    let end = rest.find(']').unwrap_or(rest.len());
    let blob = &rest[..end];
    let mut out = Vec::new();
    let mut cursor = 0usize;
    while let Some(pos) = blob[cursor..].find("\"baseUrl\":\"") {
        let chunk = &blob[cursor + pos..];
        let Some(url) = quoted_after(chunk, "baseUrl") else {
            break;
        };
        let label = quoted_after(chunk, "simpleText")
            .or_else(|| quoted_after(chunk, "name"))
            .or_else(|| quoted_after(chunk, "vssId"))
            .unwrap_or_else(|| "字幕".to_string());
        out.push(CaptionTrack { label, url });
        if out.len() >= MAX_TRACKS {
            break;
        }
        cursor += pos + 1;
    }
    out
}

/// 抠出 JSON 字符串值（含转义还原）。
fn quoted_after(blob: &str, key: &str) -> Option<String> {
    let needle = format!("\"{key}\":\"");
    let pos = blob.find(&needle)? + needle.len();
    let rest = &blob[pos..];
    let mut out = String::new();
    let mut chars = rest.chars();
    while let Some(c) = chars.next() {
        match c {
            '"' => break,
            '\\' => match chars.next() {
                Some('n') => out.push('\n'),
                Some('u') => {
                    let hex: String = chars.by_ref().take(4).collect();
                    if let Ok(cp) = u32::from_str_radix(&hex, 16) {
                        if let Some(ch) = char::from_u32(cp) {
                            out.push(ch);
                        }
                    }
                }
                Some(other) => out.push(other),
                None => break,
            },
            other => out.push(other),
        }
    }
    if out.is_empty() {
        None
    } else {
        Some(out)
    }
}

/// 按语言偏好挑字幕轨；无偏好时取第一条。
/// 语言标记可能只出现在地址里（lang=zh），所以两处都看。
pub fn pick_track(tracks: &[CaptionTrack], lang: &str) -> Option<CaptionTrack> {
    if lang.is_empty() {
        return tracks.first().cloned();
    }
    let want = lang.to_lowercase();
    tracks
        .iter()
        .find(|t| {
            t.label.to_lowercase().contains(&want)
                || t.url.to_lowercase().contains(&format!("lang={want}"))
        })
        .or_else(|| {
            tracks
                .iter()
                .find(|t| t.label.to_lowercase().starts_with(&want))
        })
        .cloned()
        .or_else(|| tracks.first().cloned())
}

/// 解析字幕正文：带时间轴的 XML 取文本节点，纯文本原样返回。
pub fn parse_caption(body: &str) -> String {
    if !body.contains("<text") && !body.contains("</text>") {
        return body.trim().to_string();
    }
    let mut out = String::new();
    let mut rest = body;
    while let Some(start) = rest.find("<text") {
        let after = &rest[start..];
        let Some(open_end) = after.find('>') else {
            break;
        };
        let inner_start = open_end + 1;
        let Some(close) = after[inner_start..].find("</text>") else {
            break;
        };
        let raw = &after[inner_start..inner_start + close];
        out.push_str(&decode_entities(raw.trim()));
        out.push('\n');
        rest = &after[inner_start + close..];
    }
    out.trim().to_string()
}

/// 解常见实体。
fn decode_entities(s: &str) -> String {
    s.replace("&amp;", "&")
        .replace("&lt;", "<")
        .replace("&gt;", ">")
        .replace("&quot;", "\"")
        .replace("&#39;", "'")
        .replace("&apos;", "'")
        .replace("&nbsp;", " ")
}

/// 按字符截断（不切 UTF-8），超长加省略说明。
fn truncate_chars(s: &str, max: usize) -> String {
    let count = s.chars().count();
    if count <= max {
        return s.to_string();
    }
    let head: String = s.chars().take(max).collect();
    format!("{head}…（已截断，原文 {count} 字符）")
}
