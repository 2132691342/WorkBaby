//! anthropic-messages 的流状态机：块累积与 SSE 折叠。
//! 与 wire 实现拆开：这里的逻辑是纯协议解析，不碰 HTTP。

use std::pin::Pin;

use futures::{Stream, StreamExt};
use serde_json::Value;
use tokio_util::sync::CancellationToken;

use crate::types::{ModelEvent, ProviderStream, StopReason, ToolCallDone, Usage};

/// 流状态机：按 content block index 累积文本 / 思考 / 工具参数。
#[derive(Default)]
pub struct Acc {
    /// 每个内容块的累积态（index → 块）
    blocks: Vec<Option<Block>>,
    /// 累积用量
    pub usage: Usage,
    /// 上游 stop_reason 归一化后的停止原因
    pub stop: Option<StopReason>,
}

/// 一个内容块的累积态。
#[derive(Clone)]
pub enum Block {
    /// 普通文本
    Text(String),
    /// 思考内容
    Thinking(String),
    /// 工具调用（id / 名称 / 参数 JSON 分片）
    Tool {
        /// 调用 id
        id: String,
        /// 工具名
        name: String,
        /// 参数 JSON 文本
        args: String,
    },
}

impl Acc {
    /// 结清累积中的块：上游流断在中途时，已经开始但没结束的块必须定稿，
    /// 否则模型要求调工具却被当成「正文到此结束」，整轮静默丢活。
    pub fn flush_blocks(&mut self) -> Vec<ModelEvent> {
        let mut out = Vec::new();
        for (idx, slot) in self.blocks.iter_mut().enumerate() {
            match slot.take() {
                Some(Block::Text(t)) => {
                    if !t.is_empty() {
                        out.push(ModelEvent::TextDelta(idx, t));
                    }
                }
                Some(Block::Thinking(t)) => {
                    if !t.is_empty() {
                        out.push(ModelEvent::ThinkingDelta(idx, t));
                    }
                }
                Some(Block::Tool { id, name, args }) if !name.is_empty() => {
                    out.push(ModelEvent::ToolCallEnd(
                        idx,
                        ToolCallDone {
                            call_id: id,
                            name,
                            arguments: args,
                        },
                    ));
                }
                Some(_) => {}
                None => {}
            }
        }
        if out.iter().any(|e| matches!(e, ModelEvent::ToolCallEnd(..))) {
            self.stop = Some(StopReason::ToolUse);
        }
        out
    }
}

/// 把 SSE 字节流折叠成 ModelEvent 流：cancel 触发即以 Aborted 收尾。
pub fn unfold_sse(
    sse: impl Stream<Item = std::result::Result<eventsource_stream::Event, impl std::fmt::Display>>
        + Send
        + 'static,
    cancel: CancellationToken,
) -> ProviderStream {
    const IDLE_TIMEOUT: std::time::Duration = std::time::Duration::from_secs(90);
    let sse: Pin<
        Box<dyn Stream<Item = std::result::Result<eventsource_stream::Event, String>> + Send>,
    > = Box::pin(sse.map(|item| item.map_err(|e| e.to_string())));
    Box::pin(futures::stream::unfold(
        (sse, Acc::default(), false, Vec::new()),
        move |(mut sse, mut acc, done, mut pending)| {
            let cancel = cancel.clone();
            async move {
                if let Some(ev) = pending.first().cloned() {
                    pending.remove(0);
                    return Some((ev, (sse, acc, done, pending)));
                }
                if done {
                    return None;
                }
                loop {
                    let next = tokio::time::timeout(IDLE_TIMEOUT, sse.next());
                    let item = tokio::select! {
                        _ = cancel.cancelled() => {
                            return Some((ModelEvent::Done(acc.usage.clone(), StopReason::Aborted), (sse, acc, true, pending)));
                        }
                        r = next => match r {
                            Err(_) => {
                                return Some((
                                    ModelEvent::Error(true, "模型流空闲超时（90s 无数据）".into()),
                                    (sse, acc, true, pending),
                                ));
                            }
                            Ok(item) => item,
                        }
                    };
                    match item {
                        None => {
                            // 流断了但上游没给 stop_reason：累积中的块必须定稿
                            let mut out = acc.flush_blocks();
                            if out.is_empty() {
                                return Some((
                                    ModelEvent::Done(
                                        acc.usage.clone(),
                                        acc.stop.unwrap_or(StopReason::Stop),
                                    ),
                                    (sse, acc, true, pending),
                                ));
                            }
                            let first = out.remove(0);
                            return Some((first, (sse, acc, true, out)));
                        }
                        Some(Err(e)) => {
                            return Some((
                                ModelEvent::Error(true, format!("模型流解析失败：{e}")),
                                (sse, acc, true, pending),
                            ));
                        }
                        Some(Ok(ev)) => {
                            let data = ev.data.trim().to_string();
                            if data.is_empty() || data == "[DONE]" {
                                let mut out = acc.flush_blocks();
                                if out.is_empty() {
                                    out.push(ModelEvent::Done(
                                        acc.usage.clone(),
                                        acc.stop.unwrap_or(StopReason::Stop),
                                    ));
                                }
                                let first = out.remove(0);
                                return Some((first, (sse, acc, true, out)));
                            }
                            let mut events = handle_chunk(&data, &mut acc);
                            if !events.is_empty() {
                                let first = events.remove(0);
                                return Some((first, (sse, acc, false, events)));
                            }
                        }
                    }
                }
            }
        },
    ))
}

/// 处理单个 SSE 帧；返回本帧产生的事件序列。
pub fn handle_chunk(data: &str, acc: &mut Acc) -> Vec<ModelEvent> {
    let mut out: Vec<ModelEvent> = Vec::new();
    let Ok(v) = serde_json::from_str::<Value>(data) else {
        return out;
    };
    let kind = v["type"].as_str().unwrap_or("");
    match kind {
        "message_start" => {
            let u = &v["message"]["usage"];
            acc.usage.input = u["input_tokens"].as_i64().unwrap_or(0);
            acc.usage.cache_read = u["cache_read_input_tokens"].as_i64().unwrap_or(0);
            acc.usage.cache_write = u["cache_creation_input_tokens"].as_i64().unwrap_or(0);
        }
        "content_block_start" => {
            let idx = v["index"].as_u64().unwrap_or(0) as usize;
            let btype = v["content_block"]["type"].as_str().unwrap_or("");
            let block = match btype {
                "text" => Block::Text(
                    v["content_block"]["text"]
                        .as_str()
                        .unwrap_or("")
                        .to_string(),
                ),
                "thinking" => Block::Thinking(
                    v["content_block"]["thinking"]
                        .as_str()
                        .unwrap_or("")
                        .to_string(),
                ),
                "tool_use" => Block::Tool {
                    id: v["content_block"]["id"].as_str().unwrap_or("").to_string(),
                    name: v["content_block"]["name"]
                        .as_str()
                        .unwrap_or("")
                        .to_string(),
                    args: String::new(),
                },
                _ => Block::Text(String::new()),
            };
            ensure_len(&mut acc.blocks, idx + 1);
            acc.blocks[idx] = Some(block);
        }
        "content_block_delta" => {
            let idx = v["index"].as_u64().unwrap_or(0) as usize;
            ensure_len(&mut acc.blocks, idx + 1);
            let delta = &v["delta"];
            match delta["type"].as_str().unwrap_or("") {
                "text_delta" => {
                    let t = delta["text"].as_str().unwrap_or("");
                    if !t.is_empty() {
                        match acc.blocks[idx].as_mut() {
                            Some(Block::Text(buf)) => {
                                buf.push_str(t);
                                out.push(ModelEvent::TextDelta(idx, t.to_string()));
                            }
                            _ => {
                                acc.blocks[idx] = Some(Block::Text(t.to_string()));
                                out.push(ModelEvent::TextDelta(idx, t.to_string()));
                            }
                        }
                    }
                }
                "thinking_delta" => {
                    let t = delta["thinking"].as_str().unwrap_or("");
                    if !t.is_empty() {
                        match acc.blocks[idx].as_mut() {
                            Some(Block::Thinking(buf)) => {
                                buf.push_str(t);
                                out.push(ModelEvent::ThinkingDelta(idx, t.to_string()));
                            }
                            _ => {
                                acc.blocks[idx] = Some(Block::Thinking(t.to_string()));
                                out.push(ModelEvent::ThinkingDelta(idx, t.to_string()));
                            }
                        }
                    }
                }
                "input_json_delta" => {
                    let frag = delta["partial_json"].as_str().unwrap_or("");
                    if !frag.is_empty() {
                        match acc.blocks[idx].as_mut() {
                            Some(Block::Tool { args, .. }) => args.push_str(frag),
                            _ => {
                                acc.blocks[idx] = Some(Block::Tool {
                                    id: String::new(),
                                    name: String::new(),
                                    args: frag.to_string(),
                                });
                            }
                        }
                    }
                }
                _ => {}
            }
        }
        "content_block_stop" => {
            // 块的结束事件本身不带内容：定稿在 flush_blocks 里统一做
        }
        "message_delta" => {
            let u = &v["usage"];
            if let Some(n) = u["output_tokens"].as_i64() {
                acc.usage.output = n;
            }
            if let Some(n) = u["input_tokens"].as_i64() {
                acc.usage.input = n;
            }
            acc.stop = v["delta"]["stop_reason"]
                .as_str()
                .and_then(normalize_stop)
                .or(acc.stop);
        }
        "error" => {
            let msg = v["error"]["message"].as_str().unwrap_or("上游错误");
            out.push(ModelEvent::Error(true, msg.to_string()));
        }
        _ => {}
    }
    out
}

/// 上游 stop_reason → 归一化停止原因。
pub fn normalize_stop(raw: &str) -> Option<StopReason> {
    Some(match raw {
        "end_turn" | "stop_sequence" => StopReason::Stop,
        "max_tokens" => StopReason::Length,
        "tool_use" => StopReason::ToolUse,
        "pause_turn" => StopReason::Stop,
        "refusal" => StopReason::Error,
        _ => return None,
    })
}

/// 把块数组撑到指定长度（Anthropic 的 index 可能跳号）。
pub fn ensure_len(v: &mut Vec<Option<Block>>, len: usize) {
    if v.len() < len {
        v.resize_with(len, || None);
    }
}
