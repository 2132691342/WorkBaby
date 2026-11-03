//! wb-provider 场景测试：wire 协议解析。流式分片与错误体解析决定模型输出能否正确落进会话。

use wb_provider::openai::{error_message, handle_chunk, Acc};
use wb_provider::{ModelEvent, StopReason, ToolCallDone};

fn tool_calls(events: &[ModelEvent]) -> Vec<&ToolCallDone> {
    events
        .iter()
        .filter_map(|e| match e {
            ModelEvent::ToolCallEnd(_, d) => Some(d),
            _ => None,
        })
        .collect()
}

/// 工具调用完整生命周期：参数分片按 index 累积、与正文同帧不互挤；
/// finish_reason=tool_calls 时并行调用全量定稿——漏一个就是丢一次调用。
#[test]
fn tool_calls_accumulate_and_finalize_across_chunks() {
    let mut acc = Acc::default();
    let first = r#"{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":"}}]},"finish_reason":null}]}"#;
    let events = handle_chunk(first, &mut acc);
    assert!(
        events
            .iter()
            .any(|e| matches!(e, ModelEvent::ToolCallDelta(..))),
        "参数分片先吐 Delta 不定稿"
    );
    assert!(tool_calls(&events).is_empty());
    handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a\"}"}}]},"finish_reason":null}]}"#,
        &mut acc,
    );
    let events = handle_chunk(
        r#"{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}"#,
        &mut acc,
    );
    let done = tool_calls(&events)
        .into_iter()
        .next()
        .expect("定稿帧应给出完整调用");
    assert_eq!(done.name, "read");
    assert_eq!(done.arguments, r#"{"path":"a"}"#, "分片参数应拼成完整 JSON");

    // 并行调用按 index 各自累积，定稿时全量吐出。
    let mut acc = Acc::default();
    handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"tool_calls":[
      {"index":0,"id":"c0","function":{"name":"read","arguments":"{\"path\":\"a\"}"}},
      {"index":1,"id":"c1","function":{"name":"grep","arguments":"{\"pattern\":\"b\"}"}}
    ]}}]}"#,
        &mut acc,
    );
    let events = handle_chunk(
        r#"{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}"#,
        &mut acc,
    );
    let names: Vec<&str> = tool_calls(&events)
        .iter()
        .map(|d| d.name.as_str())
        .collect();
    assert_eq!(names, ["read", "grep"], "两个并行调用都要定稿");

    // 一帧里正文 + 工具调用并存，两者都产出。
    let mut acc = Acc::default();
    let events = handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"content":"我去查一下","tool_calls":[
      {"index":0,"id":"c0","function":{"name":"read","arguments":"{}"}}
    ]},"finish_reason":null}]}"#,
        &mut acc,
    );
    assert!(events
        .iter()
        .any(|e| matches!(e, ModelEvent::TextDelta(_, s) if s == "我去查一下")));
    assert!(
        events
            .iter()
            .any(|e| matches!(e, ModelEvent::ToolCallDelta(..))),
        "同帧的工具分片也要产出"
    );
}

/// 网关夹具差异下的兜底：空 finish_reason 不是终止态，上游不给 finish_reason
/// 就断流时手上攒的调用必须定稿——两种情形漏一种就是一次调用凭空消失。
#[test]
fn gateway_fixture_quirks_never_lose_tool_calls() {
    let mut acc = Acc::default();
    handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":""},"index":0}]},"finish_reason":""}],"usage":null}"#,
        &mut acc,
    );
    assert_eq!(acc.stop, None, "空 finish_reason 不该定下停止原因");
    handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"tool_calls":[{"function":{"arguments":"{\"command\""},"index":0}]},"finish_reason":""}]}"#,
        &mut acc,
    );
    let events = handle_chunk(
        r#"{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10}}"#,
        &mut acc,
    );
    let done = tool_calls(&events)
        .into_iter()
        .next()
        .expect("finish_reason=tool_calls 应定稿调用");
    assert_eq!(done.name, "bash");
    assert_eq!(done.arguments, "{\"command\"", "分片参数应拼完整");
    assert_eq!(acc.stop, Some(StopReason::ToolUse));

    let mut acc = Acc::default();
    handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"tool_calls":[
      {"index":0,"id":"c0","function":{"name":"read","arguments":"{}"}},
      {"index":1,"id":"c1","function":{"name":"grep","arguments":"{}"}}
    ]},"finish_reason":""}]}"#,
        &mut acc,
    );
    let names: Vec<String> = acc
        .flush_tools()
        .iter()
        .filter_map(|e| match e {
            ModelEvent::ToolCallEnd(_, d) => Some(d.name.clone()),
            _ => None,
        })
        .collect();
    assert_eq!(names, ["read", "grep"], "断流前攒下的调用都要定稿");
    assert_eq!(acc.stop, Some(StopReason::ToolUse));
    assert!(acc.flush_tools().is_empty(), "结清后不应重复吐同一批调用");
}

/// 帧到事件映射：stop 只收尾不产事件、length 表示截断、tool_calls 映射为 ToolUse
/// （混淆会让内核在正文收尾后空转烧 token）、思考走独立 reasoning_content、
/// 策略截断要作为不可重试错误上报、上游错误体提取 message 且非 JSON 原样带出。
#[test]
fn frames_map_stop_thinking_and_error_payloads() {
    let mut acc = Acc::default();
    handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"content":"你好"},"finish_reason":null}]}"#,
        &mut acc,
    );
    let events = handle_chunk(
        r#"{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}"#,
        &mut acc,
    );
    assert!(events.is_empty(), "stop 只标记收尾，不产出增量事件");
    assert_eq!(acc.stop, Some(StopReason::Stop));

    let mut acc = Acc::default();
    handle_chunk(
        r#"{"choices":[{"index":0,"delta":{},"finish_reason":"length"}]}"#,
        &mut acc,
    );
    assert_eq!(acc.stop, Some(StopReason::Length), "截断要能与正常收尾区分");

    let mut acc = Acc::default();
    let events = handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"reasoning_content":"想一下"},"finish_reason":null}]}"#,
        &mut acc,
    );
    assert!(events
        .iter()
        .any(|e| matches!(e, ModelEvent::ThinkingDelta(_, s) if s == "想一下")));

    let mut acc = Acc::default();
    let events = handle_chunk(
        r#"{"choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}"#,
        &mut acc,
    );
    assert!(
        events
            .iter()
            .any(|e| matches!(e, ModelEvent::Error(false, _))),
        "策略截断应作为不可重试错误上报"
    );

    let m = error_message(401, r#"{"error":{"message":"bad key"}}"#);
    assert!(m.contains("bad key"), "应提取 message：{m}");
    let m = error_message(500, "upstream exploded");
    assert!(
        m.contains("upstream exploded"),
        "非 JSON 错误体应原样带出：{m}"
    );
}

// ---- 思考档位 ----
/// 界面给七档，后端必须认七档且原样落到 wire。
/// 静默降级会让「我选了 max」与「它在随便跑」不可区分。
#[test]
fn thinking_levels_roundtrip_without_silent_downgrade() {
    use wb_provider::ThinkingLevel;
    let cases = [
        ("off", ThinkingLevel::Off, None),
        ("minimal", ThinkingLevel::Minimal, Some("minimal")),
        ("low", ThinkingLevel::Low, Some("low")),
        ("medium", ThinkingLevel::Medium, Some("medium")),
        ("high", ThinkingLevel::High, Some("high")),
        ("xhigh", ThinkingLevel::XHigh, Some("xhigh")),
        ("max", ThinkingLevel::Max, Some("max")),
    ];
    for (code, expect, wire) in cases {
        let got = ThinkingLevel::parse(code).unwrap_or_else(|| panic!("{code} 应可解析"));
        assert_eq!(got, expect, "{code} 解析错了：got {got:?}");
        assert_eq!(got.wire_value(), wire, "{code} 的 wire 取值不对");
    }
    assert!(
        ThinkingLevel::parse("ultra").is_none(),
        "未知档位不能兜底成 medium"
    );
}

// ---- anthropic-messages ----
/// Anthropic 的块结构：system 独立、工具参数走 input_json_delta、thinking 走 thinking_delta。
/// 三处任一处解析错，模型就会「说了话但没调工具」或「工具参数少一半」。
#[test]
fn anthropic_blocks_accumulate_text_thinking_and_tool_args() {
    use wb_provider::anthropic_stream::{handle_chunk, Acc};
    use wb_provider::{ModelEvent, StopReason};

    let mut acc = Acc::default();
    // 文本块
    handle_chunk(
        r#"{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}"#,
        &mut acc,
    );
    let evs = handle_chunk(
        r#"{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"你好"}}"#,
        &mut acc,
    );
    assert!(
        evs.iter()
            .any(|e| matches!(e, ModelEvent::TextDelta(_, s) if s == "你好")),
        "文本增量应上报：{evs:?}"
    );

    // 思考块
    handle_chunk(
        r#"{"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":""}}"#,
        &mut acc,
    );
    let evs = handle_chunk(
        r#"{"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"想一下"}}"#,
        &mut acc,
    );
    assert!(
        evs.iter()
            .any(|e| matches!(e, ModelEvent::ThinkingDelta(_, s) if s == "想一下")),
        "思考增量应上报：{evs:?}"
    );

    // 工具块：参数分两片
    handle_chunk(
        r#"{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"read"}}"#,
        &mut acc,
    );
    handle_chunk(
        r#"{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}"#,
        &mut acc,
    );
    handle_chunk(
        r#"{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"a.txt\"}"}}"#,
        &mut acc,
    );
    let flushed = acc.flush_blocks();
    let call = flushed
        .iter()
        .find_map(|e| match e {
            ModelEvent::ToolCallEnd(_, d) => Some(d),
            _ => None,
        })
        .expect("工具调用应定稿");
    assert_eq!(call.call_id, "toolu_1");
    assert_eq!(call.name, "read");
    assert_eq!(call.arguments, r#"{"path":"a.txt"}"#);

    // stop_reason 归一化
    let evs = handle_chunk(
        r#"{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":42}}"#,
        &mut acc,
    );
    assert!(evs.is_empty(), "message_delta 不产生增量事件");
    assert_eq!(acc.stop, Some(StopReason::ToolUse));
    assert_eq!(acc.usage.output, 42);
}

/// 流断在中途时已经开始的工具块必须定稿——否则「模型要求调工具」被当成正文结束。
#[test]
fn anthropic_flushes_pending_tool_on_truncated_stream() {
    use wb_provider::anthropic_stream::{handle_chunk, Acc};
    use wb_provider::ModelEvent;

    let mut acc = Acc::default();
    handle_chunk(
        r#"{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_x","name":"bash"}}"#,
        &mut acc,
    );
    handle_chunk(
        r#"{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}"#,
        &mut acc,
    );
    let flushed = acc.flush_blocks();
    assert!(
        flushed
            .iter()
            .any(|e| matches!(e, ModelEvent::ToolCallEnd(_, d) if d.name == "bash")),
        "截断流也必须定稿工具调用：{flushed:?}"
    );
}

// ---- google-generative-ai ----
/// Gemini 的 functionCall 与 thought part：工具调用走 functionCall，思考块靠 thought 标记。
#[test]
fn gemini_reads_function_calls_thoughts_and_usage() {
    use wb_provider::gemini::{handle_chunk, Acc};
    use wb_provider::{ModelEvent, StopReason};

    let mut acc = Acc::default();
    let evs = handle_chunk(
        r#"{"candidates":[{"content":{"parts":[{"text":"正文"},{"text":"推理中","thought":true}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7}}"#,
        &mut acc,
    );
    assert!(
        evs.iter()
            .any(|e| matches!(e, ModelEvent::TextDelta(_, s) if s == "正文")),
        "正文应上报：{evs:?}"
    );
    assert!(
        evs.iter()
            .any(|e| matches!(e, ModelEvent::ThinkingDelta(_, s) if s == "推理中")),
        "思考块应走 thinking 通道：{evs:?}"
    );
    assert_eq!(acc.usage.input, 11);
    assert_eq!(acc.usage.output, 7);
    assert_eq!(acc.stop, Some(StopReason::Stop));

    // functionCall：先累积，定稿时出 ToolCallEnd
    handle_chunk(
        r#"{"candidates":[{"content":{"parts":[{"functionCall":{"id":"fc_1","name":"grep","args":{"pattern":"x"}}}]},"finishReason":"STOP"}]}"#,
        &mut acc,
    );
    let flushed = acc.flush_tools_public();
    assert!(
        flushed
            .iter()
            .any(|e| matches!(e, ModelEvent::ToolCallEnd(_, d) if d.name == "grep" && d.call_id == "fc_1")),
        "functionCall 应定稿成工具调用：{flushed:?}"
    );
}

// ---- 提示词缓存与用量口径归一 ----
/// 守的契约——缓存断点落在每轮都重发且不变的三处（系统提示词 / 最后工具 / 历史末尾）；用量字段互不重叠。
/// 断点位置错了就命不中缓存——断在中间的话后面的内容不在缓存前缀里。
#[test]
fn anthropic_places_cache_breakpoints_on_system_tools_and_history_tail() {
    use wb_provider::anthropic::AnthropicClient;
    use wb_provider::config::ResolvedProvider;
    use wb_provider::{ContentPart, ReqMessage, Role, StreamRequest, ThinkingLevel, ToolDef};

    let p = ResolvedProvider {
        provider_id: "p".into(),
        base_url: "https://api.anthropic.com".into(),
        api: "anthropic-messages".into(),
        api_key: "k".into(),
        headers: Vec::new(),
        model_id: "claude".into(),
        context_window: 200_000,
        max_tokens: 8_192,
        reasoning: false,
        vision: false,
        cache: true,
    };
    let req = StreamRequest {
        model_id: "claude".into(),
        messages: vec![
            ReqMessage {
                role: Role::System,
                parts: vec![ContentPart::Text("系统提示词".into())],
                tool_calls: Vec::new(),
                tool_call_id: None,
                tool_name: None,
            },
            ReqMessage {
                role: Role::User,
                parts: vec![ContentPart::Text("你好".into())],
                tool_calls: Vec::new(),
                tool_call_id: None,
                tool_name: None,
            },
        ],
        tools: vec![
            ToolDef {
                name: "a".into(),
                description: "d".into(),
                parameters: serde_json::json!({"type":"object"}),
            },
            ToolDef {
                name: "b".into(),
                description: "d".into(),
                parameters: serde_json::json!({"type":"object"}),
            },
        ],
        max_tokens: 8_192,
        thinking: ThinkingLevel::Off,
    };
    let body = AnthropicClient::new(p).build_body(&req);

    let system = &body["system"][0];
    assert_eq!(
        system["cache_control"]["type"], "ephemeral",
        "系统提示词要断缓存：{system}"
    );

    let tools = body["tools"].as_array().expect("工具应是数组");
    assert!(
        tools[0].get("cache_control").is_none(),
        "断点不能落在第一个工具上，否则后续工具不在缓存前缀里"
    );
    assert_eq!(
        tools[1]["cache_control"]["type"], "ephemeral",
        "断点要落在最后一个工具上"
    );

    let msgs = body["messages"].as_array().expect("消息应是数组");
    let last = &msgs[msgs.len() - 1];
    assert_eq!(
        last["content"][0]["cache_control"]["type"], "ephemeral",
        "历史末尾要断缓存：{last}"
    );
}

/// 缓存关掉时不得留任何 cache_control 字段：部分兼容网关见到就报非法参数。
#[test]
fn anthropic_omits_cache_control_when_cache_disabled() {
    use wb_provider::anthropic::AnthropicClient;
    use wb_provider::config::ResolvedProvider;
    use wb_provider::{ContentPart, ReqMessage, Role, StreamRequest, ThinkingLevel, ToolDef};

    let p = ResolvedProvider {
        provider_id: "p".into(),
        base_url: "https://api.anthropic.com".into(),
        api: "anthropic-messages".into(),
        api_key: "k".into(),
        headers: Vec::new(),
        model_id: "claude".into(),
        context_window: 200_000,
        max_tokens: 8_192,
        reasoning: false,
        vision: false,
        cache: false,
    };
    let req = StreamRequest {
        model_id: "claude".into(),
        messages: vec![ReqMessage {
            role: Role::System,
            parts: vec![ContentPart::Text("系统提示词".into())],
            tool_calls: Vec::new(),
            tool_call_id: None,
            tool_name: None,
        }],
        tools: vec![ToolDef {
            name: "a".into(),
            description: "d".into(),
            parameters: serde_json::json!({"type":"object"}),
        }],
        max_tokens: 8_192,
        thinking: ThinkingLevel::Off,
    };
    let body = AnthropicClient::new(p).build_body(&req);
    let text = body.to_string();
    assert!(
        !text.contains("cache_control"),
        "缓存关时不得出现 cache_control：{text}"
    );
}

/// OpenAI 的 prompt_tokens 已含缓存命中部分：input 必须扣掉，
/// 否则 input + cache_read 把同一批 token 算两遍。
#[test]
fn openai_input_excludes_cached_tokens() {
    use wb_provider::openai::{handle_chunk, Acc};

    let mut acc = Acc::default();
    handle_chunk(
        r#"{"usage":{"prompt_tokens":1000,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":800,"cache_write_tokens":100}}}"#,
        &mut acc,
    );
    assert_eq!(acc.usage.cache_read, 800, "缓存读要单独记");
    assert_eq!(acc.usage.cache_write, 100, "缓存写要单独记");
    assert_eq!(
        acc.usage.input, 100,
        "input 应是未命中缓存的部分：1000-800-100"
    );
    assert_eq!(
        acc.usage.total(),
        1050,
        "总量应还原成上游处理的总 token：1000 提示 + 50 补全"
    );
}

/// DeepSeek / Kimi 把缓存命中放在顶层，不在 prompt_tokens_details 里。
#[test]
fn openai_reads_cache_from_alternative_fields() {
    use wb_provider::openai::{handle_chunk, Acc};

    let mut acc = Acc::default();
    handle_chunk(
        r#"{"usage":{"prompt_tokens":600,"completion_tokens":10,"prompt_cache_hit_tokens":500}}"#,
        &mut acc,
    );
    assert_eq!(acc.usage.cache_read, 500, "顶层命中字段也要认");
    assert_eq!(acc.usage.input, 100, "同样要扣掉缓存部分");

    let mut acc = Acc::default();
    handle_chunk(
        r#"{"usage":{"prompt_tokens":600,"completion_tokens":10,"cached_tokens":400}}"#,
        &mut acc,
    );
    assert_eq!(
        acc.usage.cache_read, 400,
        "Kimi 的顶层 cached_tokens 也要认"
    );
    assert_eq!(acc.usage.input, 200);
}

/// Gemini 的 promptTokenCount 含缓存；思考 token 单列，不在候选 token 里。
#[test]
fn gemini_input_excludes_cached_and_counts_thoughts_in_output() {
    use wb_provider::gemini::{handle_chunk, Acc};

    let mut acc = Acc::default();
    handle_chunk(
        r#"{"usageMetadata":{"promptTokenCount":900,"candidatesTokenCount":40,"cachedContentTokenCount":700,"thoughtsTokenCount":60}}"#,
        &mut acc,
    );
    assert_eq!(acc.usage.cache_read, 700);
    assert_eq!(acc.usage.input, 200, "900-700");
    assert_eq!(acc.usage.output, 100, "候选 40 + 思考 60");
    assert_eq!(acc.usage.reasoning, 60);
}

/// Anthropic 的 input_tokens 本就不含缓存部分，不该再扣一次。
#[test]
fn anthropic_input_stays_as_reported() {
    use wb_provider::anthropic_stream::{handle_chunk, Acc};

    let mut acc = Acc::default();
    handle_chunk(
        r#"{"type":"message_start","message":{"usage":{"input_tokens":100,"cache_read_input_tokens":500,"cache_creation_input_tokens":200}}}"#,
        &mut acc,
    );
    assert_eq!(acc.usage.input, 100, "Anthropic 的 input 不含缓存");
    assert_eq!(acc.usage.cache_read, 500);
    assert_eq!(acc.usage.cache_write, 200);
    assert_eq!(acc.usage.total(), 800, "总量是四者之和");
}
