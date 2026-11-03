//! 文本行尾与 BOM：改文件时的跨平台口径。
//!
//! Windows 仓库里的文件多是 CRLF，而模型从 `read` 工具看到的是按行切掉的文本。
//! 两者对不上就会「明明见过这段代码，old_text 却说找不到」。

/// 文本行尾风格。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LineEnding {
    /// LF（\n）：类 Unix 与新建文件默认。
    Lf,
    /// CRLF（\r\n）：Windows 仓库主流。
    Crlf,
}

/// 探测文本的主导行尾：CRLF 与 LF 哪个多算哪个，一样多时取 LF。
/// 逐行统计而不是「首个行尾说了算」——混合行尾的文件里首个可能是偶然的。
pub fn detect_line_ending(text: &str) -> LineEnding {
    let bytes = text.as_bytes();
    let mut crlf = 0usize;
    let mut lf = 0usize;
    for (i, b) in bytes.iter().enumerate() {
        if *b != b'\n' {
            continue;
        }
        if i > 0 && bytes[i - 1] == b'\r' {
            crlf += 1;
        } else {
            lf += 1;
        }
    }
    if crlf > lf {
        LineEnding::Crlf
    } else {
        LineEnding::Lf
    }
}

/// 归一成 LF：匹配前统一口径，模型的 old_text 与文件内容就此可比。
pub fn normalize_to_lf(text: &str) -> String {
    if !text.contains('\r') {
        return text.to_string();
    }
    text.replace("\r\n", "\n").replace('\r', "\n")
}

/// 按给定行尾还原：归一后再统一换成目标风格，避免混合行尾。
pub fn apply_line_ending(text: &str, ending: LineEnding) -> String {
    let lf = normalize_to_lf(text);
    match ending {
        LineEnding::Lf => lf,
        LineEnding::Crlf => lf.replace('\n', "\r\n"),
    }
}

/// 摘掉 UTF-8 BOM：BOM 不是正文，留在正文里会让首个 old_text 匹配不上。
/// 返回 (BOM, 正文)；没有 BOM 时 BOM 为空串。
pub fn strip_bom(text: &str) -> (&str, &str) {
    match text.strip_prefix('\u{feff}') {
        Some(rest) => ("\u{feff}", rest),
        None => ("", text),
    }
}
