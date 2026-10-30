//! LSP 文本编辑：位置换算与应用。协议细节在 client.rs，这里只管「改哪一段」。

use serde_json::Value;

/// 文本编辑（LSP TextEdit）。
pub struct TextEdit {
    /// 起始行（0 起）
    pub edit_start_line: u32,
    /// 起始字符（0 起）
    pub edit_start_char: u32,
    /// 结束行（0 起）
    pub edit_end_line: u32,
    /// 结束字符（0 起）
    pub edit_end_char: u32,
    /// 替换文本
    pub edit_new_text: String,
}

/// 从 workspace edit 里收集文本编辑（支持 changes 与 documentChanges 两种形态）。
pub fn collect_edits(edit: &Value, out: &mut Vec<TextEdit>) {
    if let Some(changes) = edit.get("changes").and_then(|v| v.as_object()) {
        for (_uri, list) in changes {
            if let Some(items) = list.as_array() {
                for it in items {
                    push_edit(it, out);
                }
            }
        }
    }
    if let Some(list) = edit.get("documentChanges").and_then(|v| v.as_array()) {
        for change in list {
            if let Some(edits) = change.get("edits").and_then(|v| v.as_array()) {
                for it in edits {
                    push_edit(it, out);
                }
            }
        }
    }
}

/// 解析一条 TextEdit。
fn push_edit(item: &Value, out: &mut Vec<TextEdit>) {
    let new_text = item
        .get("newText")
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    let Some(range) = item.get("range") else {
        return;
    };
    out.push(TextEdit {
        edit_start_line: range
            .pointer("/start/line")
            .and_then(|v| v.as_u64())
            .unwrap_or(0) as u32,
        edit_start_char: range
            .pointer("/start/character")
            .and_then(|v| v.as_u64())
            .unwrap_or(0) as u32,
        edit_end_line: range
            .pointer("/end/line")
            .and_then(|v| v.as_u64())
            .unwrap_or(0) as u32,
        edit_end_char: range
            .pointer("/end/character")
            .and_then(|v| v.as_u64())
            .unwrap_or(0) as u32,
        edit_new_text: new_text,
    });
}

/// 把 LSP 文本编辑应用到原文（按位置从后往前替换，避免位移）。
pub fn apply_edits(text: &str, edits: &[TextEdit]) -> String {
    let mut resolved: Vec<(usize, usize, String)> = Vec::new();
    for e in edits {
        let start = offset_at(text, e.edit_start_line, e.edit_start_char);
        let end = offset_at(text, e.edit_end_line, e.edit_end_char);
        resolved.push((start, end.max(start), e.edit_new_text.clone()));
    }
    // 同起点只保留第一条：server 返回重叠编辑时顺序不确定
    resolved.sort_by_key(|e| e.0);
    resolved.dedup_by(|a, b| a.0 == b.0);
    let mut out = String::with_capacity(text.len());
    let mut cursor = 0usize;
    for (start, end, new_text) in resolved {
        if start < cursor {
            continue;
        }
        out.push_str(&text[cursor..start]);
        out.push_str(&new_text);
        cursor = end;
    }
    out.push_str(&text[cursor..]);
    out
}

/// 行列转字符偏移；越界时钳到文本末尾。
fn offset_at(text: &str, line: u32, character: u32) -> usize {
    let mut cur_line = 0u32;
    let mut line_start = 0usize;
    for (i, ch) in text.char_indices() {
        if cur_line == line {
            line_start = i;
            break;
        }
        if ch == '\n' {
            cur_line += 1;
            line_start = i + 1;
        }
    }
    if cur_line != line {
        return text.len();
    }
    let line_end = text[line_start..]
        .find('\n')
        .map(|p| line_start + p)
        .unwrap_or(text.len());
    (line_start + character as usize).min(line_end)
}
