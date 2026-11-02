//! 功能活状态的写入：界面上的勾选、批准、标注、删改与模型走的工具用同一份落盘。
//! 界面不该只能看——看得见点不动等于没有这个功能。

use std::sync::Arc;

use wb_domain::feature_state::{
    GoalMutateREQ, GoalRESP, MemorySaveREQ, PlanMutateREQ, PlanRESP, TodoListRESP, TodoMutateREQ,
};
use wb_pkg::{AppError, Result};

use super::feature_state::{self, MEMORY_KINDS};
use super::features::feature_ctx;
use super::Services;

/// 待办写入：转发到 wb-features 的同一套动作，回吐最新清单。
pub fn todo_mutate(sv: &Arc<Services>, req: &TodoMutateREQ) -> Result<TodoListRESP> {
    let ctx = feature_ctx(sv);
    let store = wb_features::todo::TodoStore::new(&ctx);
    wb_features::todo::apply_op(
        &store,
        &req.todo_op,
        &req.todo_id,
        &req.todo_text,
        &req.todo_status,
        req.todo_blocked_by.clone(),
    )?;
    feature_state::todo_list(sv)
}

/// 目标写入：新建 / 切状态 / 勾步骤 / 记进展 / 放弃。
pub fn goal_mutate(sv: &Arc<Services>, req: &GoalMutateREQ) -> Result<GoalRESP> {
    let ctx = feature_ctx(sv);
    let store = wb_features::goal::GoalStore::new(&ctx);
    wb_features::goal::apply_op(&store, &ctx, req)?;
    feature_state::goal_get(sv)
}

/// 计划写入：批准 / 驳回 / 勾步骤 / 标注。
pub fn plan_mutate(sv: &Arc<Services>, req: &PlanMutateREQ) -> Result<PlanRESP> {
    let ctx = feature_ctx(sv);
    let store = wb_features::plan_types::PlanStore::new(&ctx);
    wb_features::plan_types::apply_op(&store, &ctx, req)?;
    feature_state::plan_get(sv)
}

/// 记忆新增：与 memory_save 同一套密钥扫描，界面写入也不能漏密。
pub fn memory_save(sv: &Arc<Services>, req: &MemorySaveREQ) -> Result<String> {
    let text = req.memory_text.trim();
    if text.is_empty() {
        return Err(AppError::new(
            wb_domain::error::common::PARAM_INVALID,
            "记忆正文不能为空",
        ));
    }
    let kind = if MEMORY_KINDS.contains(&req.memory_kind.as_str()) {
        req.memory_kind.as_str()
    } else {
        return Err(AppError::new(
            wb_domain::error::common::PARAM_INVALID,
            format!("记忆类别只能是 {}", MEMORY_KINDS.join(" / ")),
        ));
    };
    if let Some(label) = wb_features::memory::scan_secret(text) {
        return Err(AppError::new(
            wb_domain::error::common::PARAM_INVALID,
            format!("正文像是 {label}，不要把它写进记忆"),
        ));
    }
    let ctx = feature_ctx(sv);
    let store = wb_features::memory_store::MemoryStore::open(&ctx)?;
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64;
    let (id, _created) = store.save(kind, text, now)?;
    Ok(id)
}
