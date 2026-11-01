//! 目标追踪：单活跃目标 + 步骤清单，跨压缩存活（文件存储），状态行随变更推送。

use std::path::Path;
use std::sync::Arc;
use std::time::{Instant, SystemTime, UNIX_EPOCH};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

use crate::FeatureCtx;

/// 目标状态（状态机：blocked 恢复须显式置回 active）。
const STATUSES: [&str; 4] = ["active", "blocked", "achieved", "abandoned"];

/// 目标状态（单文件单目标：活跃目标只有一个）。
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct GoalState {
    /// 目标 id（GOAL_ 前缀）
    pub goal_id: String,
    /// 目标描述
    pub goal_text: String,
    /// active / blocked / achieved / abandoned
    pub goal_status: String,
    /// 最近进展备注
    pub goal_note: String,
    /// 步骤清单
    pub goal_steps: Vec<GoalStep>,
    /// 创建时间（毫秒）
    pub goal_created_at: i64,
    /// 更新时间（毫秒）
    pub goal_updated_at: i64,
}

/// 目标下的一个步骤。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GoalStep {
    /// 步骤内容
    pub step_text: String,
    /// 是否完成
    pub step_done: bool,
}

/// 当前毫秒时间戳。
fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

/// 目标存储：feature_dir\goal.json。
pub struct GoalStore {
    path: std::path::PathBuf,
}

impl GoalStore {
    /// 按 FeatureCtx 定位。
    pub fn new(ctx: &FeatureCtx) -> Self {
        Self {
            path: ctx.feature_dir().join("goal.json"),
        }
    }

    /// 读当前目标（缺失 = None）。
    pub fn load(&self) -> Option<GoalState> {
        std::fs::read_to_string(&self.path)
            .ok()
            .and_then(|t| serde_json::from_str(&t).ok())
    }

    /// 写回。
    pub fn save(&self, g: &GoalState) -> Result<()> {
        std::fs::create_dir_all(self.path.parent().unwrap_or(Path::new(".")))
            .map_err(|e| AppError::wrap("1002", "建目标目录失败", e))?;
        let text = serde_json::to_string_pretty(g)
            .map_err(|e| AppError::new("1002", format!("目标序列化失败：{e}")))?;
        std::fs::write(&self.path, text).map_err(|e| AppError::wrap("1002", "写目标文件失败", e))
    }
}

/// 渲染状态行（chat:status / 模型可读共用）。
fn render(g: &GoalState) -> String {
    let done = g.goal_steps.iter().filter(|s| s.step_done).count();
    let steps = if g.goal_steps.is_empty() {
        String::new()
    } else {
        format!("（{}/{} 步）", done, g.goal_steps.len())
    };
    let note = if g.goal_note.is_empty() {
        String::new()
    } else {
        format!(" | {}", g.goal_note)
    };
    format!("[{}] {}{}{}", g.goal_status, g.goal_text, steps, note)
}

/// 推送状态行（footer）。
fn push_status(ctx: &FeatureCtx, g: &GoalState) {
    let done = g.goal_steps.iter().filter(|s| s.step_done).count();
    let text = if g.goal_status == "achieved" {
        format!("目标已达成：{}", g.goal_text)
    } else {
        format!(
            "目标 [{}/{}]：{}",
            done,
            g.goal_steps.len().max(1),
            g.goal_text
        )
    };
    (ctx.emit)("", "chat:status", json!({ "key": "goal", "text": text }));
}

/// 续跑提示：活跃目标且仍有未完成步骤时给出下一步提示，否则 None。
///
/// 没有步骤清单的目标不自动续跑——无从判断「还差什么」，硬推只会空转。
pub fn continuation_prompt(ctx: &FeatureCtx) -> Option<String> {
    let store = GoalStore::new(ctx);
    let g = store.load()?;
    if g.goal_status != "active" || g.goal_steps.is_empty() {
        return None;
    }
    let remaining: Vec<&GoalStep> = g.goal_steps.iter().filter(|s| !s.step_done).collect();
    if remaining.is_empty() {
        return None;
    }
    let mut prompt = format!(
        "继续推进目标「{}」。已完成 {}/{} 步。\n\n待办步骤：\n",
        g.goal_text,
        g.goal_steps.len() - remaining.len(),
        g.goal_steps.len()
    );
    for (i, s) in remaining.iter().enumerate() {
        prompt.push_str(&format!("{}. {}\n", i + 1, s.step_text));
    }
    if !g.goal_note.is_empty() {
        prompt.push_str(&format!("\n最近进展：{}\n", g.goal_note));
    }
    prompt.push_str("\n接着做下一步；做完了用 update_goal 勾选步骤。目标达成就置 achieved。");
    Some(prompt)
}

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
        let out = tokio::task::spawn_blocking(move || {
            if let Some(g) = store.load() {
                if g.goal_status == "active" || g.goal_status == "blocked" {
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
                goal_created_at: now_ms(),
                goal_updated_at: now_ms(),
            };
            store.save(&g)?;
            push_status(&ctx, &g);
            Ok(render(&g))
        })
        .await
        .map_err(|e| AppError::new("7001", e.to_string()))??;
        Ok(ToolOutput {
            text: out,
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
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
        Ok(ToolOutput {
            text: out,
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
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
            push_status(&ctx, &g);
            Ok(render(&g))
        })
        .await
        .map_err(|e| AppError::new("7001", e.to_string()))??;
        Ok(ToolOutput {
            text: out,
            is_error: false,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}
