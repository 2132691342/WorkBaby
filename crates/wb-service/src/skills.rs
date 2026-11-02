//! 技能正文供给：内核按技能名要正文，service 负责从注册表读文件。

use std::sync::Arc;

use super::Services;

/// 技能正文加载器：名字 → SKILL.md 正文（frontmatter 已剥离，读不到返回 None）。
/// 内核不依赖 wb-skills，技能存储的真相始终留在 service 这一侧。
pub fn skill_loader(sv: &Arc<Services>) -> wb_kernel::SkillLoader {
    let sv = sv.clone();
    Arc::new(move |name: &str| {
        let location = sv.skills.read().unwrap().get(name)?.location.clone();
        let text = wb_pkg::fsutil::read_to_string(&location).ok()?;
        wb_skills::parse_skill_md(&text)
            .ok()
            .map(|(_, body)| body.trim().to_string())
            .filter(|b| !b.is_empty())
    })
}
