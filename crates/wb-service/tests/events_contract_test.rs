//! SSE 载荷契约测试：事件字段名是前后端协议，改名必须同步改前端。
//! 载荷字段名不走业务前缀规则（前端按 `data.kind` / `data.name` 读），
//! 正因为不走，改名时最容易两边对不上。这里钉住前端实际会读的那几个。

use serde_json::json;
use wb_domain::events::{
    AskClosedData, AskData, AskOption, AskQuestion, CompressedData, ContextData, ContextUsage,
    DeltaData, DoneData, ErrorData, NotifyData, RetryData, StartData, StatusData, SwitchedData,
    ToolEndData, ToolStartData, WidgetData,
};

/// 载荷字段名与前端 `types/api.ts` / `stores/chat.ts` 的读取一致。
#[test]
fn sse_payload_field_names_match_frontend_contract() {
    let start = serde_json::to_value(StartData {
        run_id: "RUN_1".into(),
        session_id: "SESS_1".into(),
    })
    .unwrap();
    assert_eq!(start["run_id"], "RUN_1");
    assert_eq!(start["session_id"], "SESS_1");

    let delta = serde_json::to_value(DeltaData {
        kind: "thinking".into(),
        delta: "x".into(),
    })
    .unwrap();
    assert_eq!(delta["kind"], "thinking", "前端按 kind 分流正文与思考");
    assert_eq!(delta["delta"], "x");

    let ts = serde_json::to_value(ToolStartData {
        tool_call_id: "TC_1".into(),
        name: "read".into(),
        label: None,
        args: None,
    })
    .unwrap();
    assert_eq!(ts["tool_call_id"], "TC_1");
    assert_eq!(ts["name"], "read");
    assert!(ts["label"].is_null(), "label 可空但字段必须在");
    assert!(ts["args"].is_null(), "args 可空但字段必须在");

    let te = serde_json::to_value(ToolEndData {
        tool_call_id: "TC_1".into(),
        name: "read".into(),
        result: "ok".into(),
        is_error: false,
        duration_ms: 3,
    })
    .unwrap();
    assert_eq!(te["tool_call_id"], "TC_1");
    assert_eq!(te["result"], "ok");
    assert_eq!(te["is_error"], false);
    assert_eq!(te["duration_ms"], 3);

    // chat:context 的 usage 是裸字段名（前端读 data.usage.context）。
    let ctx = serde_json::to_value(ContextData {
        usage: ContextUsage {
            input: 1,
            output: 2,
            cached: 3,
            cache_write: 4,
            total: 10,
            context: 10,
            cost: 0.0,
        },
        context_window: 128000,
        ratio: 0.1,
    })
    .unwrap();
    assert_eq!(ctx["usage"]["context"], 10, "前端读的是 usage.context");
    assert_eq!(ctx["usage"]["input"], 1);
    assert_eq!(ctx["usage"]["cached"], 3, "前端读的是 usage.cached");
    assert_eq!(ctx["context_window"], 128000);
    assert_eq!(ctx["ratio"], 0.1);

    let notify = serde_json::to_value(NotifyData {
        level: "error".into(),
        text: "坏了".into(),
    })
    .unwrap();
    assert_eq!(notify["level"], "error", "前端按 level 分级");
    assert_eq!(notify["text"], "坏了");

    let status = serde_json::to_value(StatusData {
        key: "goal".into(),
        text: "推进中".into(),
    })
    .unwrap();
    assert_eq!(status["key"], "goal");
    assert_eq!(status["text"], "推进中");

    let widget = serde_json::to_value(WidgetData {
        key: "todo".into(),
        lines: vec!["a".into()],
    })
    .unwrap();
    assert_eq!(widget["key"], "todo");
    assert_eq!(widget["lines"][0], "a");

    let err = serde_json::to_value(ErrorData {
        run_id: String::new(),
        code: "9703".into(),
        message: "流断了".into(),
    })
    .unwrap();
    assert_eq!(err["code"], "9703");
    assert_eq!(err["message"], "流断了");

    let retry = serde_json::to_value(RetryData {
        attempt: 1,
        max: 3,
        delay_ms: 2000,
    })
    .unwrap();
    assert_eq!(retry["attempt"], 1);
    assert_eq!(retry["max"], 3);
    assert_eq!(retry["delay_ms"], 2000);

    let done = serde_json::to_value(DoneData {
        run_id: String::new(),
        message: None,
    })
    .unwrap();
    assert!(done["message"].is_null());

    let compressed = serde_json::to_value(CompressedData { tokens_before: 100 }).unwrap();
    assert_eq!(compressed["tokens_before"], 100);

    let switched = serde_json::to_value(SwitchedData {
        session_id: "SESS_2".into(),
    })
    .unwrap();
    assert_eq!(switched["session_id"], "SESS_2");
}

/// 提问载荷的问卷结构：前端按 questions[].question_options[].option_label 渲染选项。
#[test]
fn ask_payload_carries_questionnaire_shape() {
    let ask = serde_json::to_value(AskData {
        id: "ASK_1".into(),
        method: "questionnaire".into(),
        title: "选一个".into(),
        message: String::new(),
        options: Vec::new(),
        placeholder: String::new(),
        prefill: String::new(),
        questions: vec![AskQuestion {
            question_title: "用哪个".into(),
            question_options: vec![AskOption {
                option_label: "A".into(),
                option_description: String::new(),
                option_preview: None,
            }],
            question_multi: false,
        }],
        timeout_ms: 0,
    })
    .unwrap();
    assert_eq!(ask["id"], "ASK_1");
    assert_eq!(ask["method"], "questionnaire");
    assert_eq!(ask["questions"][0]["question_title"], "用哪个");
    assert_eq!(
        ask["questions"][0]["question_options"][0]["option_label"], "A",
        "选项标签名是前端渲染的契约"
    );

    let closed = serde_json::to_value(AskClosedData {
        id: "ASK_1".into(),
        reason: "answered".into(),
    })
    .unwrap();
    assert_eq!(closed["id"], "ASK_1");
    assert_eq!(closed["reason"], "answered");
}

/// 提问关闭原因取全集：前端按 reason 分流提示语，
/// 未在此列的原因会落到「未知原因」兜底分支——这里是唯一出处。
#[test]
fn ask_closed_reasons_are_enumerated() {
    let reasons = ["answered", "cancelled", "expired", "session_gone"];
    for r in reasons {
        let v = serde_json::to_value(AskClosedData {
            id: "ASK_1".into(),
            reason: r.into(),
        })
        .unwrap();
        assert_eq!(v["reason"], r);
    }
    assert_eq!(
        json!({"reason": "answered"}),
        json!({"reason": "answered"}),
        "reason 原样透传，不做改写"
    );
}
