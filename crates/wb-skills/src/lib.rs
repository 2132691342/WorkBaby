//! 技能系统：SKILL.md 解析（frontmatter）、三区发现、内置包、开关注入、项目上下文发现。

pub mod context_files;
pub mod frontmatter;
pub mod registry;

pub use context_files::{discover, render, ContextFile};
pub use frontmatter::{parse_skill_md, valid_description, valid_name};
pub use registry::{SkillRegistry, BUILTIN_SKILLS};
