//! wb-service 功能面板写操作：界面能勾、能批、能删，与模型工具走同一份落盘。

use std::path::PathBuf;
use std::sync::Arc;

use wb_domain::feature_state::{GoalMutateREQ, MemorySaveREQ, PlanMutateREQ, TodoMutateREQ};
use wb_service::Services;

/// 造一个临时数据目录 + 空库 + Services；调用方负责传回目录以便断言。
fn fresh(tag: &str) -> (PathBuf, Arc<Services>) {
    let dir = std::env::temp_dir().join(format!("wb_fw_{tag}_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    let repo = Arc::new(wb_repo::Repo::open(&dir.join("settings.db")).unwrap());
    let sv = Services::new(dir.clone(), dir.display().to_string(), repo);
    (dir, sv)
}

/// 待办写入请求的简写。
fn todo_req(op: &str, id: &str, text: &str, status: &str) -> TodoMutateREQ {
    TodoMutateREQ {
        todo_op: op.into(),
        todo_id: id.into(),
        todo_text: text.into(),
        todo_status: status.into(),
        todo_blocked_by: Vec::new(),
    }
}

/// 目标写入请求的简写。
fn goal_req(op: &str, text: &str, steps: &[&str]) -> GoalMutateREQ {
    GoalMutateREQ {
        goal_op: op.into(),
        goal_text: text.into(),
        goal_steps: steps.iter().map(|s| s.to_string()).collect(),
        goal_status: String::new(),
        goal_done_steps: Vec::new(),
        goal_note: String::new(),
    }
}

/// 计划写入请求的简写。
fn plan_req(op: &str, step_index: i64) -> PlanMutateREQ {
    PlanMutateREQ {
        plan_op: op.into(),
        plan_reject_note: String::new(),
        plan_step_index: step_index,
        annotation_scope: "plan".into(),
        annotation_step: 0,
        annotation_text: String::new(),
    }
}

// ---- 待办：界面增删改与工具同一份落盘 ----

#[test]
fn todo_panel_writes_reach_the_same_store() {
    let (dir, sv) = fresh("todo");
    // 界面新增两条
    let list =
        wb_service::feature_write::todo_mutate(&sv, &todo_req("add", "", "第一步", "")).unwrap();
    assert_eq!(list.resp_items.len(), 1, "新增后清单一条");
    wb_service::feature_write::todo_mutate(&sv, &todo_req("add", "", "第二步", "")).unwrap();

    // 按序号勾选（界面拿到的是 todo_seq，不是 id）
    let list =
        wb_service::feature_write::todo_mutate(&sv, &todo_req("status", "1", "", "completed"))
            .unwrap();
    assert_eq!(list.resp_done_count, 1, "勾选后完成数 1");

    // 工具读到的必须是同一份
    let from_read = wb_service::feature_state::todo_list(&sv).unwrap();
    assert_eq!(from_read.resp_done_count, 1, "界面写入要能被读端看到");

    // 删除后清单回落
    let list =
        wb_service::feature_write::todo_mutate(&sv, &todo_req("remove", "2", "", "")).unwrap();
    assert_eq!(list.resp_items.len(), 1, "删掉一条");
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 目标：写操作受状态机约束，勾完自动达成 ----

#[test]
fn goal_panel_respects_state_machine() {
    let (dir, sv) = fresh("goal");
    let g = wb_service::feature_write::goal_mutate(
        &sv,
        &goal_req("create", "把报表做完", &["取数", "出图"]),
    )
    .unwrap()
    .resp_goal
    .expect("建完应有目标");
    assert_eq!(g.goal_status, "active");

    // 活跃目标存在时不能再建一个：这是状态机的不变量，界面写入也不能绕过
    let again = wb_service::feature_write::goal_mutate(&sv, &goal_req("create", "第二个", &[]));
    assert_eq!(
        again.unwrap_err().code,
        wb_domain::error::tool::ARG_INVALID,
        "已有未完结目标时应拒绝新建"
    );

    // 勾完全部步骤 → 自动 achieved（否则续跑会一直触发）
    let mut done = goal_req("done_steps", "", &[]);
    done.goal_done_steps = vec![1, 2];
    let g = wb_service::feature_write::goal_mutate(&sv, &done)
        .unwrap()
        .resp_goal
        .unwrap();
    assert_eq!(g.goal_status, "achieved", "全勾完应自动收口");
    assert_eq!(g.goal_done_steps, 2);

    // 达成后才允许建新目标
    let next = wb_service::feature_write::goal_mutate(&sv, &goal_req("create", "新目标", &[]));
    assert!(next.is_ok(), "已达成后可建新目标");
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 计划：批准 / 勾步骤 / 标注，勾完自动 done ----

#[test]
fn plan_panel_approves_steps_and_annotates() {
    let (dir, sv) = fresh("plan");
    // 计划由模型起草：直接写盘模拟一次提交
    let ctx = wb_service::features::feature_ctx_for_test(&sv);
    let store = wb_features::plan_types::PlanStore::new(&ctx);
    store
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

    // 批准
    let p = wb_service::feature_write::plan_mutate(&sv, &plan_req("approve", 0))
        .unwrap()
        .resp_plan
        .unwrap();
    assert_eq!(p.plan_status, "approved");

    // 勾完最后一个步骤 → 自动 done（否则界面一直显示「执行中」）
    wb_service::feature_write::plan_mutate(&sv, &plan_req("step_done", 1)).unwrap();
    let p = wb_service::feature_write::plan_mutate(&sv, &plan_req("step_done", 2))
        .unwrap()
        .resp_plan
        .unwrap();
    assert_eq!(p.plan_status, "done", "全勾完应自动完成");

    // 标注带 scope 与步骤号
    let mut ann = plan_req("annotate", 0);
    ann.annotation_scope = "step".into();
    ann.annotation_step = 1;
    ann.annotation_text = "要加时间预估".into();
    let p = wb_service::feature_write::plan_mutate(&sv, &ann)
        .unwrap()
        .resp_plan
        .unwrap();
    assert_eq!(p.plan_annotations.len(), 1);
    assert_eq!(p.plan_annotations[0].annotation_scope, "step");
    assert_eq!(p.plan_annotations[0].annotation_step, 1);

    // 已完成的计划不能再驳回
    let mut reject = plan_req("reject", 0);
    reject.plan_reject_note = "不行".into();
    assert!(
        wb_service::feature_write::plan_mutate(&sv, &reject).is_err(),
        "done 之后不该还能驳回"
    );
    let _ = std::fs::remove_dir_all(&dir);
}

// ---- 记忆：界面写入也要过密钥扫描 ----

#[test]
fn memory_panel_save_scans_for_secrets() {
    let (dir, sv) = fresh("memsave");
    let ok = MemorySaveREQ {
        memory_kind: "user".into(),
        memory_text: "用户偏好中文回答".into(),
    };
    let id = wb_service::feature_write::memory_save(&sv, &ok).unwrap();
    assert!(id.starts_with("MEM_"), "记忆 id 带业务前缀：{id}");

    // 疑似密钥必须被拒：界面写入不能成为绕过扫描的后门
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

    // 非法类别直接拒
    let bad_kind = MemorySaveREQ {
        memory_kind: "nonsense".into(),
        memory_text: "x".into(),
    };
    assert!(wb_service::feature_write::memory_save(&sv, &bad_kind).is_err());
    let _ = std::fs::remove_dir_all(&dir);
}
