//! wb-service 场景测试：功能活状态与配置、面板写操作、工作区热切换、子代理观测。
//!
//! 守三类东西：长链路（界面写入与模型工具同一份落盘）、状态机（目标 / 计划受约束）、
//! 装配（工作区切换与运行时状态）。SSE 载荷契约在 `events_test.rs`。
//! 内置运行时解压跑真实子进程，标 #[ignore]。

mod common;

use std::collections::BTreeMap;
use std::sync::Mutex;

use common::*;
use wb_domain::feature::FeatureConfigSaveREQ;
use wb_domain::feature_state::MemorySaveREQ;
use wb_kernel::SubagentEvent;

// ---- 记忆：清单 / 删除 / 密钥扫描 ----

/// 记忆读端与写端共用一份库：清单按更新时间倒序并报四类计数，
/// 重复删除报 7301，疑似密钥被拒（界面写入不能成为绕过扫描的后门）。
#[test]
fn memory_state_and_panel_writes_share_one_store() {
    let (dir, sv) = fresh("memory");

    let empty = wb_service::feature_state::memory_list(&sv).expect("空库不该报错");
    assert_eq!(empty.resp_total, 0);
    assert_eq!(empty.resp_counts.len(), 4, "四类计数都要报出来");

    let ok = MemorySaveREQ {
        memory_kind: "user".into(),
        memory_text: "用户偏好中文回答".into(),
    };
    let id = wb_service::feature_write::memory_save(&sv, &ok).unwrap();
    assert!(id.starts_with("MEM_"), "记忆 id 带业务前缀：{id}");

    // 直接写库（绕开工具层的密钥扫描）测的是读链路本身。
    let ctx = wb_service::features::feature_ctx_for_test(&sv);
    wb_features::memory_store::MemoryStore::open(&ctx)
        .unwrap()
        .save("project", "项目用 pnpm 管理依赖", 1000)
        .unwrap();

    let list = wb_service::feature_state::memory_list(&sv).expect("清单读取失败");
    assert_eq!(list.resp_total, 2);
    assert_eq!(
        list.resp_entries[0].memory_text, "用户偏好中文回答",
        "全局按更新时间倒序，后写的在前"
    );
    let project = list
        .resp_counts
        .iter()
        .find(|c| c.kind_name == "project")
        .expect("project 类应在清单里");
    assert_eq!(project.kind_count, 1);

    assert!(wb_service::feature_state::memory_delete(&sv, &id).unwrap());
    let err = wb_service::feature_state::memory_delete(&sv, &id).unwrap_err();
    assert_eq!(err.code, "7301", "重复删除应报 7301");

    let secret = MemorySaveREQ {
        memory_kind: "user".into(),
        memory_text: "key: sk-abcdef1234567890xyz".into(),
    };
    let err = wb_service::feature_write::memory_save(&sv, &secret).unwrap_err();
    assert_eq!(err.code, wb_domain::error::common::PARAM_INVALID);
    assert!(
        err.message.contains("密钥"),
        "错误要说清是密钥：{}",
        err.message
    );

    let bad_kind = MemorySaveREQ {
        memory_kind: "nonsense".into(),
        memory_text: "x".into(),
    };
    assert!(wb_service::feature_write::memory_save(&sv, &bad_kind).is_err());
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 功能配置：字段表 / 越界收口 / 未知键拒绝 ----

#[test]
fn feature_config_builds_field_table_and_guards_values() {
    let (dir, sv) = fresh("config");

    let before = wb_service::feature_config::get(&sv, "web").unwrap();
    assert!(before.resp_configurable, "web 应可配置");
    let limit_field = before
        .resp_fields
        .iter()
        .find(|f| f.field_name == "web_search_limit")
        .expect("应有结果条数字段");
    assert_eq!(limit_field.field_value, serde_json::json!(8), "默认 8 条");
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
        "搜索源候选里必须有「自动」那一项"
    );

    let mut values = BTreeMap::new();
    values.insert("web_search_limit".to_string(), serde_json::json!(99));
    values.insert(
        "web_proxy".to_string(),
        serde_json::json!("http://127.0.0.1:7890"),
    );
    wb_service::feature_config::save(
        &sv,
        "web",
        &FeatureConfigSaveREQ {
            config_values: values,
        },
    )
    .unwrap();

    let after = wb_service::feature_config::get(&sv, "web").unwrap();
    let limit = after
        .resp_fields
        .iter()
        .find(|f| f.field_name == "web_search_limit")
        .unwrap();
    // 存的仍是用户值，读时守卫收口到 20
    assert_eq!(
        limit.field_value,
        serde_json::json!(20),
        "越界值应在读时收口"
    );

    let mut bad = BTreeMap::new();
    bad.insert("web_nonsense".to_string(), serde_json::json!(1));
    assert_eq!(
        wb_service::feature_config::reject_unknown("web", &bad),
        vec!["web_nonsense".to_string()],
        "未知键应被拒"
    );

    let todo = wb_service::feature_config::get(&sv, "todo").unwrap();
    assert!(!todo.resp_configurable, "todo 没有可配项");
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 工作区热切换 ----

/// 换目录即重扫作用域状态并写回设置：不写回则重启后回到旧目录，
/// 不重建待办存储则旧目录的待办会出现在新工作区。
#[test]
fn workspace_switch_rescans_and_persists() {
    let (dir, sv) = fresh("workspace");
    let target = dir.join("proj");
    std::fs::create_dir_all(&target).unwrap();

    let sw = sv.switch_workspace(&target.display().to_string()).unwrap();
    assert_eq!(sw.to, wb_pkg::realpath::real_path(&target).unwrap());
    assert_eq!(
        sv.repo.settings_get("workspace").unwrap().as_deref(),
        Some(sw.to.display().to_string().as_str()),
        "切换必须写回设置"
    );
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

    let again = sv.switch_workspace(&target.display().to_string()).unwrap();
    assert_eq!(again.from, again.to, "同一目录重复切换应幂等");

    let bad = sv.switch_workspace(&dir.join("nope").display().to_string());
    assert_eq!(
        bad.unwrap_err().code,
        wb_domain::error::common::PATH_MISSING
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 子代理观测 ----

/// 生命周期入表 / 累积 / 摘表，且三帧事件的顺序与载荷是界面时序的契约。
#[test]
fn subagent_observability_tracks_lifecycle() {
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
    assert!(board.list().is_empty(), "结束应摘出活跃表");

    let frames = seen.lock().unwrap();
    let names: Vec<&str> = frames.iter().map(|f| f.1.as_str()).collect();
    assert_eq!(names, ["agent:start", "agent:delta", "agent:done"]);
    assert_eq!(frames[0].0, "SESS_p", "事件发到父会话通道");
    assert_eq!(frames[0].2["agent_role_code"], "scout");
    assert_eq!(frames[2].2["agent_conclusion"], "结论一半");
    assert_eq!(frames[2].2["agent_failed"], false);
}

// ---- 面板写操作：与模型工具同一份落盘、状态机在写入侧强制 ----

/// 三块面板的写入都走与模型工具同一份落盘，且状态机的不变量在写入端强制：
/// 待办按序号勾选立刻能被读端看到；已有未完结目标时拒新建、全勾完自动 achieved；
/// 计划全勾完自动 done、完成之后不得再驳回。
#[test]
fn feature_panels_share_one_store_and_enforce_state_machines() {
    let (dir, sv) = fresh("panels");

    // ---- 待办 ----
    let list =
        wb_service::feature_write::todo_mutate(&sv, &todo_req("add", "", "第一步", "")).unwrap();
    assert_eq!(list.resp_items.len(), 1, "新增后清单一条");
    wb_service::feature_write::todo_mutate(&sv, &todo_req("add", "", "第二步", "")).unwrap();
    wb_service::feature_write::todo_mutate(&sv, &todo_req("status", "1", "", "completed")).unwrap();
    let from_read = wb_service::feature_state::todo_list(&sv).unwrap();
    assert_eq!(from_read.resp_done_count, 1, "界面写入要能被读端看到");
    let list =
        wb_service::feature_write::todo_mutate(&sv, &todo_req("remove", "2", "", "")).unwrap();
    assert_eq!(list.resp_items.len(), 1, "删掉一条");

    // ---- 目标 ----
    assert!(
        wb_service::feature_state::goal_get(&sv)
            .unwrap()
            .resp_goal
            .is_none(),
        "没有目标时必须是 None"
    );
    let g = wb_service::feature_write::goal_mutate(
        &sv,
        &goal_req("create", "把报表做完", &["取数", "出图"]),
        "",
    )
    .unwrap()
    .resp_goal
    .expect("建完应有目标");
    assert_eq!(g.goal_status, "active");
    assert_eq!(
        wb_service::feature_write::goal_mutate(&sv, &goal_req("create", "第二个", &[]), "")
            .unwrap_err()
            .code,
        wb_domain::error::tool::ARG_INVALID,
        "已有未完结目标时应拒绝新建"
    );
    let mut done = goal_req("done_steps", "", &[]);
    done.goal_done_steps = vec![1, 2];
    let g = wb_service::feature_write::goal_mutate(&sv, &done, "")
        .unwrap()
        .resp_goal
        .unwrap();
    assert_eq!(g.goal_status, "achieved", "全勾完应自动收口");
    assert_eq!(g.goal_done_steps, 2);
    assert!(
        wb_service::feature_write::goal_mutate(&sv, &goal_req("create", "新目标", &[]), "").is_ok(),
        "已达成后可建新目标"
    );

    // ---- 计划 ----
    let ctx = wb_service::features::feature_ctx_for_test(&sv);
    wb_features::plan_types::PlanStore::new(&ctx)
        .mutate(|slot| {
            *slot = Some(wb_features::plan_types::PlanState {
                plan_id: "PLAN_t".into(),
                plan_title: "试点".into(),
                plan_status: "draft".into(),
                plan_reject_note: String::new(),
                plan_steps: vec![
                    wb_features::plan_types::PlanStep {
                        step_text: "读需求".into(),
                        step_done: false,
                    },
                    wb_features::plan_types::PlanStep {
                        step_text: "出方案".into(),
                        step_done: false,
                    },
                ],
                plan_annotations: Vec::new(),
                plan_updated_at: 1,
            });
            Ok(())
        })
        .unwrap();
    assert_eq!(
        wb_service::feature_write::plan_mutate(&sv, &plan_req("approve", 0), "")
            .unwrap()
            .resp_plan
            .unwrap()
            .plan_status,
        "approved"
    );
    wb_service::feature_write::plan_mutate(&sv, &plan_req("step_done", 1), "").unwrap();
    let p = wb_service::feature_write::plan_mutate(&sv, &plan_req("step_done", 2), "")
        .unwrap()
        .resp_plan
        .unwrap();
    assert_eq!(p.plan_status, "done", "全勾完应自动完成");

    let mut ann = plan_req("annotate", 0);
    ann.annotation_scope = "step".into();
    ann.annotation_step = 1;
    ann.annotation_text = "要加时间预估".into();
    let p = wb_service::feature_write::plan_mutate(&sv, &ann, "")
        .unwrap()
        .resp_plan
        .unwrap();
    assert_eq!(p.plan_annotations[0].annotation_scope, "step");
    assert_eq!(p.plan_annotations[0].annotation_step, 1);

    let mut reject = plan_req("reject", 0);
    reject.plan_reject_note = "不行".into();
    assert!(
        wb_service::feature_write::plan_mutate(&sv, &reject, "").is_err(),
        "done 之后不该还能驳回"
    );
    let _ = std::fs::remove_dir_all(&dir);
}
// ---- 斜杠命令清单 ----

/// 内置命令清单必须与前端输入区的命令表一致：两边各写一份，不校验就会漂。
/// 漂了的表现是「菜单里点得到，接口里查不到」或反过来。
#[test]
fn builtin_slash_commands_match_frontend_command_table() {
    let (dir, sv) = fresh("commands");
    let list = wb_service::misc::commands_list(&sv);

    let builtin: Vec<&str> = list
        .iter()
        .filter(|c| c.command_source == "builtin")
        .map(|c| c.command_name.as_str())
        .collect();
    assert_eq!(
        builtin,
        vec![
            "new", "model", "thinking", "compact", "session", "export", "copy", "reload", "login",
            "settings"
        ],
        "内置命令清单要与前端 composerMenu.ts 的 COMMANDS 一致"
    );

    // 技能命令仍挂在后面，前缀区分来源
    for c in &list {
        if c.command_source == "skill" {
            assert!(
                c.command_name.starts_with("skill:"),
                "技能命令要带 skill: 前缀，实际 {}",
                c.command_name
            );
        }
    }
    let _ = std::fs::remove_dir_all(&dir);
}

/// 工具定义占多少上下文：工具按需加载值不值得做，取决于这个数。
/// 全部功能开启时工具定义要占掉几千 token，所以延迟加载有实际收益。
/// 守住上限：新增工具时若定义写得过长，这里会先报出来。
#[test]
fn tool_schema_overhead_stays_bounded() {
    let (dir, sv) = fresh("toolcost");
    for code in [
        "todo",
        "ask_user",
        "fff",
        "context_view",
        "memory",
        "web",
        "loadout",
        "context_mode",
        "lsp",
        "plan",
        "goal",
        "inspect",
    ] {
        sv.repo.feature_set_enabled(code, true).unwrap();
    }
    let reg = wb_service::features::build_registry(&sv);
    let all = reg.all();
    let mut total = 0i64;
    for t in &all {
        total += wb_kernel::tokenest::estimate_text_tokens(t.description());
        total += wb_kernel::tokenest::estimate_text_tokens(&t.parameters().to_string());
    }
    println!("全开：工具数 {}，定义约 {} token", all.len(), total);
    assert!(
        total < 12_000,
        "工具定义合计 {total} token 偏大：说明某个工具的描述写太长，或新增了太多工具"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 安全闸门 ----

/// 危险信号必须被认出来：递归删除、全局安装、force push、动注册表、改系统路径。
/// 漏一个就是闸门形同虚设——而用户以为它拦住了。
#[test]
fn approval_gate_recognizes_dangerous_operations() {
    use serde_json::json;
    use wb_service::approval::needs_approval;

    let dangerous: Vec<(&str, &str)> = vec![
        ("rm -rf /", "递归删除"),
        ("rm -rf node_modules", "递归删除"),
        (r"RD /S /Q C:\Windows", "递归删除"),
        ("pip install requests", "全局安装"),
        ("npm install -g typescript", "全局安装"),
        ("cargo install ripgrep", "全局安装"),
        ("git push --force origin main", "不可逆 git"),
        ("git reset --hard HEAD~5", "不可逆 git"),
        ("git clean -fd", "不可逆 git"),
        (r"reg add HKLM\Software\Test", "系统级变更"),
        ("sc delete MyService", "系统级变更"),
        ("format C:", "磁盘级操作"),
        ("diskpart /s script.txt", "磁盘级操作"),
        ("chmod 777 /etc", "权限变更"),
        (r"icacls C:\Windows /grant Everyone:F", "权限变更"),
    ];
    for (cmd, why) in dangerous {
        let got = needs_approval("bash", &json!({ "command": cmd }));
        assert!(got.is_some(), "危险命令没被拦住：{cmd}（{why}）");
    }

    // 日常命令不该被拦：拦住它们只会让助手变得难用
    for cmd in [
        "ls -la",
        "cat README.md",
        "git status",
        "npm run build",
        "cargo test",
        "python script.py",
        "grep -r TODO src",
    ] {
        assert!(
            needs_approval("bash", &json!({ "command": cmd })).is_none(),
            "日常命令不该被拦：{cmd}"
        );
    }

    // 改系统路径也要拦
    assert!(
        needs_approval(
            "write",
            &json!({ "path": r"C:\Windows\System32\drivers\etc\hosts" })
        )
        .is_some(),
        "改系统路径该被拦"
    );
    assert!(
        needs_approval("write", &json!({ "path": "src/main.rs" })).is_none(),
        "改工作区文件不该被拦"
    );
}

/// 审批模式解析：未知值按「危险时才问」处理——宁可多问一次。
#[test]
fn approval_mode_falls_back_to_ask() {
    use wb_service::approval::ApprovalMode;
    assert_eq!(ApprovalMode::parse("off"), ApprovalMode::Off);
    assert_eq!(ApprovalMode::parse("all"), ApprovalMode::All);
    assert_eq!(ApprovalMode::parse("ask"), ApprovalMode::Ask);
    assert_eq!(ApprovalMode::parse(""), ApprovalMode::Ask);
    assert_eq!(ApprovalMode::parse("乱填"), ApprovalMode::Ask);
    assert_eq!(ApprovalMode::Ask.code(), "ask");
}

// ---- 内置运行时状态与预设 ----

/// 就绪标志与路径必须一致：报了 ready 却没有路径，工具就会拿空路径去起进程。
#[test]
fn runtime_status_reports_each_binary() {
    let (dir, sv) = fresh("runtime");
    let status = wb_service::runtime::status(&sv.data_dir, &sv.runtime);
    assert_eq!(status.resp_total_count, 2, "PortableGit + Python 两件");
    let names: Vec<&str> = status
        .resp_binaries
        .iter()
        .map(|b| b.binary_name.as_str())
        .collect();
    // powershell 走「用户装的 pwsh 7 → 系统 5.1」级联，不参与解压进度，
    // 但同样要报出可达性
    assert_eq!(
        names,
        vec!["bash", "git", "python", "powershell"],
        "got {names:?}"
    );
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

/// 预设添加后必须两件事都成立：配置里有它，且默认模型指到它。
#[test]
fn provider_preset_applies_and_claims_default_once() {
    let (dir, sv) = fresh("preset");
    let before = wb_service::providers::presets_vo(&sv).unwrap();
    assert!(before.iter().any(|p| p.preset_id == "deepseek"));
    assert!(
        before.iter().all(|p| !p.preset_installed),
        "新库不该有已装预设"
    );

    let pid = wb_service::providers::apply_preset(&sv, "deepseek", Some("sk-test")).unwrap();
    assert_eq!(pid, "deepseek");
    let dp = sv.repo.settings_get("default_provider").unwrap();
    assert_eq!(dp.as_deref(), Some("deepseek"), "首次添加应自动占位默认");
    let auth = sv.providers.load_auth().unwrap();
    assert!(
        auth.providers.contains_key("deepseek"),
        "给了密钥就该写进 auth.json"
    );

    // 第二次添加同名预设必须被拒：覆盖会冲掉用户改过的地址与追加的模型
    let again = wb_service::providers::apply_preset(&sv, "deepseek", None);
    assert!(again.is_err(), "重复添加应当报错");

    // 已有默认时，新预设不能抢走它
    wb_service::providers::apply_preset(&sv, "ollama", None).unwrap();
    let dp = sv.repo.settings_get("default_provider").unwrap();
    assert_eq!(
        dp.as_deref(),
        Some("deepseek"),
        "用户已选的默认不该被后加的预设抢走"
    );

    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 工具按需加载与 schema 开销 ----

/// 延迟加载：请求里只留 tool_search，其余工具等模型搜到才进来。
/// 不开这个开关时行为不变——工具表固定，少一轮往返。
#[test]
fn deferred_tools_split_keeps_only_the_search_entry_in_request() {
    let (dir, sv) = fresh("defer");
    sv.repo.feature_set_enabled("web", true).unwrap();
    // 开关变了要重建注册表：工具表是装配边界，不是运行时判断
    wb_service::features::refresh_registry(&sv);

    // 默认关：全部工具直接进请求
    let (direct, deferred) = wb_service::tool_sets(&sv);
    assert!(deferred.is_empty(), "默认不开延迟加载：{deferred:?}");
    assert!(
        direct.iter().all(|n| n != "tool_search"),
        "tool_search 不该在固定工具表里：{direct:?}"
    );
    assert!(direct.contains(&"web_search".to_string()), "web 要在");

    // 开启：只留 tool_search，其余转延迟
    sv.repo.settings_set("defer_tools", "true").unwrap();
    let (direct, deferred) = wb_service::tool_sets(&sv);
    assert_eq!(direct, vec!["tool_search".to_string()], "只留入口");
    assert!(
        deferred.contains(&"web_search".to_string()),
        "web_search 转延迟：{deferred:?}"
    );
    assert!(
        !deferred.contains(&"tool_search".to_string()),
        "入口不能同时在延迟列表里"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

/// 检索排序：查「网页」要能命中 web_search，查「改文件」要能命中 edit。
#[test]
fn tool_search_ranks_by_relevance() {
    use wb_features::tool_search::{rank, ToolDocument};

    let docs = vec![
        ToolDocument {
            name: "web_search".into(),
            text: "联网搜索，返回编号的结果清单".into(),
        },
        ToolDocument {
            name: "edit".into(),
            text: "精确替换文件内容".into(),
        },
        ToolDocument {
            name: "bash".into(),
            text: "执行命令".into(),
        },
    ];
    let hits = rank("查网页", &docs, 3);
    assert!(!hits.is_empty(), "中文查询要能命中");
    assert_eq!(hits[0].name, "web_search", "最相关的排第一：{hits:?}");

    let hits = rank("edit file", &docs, 3);
    assert_eq!(hits[0].name, "edit", "英文查询按词干匹配：{hits:?}");

    let hits = rank("zzzzz", &docs, 3);
    assert!(hits.is_empty(), "无匹配时返回空：{hits:?}");
}
