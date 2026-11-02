//! 文本改动 → 差异行：写文件类工具改完就地生成行级 diff，界面直接着色。
//! 只做按行的最长公共子序列之外的朴素比对（逐行比对 + 前缀/后缀裁剪），
//! 目的只是让用户看清「改了哪几行」，不是产出一份可 patch 的补丁。

use crate::registry::DiffLine;

/// 单次比对最多保留的差异行数：超长改动只保留头尾，避免把界面撑爆。
const MAX_DIFF_LINES: usize = 400;

/// 算两份文本的行级差异（裁剪公共前后缀后再逐行标注）。
pub fn line_diff(before: &str, after: &str) -> Vec<DiffLine> {
    let a: Vec<&str> = before.lines().collect();
    let b: Vec<&str> = after.lines().collect();
    let mut head = 0usize;
    while head < a.len() && head < b.len() && a[head] == b[head] {
        head += 1;
    }
    let mut tail = 0usize;
    while tail < a.len().saturating_sub(head)
        && tail < b.len().saturating_sub(head)
        && a[a.len() - 1 - tail] == b[b.len() - 1 - tail]
    {
        tail += 1;
    }
    let mut out: Vec<DiffLine> = Vec::new();
    // 头部上下文：留两行让读者知道改动落在哪
    let ctx_start = head.saturating_sub(2);
    for line in &a[ctx_start..head] {
        out.push(ctx(line));
    }
    for line in &a[head..a.len() - tail] {
        out.push(diff_line("del", line));
    }
    for line in &b[head..b.len() - tail] {
        out.push(diff_line("add", line));
    }
    // 尾部上下文同样两行
    let tail_end = (a.len() - tail + 2).min(a.len());
    for line in &a[a.len() - tail..tail_end] {
        out.push(ctx(line));
    }
    if out.len() > MAX_DIFF_LINES {
        out.truncate(MAX_DIFF_LINES);
        out.push(diff_line("ctx", "…（改动过长，只显示前 400 行）"));
    }
    out
}

/// 上下文行。
fn ctx(line: &str) -> DiffLine {
    diff_line("ctx", line)
}

/// 一行差异。
fn diff_line(kind: &str, line: &str) -> DiffLine {
    DiffLine {
        line_kind: kind.to_string(),
        line_text: line.to_string(),
    }
}
