//! wb-service 场景测试：功能活状态读取 / 配置读写、内置运行时状态、子代理观测、SSE 载荷契约、
//! 内置运行时（PortableGit + Python）装配。守住两条契约——界面字段对齐与「解压即可用」。

use std::path::PathBuf;
use std::sync::Arc;

use wb_domain::events::{
    AskClosedData, AskData, AskOption, AskQuestion, CompressedData, ContextData, ContextUsage,
    DeltaData, DoneData, ErrorData, NotifyData, RetryData, StartData, StatusData, ToolEndData,
    ToolStartData, WidgetData,
};
use wb_domain::feature_state::MemoryDeleteREQ;
use wb_kernel::SubagentEvent;
use wb_service::Services;

/// 造一个临时数据目录 + 空库 + Services；调用方负责传回目录以便断言。
fn fresh(tag: &str) -> (PathBuf, Arc<Services>) {
    let dir = std::env::temp_dir().join(format!("wb_state_{tag}_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    let repo = Arc::new(wb_repo::Repo::open(&dir.join("settings.db")).unwrap());
    let sv = Services::new(dir.clone(), dir.display().to_string(), repo);
    (dir, sv)
}

/// 造一个 FeatureCtx（与 service 装配时的形态一致，但不依赖 service 内部函数）。
fn build_ctx(sv: &Arc<Services>) -> wb_features::FeatureCtx {
    wb_features::FeatureCtx {
        data_dir: sv.data_dir.clone(),
        workspace: sv.workspace.lock().unwrap().clone().into(),
        emit: Arc::new(|_, _, _| {}),
        configs: Default::default(),
    }
}

// ---- 功能活状态：空态 / 记忆 / 配置 ----

#[test]
fn feature_state_reads_empty_without_error() {
    let (dir, sv) = fresh("empty");
    // 三份文件都不存在：应该是空清单，不是报错——界面要能画出空态
    let todo = wb_service::feature_state::todo_list(&sv).expect("空待办不该报错");
    assert!(todo.resp_items.is_empty(), "got {:?}", todo.resp_items);
    assert_eq!(todo.resp_done_count, 0);

    let goal = wb_service::feature_state::goal_get(&sv).expect("空目标不该报错");
    assert!(goal.resp_goal.is_none(), "没有目标时必须是 None");

    let plan = wb_service::feature_state::plan_get(&sv).expect("空计划不该报错");
    assert!(plan.resp_plan.is_none(), "没有计划时必须是 None");

    let _ = std::fs::remove_dir_all(&dir);
}

#[test]
fn memory_list_and_delete_roundtrip() {
    let (dir, sv) = fresh("memory");
    let ctx = build_ctx(&sv);

    // 直接写库（绕开工具层的密钥扫描，测的是读写链路本身）
    let store = wb_features::memory_store::MemoryStore::open(&ctx).unwrap();
    let (id_a, _) = store.save("project", "项目用 pnpm 管理依赖", 1000).unwrap();
    store.save("user", "用户偏好中文回答", 2000).unwrap();

    let list = wb_service::feature_state::memory_list(&sv).expect("清单读取失败");
    assert_eq!(list.resp_entries.len(), 2, "应读到两条记忆");
    assert_eq!(list.resp_total, 2);
    // 全局按更新时间倒序：后写的排在前面
    assert_eq!(
        list.resp_entries[0].memory_text, "用户偏好中文回答",
        "got {:?}",
        list.resp_entries[0]
    );
    assert_eq!(list.resp_entries[0].memory_updated_at, 2000);
    // 四个类别都报出来，没有的记 0
    assert_eq!(list.resp_counts.len(), 4, "类别数应为 4");
    let project = list
        .resp_counts
        .iter()
        .find(|c| c.kind_name == "project")
        .expect("project 类应在清单里");
    assert_eq!(project.kind_count, 1);

    let req = MemoryDeleteREQ {
        memory_id: id_a.clone(),
    };
    assert!(wb_service::feature_state::memory_delete(&sv, &req.memory_id).unwrap());
    let after = wb_service::feature_state::memory_list(&sv).unwrap();
    assert_eq!(after.resp_entries.len(), 1, "删除后应剩一条");
    assert_eq!(after.resp_total, 1);

    // 再删同一条：7301 记忆条目不存在
    let err = wb_service::feature_state::memory_delete(&sv, &req.memory_id).unwrap_err();
    assert_eq!(err.code, "7301", "重复删除应报 7301");

    let _ = std::fs::remove_dir_all(&dir);
}

#[test]
fn feature_config_save_then_read_back() {
    let (dir, sv) = fresh("config");

    let before = wb_service::feature_config::get(&sv, "web").unwrap();
    assert!(before.resp_configurable, "web 应可配置");
    assert_eq!(before.resp_fields.len(), 3, "web 有三个字段");
    let limit_field = before
        .resp_fields
        .iter()
        .find(|f| f.field_name == "web_search_limit")
        .expect("应有结果条数字段");
    assert_eq!(limit_field.field_value, serde_json::json!(8), "默认 8 条");
    // 搜索源候选里必须有「自动」那一项
    let provider_field = before
        .resp_fields
        .iter()
        .find(|f| f.field_name == "web_provider")
        .expect("应有搜索源字段");
    assert!(
        provider_field
            .field_options
            .iter()
            .any(|o| o.option_value.is_empty()),
        "应有自动选项"
    );
    assert!(provider_field.field_options.len() > 1, "应有具体搜索源候选");

    let mut values = std::collections::BTreeMap::new();
    values.insert("web_search_limit".to_string(), serde_json::json!(99));
    values.insert(
        "web_proxy".to_string(),
        serde_json::json!("http://127.0.0.1:7890"),
    );
    let req = wb_domain::feature::FeatureConfigSaveREQ {
        config_values: values,
    };
    assert!(wb_service::feature_config::save(&sv, "web", &req).is_ok());

    let after = wb_service::feature_config::get(&sv, "web").unwrap();
    let limit = after
        .resp_fields
        .iter()
        .find(|f| f.field_name == "web_search_limit")
        .unwrap();
    // 99 越界，读回时收口到 20；存的仍是用户值，读时守卫
    assert_eq!(
        limit.field_value,
        serde_json::json!(20),
        "越界值应收口到 20"
    );
    let proxy = after
        .resp_fields
        .iter()
        .find(|f| f.field_name == "web_proxy")
        .unwrap();
    assert_eq!(
        proxy.field_value,
        serde_json::json!("http://127.0.0.1:7890")
    );

    // 未知键被拒
    let mut bad = std::collections::BTreeMap::new();
    bad.insert("web_nonsense".to_string(), serde_json::json!(1));
    let rejected = wb_service::feature_config::reject_unknown("web", &bad);
    assert_eq!(rejected, vec!["web_nonsense".to_string()], "未知键应被拒");

    // 不可配的功能返回 configurable=false
    let todo = wb_service::feature_config::get(&sv, "todo").unwrap();
    assert!(!todo.resp_configurable, "todo 没有可配项");
    assert!(todo.resp_fields.is_empty());

    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 内置运行时状态：三件可执行文件逐项报告 ----

#[test]
fn runtime_status_reports_each_binary() {
    let (dir, sv) = fresh("runtime");
    let status = wb_service::runtime::status(&sv.data_dir, &sv.runtime_failures);
    assert_eq!(status.resp_binaries.len(), 3, "应报 bash / git / python");
    let names: Vec<&str> = status
        .resp_binaries
        .iter()
        .map(|b| b.binary_name.as_str())
        .collect();
    assert_eq!(names, vec!["bash", "git", "python"], "got {names:?}");
    // 解压成功时每条都应带绝对路径；失败时 ready=false 且路径为空
    for b in &status.resp_binaries {
        assert_eq!(
            b.binary_ready,
            !b.binary_path.is_empty(),
            "{} 的就绪标志与路径不一致",
            b.binary_name
        );
    }
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 子代理观测：生命周期入表 / 累积 / 摘表 ----

#[test]
fn subagent_observability_tracks_lifecycle() {
    use std::sync::Mutex;
    let board = wb_service::agents::AgentBoard::new();
    let seen: Mutex<Vec<(String, String, serde_json::Value)>> = Mutex::new(Vec::new());
    let emit = |sid: &str, event: &str, data: serde_json::Value| {
        seen.lock()
            .unwrap()
            .push((sid.to_string(), event.to_string(), data));
    };

    board.on_event(
        "SESS_p",
        SubagentEvent::Started {
            session_id: "SUB_1".into(),
            role_code: "scout".into(),
            task: "数文件".into(),
        },
        &emit,
    );
    assert_eq!(board.list().len(), 1, "开跑后应进活跃表");
    assert_eq!(board.count_of("SESS_p"), 1);

    board.on_event(
        "SESS_p",
        SubagentEvent::TextDelta {
            session_id: "SUB_1".into(),
            delta: "结论一半".into(),
        },
        &emit,
    );
    assert_eq!(
        board.transcript("SUB_1").resp_transcript,
        "结论一半",
        "过程正文应逐段累积"
    );

    board.on_event(
        "SESS_p",
        SubagentEvent::Finished {
            session_id: "SUB_1".into(),
            failed: false,
            conclusion: "结论一半".into(),
            usage: wb_provider::Usage::default(),
            duration_ms: 12,
        },
        &emit,
    );
    assert!(board.list().is_empty(), "结束应掊出活跃表");

    let frames = seen.lock().unwrap();
    let names: Vec<&str> = frames.iter().map(|f| f.1.as_str()).collect();
    assert_eq!(
        names,
        ["agent:start", "agent:delta", "agent:done"],
        "事件顺序是界面时序的契约"
    );
    assert_eq!(frames[0].0, "SESS_p", "事件发到父会话通道");
    assert_eq!(frames[0].2["agent_role_code"], "scout");
    assert_eq!(frames[2].2["agent_conclusion"], "结论一半");
    assert_eq!(frames[2].2["agent_failed"], false);
}

// ---- SSE 载荷契约：字段名与前端读取一致 ----

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
        file_path: None,
        diff: None,
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

// ---- 内置运行时装配（慢，#[ignore]） ----

/// 内置运行时：解压一次后 bash / git / python 真实可执行，经注入 PATH 裸名可达，指纹损坏自愈重解压。
///
/// 解压内置 PortableGit / Python 归档并跑真实子进程，磁盘与耗时都重，
/// 默认 `cargo test` 不跑（与 git 真仓库测试同列）。显式：`cargo test -p wb-service -- --ignored`
#[test]
#[ignore = "解压内置 PortableGit/Python 并跑真实子进程，慢"]
fn bundled_runtimes_extracted_executable_and_reachable() {
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

    // 经注入 PATH，bash 里裸 `git` / `python` 命中内置运行时（模型只写工具名就能用）。
    let py_root = wb_domain::runtime::python_root(&dir);
    let out = std::process::Command::new(&sh)
        .args([
            "-c",
            "git --version; python -c 'import sys; print(sys.prefix)'",
        ])
        .current_dir(&dir)
        .env("PATH", wb_domain::runtime::child_path(&dir))
        .output()
        .expect("启动内置 bash 失败");
    let stdout = String::from_utf8_lossy(&out.stdout);
    let stderr = String::from_utf8_lossy(&out.stderr);
    assert!(
        stdout.contains("git version"),
        "裸 git 未命中内置运行时：{stdout}{stderr}"
    );
    // 比路径而不是比关键词：临时目录名里没有「runtime」，靠关键词会假失败。
    let want = py_root.display().to_string().to_lowercase();
    assert!(
        stdout.to_lowercase().contains(&want),
        "裸 python 未命中内置运行时（期望 {want}）：{stdout}{stderr}"
    );

    // 指纹损坏触发重解压自愈。
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
