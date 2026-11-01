//! openai-completions wire：chat/completions 流式实现（覆盖 OpenAI 兼容网关）。

use std::pin::Pin;

use eventsource_stream::Eventsource;
use futures::{Stream, StreamExt};
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};

use crate::config::ResolvedProvider;
use crate::types::{
    ContentPart, ModelClient, ModelEvent, ProviderStream, ReqMessage, Role, StopReason,
    StreamRequest, ThinkingLevel, ToolCallDone, Usage,
};

/// openai 系客户端。
pub struct OpenAiClient {
    /// 解析后的 provider（地址 / 密钥 / 模型元信息）
    p: ResolvedProvider,
    /// 共享 HTTP 客户端
    http: reqwest::Client,
}

/// 流状态机：SSE 解析与工具调用参数累积。
#[derive(Default)]
pub struct Acc {
    /// 逐 index 累积的工具调用
    tools: Vec<Option<(String, String, String)>>,
    /// 累积用量
    usage: Usage,
    /// 上游 finish_reason 归一化后的停止原因
    pub stop: Option<StopReason>,
    /// 是否已见过有效的 finish_reason
    finished: bool,
}

impl Acc {
    /// 结清累积中的工具调用：upstream 没给 finish_reason 时流就断了，
    /// 这些调用必须定稿——否则模型要求调工具却被当成「正文到此结束」，整轮静默丢活。
    pub fn flush_tools(&mut self) -> Vec<ModelEvent> {
        let mut out = Vec::new();
        for (idx, slot) in self.tools.iter().enumerate() {
            if let Some((id, name, args)) = slot {
                out.push(ModelEvent::ToolCallEnd(
                    idx,
                    ToolCallDone {
                        call_id: id.clone(),
                        name: name.clone(),
                        arguments: args.clone(),
                    },
                ));
            }
        }
        if !out.is_empty() {
            self.tools.clear();
            self.stop = Some(StopReason::ToolUse);
        }
        out
    }
}

impl OpenAiClient {
    /// 构造（密钥为空也允许，测试端点可能免鉴权）。
    pub fn new(p: ResolvedProvider) -> Self {
        Self {
            p,
            http: reqwest::Client::new(),
        }
    }

    fn build_body(&self, req: &StreamRequest) -> Value {
        let mut msgs = Vec::new();
        for m in &req.messages {
            msgs.push(match m.role {
                Role::System => json!({ "role": "system", "content": join_text(m) }),
                Role::User => json!({ "role": "user", "content": user_content(m) }),
                Role::Assistant => {
                    let mut o = json!({ "role": "assistant", "content": join_text(m) });
                    if !m.tool_calls.is_empty() {
                        o["tool_calls"] = Value::Array(
                            m.tool_calls
                                .iter()
                                .map(|tc| {
                                    json!({
                                        "id": tc.call_id,
                                        "type": "function",
                                        "function": { "name": tc.name, "arguments": tc.arguments },
                                    })
                                })
                                .collect(),
                        );
                    }
                    o
                }
                Role::ToolResult => json!({
                    "role": "tool",
                    "tool_call_id": m.tool_call_id.clone().unwrap_or_default(),
                    "content": join_text(m),
                }),
            });
        }
        let mut body = json!({
            "model": self.p.model_id,
            "messages": msgs,
            "stream": true,
            "stream_options": { "include_usage": true },
            "max_tokens": req.max_tokens,
        });
        if !req.tools.is_empty() {
            body["tools"] = Value::Array(
                req.tools
                    .iter()
                    .map(|t| {
                        json!({
                            "type": "function",
                            "function": {
                                "name": t.name,
                                "description": t.description,
                                "parameters": t.parameters,
                            },
                        })
                    })
                    .collect(),
            );
            // 只有确实送工具时才需要让上游选工具；否则这字段只是噪声
            body["tool_choice"] = json!("auto");
        }
        // 思考档位落到各家通用字段：关 = 不送（与不支持的模型完全等价）
        match req.thinking {
            ThinkingLevel::Off => {}
            ThinkingLevel::Low => body["reasoning_effort"] = json!("low"),
            ThinkingLevel::Medium => body["reasoning_effort"] = json!("medium"),
            ThinkingLevel::High => body["reasoning_effort"] = json!("high"),
        }
        body
    }
}

/// 纯文本拼接（系统/助手消息只有文本）。
fn join_text(m: &ReqMessage) -> String {
    m.parts
        .iter()
        .filter_map(|p| match p {
            ContentPart::Text(t) => Some(t.clone()),
            ContentPart::Image { .. } => None,
        })
        .collect::<Vec<_>>()
        .join("\n")
}

/// 用户消息内容：带图片时用多段数组，否则纯字符串。
fn user_content(m: &ReqMessage) -> Value {
    // tool 角色的 content 必须是字符串，不能是数组；只有 user 能带图
    if !m
        .parts
        .iter()
        .any(|p| matches!(p, ContentPart::Image { .. }))
    {
        return Value::String(join_text(m));
    }
    Value::Array(
        m.parts
            .iter()
            .map(|p| match p {
                ContentPart::Text(t) => json!({ "type": "text", "text": t }),
                ContentPart::Image { mime, base64 } => json!({
                    "type": "image_url",
                    "image_url": { "url": format!("data:{mime};base64,{base64}") },
                }),
            })
            .collect(),
    )
}

/// HTTP 状态 → 是否可重试。
fn retryable_status(status: u16) -> bool {
    matches!(status, 429 | 500 | 502 | 503 | 504 | 520 | 524)
}

/// 从错误体提取人话（兼容 {"error":{"message"}} 与纯文本）。
pub fn error_message(status: u16, body: &str) -> String {
    if let Ok(v) = serde_json::from_str::<Value>(body) {
        if let Some(msg) = v["error"]["message"].as_str() {
            return format!("HTTP {status}: {msg}");
        }
    }
    let head: String = body.chars().take(200).collect();
    format!("HTTP {status}: {head}")
}

#[async_trait::async_trait]
impl ModelClient for OpenAiClient {
    async fn stream(
        &self,
        req: StreamRequest,
        cancel: CancellationToken,
    ) -> Result<ProviderStream> {
        let body = self.build_body(&req);
        let mut builder = self
            .http
            .post(format!("{}/chat/completions", self.p.base_url))
            .header("Content-Type", "application/json");
        if !self.p.api_key.is_empty() {
            builder = builder.header("Authorization", format!("Bearer {}", self.p.api_key));
        }
        for (k, v) in &self.p.headers {
            builder = builder.header(k, v);
        }
        let resp = builder
            .json(&body)
            .send()
            .await
            .map_err(|e| AppError::wrap("3200", "模型请求失败", e))?;
        if !resp.status().is_success() {
            let status = resp.status().as_u16();
            let text = resp.text().await.unwrap_or_default();
            return Ok(Box::pin(futures::stream::once(async move {
                ModelEvent::Error(retryable_status(status), error_message(status, &text))
            })));
        }
        let sse = resp.bytes_stream().eventsource();
        Ok(Box::pin(unfold_sse(Box::pin(sse), cancel)))
    }
}

/// 把 SSE 字节流折叠成 ModelEvent 流：cancel 触发即以 Aborted 收尾。
/// 错误统一映射成 String，避免 wire 库的泛型渗进状态机。
fn unfold_sse(
    sse: impl Stream<Item = std::result::Result<eventsource_stream::Event, impl std::fmt::Display>>
        + Send
        + 'static,
    cancel: CancellationToken,
) -> ProviderStream {
    // SSE 空闲上限：超过即按可重试错误收尾（对齐 docs/PROVIDER.md）
    const IDLE_TIMEOUT: std::time::Duration = std::time::Duration::from_secs(90);
    let sse: Pin<
        Box<dyn Stream<Item = std::result::Result<eventsource_stream::Event, String>> + Send>,
    > = Box::pin(sse.map(|item| item.map_err(|e| e.to_string())));
    // 状态：(SSE 流, 累积器, 已结束, 待发事件队列)
    Box::pin(futures::stream::unfold(
        (sse, Acc::default(), false, Vec::new()),
        move |(mut sse, mut acc, done, mut pending)| {
            let cancel = cancel.clone();
            async move {
                // 先把本帧攒下的事件逐个发完，再去读下一帧。
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
                        }                        r = next => match r {
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
                            // 流断了但上游没给 finish_reason：累积中的调用必须定稿
                            let mut flushed = acc.flush_tools();
                            if !flushed.is_empty() {
                                let first = flushed.remove(0);
                                return Some((first, (sse, acc, true, flushed)));
                            }
                            return Some((
                                ModelEvent::Done(
                                    acc.usage.clone(),
                                    acc.stop.unwrap_or(StopReason::Stop),
                                ),
                                (sse, acc, true, pending),
                            ));
                        }
                        Some(Err(e)) => {
                            return Some((
                                ModelEvent::Error(true, format!("模型流解析失败：{e}")),
                                (sse, acc, true, pending),
                            ));
                        }
                        Some(Ok(ev)) => {
                            let data = ev.data.trim().to_string();
                            if data == "[DONE]" {
                                // [DONE] 前没收到 finish_reason 的网关：同样要结清调用
                                let mut flushed = acc.flush_tools();
                                if flushed.is_empty() {
                                    return Some((
                                        ModelEvent::Done(
                                            acc.usage.clone(),
                                            acc.stop.unwrap_or(StopReason::Stop),
                                        ),
                                        (sse, acc, true, pending),
                                    ));
                                }
                                let first = flushed.remove(0);
                                return Some((first, (sse, acc, true, flushed)));
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

/// 处理单个 chunk；返回本帧产生的事件序列。
/// 一帧可能同时带正文、思考与多个工具调用的分片——所以返回列表而非单值：
/// 并行调用各占一个 index，逐个定稿，漏一个就是丢掉一次调用。
pub fn handle_chunk(data: &str, acc: &mut Acc) -> Vec<ModelEvent> {
    let mut out: Vec<ModelEvent> = Vec::new();
    let Ok(v) = serde_json::from_str::<Value>(data) else {
        return out;
    };
    if let Some(u) = v.get("usage").filter(|u| !u.is_null()) {
        acc.usage.input = u["prompt_tokens"].as_i64().unwrap_or(acc.usage.input);
        acc.usage.output = u["completion_tokens"].as_i64().unwrap_or(acc.usage.output);
        acc.usage.cache_read = u["prompt_tokens_details"]["cached_tokens"]
            .as_i64()
            .unwrap_or(acc.usage.cache_read);
        acc.usage.cache_write = u["prompt_tokens_details"]["cache_write_tokens"]
            .as_i64()
            .unwrap_or(acc.usage.cache_write);
        acc.usage.reasoning = u["completion_tokens_details"]["reasoning_tokens"]
            .as_i64()
            .unwrap_or(acc.usage.reasoning);
    }
    let Some(choices) = v["choices"].as_array() else {
        return out;
    };
    let Some(choice) = choices.first() else {
        return out;
    };
    let delta = &choice["delta"];
    // 思考增量（deepseek 风格 reasoning_content）
    if let Some(t) = delta["reasoning_content"].as_str() {
        if !t.is_empty() {
            out.push(ModelEvent::ThinkingDelta(0, t.to_string()));
        }
    }
    if let Some(t) = delta["content"].as_str() {
        if !t.is_empty() {
            out.push(ModelEvent::TextDelta(0, t.to_string()));
        }
    }
    if let Some(calls) = delta["tool_calls"].as_array() {
        for tc in calls {
            let idx = tc["index"].as_u64().unwrap_or(0) as usize;
            if acc.tools.len() <= idx {
                acc.tools.resize(idx + 1, None);
            }
            let slot = &mut acc.tools[idx];
            let id = tc["id"].as_str().unwrap_or_default().to_string();
            let name = tc["function"]["name"]
                .as_str()
                .unwrap_or_default()
                .to_string();
            let args = tc["function"]["arguments"]
                .as_str()
                .unwrap_or_default()
                .to_string();
            match slot {
                None => {
                    *slot = Some((id, name, args.clone()));
                }
                Some((sid, sname, sargs)) => {
                    if !id.is_empty() {
                        *sid = id;
                    }
                    if !name.is_empty() {
                        *sname = name;
                    }
                    sargs.push_str(&args);
                }
            }
            if !args.is_empty() {
                out.push(ModelEvent::ToolCallDelta(idx, args));
            }
        }
    }
    if acc.finished {
        return out;
    }
    // 空串是「本帧还没下结论」而不是一个停止理由：部分网关（deepseek 系）
    // 每个 chunk 都带 finish_reason:""，按终止态处理会让紧随其后的工具调用分片全部失效。
    let reason = choice["finish_reason"].as_str().unwrap_or("");
    match reason {
        "tool_calls" | "function_call" => {
            acc.finished = true;
            acc.stop = Some(StopReason::ToolUse);
            // 按 index 序逐个定稿；并行调用一个都不能少
            for (idx, slot) in acc.tools.iter().enumerate() {
                if let Some((id, name, args)) = slot.clone() {
                    out.push(ModelEvent::ToolCallEnd(
                        idx,
                        ToolCallDone {
                            call_id: id,
                            name,
                            arguments: args,
                        },
                    ));
                }
            }
            acc.tools.clear();
        }
        "length" => {
            acc.finished = true;
            acc.stop = Some(StopReason::Length);
        }
        "content_filter" => {
            acc.finished = true;
            acc.stop = Some(StopReason::Stop);
            out.push(ModelEvent::Error(
                false,
                "上游按内容策略截断了这一轮输出".into(),
            ));
        }
        "" => {}
        _ => {
            acc.finished = true;
            acc.stop = Some(StopReason::Stop);
        }
    }
    if acc.finished {
        // 定稿帧可能只带 finish_reason，正文本帧已产出的增量不能丢
        return out;
    }
    out
}
