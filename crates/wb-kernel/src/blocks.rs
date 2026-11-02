//! 消息块构造：文本与思考合并、工具调用定稿、工具执行。

use std::time::Instant;

use tokio_util::sync::CancellationToken;

use crate::agent::AgentConfig;
use wb_provider::ToolCallDone;

use crate::session::Block;

/// 执行一个工具调用：取不到工具算错不算崩，返回 (正文, 是否错, 耗时)。
/// 参数不过 schema 也不预先拒，交给工具自己报错（错误文案更具体）。
pub async fn execute_tool(
    cfg: &AgentConfig,
    name: &str,
    arguments: &str,
    cancel: &CancellationToken,
) -> ToolResultOut {
    let started = Instant::now();
    let Some(tool) = cfg.registry.get(name) else {
        return ToolResultOut {
            result_text: "[工具不存在或已停用]".into(),
            is_error: true,
            duration_ms: 0,
            file_path: None,
            diff: None,
        };
    };
    let args: serde_json::Value =
        serde_json::from_str(arguments).unwrap_or(serde_json::Value::Null);
    match tool.execute(args, cancel.clone(), &cfg.workdir).await {
        Ok(out) => ToolResultOut {
            result_text: out.text,
            is_error: out.is_error,
            duration_ms: out.duration_ms,
            file_path: out.file_path,
            diff: out.diff,
        },
        Err(e) => ToolResultOut {
            result_text: format!("[错误 {}] {}", e.code, e.message),
            is_error: true,
            duration_ms: started.elapsed().as_millis() as i64,
            file_path: None,
            diff: None,
        },
    }
}

/// 一次工具执行的结果（含改动文件，供界面就地着色）。
pub struct ToolResultOut {
    /// 结果文本（已截断）
    pub result_text: String,
    /// 是否按错误处理
    pub is_error: bool,
    /// 耗时毫秒
    pub duration_ms: i64,
    /// 改动的文件（相对工作区）
    pub file_path: Option<String>,
    /// 改动差异行
    pub diff: Option<Vec<wb_domain::events::DiffLine>>,
}

/// 同批工具调用的并发分组：相邻只读调用并一组一起跑，
/// 带副作用的调用自成一组串行跑——两个 write 并发改同一文件会互相踩。
/// 返回原 calls 的下标分组，组内组间都保持调用顺序。
pub fn group_calls_for_concurrency(
    registry: &wb_tools::ToolRegistry,
    calls: &[(String, String, String)],
) -> Vec<Vec<usize>> {
    let mut groups: Vec<Vec<usize>> = Vec::new();
    let mut cur_parallel = false;
    for (idx, (_, name, _)) in calls.iter().enumerate() {
        let parallel = registry
            .get(name)
            .map(|t| t.annotations().read_only)
            .unwrap_or(false);
        if parallel && cur_parallel {
            if let Some(last) = groups.last_mut() {
                last.push(idx);
                continue;
            }
        }
        groups.push(vec![idx]);
        cur_parallel = parallel;
    }
    groups
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

/// 工具调用定稿：按 call_id 归位，重复定稿只补空字段。
/// 并行调用各占一个块：不同 call_id 是新的一次调用，塞进同一块模型就只看到一个。
pub fn upsert_tool(blocks: &mut Vec<Block>, done: ToolCallDone) {
    if let Some(Block::ToolCall { name, arguments, .. }) = blocks.iter_mut().rev().find(|b| {
        matches!(b, Block::ToolCall { tool_call_id, .. } if !tool_call_id.is_empty() && *tool_call_id == done.call_id)
    }) {
        if name.is_empty() {
            *name = done.name;
        }
        if arguments.is_empty() {
            *arguments = done.arguments;
        }
        return;
    }
    // 没有可归位的槽位：新建一个块。
    blocks.push(Block::ToolCall {
        tool_call_id: done.call_id,
        name: done.name,
        arguments: done.arguments,
    });
}
