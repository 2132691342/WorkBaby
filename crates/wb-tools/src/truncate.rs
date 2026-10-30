//! 输出截断：文本 2000 行 / 50KB；grep 单行 500 字符。

/// 最大行数
pub const MAX_LINES: usize = 2000;
/// 最大字节数
pub const MAX_BYTES: usize = 50 * 1024;
/// grep 单行上限
pub const GREP_MAX_LINE: usize = 500;

/// 按行数与字节双限截断；发生截断时尾部加提示行。
pub fn truncate(text: &str) -> String {
    if text.lines().count() <= MAX_LINES && text.len() <= MAX_BYTES {
        return text.to_string();
    }
    let mut out = String::new();
    let mut bytes = 0usize;
    for (lines, line) in text.lines().enumerate() {
        if lines >= MAX_LINES || bytes + line.len() > MAX_BYTES {
            out.push_str(&format!(
                "\n[输出已截断：超过 {MAX_LINES} 行 / {MAX_BYTES} 字节上限，仅展示前段]"
            ));
            return out;
        }
        out.push_str(line);
        out.push('\n');
        bytes += line.len() + 1;
    }
    out
}

/// grep 行级截断：超长行剪到 500 字符加省略号。
pub fn clip_line(line: &str) -> String {
    if line.len() <= GREP_MAX_LINE {
        return line.to_string();
    }
    let cut: String = line.chars().take(GREP_MAX_LINE).collect();
    format!("{cut}…[行截断]")
}
