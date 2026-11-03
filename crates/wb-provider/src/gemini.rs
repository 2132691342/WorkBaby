//! google-generative-ai wire：generateContent 流式实现（直连 Gemini 与其兼容网关）。
//! 差异：systemInstruction 独立传、functionCall/Response 都在 user 角色、思考块是 part 的 thought 标记。

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

/// gemini 系客户端。
pub struct GeminiClient {
    /// 解析后的 provider（地址 / 密钥 / 模型元信息）
    p: ResolvedProvider,
    /// 共享 HTTP 客户端
    http: reqwest::Client,
}

/// 流状态机：累积工具调用参数（Gemini 的 functionCall 参数是一次性整包给的）。
#[derive(Default)]
pub struct Acc {
    /// 累积中的工具调用（按出现顺序）
    tools: Vec<PendingTool>,
    /// 累积用量
    pub usage: Usage,
    /// 上游 finishReason 归一化后的停止原因
    pub stop: Option<StopReason>,
}

/// 一个待定稿的工具调用。
struct PendingTool {
    /// 调用 id
    id: String,
    /// 工具名
    name: String,
    /// 参数 JSON 文本
    args: String,
}

impl GeminiClient {
    /// 构造（密钥为空也允许，兼容网关可能免鉴权）。
    pub fn new(p: ResolvedProvider) -> Self {
        Self {
            p,
            http: reqwest::Client::new(),
        }
    }

    /// 组请求体：systemInstruction 独立传，工具结果装成 user 角色的 functionResponse。
    fn build_body(&self, req: &StreamRequest) -> Value {
        let mut system = String::new();
        let mut contents: Vec<Value> = Vec::new();
        for m in &req.messages {
            match m.role {
                Role::System => {
                    if !system.is_empty() {
                        system.push('\n');
                    }
                    system.push_str(&join_text(m));
                }
                Role::User => {
                    let parts = user_parts(m);
                    if !parts.is_empty() {
                        contents.push(json!({ "role": "user", "parts": parts }));
                    }
                }
                Role::Assistant => {
                    let mut parts: Vec<Value> = Vec::new();
                    for part in &m.parts {
                        if let ContentPart::Text(t) = part {
                            if !t.is_empty() {
                                parts.push(json!({ "text": t }));
                            }
                        }
                    }
                    for tc in &m.tool_calls {
                        parts.push(json!({
                            "functionCall": {
                                "id": tc.call_id,
                                "name": tc.name,
                                "args": parse_arguments(&tc.arguments),
                            }
                        }));
                    }
                    if !parts.is_empty() {
                        contents.push(json!({ "role": "model", "parts": parts }));
                    }
                }
                Role::ToolResult => {
                    let mut parts: Vec<Value> = Vec::new();
                    parts.push(json!({
                        "functionResponse": {
                            "id": m.tool_call_id.clone().unwrap_or_default(),
                            "name": m.tool_name.clone().unwrap_or_default(),
                            "response": { "result": join_text(m) },
                        }
                    }));
                    // 图片另起一条 user 内容：functionResponse 只接受文本
                    if let Some(imgs) = image_parts(m) {
                        contents.push(json!({ "role": "user", "parts": parts }));
                        contents.push(json!({ "role": "user", "parts": imgs }));
                        continue;
                    }
                    contents.push(json!({ "role": "user", "parts": parts }));
                }
            }
        }
        let mut body = json!({
            "contents": contents,
            "generationConfig": {
                "maxOutputTokens": req.max_tokens,
            },
        });
        if !system.is_empty() {
            body["systemInstruction"] = json!({ "parts": [{ "text": system }] });
        }
        if !req.tools.is_empty() {
            body["tools"] = Value::Array(vec![json!({
                "functionDeclarations": req.tools.iter().map(|t| json!({
                    "name": t.name,
                    "description": t.description,
                    "parameters": strip_unsupported(t.parameters.clone()),
                })).collect::<Vec<_>>(),
            })]);
        }
        // 思考档位：Gemini 用 thinkingConfig 的 thinkingBudget（token 数，-1 = 动态）
        if let Some(budget) = thinking_budget(req) {
            body["generationConfig"]["thinkingConfig"] = json!({ "thinkingBudget": budget });
        }
        body
    }
}

/// 思考档位 → token 预算；`Off` 不开思考。
///
/// Gemini 的 -1 表示「动态思考」，0 表示「关闭」，正数是固定预算。
fn thinking_budget(req: &StreamRequest) -> Option<i64> {
    use crate::types::ThinkingLevel;
    Some(match req.thinking {
        ThinkingLevel::Off => 0,
        ThinkingLevel::Minimal => 1024,
        ThinkingLevel::Low => 2048,
        ThinkingLevel::Medium => 8192,
        ThinkingLevel::High => 16_384,
        ThinkingLevel::XHigh => 32_768,
        ThinkingLevel::Max => 65_536,
    })
}

/// 剥掉 Gemini 不认的 JSON Schema 关键字：它对 schema 校验很严，
/// 带上 `additionalProperties` / `$schema` 这类会直接 400。
fn strip_unsupported(schema: Value) -> Value {
    let mut out = schema;
    if let Some(obj) = out.as_object_mut() {
        for key in [
            "additionalProperties",
            "$schema",
            "$id",
            "default",
            "examples",
            "exclusiveMinimum",
            "exclusiveMaximum",
            "const",
        ] {
            obj.remove(key);
        }
        if let Some(props) = obj.get_mut("properties").and_then(|p| p.as_object_mut()) {
            for (_, v) in props.iter_mut() {
                *v = strip_unsupported(v.clone());
            }
        }
    }
    out
}

/// 工具参数：Gemini 的 args 是对象，参数是 JSON 文本时要解出来。
fn parse_arguments(raw: &str) -> Value {
    serde_json::from_str(raw).unwrap_or_else(|_| json!({}))
}

/// 纯文本拼接（系统 / 工具结果只有文本）。
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

/// 用户消息的 parts：文本 + 内联图片。
fn user_parts(m: &ReqMessage) -> Vec<Value> {
    m.parts
        .iter()
        .filter_map(|p| match p {
            ContentPart::Text(t) if !t.is_empty() => Some(json!({ "text": t })),
            ContentPart::Image { mime, base64 } => Some(json!({
                "inlineData": { "mimeType": mime, "data": base64 },
            })),
            _ => None,
        })
        .collect()
}

/// 工具结果里的图片：另起一条 user 内容。
fn image_parts(m: &ReqMessage) -> Option<Vec<Value>> {
    let parts: Vec<Value> = m
        .parts
        .iter()
        .filter_map(|p| match p {
            ContentPart::Image { mime, base64 } => Some(json!({
                "inlineData": { "mimeType": mime, "data": base64 },
            })),
            ContentPart::Text(_) => None,
        })
        .collect();
    if parts.is_empty() {
        return None;
    }
    Some(parts)
}

/// HTTP 状态 → 是否可重试。
fn retryable_status(status: u16) -> bool {
    matches!(status, 429 | 500 | 502 | 503 | 504)
}

/// 从错误体提取人话（Gemini 是 {"error":{"message"}}）。
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
impl ModelClient for GeminiClient {
    async fn stream(
        &self,
        req: StreamRequest,
        cancel: CancellationToken,
    ) -> Result<ProviderStream> {
        let body = self.build_body(&req);
        // 官方端点是 /v1beta/models/{model}:streamGenerateContent?alt=sse
        let url = format!(
            "{}/v1beta/models/{}:streamGenerateContent?alt=sse",
            self.p.base_url, self.p.model_id
        );
        let mut builder = self
            .http
            .post(url)
            .header("Content-Type", "application/json");
        if !self.p.api_key.is_empty() {
            builder = builder.header("x-goog-api-key", self.p.api_key.clone());
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
fn unfold_sse(
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
                            let mut out = acc.flush_tools();
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
                                let mut out = acc.flush_tools();
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

impl Acc {
    /// 结清累积中的工具调用（对外可见，测试与上层结算共用）。
    pub fn flush_tools_public(&mut self) -> Vec<ModelEvent> {
        self.flush_tools()
    }

    /// 结清累积中的工具调用：Gemini 的 functionCall 参数是整包给的，
    /// 但仍可能跨帧出现（args 的子字段分帧），所以累积到定稿。
    fn flush_tools(&mut self) -> Vec<ModelEvent> {
        let mut out = Vec::new();
        for t in self.tools.drain(..) {
            out.push(ModelEvent::ToolCallEnd(
                out.len(),
                ToolCallDone {
                    call_id: t.id,
                    name: t.name,
                    arguments: t.args,
                },
            ));
        }
        if !out.is_empty() {
            self.stop = Some(StopReason::ToolUse);
        }
        out
    }
}

/// 处理单个 SSE 帧；返回本帧产生的事件序列。
pub fn handle_chunk(data: &str, acc: &mut Acc) -> Vec<ModelEvent> {
    let mut out: Vec<ModelEvent> = Vec::new();
    let Ok(v) = serde_json::from_str::<Value>(data) else {
        return out;
    };
    // 用量在帧顶层的 usageMetadata 里
    if let Some(u) = v.get("usageMetadata") {
        // promptTokenCount 已含缓存部分，input 扣掉才是未命中缓存的输入。
        // thoughtsTokenCount 是思考 token，Gemini 把它单列而不计入 candidatesTokenCount。
        let cache_read = u["cachedContentTokenCount"]
            .as_i64()
            .unwrap_or(acc.usage.cache_read);
        let thoughts = u["thoughtsTokenCount"].as_i64().unwrap_or(0);
        let prompt = u["promptTokenCount"].as_i64().unwrap_or(acc.usage.input);
        acc.usage.input = (prompt - cache_read).max(0);
        acc.usage.output = u["candidatesTokenCount"]
            .as_i64()
            .unwrap_or(acc.usage.output)
            + thoughts;
        acc.usage.cache_read = cache_read;
        acc.usage.reasoning = thoughts;
    }
    // 末帧可能只有 usageMetadata 没有 candidates：用量要在早退之前处理，
    // 否则最后一帧的 token 全丢
    let Some(candidates) = v["candidates"].as_array() else {
        return out;
    };
    for cand in candidates {
        if let Some(fr) = cand["finishReason"].as_str() {
            acc.stop = normalize_finish(fr).or(acc.stop);
        }
        let Some(parts) = cand["content"]["parts"].as_array() else {
            continue;
        };
        for part in parts {
            // 思考块：part 里带 thought 标记的才是思考，正文不带
            if part["thought"].as_bool().unwrap_or(false) {
                if let Some(t) = part["text"].as_str() {
                    if !t.is_empty() {
                        out.push(ModelEvent::ThinkingDelta(out.len(), t.to_string()));
                    }
                }
                continue;
            }
            if let Some(t) = part["text"].as_str() {
                if !t.is_empty() {
                    out.push(ModelEvent::TextDelta(out.len(), t.to_string()));
                }
            }
            if let Some(fc) = part.get("functionCall") {
                let name = fc["name"].as_str().unwrap_or("").to_string();
                let id = fc["id"].as_str().unwrap_or("").to_string();
                let args = fc.get("args").map(|a| a.to_string()).unwrap_or_default();
                if !name.is_empty() {
                    acc.tools.push(PendingTool { id, name, args });
                }
            }
        }
    }
    out
}

/// 上游 finishReason → 归一化停止原因。
pub fn normalize_finish(raw: &str) -> Option<StopReason> {
    Some(match raw {
        "STOP" => StopReason::Stop,
        "MAX_TOKENS" => StopReason::Length,
        "SAFETY" | "RECITATION" | "PROHIBITED_CONTENT" => StopReason::Error,
        "OTHER" => StopReason::Stop,
        _ => return None,
    })
}
