//! wb-service 场景测试：功能活状态与配置读写、内置运行时状态、工作区热切换、子代理观测。

use std::path::PathBuf;
use std::sync::Arc;

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
        workspace: sv.workspace_dir(),
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
    assert_eq!(
        before.resp_fields.len(),
        4,
        "web 有四个字段（含搜索源密钥）"
    );
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
    let status = wb_service::runtime::status(&sv.data_dir, &sv.runtime);
    assert_eq!(status.resp_total_count, 2, "PortableGit + Python 两件");
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

// ---- 工作区热切换：换目录即重扫作用域状态并写回设置 ----

#[test]
fn workspace_switch_rescans_and_persists() {
    let (dir, sv) = fresh("workspace");
    let target = dir.join("proj");
    std::fs::create_dir_all(&target).unwrap();

    let sw = sv.switch_workspace(&target.display().to_string()).unwrap();
    assert_eq!(sw.to, wb_pkg::realpath::real_path(&target).unwrap());
    assert_eq!(sv.workspace_text(), sw.to.display().to_string());
    assert_eq!(
        sv.repo.settings_get("workspace").unwrap().as_deref(),
        Some(sw.to.display().to_string().as_str()),
        "切换必须写回设置，否则重启后回到旧目录"
    );
    // 待办存储跟着换目录：旧目录待办不能出现在新工作区
    let todo = sv.todos().expect("切换后应重建待办存储");
    let encoded = sw
        .to
        .display()
        .to_string()
        .replace([char::from(92), '/', ':'], "-");
    assert!(
        todo.path().to_string_lossy().contains(&encoded),
        "待办存储未按新工作区隔离：{:?}",
        todo.path()
    );

    // 同一目录重复切换是幂等的
    let again = sv.switch_workspace(&target.display().to_string()).unwrap();
    assert_eq!(again.from, again.to);

    // 不存在的目录直接拒
    let bad = sv.switch_workspace(&dir.join("nope").display().to_string());
    assert_eq!(
        bad.unwrap_err().code,
        wb_domain::error::common::PATH_MISSING
    );
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
