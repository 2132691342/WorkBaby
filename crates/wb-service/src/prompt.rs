//! 系统提示词组装：有序命名段（docs/KERNEL.md）。

use wb_skills::SkillRegistry;

/// 组装系统提示词：正文 + tools/rules/skills/cwd 命名段。
pub fn build_system_prompt(
    workspace: &str,
    skills: &SkillRegistry,
    enabled_tools: &[String],
    live_state: Option<&str>,
) -> String {
    let mut out = String::from(
        "You are WorkBaby, a capable personal assistant running on the user's Windows desktop. \
         You help by reading files, executing commands, editing code, and writing files. \
         Be concise. Show file paths clearly when working with files.\n",
    );
    out.push_str("\n<runtime>\n");
    out.push_str(
        "Bundled and already on PATH: `bash`, `git`, `python` (3.13). Use them directly.\n\
         Do not install runtime managers, do not modify PATH, do not write to system directories.\n\
         Install extra Python packages with `python -m pip install --user <pkg>` only when necessary.",
    );
    out.push_str("\n</runtime>\n");
    out.push_str(&format!("\n<cwd>{}</cwd>\n", workspace.replace('\\', "/")));
    if !enabled_tools.is_empty() {
        out.push_str("\n<tools>\n");
        for n in enabled_tools {
            out.push_str(&format!("- {n}\n"));
        }
        out.push_str("</tools>\n");
    }
    out.push_str("\n<skills>\n");
    out.push_str(&skills.format_available_skills(|_| true));
    out.push_str("\n</skills>\n");
    // 活状态（待办 / 目标）每轮重读：压缩和重启都不会让它从视野里消失
    if let Some(state) = live_state {
        if !state.trim().is_empty() {
            out.push_str("\n<live_state priority=\"high\">\n");
            out.push_str(state.trim());
            out.push_str("\n</live_state>\n");
        }
    }
    out
}
