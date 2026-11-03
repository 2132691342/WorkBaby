//! 会话导出：自包含单文件 HTML（无外部依赖，双击即开）。

use std::sync::Arc;

use wb_pkg::{AppError, Result};

use super::store::detail;
use super::Services;

fn esc(s: &str) -> String {
    s.replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
}

/// 导出会话为 HTML（自包含单文件，无外部依赖）。
pub fn export_html(sv: &Arc<Services>, session_id: &str, out_path: &str) -> Result<String> {
    let detail = detail(sv, session_id)?;
    let mut body = String::new();
    for m in &detail.detail_messages {
        if m.message_hidden {
            continue;
        }
        let who = match m.message_role.as_str() {
            "user" => "用户",
            "assistant" => "助手",
            "tool" => "工具",
            _ => "提示",
        };
        let text = m
            .message_notice
            .clone()
            .or_else(|| m.message_content.clone())
            .unwrap_or_default();
        body.push_str(&format!(
            "<div class=\"m {}\"><span class=\"who\">{}</span><pre>{}</pre></div>\n",
            m.message_role,
            who,
            esc(&text)
        ));
    }
    let html = format!(
        "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>{}</title>\
<style>body{{font-family:system-ui,sans-serif;max-width:820px;margin:0 auto;padding:24px;color:#1e293b}}\
.m{{margin:12px 0;padding:12px;border-radius:8px;background:#f8fafc}}\
.m.assistant{{background:#e0e7ff}}\
.m .who{{font-weight:700;font-size:12px;color:#6366f1;display:block;margin-bottom:4px}}\
pre{{white-space:pre-wrap;word-break:break-word;margin:0;font-family:inherit}}</style></head>\
<body><h1>{}</h1>{body}</body></html>",
        esc(&detail.detail_session.session_title),
        esc(&detail.detail_session.session_title),
    );
    let path = if out_path.is_empty() {
        sv.data_dir
            .join("exports")
            .join(format!("{session_id}.html"))
    } else {
        std::path::PathBuf::from(out_path)
    };
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).map_err(|e| AppError::wrap("1002", "建导出目录失败", e))?;
    }
    std::fs::write(&path, html).map_err(|e| AppError::wrap("1002", "写导出文件失败", e))?;
    Ok(path.display().to_string())
}
