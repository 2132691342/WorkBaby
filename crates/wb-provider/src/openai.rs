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
    StreamRequest, ToolCallDone, Usage,
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
    /// 是否已见过 finish_reason
    finished: bool,
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
    if m.parts.iter().all(|p| matches!(p, ContentPart::Text(_))) {
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
    // SSE 空闲上限：超过即按可重试错误收尾（对齐 specs/03）
    const IDLE_TIMEOUT: std::time::Duration = std::time::Duration::from_secs(90);
    let sse: Pin<
        Box<dyn Stream<Item = std::result::Result<eventsource_stream::Event, String>> + Send>,
    > = Box::pin(sse.map(|item| item.map_err(|e| e.to_string())));
    Box::pin(futures::stream::unfold(
        (sse, Acc::default(), false),
        move |(mut sse, mut acc, done)| {
            let cancel = cancel.clone();
            async move {
                if done {
                    return None;
                }
                loop {
                    let next = tokio::time::timeout(IDLE_TIMEOUT, sse.next());
                    let item = tokio::select! {
                        _ = cancel.cancelled() => {
                            return Some((ModelEvent::Done(acc.usage.clone(), StopReason::Aborted), (sse, acc, true)));
                        }
                        r = next => match r {
                            Err(_) => {
                                return Some((
                                    ModelEvent::Error(true, "模型流空闲超时（90s 无数据）".into()),
                                    (sse, acc, true),
                                ));
                            }
                            Ok(item) => item,
                        }
                    };
                    match item {
                        None => {
                            let reason = if acc.finished {
                                StopReason::ToolUse
                            } else {
                                StopReason::Stop
                            };
                            return Some((
                                ModelEvent::Done(acc.usage.clone(), reason),
                                (sse, acc, true),
                            ));
                        }
                        Some(Err(e)) => {
                            return Some((
                                ModelEvent::Error(true, format!("模型流解析失败：{e}")),
                                (sse, acc, true),
                            ));
                        }
                        Some(Ok(ev)) => {
                            let data = ev.data.trim().to_string();
                            if data == "[DONE]" {
                                let reason = if acc.finished {
                                    StopReason::ToolUse
                                } else {
                                    StopReason::Stop
                                };
                                return Some((
                                    ModelEvent::Done(acc.usage.clone(), reason),
                                    (sse, acc, true),
                                ));
                            }
                            if let Some(out) = handle_chunk(&data, &mut acc) {
                                return Some((out, (sse, acc, false)));
                            }
                        }
                    }
                }
            }
        },
    ))
}

/// 处理单个 chunk；要立刻上抛的事件返回 Some。
pub fn handle_chunk(data: &str, acc: &mut Acc) -> Option<ModelEvent> {
    let v: Value = serde_json::from_str(data).ok()?;
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
    let choice = v["choices"].as_array()?.first()?.clone();
    let delta = &choice["delta"];
    // 思考增量（deepseek 风格 reasoning_content）
    if let Some(t) = delta["reasoning_content"].as_str() {
        if !t.is_empty() {
            return Some(ModelEvent::ThinkingDelta(0, t.to_string()));
        }
    }
    if let Some(t) = delta["content"].as_str() {
        if !t.is_empty() {
            return Some(ModelEvent::TextDelta(0, t.to_string()));
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
                return Some(ModelEvent::ToolCallDelta(idx, args));
            }
        }
    }
    if acc.finished {
        return None;
    }
    match choice["finish_reason"].as_str() {
        Some("tool_calls") | Some("function_call") => {
            acc.finished = true;
            // 按 index 序补发完整调用；参数累积已在中途完成
            let mut last = None;
            for (idx, slot) in acc.tools.iter().enumerate() {
                if let Some((id, name, args)) = slot.clone() {
                    last = Some(ModelEvent::ToolCallEnd(
                        idx,
                        ToolCallDone {
                            call_id: id,
                            name,
                            arguments: args,
                        },
                    ));
                }
            }
            last
        }
        Some("length") => {
            acc.finished = true;
            Some(ModelEvent::Done(acc.usage.clone(), StopReason::Length))
        }
        Some(_) => {
            acc.finished = true;
            None
        }
        None => None,
    }
}
