//! 输出解析：porcelain -z 状态、numstat、diff 行、log 记录。全部纯函数。

use std::collections::BTreeMap;

use wb_domain::git::{GitCommitVO, GitDiffLineVO};

/// porcelain -z 的一个条目。
#[derive(Debug, Clone, PartialEq)]
pub struct PorcelainEntry {
    /// 暂存区状态字符（' ' 表示无）
    pub staged: char,
    /// 工作区状态字符（' ' 表示无）
    pub unstaged: char,
    /// 相对仓库根的路径（改名条目为新路径）
    pub path: String,
}

/// 冲突态的 XY 组合。
fn is_conflict(staged: char, unstaged: char) -> bool {
    matches!(
        (staged, unstaged),
        ('U', _) | (_, 'U') | ('A', 'A') | ('D', 'D')
    )
}

/// 解析 `status --porcelain=v1 -z` 输出。
pub fn porcelain_z(out: &str) -> Vec<PorcelainEntry> {
    let mut parts = out.split('\0');
    let mut entries = Vec::new();
    while let Some(head) = parts.next() {
        if head.len() < 4 {
            continue;
        }
        let staged = head.as_bytes()[0] as char;
        let unstaged = head.as_bytes()[1] as char;
        let path = head[3..].to_string();
        // 改名 / 复制条目：原始路径跟在下一个 NUL 段
        let orig = if staged == 'R' || staged == 'C' || unstaged == 'R' || unstaged == 'C' {
            parts.next().unwrap_or("")
        } else {
            ""
        };
        let _ = orig;
        entries.push(PorcelainEntry {
            staged,
            unstaged,
            path,
        });
    }
    entries
}

/// 单文件增删读数。
#[derive(Debug, Clone, Copy, Default)]
pub struct NumStat {
    /// 新增行数
    pub additions: i64,
    /// 删除行数
    pub deletions: i64,
}

/// numstat 的路径归一：`a/{old => new}/b` 与 `old => new` 折成新路径。
fn numstat_path(raw: &str) -> String {
    if let Some(brace) = raw.find('{') {
        if let Some(arrow) = raw.find(" => ") {
            let close = raw[arrow..].find('}').map(|i| arrow + i);
            if let Some(close) = close {
                let prefix = &raw[..brace];
                let new_part = &raw[arrow + 4..close];
                let suffix = &raw[close + 1..];
                return format!("{prefix}{new_part}{suffix}");
            }
        }
    }
    if let Some(arrow) = raw.find(" => ") {
        return raw[arrow + 4..].trim().to_string();
    }
    raw.to_string()
}

/// 解析 numstat 输出为 路径→增删 表。
pub fn numstat(out: &str) -> BTreeMap<String, NumStat> {
    let mut map = BTreeMap::new();
    for line in out.lines() {
        if line.trim().is_empty() {
            continue;
        }
        let mut it = line.splitn(3, '\t');
        let (add, del, path) = match (it.next(), it.next(), it.next()) {
            (Some(a), Some(d), Some(p)) => (a, d, p),
            _ => continue,
        };
        let stat = NumStat {
            additions: add.parse().unwrap_or(0),
            deletions: del.parse().unwrap_or(0),
        };
        map.insert(numstat_path(path.trim()), stat);
    }
    map
}

/// 归类 porcelain 条目为界面文件清单（同一文件可拆暂存 / 未暂存两条）。
pub fn classify(entries: &[PorcelainEntry]) -> Vec<(String, char, bool, bool)> {
    let mut out = Vec::new();
    for e in entries {
        if is_conflict(e.staged, e.unstaged) {
            out.push((e.path.clone(), 'U', false, true));
            continue;
        }
        if e.staged == '?' {
            out.push((e.path.clone(), '?', false, false));
            continue;
        }
        if matches!(e.staged, 'M' | 'A' | 'D' | 'R' | 'C' | 'T') {
            out.push((e.path.clone(), e.staged, true, false));
        }
        if matches!(e.unstaged, 'M' | 'D' | 'R' | 'C' | 'T') {
            out.push((e.path.clone(), e.unstaged, false, false));
        }
    }
    out
}

/// 单文件 diff 输出的最大行数（超出截断）。
pub const DIFF_MAX_LINES: usize = 2000;

/// 解析 `diff --no-color` 输出为行序列；超限截断并返回丢弃数。
pub fn diff_lines(out: &str) -> (Vec<GitDiffLineVO>, bool, i64) {
    let mut lines = Vec::new();
    let mut old_no: i64 = 0;
    let mut new_no: i64 = 0;
    let mut dropped: i64 = 0;
    let mut truncated = false;
    for raw in out.lines() {
        if raw.starts_with("@@") {
            let (o, n) = parse_hunk_head(raw);
            old_no = o;
            new_no = n;
            lines.push(GitDiffLineVO {
                git_line_kind: "hunk".into(),
                git_line_old_no: 0,
                git_line_new_no: 0,
                git_line_text: raw.to_string(),
            });
        } else if let Some(rest) = raw.strip_prefix('+') {
            push_line(
                &mut lines,
                &mut dropped,
                &mut truncated,
                "add",
                0,
                new_no,
                rest,
            );
            new_no += 1;
        } else if let Some(rest) = raw.strip_prefix('-') {
            push_line(
                &mut lines,
                &mut dropped,
                &mut truncated,
                "del",
                old_no,
                0,
                rest,
            );
            old_no += 1;
        } else if let Some(rest) = raw.strip_prefix('\\') {
            push_line(&mut lines, &mut dropped, &mut truncated, "meta", 0, 0, rest);
        } else if let Some(rest) = raw.strip_prefix(' ') {
            push_line(
                &mut lines,
                &mut dropped,
                &mut truncated,
                "ctx",
                old_no,
                new_no,
                rest,
            );
            old_no += 1;
            new_no += 1;
        }
        // 文件头（diff --git / index / +++ / ---）忽略：path 由调用方自带
    }
    (lines, truncated, dropped)
}

/// 追加一行；超过上限后只计数。
fn push_line(
    lines: &mut Vec<GitDiffLineVO>,
    dropped: &mut i64,
    truncated: &mut bool,
    kind: &str,
    old_no: i64,
    new_no: i64,
    text: &str,
) {
    if lines.len() >= DIFF_MAX_LINES {
        *dropped += 1;
        *truncated = true;
        return;
    }
    lines.push(GitDiffLineVO {
        git_line_kind: kind.into(),
        git_line_old_no: old_no,
        git_line_new_no: new_no,
        git_line_text: text.to_string(),
    });
}

/// 解析 `@@ -a,b +c,d @@` 的两个起始行号。
fn parse_hunk_head(head: &str) -> (i64, i64) {
    let nums: Vec<i64> = head
        .split(&['-', '+'][..])
        .skip(1)
        .filter_map(|s| s.split(',').next()?.trim().parse().ok())
        .collect();
    let old = nums.first().copied().unwrap_or(0);
    let new = nums.get(1).copied().unwrap_or(0);
    (old, new)
}

/// log 记录：\x1e 分提交，\x1f 分字段，字段后跟可选 numstat 块。
pub fn log_commits(out: &str) -> Vec<GitCommitVO> {
    let mut commits = Vec::new();
    for record in out.split('\x1e') {
        let record = record.trim_start_matches('\n');
        if record.trim().is_empty() {
            continue;
        }
        let mut it = record.splitn(2, '\n');
        let head = it.next().unwrap_or("");
        let body = it.next().unwrap_or("");
        let f: Vec<&str> = head.split('\x1f').collect();
        if f.len() < 5 {
            continue;
        }
        let mut commit = GitCommitVO {
            git_hash: f[0].to_string(),
            git_short: f[1].to_string(),
            git_subject: f[2].to_string(),
            git_author: f[3].to_string(),
            git_when: f[4].parse::<i64>().unwrap_or(0) * 1000,
            git_files: 0,
            git_additions: 0,
            git_deletions: 0,
        };
        for line in body.lines() {
            if line.trim().is_empty() || line.starts_with("diff --git") {
                continue;
            }
            let mut it = line.splitn(3, '\t');
            if let (Some(a), Some(d), Some(p)) = (it.next(), it.next(), it.next()) {
                commit.git_files += 1;
                commit.git_additions += a.parse().unwrap_or(0);
                commit.git_deletions += d.parse().unwrap_or(0);
                let _ = p;
            }
        }
        commits.push(commit);
    }
    commits
}
