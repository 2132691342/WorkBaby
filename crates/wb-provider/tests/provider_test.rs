//! wb-provider 场景测试：wire 协议解析。
//! 流式分片与错误体解析决定了模型输出能否正确落到会话里——错了会静默丢内容。

use wb_provider::openai::{error_message, handle_chunk, Acc};
use wb_provider::{ModelEvent, StopReason, ToolCallDone};

/// 工具调用的参数分多片到达，累积到 finish_reason=tool_calls 时才吐 ToolCallEnd。
/// 提前吐出会拿到半个 JSON，参数校验必然失败。
#[test]
fn tool_call_arguments_accumulate_across_chunks() {
    let mut acc = Acc::default();
    let first = r#"{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":"}}]},"finish_reason":null}]}"#;
    let events = handle_chunk(first, &mut acc);
    assert!(
        events
            .iter()
            .any(|e| matches!(e, ModelEvent::ToolCallDelta(..))),
        "参数分片应先吐 ToolCallDelta，不提前定稿"
    );
    assert!(
        !events
            .iter()
            .any(|e| matches!(e, ModelEvent::ToolCallEnd(..))),
        "参数没齐不能定稿"
    );

    let second = r#"{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a\"}"}}]},"finish_reason":null}]}"#;
    handle_chunk(second, &mut acc);

    let third = r#"{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}"#;
    let events = handle_chunk(third, &mut acc);
    let done = events
        .iter()
        .find_map(|e| match e {
            ModelEvent::ToolCallEnd(_, d) => Some(d),
            _ => None,
        })
        .expect("定稿帧应给出完整调用");
    assert_eq!(done.name, "read");
    assert_eq!(done.arguments, r#"{"path":"a"}"#, "分片参数应拼成完整 JSON");
}

/// 并行工具调用按 index 各自累积，定稿时一个都不能少。
/// 漏掉一个就是丢掉一次调用——模型以为做过了，实际没做。
#[test]
fn parallel_tool_calls_are_all_finalized() {
    let mut acc = Acc::default();
    let chunk = r#"{"choices":[{"index":0,"delta":{"tool_calls":[
      {"index":0,"id":"c0","function":{"name":"read","arguments":"{\"path\":\"a\"}"}},
      {"index":1,"id":"c1","function":{"name":"grep","arguments":"{\"pattern\":\"b\"}"}}
    ]}}]}"#;
    handle_chunk(chunk, &mut acc);
    let done = r#"{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}"#;
    let events = handle_chunk(done, &mut acc);
    let calls: Vec<&ToolCallDone> = events
        .iter()
        .filter_map(|e| match e {
            ModelEvent::ToolCallEnd(_, d) => Some(d),
            _ => None,
        })
        .collect();
    assert_eq!(
        calls.len(),
        2,
        "两个并行调用都要定稿，实际 {} 个",
        calls.len()
    );
    assert_eq!(calls[0].name, "read");
    assert_eq!(calls[0].arguments, r#"{"path":"a"}"#);
    assert_eq!(calls[1].name, "grep");
    assert_eq!(calls[1].arguments, r#"{"pattern":"b"}"#);
}

/// 一帧里既有正文又有工具调用时，两者都要产出事件。
#[test]
fn one_frame_yields_text_and_tool_events() {
    let mut acc = Acc::default();
    let chunk = r#"{"choices":[{"index":0,"delta":{"content":"我去查一下","tool_calls":[
      {"index":0,"id":"c0","function":{"name":"read","arguments":"{}"}}
    ]},"finish_reason":null}]}"#;
    let events = handle_chunk(chunk, &mut acc);
    assert!(
        events
            .iter()
            .any(|e| matches!(e, ModelEvent::TextDelta(_, s) if s == "我去查一下")),
        "正文增量不能被工具分片挤掉"
    );
    assert!(
        events
            .iter()
            .any(|e| matches!(e, ModelEvent::ToolCallDelta(..))),
        "工具分片也要产出"
    );
}

/// 三种 finish_reason 各归各位：stop 只收尾不产事件；length 表示截断（内核据此重试）；
/// tool_calls 映射为 tool_use——混淆会让内核在正文收尾后继续空转烧 token。
#[test]
fn stop_reasons_follow_finish_reason() {
    let mut acc = Acc::default();
    let events = handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"content":"你好"},"finish_reason":null}]}"#,
        &mut acc,
    );
    assert!(events
        .iter()
        .any(|e| matches!(e, ModelEvent::TextDelta(_, s) if s == "你好")));

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
    assert_eq!(acc.stop, Some(StopReason::Length));

    let mut acc = Acc::default();
    handle_chunk(
        r#"{"choices":[{"index":0,"delta":{"tool_calls":[
      {"index":0,"id":"c0","function":{"name":"read","arguments":"{}"}}
    ]}}]}"#,
        &mut acc,
    );
    handle_chunk(
        r#"{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}"#,
        &mut acc,
    );
    assert_eq!(acc.stop, Some(StopReason::ToolUse));
}

/// 思考增量走独立的 reasoning_content 通道，不与正文混。
#[test]
fn reasoning_content_becomes_thinking_delta() {
    let mut acc = Acc::default();
    let chunk =
        r#"{"choices":[{"index":0,"delta":{"reasoning_content":"想一下"},"finish_reason":null}]}"#;
    let events = handle_chunk(chunk, &mut acc);
    assert!(events
        .iter()
        .any(|e| matches!(e, ModelEvent::ThinkingDelta(_, s) if s == "想一下")));
}

/// 上游错误体：JSON 提取 message 字段；非 JSON 原样带出，不能吞掉。
#[test]
fn error_message_extracts_json_field_and_keeps_plain_text() {
    let m = error_message(401, r#"{"error":{"message":"bad key"}}"#);
    assert!(m.contains("bad key"), "应提取 message 字段：{m}");
    let m = error_message(500, "upstream exploded");
    assert!(
        m.contains("upstream exploded"),
        "非 JSON 错误体应原样带出：{m}"
    );
}
