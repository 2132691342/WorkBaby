//! wb-service 场景测试的共享夹具：临时数据目录、空的 Services 与功能写入请求简写。

#![allow(dead_code)]

use std::path::PathBuf;
use std::sync::Arc;

use wb_domain::feature_state::{GoalMutateREQ, PlanMutateREQ, TodoMutateREQ};

/// 临时数据目录 + 空库 + Services；调用方负责传回目录以便断言。
pub fn fresh(tag: &str) -> (PathBuf, Arc<wb_service::Services>) {
    let dir = std::env::temp_dir().join(format!("wb_state_{tag}_{}", std::process::id()));
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir).unwrap();
    let repo = Arc::new(wb_repo::Repo::open(&dir.join("settings.db")).unwrap());
    let sv = wb_service::Services::new(dir.clone(), dir.display().to_string(), repo);
    (dir, sv)
}

pub fn todo_req(op: &str, id: &str, text: &str, status: &str) -> TodoMutateREQ {
    TodoMutateREQ {
        todo_op: op.into(),
        todo_id: id.into(),
        todo_text: text.into(),
        todo_status: status.into(),
        todo_blocked_by: Vec::new(),
    }
}

pub fn goal_req(op: &str, text: &str, steps: &[&str]) -> GoalMutateREQ {
    GoalMutateREQ {
        goal_op: op.into(),
        goal_text: text.into(),
        goal_steps: steps.iter().map(|s| s.to_string()).collect(),
        goal_status: String::new(),
        goal_done_steps: Vec::new(),
        goal_note: String::new(),
    }
}

pub fn plan_req(op: &str, step_index: i64) -> PlanMutateREQ {
    PlanMutateREQ {
        plan_op: op.into(),
        plan_reject_note: String::new(),
        plan_step_index: step_index,
        annotation_scope: "plan".into(),
        annotation_step: 0,
        annotation_text: String::new(),
    }
}
