//! anthropic-messages wire：Messages API 流式实现（直连 Anthropic 与其兼容网关）。
//! 与 openai 的差异：system 独立传、tool_result 装 user 消息、思考与工具各有独立块类型。

use eventsource_stream::Eventsource;

use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;

use wb_pkg::{AppError, Result};

use crate::anthropic_stream::unfold_sse;
use crate::config::ResolvedProvider;
use crate::types::{
    ContentPart, ModelClient, ModelEvent, ProviderStream, ReqMessage, Role, StreamRequest, ToolDef,
};

/// anthropic 系客户端。
pub struct AnthropicClient {
    /// 解析后的 provider（地址 / 密钥 / 模型元信息）
    p: ResolvedProvider,
    /// 共享 HTTP 客户端
    http: reqwest::Client,
}

/// Anthropic 的 api-version 头：老版本网关不认这头会直接 400。
const API_VERSION: &str = "2023-06-01";

/// 缓存断点标记：ephemeral 是 Anthropic 的标准缓存类型。
/// Anthropic 只认这四个字，多写字段部分兼容网关会报错。
fn cache_control() -> Value {
    json!({ "type": "ephemeral" })
}

/// 在历史消息的最后一处断缓存点。
///
/// 缓存按「前缀」命中：断点之前的内容完全一致才命中。所以断点要落在
/// 最后一个 user / tool_result 消息上——它前面的整条历史与工具定义
/// 都进了缓存前缀，下一轮只有新增部分按增量价计费。
///
/// 断点超过 Anthropic 的上限（4 个）会被拒，所以只断一处。
fn mark_history_cache_point(body: &mut Value) {
    let Some(msgs) = body.get_mut("messages").and_then(|m| m.as_array_mut()) else {
        return;
    };
    for msg in msgs.iter_mut().rev() {
        let role = msg.get("role").and_then(|r| r.as_str()).unwrap_or("");
        if !matches!(role, "user" | "assistant") {
            continue;
        }
        let Some(content) = msg.get_mut("content") else {
            continue;
        };
        // content 可能是字符串也可能是块数组；只有块数组能挂 cache_control
        let Some(blocks) = content.as_array_mut() else {
            continue;
        };
        let Some(last) = blocks.last_mut() else {
            continue;
        };
        // 只给文本块与工具结果块断点：图片块挂了部分网关不认
        let kind = last.get("type").and_then(|t| t.as_str()).unwrap_or("");
        if matches!(kind, "text" | "tool_result") {
            last["cache_control"] = cache_control();
        }
        return;
    }
}

impl AnthropicClient {
    /// 构造（密钥为空也允许，兼容网关可能免鉴权）。
    pub fn new(p: ResolvedProvider) -> Self {
        Self {
            p,
            http: reqwest::Client::new(),
        }
    }

    /// 组请求体：system 独立传，工具结果包成 user 消息里的 tool_result 块。
    /// 公开是为了能直接断言请求体形状：缓存断点、工具 schema、思考预算都在这里定。
    pub fn build_body(&self, req: &StreamRequest) -> Value {
        let mut system = String::new();
        let mut msgs: Vec<Value> = Vec::new();
        for m in &req.messages {
            match m.role {
                Role::System => {
                    if !system.is_empty() {
                        system.push('\n');
                    }
                    system.push_str(&join_text(m));
                }
                Role::User => msgs.push(json!({
                    "role": "user",
                    "content": user_content(m),
                })),
                Role::Assistant => {
                    let mut blocks: Vec<Value> = Vec::new();
                    for part in &m.parts {
                        if let ContentPart::Text(t) = part {
                            if !t.is_empty() {
                                blocks.push(json!({ "type": "text", "text": t }));
                            }
                        }
                    }
                    for tc in &m.tool_calls {
                        blocks.push(json!({
                            "type": "tool_use",
                            "id": tc.call_id,
                            "name": tc.name,
                            "input": parse_arguments(&tc.arguments),
                        }));
                    }
                    if blocks.is_empty() {
                        continue;
                    }
                    msgs.push(json!({ "role": "assistant", "content": blocks }));
                }
                Role::ToolResult => {
                    let mut blocks: Vec<Value> = Vec::new();
                    blocks.push(json!({
                        "type": "tool_result",
                        "tool_use_id": m.tool_call_id.clone().unwrap_or_default(),
                        "content": join_text(m),
                    }));
                    // 图片另起一条 user 消息：tool_result 块只接受文本。
                    // 丢掉图片等于「读图」这个能力在 Anthropic 下没做。
                    if let Some(imgs) = image_parts(m) {
                        msgs.push(json!({ "role": "user", "content": blocks }));
                        msgs.push(imgs);
                        continue;
                    }
                    msgs.push(json!({ "role": "user", "content": blocks }));
                }
            }
        }
        let mut body = json!({
            "model": self.p.model_id,
            "messages": msgs,
            "max_tokens": req.max_tokens,
            "stream": true,
        });
        if !system.is_empty() {
            // system 改成块数组才能挂 cache_control；缓存关时退回字符串形式
            body["system"] = if self.p.cache {
                json!([{
                    "type": "text",
                    "text": system,
                    // 系统提示词每轮都重发：在这里断一个缓存点，命中时只计增量价
                    "cache_control": cache_control(),
                }])
            } else {
                json!(system)
            };
        }
        if !req.tools.is_empty() {
            body["tools"] = Value::Array(
                req.tools
                    .iter()
                    .enumerate()
                    .map(|(i, t)| {
                        let mut def = json!({
                            "name": t.name,
                            "description": t.description,
                            "input_schema": t.parameters.clone(),
                        });
                        // 工具定义同样每轮重发，断点在最后一个工具上：
                        // 断在中间的话后续工具不在缓存前缀里，等于没断
                        if self.p.cache && i + 1 == req.tools.len() {
                            def["cache_control"] = cache_control();
                        }
                        def
                    })
                    .collect(),
            );
        }
        if self.p.cache {
            mark_history_cache_point(&mut body);
        }
        // 思考档位：Anthropic 用 budget_tokens 而不是 effort 取值。
        // 关档不送 thinking 字段（与不支持推理的模型等价）。
        if let Some(budget) = thinking_budget(req) {
            body["thinking"] = json!({ "type": "enabled", "budget_tokens": budget });
            // 开启思考时必须把 max_tokens 抬到预算之上，否则两者相加会超上限被拒
            if body["max_tokens"].as_i64().unwrap_or(0) <= budget {
                body["max_tokens"] = json!(budget + 4096);
            }
        }
        body
    }
}

/// 思考档位 → token 预算。档位越高预算越大；`Off` 不开思考。
/// Anthropic 的预算以 token 计（不是 effort 枚举），这里自己定一档梯度；
/// 预算不能等于 max_tokens（上游会拒：思考预算必须小于输出上限）。
fn thinking_budget(req: &StreamRequest) -> Option<i64> {
    let base = match req.thinking {
        crate::types::ThinkingLevel::Off => return None,
        crate::types::ThinkingLevel::Minimal => 1024,
        crate::types::ThinkingLevel::Low => 2048,
        crate::types::ThinkingLevel::Medium => 8192,
        crate::types::ThinkingLevel::High => 16_384,
        crate::types::ThinkingLevel::XHigh => 32_768,
        crate::types::ThinkingLevel::Max => 65_536,
    };
    Some(base.min(req.max_tokens.saturating_sub(1).max(1024)))
}

/// 工具参数：Anthropic 的 input 是对象，参数是 JSON 文本时要解出来。
/// 解不出来就传空对象：让上游报参数错，比把字符串塞进去报更难懂的错好。
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

/// 用户消息内容：文本块 + 图片块。
fn user_content(m: &ReqMessage) -> Value {
    let mut blocks: Vec<Value> = Vec::new();
    for p in &m.parts {
        match p {
            ContentPart::Text(t) => {
                if !t.is_empty() {
                    blocks.push(json!({ "type": "text", "text": t }));
                }
            }
            ContentPart::Image { mime, base64 } => {
                blocks.push(json!({
                    "type": "image",
                    "source": { "type": "base64", "media_type": mime, "data": base64 },
                }));
            }
        }
    }
    if blocks.is_empty() {
        blocks.push(json!({ "type": "text", "text": "" }));
    }
    Value::Array(blocks)
}

/// 工具结果里的图片：另起一条 user 消息（tool_result 块只接受文本）。
fn image_parts(m: &ReqMessage) -> Option<Value> {
    let blocks: Vec<Value> = m
        .parts
        .iter()
        .filter_map(|p| match p {
            ContentPart::Image { mime, base64 } => Some(json!({
                "type": "image",
                "source": { "type": "base64", "media_type": mime, "data": base64 },
            })),
            ContentPart::Text(_) => None,
        })
        .collect();
    if blocks.is_empty() {
        return None;
    }
    Some(json!({ "role": "user", "content": blocks }))
}

/// HTTP 状态 → 是否可重试。
fn retryable_status(status: u16) -> bool {
    matches!(status, 429 | 500 | 502 | 503 | 504 | 529)
}

/// 从错误体提取人话（Anthropic 是 {"error":{"message"}}）。
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
impl ModelClient for AnthropicClient {
    async fn stream(
        &self,
        req: StreamRequest,
        cancel: CancellationToken,
    ) -> Result<ProviderStream> {
        let body = self.build_body(&req);
        let mut builder = self
            .http
            .post(format!("{}/v1/messages", self.p.base_url))
            .header("Content-Type", "application/json")
            .header("anthropic-version", API_VERSION);
        if !self.p.api_key.is_empty() {
            builder = builder.header("x-api-key", self.p.api_key.clone());
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

/// 工具声明的转出形态（测试用）。
pub fn tool_defs(tools: &[ToolDef]) -> Value {
    Value::Array(
        tools
            .iter()
            .map(|t| {
                json!({
                    "name": t.name,
                    "description": t.description,
                    "input_schema": t.parameters.clone(),
                })
            })
            .collect(),
    )
}
