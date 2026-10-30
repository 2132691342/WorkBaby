//! SKILL.md 解析：YAML frontmatter + 正文。description 必填（≤1024），缺失则整技能跳过。

use wb_domain::skill::SkillFrontmatter;
use wb_pkg::{AppError, Result};

/// frontmatter 起始行
const FM_START: &str = "---";

/// 解析 SKILL.md 文本 → (frontmatter, 正文)。
pub fn parse_skill_md(text: &str) -> Result<(SkillFrontmatter, String)> {
    let mut lines = text.lines();
    if lines.next() != Some(FM_START) {
        return Err(AppError::new(
            wb_domain::error::skill::PARSE_FAILED,
            "缺少 frontmatter 起始行 ---",
        ));
    }
    let mut fm_lines = Vec::new();
    let mut body_start = None;
    for (i, line) in lines.enumerate() {
        if line.trim() == FM_START {
            body_start = Some(i + 1);
            break;
        }
        fm_lines.push(line);
    }
    let Some(body_start) = body_start else {
        return Err(AppError::new(
            wb_domain::error::skill::PARSE_FAILED,
            "frontmatter 未闭合",
        ));
    };
    let fm_text = fm_lines.join("\n");
    let fm: SkillFrontmatter = serde_yaml::from_str(&fm_text).map_err(|e| {
        AppError::wrap(
            wb_domain::error::skill::PARSE_FAILED,
            "frontmatter 解析失败",
            e,
        )
    })?;
    let body: Vec<&str> = text.lines().skip(body_start).collect();
    Ok((fm, body.join("\n")))
}

/// 校验技能名（^[a-z0-9-]+$，≤64，不以 - 开头/结尾，不含 --）。
pub fn valid_name(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 64
        && name
            .chars()
            .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
        && !name.starts_with('-')
        && !name.ends_with('-')
        && !name.contains("--")
}

/// 校验描述（必填 ≤1024）。
pub fn valid_description(desc: &str) -> bool {
    !desc.trim().is_empty() && desc.chars().count() <= 1024
}
