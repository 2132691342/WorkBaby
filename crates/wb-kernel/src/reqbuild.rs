//! 内核消息 → provider 请求消息：技能按需展开、图片转内容片段。

use wb_provider::{ContentPart, ReqMessage, ReqToolCall, Role};

use crate::session::{Block, Msg};

/// 取一个技能的 SKILL.md 正文；没有就返回 None（名字失效时按无技能处理）。
pub type SkillLoader = std::sync::Arc<dyn Fn(&str) -> Option<String> + Send + Sync>;

/// 单条消息转请求形态。`skill` 只在用户消息上有意义：非空时把技能正文包在正文之前。
pub(crate) fn to_req(m: Msg, skill: Option<&SkillLoader>) -> ReqMessage {
    match m {
        Msg::User {
            content,
            skill: skill_name,
            images,
            ..
        } => {
            let mut parts = Vec::new();
            // 顺序即优先级：技能正文在前，用户这句话在后——模型先读规范再读诉求。
            if let Some(text) = skill_body(skill_name.as_deref(), skill) {
                parts.push(ContentPart::Text(text));
            }
            parts.push(ContentPart::Text(content));
            for img in images {
                parts.push(ContentPart::Image {
                    mime: img.mime,
                    base64: img.base64,
                });
            }
            ReqMessage {
                role: Role::User,
                parts,
                tool_calls: Vec::new(),
                tool_call_id: None,
            }
        }
        Msg::Assistant { content, .. } => {
            let text = content
                .iter()
                .filter_map(|b| match b {
                    Block::Text { text } => Some(text.clone()),
                    _ => None,
                })
                .collect::<Vec<_>>()
                .join("\n");
            let calls = content
                .iter()
                .filter_map(|b| match b {
                    Block::ToolCall {
                        tool_call_id,
                        name,
                        arguments,
                    } => Some(ReqToolCall {
                        call_id: tool_call_id.clone(),
                        name: name.clone(),
                        arguments: arguments.clone(),
                    }),
                    _ => None,
                })
                .collect();
            ReqMessage {
                role: Role::Assistant,
                parts: vec![ContentPart::Text(text)],
                tool_calls: calls,
                tool_call_id: None,
            }
        }
        Msg::ToolResult {
            tool_call_id,
            content,
            ..
        } => ReqMessage {
            role: Role::ToolResult,
            parts: vec![ContentPart::Text(content)],
            tool_calls: Vec::new(),
            tool_call_id: Some(tool_call_id),
        },
    }
}

/// 技能展开文本：`<skill>` 块包住 SKILL.md 正文，正文里引用的相对路径仍能落到技能目录。
fn skill_body(name: Option<&str>, skill: Option<&SkillLoader>) -> Option<String> {
    let name = name.filter(|n| !n.is_empty())?;
    let loader = skill?;
    let body = loader(name)?;
    Some(format!(
        "<skill name=\"{name}\">\nThe following skill instructions apply to this request.\n\n{body}\n</skill>"
    ))
}
