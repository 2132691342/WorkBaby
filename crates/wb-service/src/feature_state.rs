//! 功能活状态读取：把 wb-features 落盘的 todo / goal / plan / memory 读成 VO。
//! 界面只读这里，不自己解析 widget 文本，也不再各自猜文件位置。

use std::sync::Arc;

use wb_domain::feature_state::{
    GoalRESP, GoalStepVO, GoalVO, MemoryEntryVO, MemoryKindCountVO, MemoryListRESP,
    PlanAnnotationVO, PlanRESP, PlanStepVO, PlanVO, TodoItemVO, TodoListRESP,
};
use wb_pkg::{AppError, Result};

use super::features::feature_ctx;
use super::Services;

/// 记忆四类（界面按这个顺序分组，缺类的显示为 0）。
pub const MEMORY_KINDS: [&str; 4] = ["memory", "user", "project", "failure"];

/// 单次读取记忆清单的上限（防止把整库一次拉给界面）。
const MEMORY_LIST_LIMIT: usize = 500;

/// 读待办清单（功能关掉时返回空清单，不算错误）。
pub fn todo_list(sv: &Arc<Services>) -> Result<TodoListRESP> {
    let ctx = feature_ctx(sv);
    let items = wb_features::todo::TodoStore::new(&ctx).load();
    let resp_done_count = items
        .iter()
        .filter(|t| t.todo_status == "completed")
        .count();
    Ok(TodoListRESP {
        resp_items: items
            .into_iter()
            .map(|t| TodoItemVO {
                todo_id: t.todo_id,
                todo_seq: t.todo_seq,
                todo_text: t.todo_text,
                todo_status: t.todo_status,
                todo_blocked_by: t.todo_blocked_by,
                todo_created_at: t.todo_created_at,
                todo_updated_at: t.todo_updated_at,
            })
            .collect(),
        resp_done_count,
    })
}

/// 读当前目标。
pub fn goal_get(sv: &Arc<Services>) -> Result<GoalRESP> {
    let ctx = feature_ctx(sv);
    let goal = wb_features::goal::GoalStore::new(&ctx)
        .load()
        .map(|g| GoalVO {
            goal_done_steps: g.goal_steps.iter().filter(|s| s.step_done).count(),
            goal_session_id: g.goal_session_id,
            goal_id: g.goal_id,
            goal_text: g.goal_text,
            goal_status: g.goal_status,
            goal_note: g.goal_note,
            goal_steps: g
                .goal_steps
                .into_iter()
                .map(|s| GoalStepVO {
                    step_text: s.step_text,
                    step_done: s.step_done,
                })
                .collect(),
            goal_created_at: g.goal_created_at,
            goal_updated_at: g.goal_updated_at,
        });
    let resume = sv
        .goal_resumes
        .lock()
        .unwrap()
        .values()
        .copied()
        .max()
        .unwrap_or(0);
    Ok(GoalRESP {
        resp_goal: goal,
        resp_resume_count: resume,
    })
}

/// 读当前计划。
pub fn plan_get(sv: &Arc<Services>) -> Result<PlanRESP> {
    let ctx = feature_ctx(sv);
    let plan = wb_features::plan_types::PlanStore::new(&ctx)
        .load()
        .map(|p| PlanVO {
            plan_done_steps: p.plan_steps.iter().filter(|s| s.step_done).count(),
            plan_id: p.plan_id,
            plan_title: p.plan_title,
            plan_status: p.plan_status,
            plan_reject_note: p.plan_reject_note,
            plan_steps: p
                .plan_steps
                .into_iter()
                .map(|s| PlanStepVO {
                    step_text: s.step_text,
                    step_done: s.step_done,
                })
                .collect(),
            plan_annotations: p
                .plan_annotations
                .into_iter()
                .map(|a| PlanAnnotationVO {
                    annotation_id: a.annotation_id,
                    annotation_scope: a.annotation_scope,
                    annotation_step: a.annotation_step,
                    annotation_text: a.annotation_text,
                    annotation_author: a.annotation_author,
                    annotation_created_at: a.annotation_created_at,
                })
                .collect(),
            plan_updated_at: p.plan_updated_at,
        });
    Ok(PlanRESP { resp_plan: plan })
}

/// 读记忆清单：逐类各取一批再合并，附带各类计数。
pub fn memory_list(sv: &Arc<Services>) -> Result<MemoryListRESP> {
    let ctx = feature_ctx(sv);
    let store = wb_features::memory_store::MemoryStore::open(&ctx)?;
    let mut entries: Vec<MemoryEntryVO> = Vec::new();
    for kind in MEMORY_KINDS {
        for row in store.list_full_by_kind(kind, MEMORY_LIST_LIMIT)? {
            entries.push(MemoryEntryVO {
                memory_id: row.memory_id,
                memory_kind: kind.to_string(),
                memory_text: row.memory_text,
                memory_created_at: row.memory_created_at,
                memory_updated_at: row.memory_updated_at,
            });
        }
    }
    // 全库按更新时间倒序：逐类倒序拼起来不等于全局顺序，界面要的是全局最新在前
    entries.sort_by_key(|e| std::cmp::Reverse(e.memory_updated_at));
    // 一次取全库计数，比逐类 count 少开四次查询
    let counts = store.count_by_kind()?;
    let resp_counts = MEMORY_KINDS
        .iter()
        .map(|kind| MemoryKindCountVO {
            kind_name: kind.to_string(),
            kind_count: counts
                .iter()
                .find(|(k, _)| k == kind)
                .map(|(_, n)| *n)
                .unwrap_or(0),
        })
        .collect();
    let resp_total = store.total()?;
    Ok(MemoryListRESP {
        resp_entries: entries,
        resp_counts,
        resp_total,
    })
}

/// 删一条记忆；不存在报 7301。
pub fn memory_delete(sv: &Arc<Services>, memory_id: &str) -> Result<bool> {
    let id = memory_id.trim();
    if id.is_empty() {
        return Err(AppError::new(
            wb_domain::error::common::PARAM_INVALID,
            "memory_id 不能为空",
        ));
    }
    let ctx = feature_ctx(sv);
    let store = wb_features::memory_store::MemoryStore::open(&ctx)?;
    if !store.remove(id)? {
        return Err(AppError::new(
            wb_domain::error::tool::MEMORY_NOT_FOUND,
            format!("{id} 不存在"),
        ));
    }
    Ok(true)
}
