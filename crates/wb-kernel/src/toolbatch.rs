//! 工具批结算：把一批执行结果同时推给界面并落进会话文件。
//! 事件与落盘必须成对——只推不落，重开会话就看不到这次调用；只落不推，界面停在「执行中」。

use crate::blocks::ToolResultOut;
use crate::events::{EmitFn, KernelEvent};
use crate::session::{now_ms, Entry, Msg, SessionFile};
use wb_pkg::Result;

/// 一条待结算的工具执行结果。
pub type BatchItem = (String, String, ToolResultOut);

/// 结算一批工具：逐个 emit ToolExecutionEnd 并追加 ToolResult 条目（保持入参顺序）。
pub fn settle(session: &mut SessionFile, items: Vec<BatchItem>, emit: &EmitFn) -> Result<()> {
    for (call_id, name, out) in items {
        emit(&KernelEvent::ToolExecutionEnd {
            call_id: call_id.clone(),
            name: name.clone(),
            result: out.result_text.clone(),
            is_error: out.is_error,
            duration_ms: out.duration_ms,
            file_path: out.file_path.clone(),
            diff: out.diff.clone(),
        });
        session
            .append(Entry::Message {
                message: Msg::ToolResult {
                    tool_call_id: call_id,
                    tool_name: name,
                    content: out.result_text,
                    is_error: out.is_error,
                    timestamp: now_ms(),
                    duration_ms: out.duration_ms,
                    file_path: out.file_path,
                    diff: out.diff,
                    images: out
                        .images
                        .into_iter()
                        .map(|i| crate::session::ImagePart {
                            mime: i.mime,
                            base64: i.base64,
                        })
                        .collect(),
                },
            })
            .map_err(|e| wb_pkg::AppError::new(&e.code, e.message))?;
    }
    Ok(())
}
