//! wb-service 场景测试：SSE 载荷契约、内置运行时（PortableGit + Python）装配。

use wb_domain::events::{
    AskClosedData, AskData, AskOption, AskQuestion, CompressedData, ContextData, ContextUsage,
    DeltaData, DoneData, ErrorData, NotifyData, RetryData, StartData, StatusData, ToolEndData,
    ToolStartData, WidgetData,
};

/// 全部 chat:* 载荷的字段名与前端 `types/api.ts` / `stores/chat.ts` 的读取一致；
/// 提问载荷带问卷结构（questions[].question_options[].option_label）；
/// 关闭原因取全集，未列的原因会落到前端「未知原因」兜底分支。
#[test]
fn chat_event_payloads_match_frontend_contract() {
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

    for reason in ["answered", "cancelled", "expired", "session_gone"] {
        let closed = serde_json::to_value(AskClosedData {
            id: "ASK_1".into(),
            reason: reason.into(),
        })
        .unwrap();
        assert_eq!(closed["id"], "ASK_1");
        assert_eq!(closed["reason"], reason, "reason 原样透传，不做改写");
    }
}

// ---- 内置运行时 ----

/// 解压后 bash / git / python 真实可执行；指纹幂等，坏指纹触发重解压自愈。
#[test]
fn bundled_runtimes_extracted_and_executable() {
    let dir = std::env::temp_dir().join(format!("wb-runtime-test-{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    let git_root = wb_domain::runtime::portablegit_root(&dir);

    wb_service::runtime::ensure_all(&dir);
    let sh = wb_domain::runtime::find_bundled_bash(&dir).unwrap_or_else(|| {
        panic!(
            "内置 bash 未解压：{}\n{:?}",
            git_root.display(),
            wb_domain::runtime::bash_candidates(&git_root)
        )
    });
    let git_exe = wb_domain::runtime::find_bundled_git(&dir)
        .unwrap_or_else(|| panic!("内置 git 未解压：{}", git_root.display()));
    let py_exe = wb_domain::runtime::find_bundled_python(&dir).unwrap_or_else(|| {
        panic!(
            "内置 python 未解压：{}",
            wb_domain::runtime::python_root(&dir).display()
        )
    });

    // 真跑一把：$BASH_VERSION 非空证明是 bash 而非 dash；python 打印版本串
    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(async {
        let cancel = tokio_util::sync::CancellationToken::new();
        let out = wb_tools::exec::run(
            &sh.display().to_string(),
            &[
                "-c".into(),
                "if test -n \"$BASH_VERSION\"; then echo ok; else echo not-bash; fi".into(),
            ],
            &dir,
            60,
            &cancel,
        )
        .await
        .unwrap();
        assert_eq!(
            out.stdout.trim(),
            "ok",
            "内置 bash 行为异常：{}",
            out.stdout
        );
        let g = wb_tools::exec::run(
            &git_exe.display().to_string(),
            &["--version".into()],
            &dir,
            60,
            &cancel,
        )
        .await
        .unwrap();
        assert!(
            g.stdout.contains("git version"),
            "内置 git 异常：{}",
            g.stdout
        );
        let p = wb_tools::exec::run(
            &py_exe.display().to_string(),
            &["-c".into(), "import sys; print(sys.version_info[0])".into()],
            &dir,
            60,
            &cancel,
        )
        .await
        .unwrap();
        assert_eq!(
            p.stdout.trim(),
            "3",
            "内置 python 异常：{}{}",
            p.stdout,
            p.stderr
        );
    });

    let marker = wb_domain::runtime::portablegit_root(&dir).join(".pack_stamp");
    let stamp = std::fs::read_to_string(&marker).unwrap();
    assert_eq!(
        stamp,
        wb_service::runtime::PORTABLEGIT_ZIP.len().to_string()
    );
    std::fs::write(&marker, "0").unwrap();
    wb_service::runtime::ensure_all(&dir);
    assert_eq!(
        std::fs::read_to_string(&marker).unwrap(),
        stamp,
        "坏指纹应触发重解压并修复"
    );
    let _ = std::fs::remove_dir_all(&dir);
}
