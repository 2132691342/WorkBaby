//! 技能系统：SKILL.md 解析（frontmatter）、三区发现、内置包、开关注入。

pub mod frontmatter;
pub mod registry;

pub use frontmatter::{parse_skill_md, valid_description, valid_name};
pub use registry::{SkillRegistry, BUILTIN_SKILLS};
