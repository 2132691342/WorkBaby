//! 系统提示词组装：有序命名段（docs/KERNEL.md）。

use wb_skills::SkillRegistry;

/// 组装系统提示词：正文 + runtime/cwd/project_context/tools/skills/live_state 命名段。
/// `skill_enabled` 决定技能段内容：关掉的技能既不出现在提示词里，也不该被模型引用。
/// `project_context` 是已发现的项目指令文件（AGENTS.md 一类），空串即无。
pub fn build_system_prompt(
    workspace: &str,
    skills: &SkillRegistry,
    enabled_tools: &[String],
    live_state: Option<&str>,
    project_context: &str,
    skill_enabled: impl Fn(&str) -> bool,
) -> String {
    let mut out = String::from(
        "You are WorkBaby, a capable personal assistant running on the user's Windows desktop. \
         You help by reading files, executing commands, editing code, and writing files. \
         Be concise. Show file paths clearly when working with files.\n",
    );
    out.push_str("\n<runtime>\n");
    out.push_str(
        "Bundled and already on PATH: `bash` (Git Bash with sed/awk/grep/tar/ssh), `git`, `python` (3.13).\n\
         Use `bash` for almost everything. Use the `powershell` tool only for Windows-native work\n\
         (registry, services, WMI/CIM, COM registration, group policy).\n\
         When calling a Windows-native exe from inside bash, `/c` is rewritten by MSYS — use `//c`\n\
         (e.g. `cmd.exe //c dir`). Do not install runtime managers, do not modify PATH, do not write\n\
         to system directories. Install extra Python packages with\n\
         `python -m pip install --user <pkg>` only when necessary.",
    );
    out.push_str("\n</runtime>\n");
    out.push_str(&format!("\n<cwd>{}</cwd>\n", workspace.replace('\\', "/")));
    // 项目指令紧随 cwd：模型先知道「在哪」，再读到「这里的规矩」
    if !project_context.trim().is_empty() {
        out.push('\n');
        out.push_str(project_context.trim());
        out.push('\n');
    }
    if !enabled_tools.is_empty() {
        out.push_str("\n<tools>\n");
        for n in enabled_tools {
            out.push_str(&format!("- {n}\n"));
        }
        out.push_str("</tools>\n");
    }
    out.push_str("\n<skills>\n");
    out.push_str(&skills.format_available_skills(skill_enabled));
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
