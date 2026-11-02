//! 子代理的提示词与消息转换：角色职责拼装、父提示词上下文段提取、内核消息 → provider 请求。
//!
//! 这些都是纯函数，放在这里让 subagent.rs 只管「跑一次子会话」那一件事。

use wb_provider::ReqMessage;

use crate::session::Msg;

/// 子代理系统提示词：角色职责在前，父提示词的上下文段在后。
pub(crate) fn build_sub_prompt(
    role_prompt: &str,
    parent_prompt: &str,
    tool_names: &[String],
) -> String {
    let mut out = String::new();
    out.push_str(
        "You are a subagent running inside WorkBaby. You complete one delegated task and \
         report the conclusion. Work autonomously; the parent agent only sees your final report, \
         so make it self-contained and concrete.\n",
    );
    if !role_prompt.trim().is_empty() {
        out.push_str(&format!("\n<role>\n{}\n</role>\n", role_prompt.trim()));
    }
    out.push_str(&format!(
        "\n<tools>\n{}\n</tools>\n",
        tool_names
            .iter()
            .map(|n| format!("- {n}\n"))
            .collect::<String>()
    ));
    // 父提示词里的 cwd / skills 段对子代理同样有效。
    for tag in ["<cwd>", "<skills>"] {
        if let Some(part) = extract_tag(parent_prompt, tag) {
            out.push('\n');
            out.push_str(&part);
            out.push('\n');
        }
    }
    out.push_str(
        "\nReport format: state the outcome first, then the key facts you verified \
         (file paths, line numbers, command output). If the task failed, say so and why.\n",
    );
    out
}

/// 从父提示词里取出一个标签段（含首尾标签）。
fn extract_tag(text: &str, tag: &str) -> Option<String> {
    let start = text.find(tag)?;
    let end = text[start..].find("</")? + start;
    Some(text[start..end].to_string())
}

/// 内核消息 → provider 请求消息（与主循环同形：技能不展开，子会话不带父会话的图片）。
pub(crate) fn to_req(m: Msg) -> ReqMessage {
    crate::reqbuild::to_req(m, None)
}
