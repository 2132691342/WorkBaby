//! 目标追踪的模型工具：create_goal / get_goal / update_goal。
//! 与状态机拆开：这里是 Tool 契约实现，状态与渲染在 goal.rs。

use super::goal::{now_ms, push_status, render, GoalState, GoalStep, GoalStore, STATUSES};
use crate::FeatureCtx;
use async_trait::async_trait;
use serde_json::{json, Value};
use std::path::Path;
use std::sync::Arc;
use std::time::Instant;
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

/// create_goal 工具：建目标（已有活跃目标时要求先更新或放弃它）。
pub struct CreateGoalTool {
    store: Arc<GoalStore>,
    ctx: FeatureCtx,
}

impl CreateGoalTool {
    /// 装配。
    pub fn new(store: Arc<GoalStore>, ctx: FeatureCtx) -> Self {
        Self { store, ctx }
    }
}

#[async_trait]
impl Tool for CreateGoalTool {
    fn name(&self) -> &str {
        "create_goal"
    }

    fn label(&self) -> &str {
        "创建目标"
    }

    fn description(&self) -> &str {
        "登记一个需要跨多轮推进的目标（同一时间只有一个活跃目标）。多轮长任务、被中断后要续跑的任务用它记录目标与步骤，被阻塞时置 blocked，完成时置 achieved。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "goal_text": { "type": "string", "description": "目标描述（一句话说清要达成什么）" },
                "steps": { "type": "array", "items": { "type": "string" }, "description": "步骤清单（可选）" }
            },
            "required": ["goal_text"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        _workdir: &Path,
        tool_ctx: &wb_tools::ToolCtx,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let text = args
            .get("goal_text")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        if text.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "goal_text 不能为空"));
        }
        let steps: Vec<String> = args
            .get("steps")
            .and_then(|v| v.as_array())
            .map(|a| {
                a.iter()
                    .filter_map(|x| x.as_str().map(|s| s.to_string()))
                    .collect()
            })
            .unwrap_or_default();
        let store = self.store.clone();
        let ctx = self.ctx.clone();
        let sid = tool_ctx.session_id.clone();
        let out = tokio::task::spawn_blocking(move || {
            if let Some(g) = store.load() {
                // 只拦同一会话的旧目标：别的会话留下的目标不驱动这里，也不该挡住这里
                if g.goal_session_id == sid
                    && (g.goal_status == "active" || g.goal_status == "blocked")
                {
                    return Err(AppError::new(
                        toolcode::ARG_INVALID,
                        format!(
                            "已有未完结目标「{}」，先 update_goal 收尾再建新的",
                            g.goal_text
                        ),
                    ));
                }
            }
            let g = GoalState {
                goal_id: wb_pkg::ulid::new_id(wb_pkg::ulid::Prefix::Goal),
                goal_text: text,
                goal_status: "active".into(),
                goal_note: String::new(),
                goal_steps: steps
                    .into_iter()
                    .map(|s| GoalStep {
                        step_text: s,
                        step_done: false,
                    })
                    .collect(),
                goal_session_id: sid.clone(),
                goal_created_at: now_ms(),
                goal_updated_at: now_ms(),
            };
            store.save(&g)?;
            push_status(&ctx, &g, &sid);
            Ok(render(&g))
        })
        .await
        .map_err(|e| AppError::new("7001", e.to_string()))??;
        Ok(ToolOutput::plain(
            out,
            false,
            started.elapsed().as_millis() as i64,
        ))
    }
}

/// get_goal 工具：读当前目标。
pub struct GetGoalTool {
    store: Arc<GoalStore>,
}

impl GetGoalTool {
    /// 装配。
    pub fn new(store: Arc<GoalStore>) -> Self {
        Self { store }
    }
}

#[async_trait]
impl Tool for GetGoalTool {
    fn name(&self) -> &str {
        "get_goal"
    }

    fn label(&self) -> &str {
        "查看目标"
    }

    fn description(&self) -> &str {
        "查看当前目标的状态、步骤与备注。每轮开始推进前先看一眼，避免偏离。"
    }

    fn parameters(&self) -> Value {
        json!({ "type": "object", "properties": {} })
    }

    async fn execute(
        &self,
        _args: Value,
        _cancel: CancellationToken,
        _workdir: &Path,
        _ctx: &wb_tools::ToolCtx,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let store = self.store.clone();
        let out = tokio::task::spawn_blocking(move || {
            Ok(store
                .load()
                .map(|g| render(&g))
                .unwrap_or_else(|| "（当前没有目标）".into()))
        })
        .await
        .map_err(|e| AppError::new("7001", e.to_string()))??;
        Ok(ToolOutput::plain(
            out,
            false,
            started.elapsed().as_millis() as i64,
        ))
    }
}

/// update_goal 工具：状态机 / 备注 / 勾步骤。
pub struct UpdateGoalTool {
    store: Arc<GoalStore>,
    ctx: FeatureCtx,
}

impl UpdateGoalTool {
    /// 装配。
    pub fn new(store: Arc<GoalStore>, ctx: FeatureCtx) -> Self {
        Self { store, ctx }
    }
}

#[async_trait]
impl Tool for UpdateGoalTool {
    fn name(&self) -> &str {
        "update_goal"
    }

    fn label(&self) -> &str {
        "更新目标"
    }

    fn description(&self) -> &str {
        "更新目标：goal_status 切状态（blocked 恢复须显式置回 active）；goal_done_steps 给要勾选完成的步骤序号（从 1 开始）；goal_note 记录最近进展。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "goal_status": { "type": "string", "enum": STATUSES, "description": "目标状态" },
                "goal_done_steps": { "type": "array", "items": { "type": "integer" }, "description": "要标记完成的步骤序号" },
                "goal_note": { "type": "string", "description": "最近进展备注" }
            }
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        _workdir: &Path,
        tool_ctx: &wb_tools::ToolCtx,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let status = args
            .get("goal_status")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let done: Vec<usize> = args
            .get("goal_done_steps")
            .and_then(|v| v.as_array())
            .map(|a| {
                a.iter()
                    .filter_map(|x| x.as_u64().map(|n| n as usize))
                    .collect()
            })
            .unwrap_or_default();
        let note = args
            .get("goal_note")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        if !status.is_empty() && !STATUSES.contains(&status.as_str()) {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                format!("goal_status 取值非法：{status}"),
            ));
        }
        let store = self.store.clone();
        let ctx = self.ctx.clone();
        let sid = tool_ctx.session_id.clone();
        let out = tokio::task::spawn_blocking(move || {
            let mut g = store.load().ok_or_else(|| {
                AppError::new(toolcode::ARG_INVALID, "当前没有目标，先 create_goal")
            })?;
            if !status.is_empty() {
                g.goal_status = status;
            }
            if !note.is_empty() {
                g.goal_note = note;
            }
            for n in &done {
                let step = g.goal_steps.get_mut(n.saturating_sub(1)).ok_or_else(|| {
                    AppError::new(toolcode::ARG_INVALID, format!("步骤 {n} 不存在"))
                })?;
                step.step_done = true;
            }
            g.goal_updated_at = now_ms();
            store.save(&g)?;
            push_status(&ctx, &g, &sid);
            Ok(render(&g))
        })
        .await
        .map_err(|e| AppError::new("7001", e.to_string()))??;
        Ok(ToolOutput::plain(
            out,
            false,
            started.elapsed().as_millis() as i64,
        ))
    }
}
