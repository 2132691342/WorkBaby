//! 计划的数据形态：状态机、步骤、标注与存储。

use std::path::Path;

use serde::{Deserialize, Serialize};
use wb_pkg::{AppError, Result};

/// 计划状态：draft 提交待审 / approved 批准执行中 / rejected 已驳回 / done 全部完成。
pub const STATUSES: [&str; 4] = ["draft", "approved", "rejected", "done"];

/// 计划状态。
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct PlanState {
    /// 计划 id（PLAN_ 前缀）
    pub plan_id: String,
    /// 计划标题
    pub plan_title: String,
    /// draft / approved / rejected / done
    pub plan_status: String,
    /// 用户驳回时留下的意见（approved 后为空）
    pub plan_reject_note: String,
    /// 计划步骤
    pub plan_steps: Vec<PlanStep>,
    /// 用户对计划的标注（逐条意见）
    pub plan_annotations: Vec<PlanAnnotation>,
    /// 更新时间（毫秒）
    pub plan_updated_at: i64,
}

/// 用户对计划的一条标注。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PlanAnnotation {
    /// 标注 id（ANN_ 前缀）
    pub annotation_id: String,
    /// 标注对象：step（步骤）/ plan（整体）
    pub annotation_scope: String,
    /// 关联的步骤序号（scope=step 时有效，从 1 开始）
    pub annotation_step: i64,
    /// 标注内容
    pub annotation_text: String,
    /// 标注人：user / assistant
    pub annotation_author: String,
    /// 创建时间（毫秒）
    pub annotation_created_at: i64,
}

impl PlanAnnotation {
    /// 单行渲染：整体标注带前缀，步骤标注带序号。
    pub fn render_line(&self) -> String {
        match self.annotation_scope.as_str() {
            "step" => format!("[标注·步骤 {}] {}", self.annotation_step, self.annotation_text),
            _ => format!("[标注] {}", self.annotation_text),
        }
    }
}

/// 计划的一个步骤。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PlanStep {
    /// 步骤内容
    pub step_text: String,
    /// 是否已完成
    pub step_done: bool,
}

/// 当前毫秒时间戳。
pub fn now_ms() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

/// 计划存储：feature_dir\plan.json。
pub struct PlanStore {
    /// 计划文件路径
    pub path: std::path::PathBuf,
}

impl PlanStore {
    /// 按 FeatureCtx 定位。
    pub fn new(ctx: &crate::FeatureCtx) -> Self {
        Self {
            path: ctx.feature_dir().join("plan.json"),
        }
    }

    /// 读当前计划（缺失 = None）。
    pub fn load(&self) -> Option<PlanState> {
        std::fs::read_to_string(&self.path)
            .ok()
            .and_then(|t| serde_json::from_str(&t).ok())
    }

    /// 读改写一次；闭包报错则不落盘。
    pub fn mutate(
        &self,
        f: impl FnOnce(&mut Option<PlanState>) -> Result<()>,
    ) -> Result<Option<PlanState>> {
        let mut state = self.load();
        f(&mut state)?;
        if let Some(g) = &state {
            std::fs::create_dir_all(self.path.parent().unwrap_or(Path::new(".")))
                .map_err(|e| AppError::wrap("1002", "建计划目录失败", e))?;
            let text = serde_json::to_string_pretty(g)
                .map_err(|e| AppError::new("1002", format!("计划序列化失败：{e}")))?;
            std::fs::write(&self.path, text)
                .map_err(|e| AppError::wrap("1002", "写计划文件失败", e))?;
        }
        Ok(state)
    }
}

/// checklist 渲染（模型可读）。
pub fn render(p: &PlanState) -> String {
    let head = format!("[{}] {}", p.plan_status, p.plan_title);
    if p.plan_steps.is_empty() {
        return head;
    }
    let mut out = vec![head];
    for (i, s) in p.plan_steps.iter().enumerate() {
        let mark = if s.step_done { "[x]" } else { "[ ]" };
        out.push(format!("{}. {} {}", i + 1, mark, s.step_text));
    }
    if !p.plan_annotations.is_empty() {
        out.push("标注：".to_string());
        for a in &p.plan_annotations {
            out.push(format!("  {}", a.render_line()));
        }
    }
    if !p.plan_reject_note.is_empty() {
        out.push(format!("驳回意见：{}", p.plan_reject_note));
    }
    out.join("\n")
}

/// widget 行推送。
pub fn push_widget(ctx: &crate::FeatureCtx, p: &PlanState) {
    let mut lines = vec![format!("计划：{}", p.plan_title)];
    for (i, s) in p.plan_steps.iter().enumerate() {
        let mark = if s.step_done { "[x]" } else { "[ ]" };
        lines.push(format!("{}. {} {}", i + 1, mark, s.step_text));
    }
    for a in &p.plan_annotations {
        lines.push(a.render_line());
    }
    (ctx.emit)("", "chat:widget", serde_json::json!({ "key": "plan", "lines": lines }));
}

/// 状态常量给测试用。
pub fn known_statuses() -> [&'static str; 4] {
    STATUSES
}
