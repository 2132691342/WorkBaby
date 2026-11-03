//! token 估算：按字符类别加权。中文一字一 token，西文约四字一 token，
//! 其余文字折中——比「字节数 ÷ 4」更准，压缩与水位线不会因中文被低估。

use crate::session::{Block, Msg};

/// 西文字符数对应的 token 数。
const WESTERN_CHARS_PER_TOKEN: f64 = 4.0;
/// 中日韩字符数对应的 token 数。
const CJK_CHARS_PER_TOKEN: f64 = 1.0;
/// 其他文字字符数对应的 token 数。
const OTHER_CHARS_PER_TOKEN: f64 = 2.0;
/// 一张图片折算的 token 数：视觉模型给的是 tile 数，估个均值够用。
const IMAGE_TOKENS: i64 = 1_500;

/// 文本的 token 估算。
pub fn estimate_text_tokens(text: &str) -> i64 {
    let mut western = 0i64;
    let mut cjk = 0i64;
    let mut other = 0i64;
    for ch in text.chars() {
        if is_cjk(ch) {
            cjk += 1;
        } else if ch.is_ascii() {
            western += 1;
        } else {
            other += 1;
        }
    }
    (western as f64 / WESTERN_CHARS_PER_TOKEN
        + cjk as f64 / CJK_CHARS_PER_TOKEN
        + other as f64 / OTHER_CHARS_PER_TOKEN)
        .ceil() as i64
}

/// 是否是中日韩文字（含日文假名、韩文谚文与全角标点）。
fn is_cjk(ch: char) -> bool {
    matches!(ch as u32,
        0x3000..=0x30FF // CJK 标点与日文假名
        | 0x3400..=0x4DBF // 扩展 A
        | 0x4E00..=0x9FFF // 基本汉字
        | 0xF900..=0xFAFF // 兼容汉字
        | 0xAC00..=0xD7AF // 谚文音节
        | 0xFF00..=0xFFEF // 全角字符
        | 0x20000..=0x2FA1F // 扩展 B~F
    )
}

/// 消息序列的 token 估算。
/// 图片按固定值折算：视觉模型给的是 tile 数，没有统一口径时估算即可。
pub fn estimate_tokens(msgs: &[Msg]) -> i64 {
    let mut total = 0i64;
    for m in msgs {
        match m {
            Msg::User {
                content, images, ..
            } => {
                total += estimate_text_tokens(content);
                total += images.len() as i64 * IMAGE_TOKENS;
            }
            Msg::Assistant { content, .. } => {
                for b in content {
                    match b {
                        Block::Text { text } => total += estimate_text_tokens(text),
                        Block::Thinking { thinking } => total += estimate_text_tokens(thinking),
                        Block::ToolCall {
                            name, arguments, ..
                        } => {
                            total += estimate_text_tokens(name);
                            total += estimate_text_tokens(arguments);
                        }
                    }
                }
            }
            Msg::ToolResult {
                content, images, ..
            } => {
                total += estimate_text_tokens(content);
                total += images.len() as i64 * IMAGE_TOKENS;
            }
        }
    }
    total
}

/// 上下文占用：优先用上游报的真实用量，而不是自己估。
///
/// 估算永远有偏差，而上游报的是「实际处理了多少 token」。所以口径是：
/// 找到最近一条带用量的助手消息，以它的上下文占用为基线，
/// 再加上它之后新增消息的估算值。
///
/// 没有可用用量时（会话刚开始、上游没报）退回纯估算。
pub fn context_tokens(msgs: &[Msg], overhead: i64) -> i64 {
    let Some((idx, base)) = last_usage_baseline(msgs) else {
        return estimate_tokens(msgs) + overhead;
    };
    let trailing: i64 = msgs[idx + 1..].iter().map(estimate_tokens_of).sum();
    base + trailing + overhead
}

/// 估算单条消息的 token。
fn estimate_tokens_of(m: &Msg) -> i64 {
    estimate_tokens(std::slice::from_ref(m))
}

/// 最近一条带用量的助手消息及其上下文占用基线。
/// 返回 (下标, 基线 token)。
fn last_usage_baseline(msgs: &[Msg]) -> Option<(usize, i64)> {
    let mut found: Option<(usize, i64)> = None;
    for (i, m) in msgs.iter().enumerate() {
        if let Msg::Assistant {
            usage: Some(u),
            stop_reason,
            ..
        } = m
        {
            // 异常终止的那轮没跑完，上下文没真正建立起来，不能当基线
            if matches!(stop_reason.as_str(), "aborted" | "error") {
                continue;
            }
            let total = u.total();
            if total > 0 {
                found = Some((i, total));
            }
        }
    }
    found
}
