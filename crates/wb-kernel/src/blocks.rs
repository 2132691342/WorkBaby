//! 消息块构造：文本与思考合并、工具调用定稿、工具执行。

use std::time::Instant;

use tokio_util::sync::CancellationToken;

use crate::agent::AgentConfig;
use wb_provider::ToolCallDone;

use crate::session::Block;

pub async fn execute_tool(
    cfg: &AgentConfig,
    name: &str,
    arguments: &str,
    cancel: &CancellationToken,
) -> (String, bool, i64) {
    let started = Instant::now();
    let Some(tool) = cfg.registry.get(name) else {
        return ("[工具不存在或已停用]".into(), true, 0);
    };
    let args: serde_json::Value =
        serde_json::from_str(arguments).unwrap_or(serde_json::Value::Null);
    match tool.execute(args, cancel.clone(), &tool_workdir()).await {
        Ok(out) => (out.text, out.is_error, out.duration_ms),
        Err(e) => (
            format!("[错误 {}] {}", e.code, e.message),
            true,
            started.elapsed().as_millis() as i64,
        ),
    }
}

/// 工具的工作目录：进程当前目录（service 保证它等于工作区）。
pub fn tool_workdir() -> std::path::PathBuf {
    std::env::current_dir().unwrap_or_else(|_| std::path::PathBuf::from("."))
}

/// 追加文本块（连续文本合并）。
pub fn push_text(blocks: &mut Vec<Block>, t: String) {
    if let Some(Block::Text { text }) = blocks.last_mut() {
        text.push_str(&t);
    } else {
        blocks.push(Block::Text { text: t });
    }
}

pub fn push_thinking(blocks: &mut Vec<Block>, t: String) {
    if let Some(Block::Thinking { thinking }) = blocks.last_mut() {
        thinking.push_str(&t);
    } else {
        blocks.push(Block::Thinking { thinking: t });
    }
}

pub fn last_tool_mut(blocks: &mut [Block]) -> Option<&mut Block> {
    blocks.last_mut()
}

/// 工具调用定稿：新建或补全槽位。
pub fn upsert_tool(blocks: &mut Vec<Block>, done: ToolCallDone) {
    if let Some(Block::ToolCall {
        tool_call_id,
        name,
        arguments,
    }) = blocks.last_mut()
    {
        if tool_call_id.is_empty() {
            *tool_call_id = done.call_id;
        }
        if name.is_empty() {
            *name = done.name;
        }
        if arguments.is_empty() {
            *arguments = done.arguments;
        }
        return;
    }
    blocks.push(Block::ToolCall {
        tool_call_id: done.call_id,
        name: done.name,
        arguments: done.arguments,
    });
}
