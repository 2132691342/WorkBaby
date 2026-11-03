//! 目标追踪：单活跃目标 + 步骤清单，跨压缩存活（文件存储），状态行随变更推送。

use std::path::Path;
use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use serde_json::json;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};

use crate::FeatureCtx;

/// 目标状态（状态机：blocked 恢复须显式置回 active）。
pub(crate) const STATUSES: [&str; 4] = ["active", "blocked", "achieved", "abandoned"];

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
    /// 登记目标的会话（旧文件无此字段时为空）
    #[serde(default)]
    pub goal_session_id: String,
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
pub(crate) fn now_ms() -> i64 {
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
pub(crate) fn render(g: &GoalState) -> String {
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

/// 界面侧写入：与工具同一份落盘与状态机，不做第二套规则。
/// 界面能做的只有「切状态 / 勾步骤 / 记进展 / 新建 / 放弃」这五件事。
pub fn apply_op(
    store: &GoalStore,
    ctx: &FeatureCtx,
    req: &wb_domain::feature_state::GoalMutateREQ,
    sid: &str,
) -> Result<GoalState> {
    let now = now_ms();
    match req.goal_op.as_str() {
        "create" => {
            if req.goal_text.trim().is_empty() {
                return Err(AppError::new(toolcode::ARG_INVALID, "目标描述不能为空"));
            }
            // 只有一个活跃目标——但只对「同一会话」成立。别的会话留下的旧目标
            // 不该挡住新会话建自己的目标：它本来就不驱动这个会话，两道门禁
            // 会让用户卡在「有目标却建不了新的」。
            if let Some(g) = store.load() {
                let same_owner = g.goal_session_id == sid;
                if same_owner && (g.goal_status == "active" || g.goal_status == "blocked") {
                    return Err(AppError::new(
                        toolcode::ARG_INVALID,
                        format!("已有未完结目标「{}」，先放弃它再建新的", g.goal_text),
                    ));
                }
            }
            let g = GoalState {
                goal_id: wb_pkg::ulid::new_id(wb_pkg::ulid::Prefix::Goal),
                goal_text: req.goal_text.trim().to_string(),
                goal_status: "active".into(),
                goal_note: String::new(),
                goal_steps: req
                    .goal_steps
                    .iter()
                    .filter(|s| !s.trim().is_empty())
                    .map(|s| GoalStep {
                        step_text: s.trim().to_string(),
                        step_done: false,
                    })
                    .collect(),
                goal_session_id: sid.to_string(),
                goal_created_at: now,
                goal_updated_at: now,
            };
            store.save(&g)?;
            push_status(ctx, &g, sid);
            Ok(g)
        }
        op => {
            let mut g = store
                .load()
                .ok_or_else(|| AppError::new(toolcode::ARG_INVALID, "当前没有目标"))?;
            match op {
                "status" => {
                    if !STATUSES.contains(&req.goal_status.as_str()) {
                        return Err(AppError::new(toolcode::ARG_INVALID, "目标状态取值非法"));
                    }
                    g.goal_status = req.goal_status.clone();
                }
                "done_steps" => {
                    for seq in &req.goal_done_steps {
                        let idx = (*seq - 1).max(0) as usize;
                        let Some(step) = g.goal_steps.get_mut(idx) else {
                            return Err(AppError::new(
                                toolcode::ARG_INVALID,
                                format!("步骤 {seq} 不存在"),
                            ));
                        };
                        step.step_done = true;
                    }
                    // 全部勾完就自动收口：留着 active 会让续跑一直触发
                    if !g.goal_steps.is_empty() && g.goal_steps.iter().all(|s| s.step_done) {
                        g.goal_status = "achieved".into();
                    }
                }
                "note" => g.goal_note = req.goal_note.trim().to_string(),
                "abandon" => g.goal_status = "abandoned".into(),
                other => {
                    return Err(AppError::new(
                        toolcode::ARG_INVALID,
                        format!("不认识的目标动作 {other}"),
                    ))
                }
            }
            g.goal_updated_at = now;
            store.save(&g)?;
            push_status(ctx, &g, sid);
            Ok(g)
        }
    }
}

/// 推送状态行（footer）。
pub(crate) fn push_status(ctx: &FeatureCtx, g: &GoalState, sid: &str) {
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
    (ctx.emit)(sid, "chat:status", json!({ "key": "goal", "text": text }));
}

/// 续跑提示：活跃目标且仍有未完成步骤时给出下一步，否则 None。
/// 目标属于**登记它的那个会话**：旧目标不该让新对话都被灌一句无关指令；
/// 没有步骤清单的目标也不自动续跑（无从判断「还差什么」，硬推只会空转）。
pub fn continuation_prompt(ctx: &FeatureCtx, session_id: &str) -> Option<String> {
    let store = GoalStore::new(ctx);
    let g = store.load()?;
    if g.goal_status != "active" || g.goal_steps.is_empty() {
        return None;
    }
    if g.goal_session_id != session_id {
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
