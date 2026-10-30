//! wb-provider 场景测试：流式响应的分片累积与错误体解析。
//! 这两处决定了模型输出能否正确落到会话里，错了会静默丢内容。

use wb_provider::openai::{error_message, handle_chunk, Acc};
use wb_provider::{ModelEvent, ToolCallDone};

/// 工具调用的参数分多片到达，累积到 finish_reason=tool_calls 时才吐 ToolCallEnd。
#[test]
fn tool_call_arguments_accumulate_across_chunks() {
    let mut acc = Acc::default();
    let first = r#"{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":"}}]},"finish_reason":null}]}"#;
    assert!(handle_chunk(first, &mut acc).is_some());
    let second = r#"{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a\"}"}}]},"finish_reason":null}]}"#;
    assert!(handle_chunk(second, &mut acc).is_some());
    let third = r#"{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}"#;
    match handle_chunk(third, &mut acc) {
        Some(ModelEvent::ToolCallEnd(
            _,
            ToolCallDone {
                name, arguments, ..
            },
        )) => {
            assert_eq!(name, "read");
            assert_eq!(arguments, r#"{"path":"a"}"#, "分片参数应拼成完整 JSON");
        }
        other => panic!("期望 ToolCallEnd，得到 {other:?}"),
    }
}

/// 上游错误体是 JSON 时要把里面的 message 提出来，而不是把整坨 JSON 甩给用户。
#[test]
fn error_body_message_is_extracted() {
    let m = error_message(401, r#"{"error":{"message":"bad key"}}"#);
    assert!(m.contains("bad key"));
}
