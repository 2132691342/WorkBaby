//! 计划评审：模型提交计划 → 用户批准/驳回 → 执行期勾选（widget checklist + ask 桥）。

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

use crate::ask_user::AnswerBoard;
use crate::FeatureCtx;

/// 计划状态：draft 提交待审 / approved 批准执行中 / rejected 已驳回 / done 全部完成。
const STATUSES: [&str; 4] = ["draft", "approved", "rejected", "done"];

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
    /// 更新时间（毫秒）
    pub plan_updated_at: i64,
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
fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as i64
}

/// 计划存储：feature_dir\plan.json。
pub struct PlanStore {
    path: std::path::PathBuf,
}

impl PlanStore {
    /// 按 FeatureCtx 定位。
    pub fn new(ctx: &FeatureCtx) -> Self {
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
fn render(p: &PlanState) -> String {
    let head = format!("[{}] {}", p.plan_status, p.plan_title);
    if p.plan_steps.is_empty() {
        return head;
    }
    let mut out = vec![head];
    for (i, s) in p.plan_steps.iter().enumerate() {
        let mark = if s.step_done { "[x]" } else { "[ ]" };
        out.push(format!("{}. {} {}", i + 1, mark, s.step_text));
    }
    if !p.plan_reject_note.is_empty() {
        out.push(format!("驳回意见：{}", p.plan_reject_note));
    }
    out.join("\n")
}

/// widget 行推送。
fn push_widget(ctx: &FeatureCtx, p: &PlanState) {
    let mut lines = vec![format!("计划：{}", p.plan_title)];
    for (i, s) in p.plan_steps.iter().enumerate() {
        let mark = if s.step_done { "[x]" } else { "[ ]" };
        lines.push(format!("{}. {} {}", i + 1, mark, s.step_text));
    }
    (ctx.emit)("", "chat:widget", json!({ "key": "plan", "lines": lines }));
}

/// 解析步骤清单参数。
fn parse_steps(steps: &[Value]) -> Result<Vec<PlanStep>> {
    let out: Vec<PlanStep> = steps
        .iter()
        .filter_map(|v| {
            v.as_str().map(|s| PlanStep {
                step_text: s.to_string(),
                step_done: false,
            })
        })
        .collect();
    if out.is_empty() {
        return Err(AppError::new(toolcode::ARG_INVALID, "steps 不能为空"));
    }
    Ok(out)
}

/// plan_submit 工具：提交计划并阻塞等用户裁决（批准才返回，驳回带意见）。
pub struct PlanSubmitTool {
    store: Arc<PlanStore>,
    ctx: FeatureCtx,
    answers: Arc<AnswerBoard>,
}

impl PlanSubmitTool {
    /// 装配。
    pub fn new(store: Arc<PlanStore>, ctx: FeatureCtx, answers: Arc<AnswerBoard>) -> Self {
        Self {
            store,
            ctx,
            answers,
        }
    }
}

#[async_trait]
impl Tool for PlanSubmitTool {
    fn name(&self) -> &str {
        "plan_submit"
    }

    fn label(&self) -> &str {
        "提交计划"
    }

    fn description(&self) -> &str {
        "把多步执行计划提交给用户评审（批准/驳回）。适合改动面大、有破坏性或方向性选择任务：先给步骤清单，批准后再动手。驳回会带意见返回，修订后可重新提交。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "title": { "type": "string", "description": "计划标题（一句话）" },
                "steps": { "type": "array", "items": { "type": "string" }, "description": "步骤清单，按执行顺序" }
            },
            "required": ["title", "steps"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let title = args
            .get("title")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        if title.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "title 不能为空"));
        }
        let steps = parse_steps(
            args.get("steps")
                .and_then(|v| v.as_array())
                .map(|a| a.as_slice())
                .unwrap_or(&[]),
        )?;
        let store = self.store.clone();
        let answers = self.answers.clone();
        let title2 = title.clone();
        // 提交裁决前的落盘放阻塞线程；ask 等待在异步侧
        let plan = tokio::task::spawn_blocking(move || {
            store.mutate(|slot| {
                *slot = Some(PlanState {
                    plan_id: format!("PLAN_{}", wb_pkg::ulid::raw_ulid()),
                    plan_title: title2,
                    plan_status: "draft".into(),
                    plan_reject_note: String::new(),
                    plan_steps: steps,
                    plan_updated_at: now_ms(),
                });
                Ok(())
            })?;
            Ok(store.load().expect("刚写入必能读回"))
        })
        .await
        .map_err(|e| AppError::new("7001", e.to_string()))??;
        push_widget(&self.ctx, &plan);

        let ask_id = wb_pkg::ulid::new_id(wb_pkg::Prefix::Ask);
        (self.ctx.emit)(
            "",
            "chat:ask",
            json!({
                "id": ask_id,
                "method": "confirm",
                "title": "计划评审",
                "message": render(&plan),
                "options": [],
                "placeholder": "批准请点确认；驳回可先填意见再点取消",
                "prefill": "",
                "timeout_ms": 0,
            }),
        );
        let rx = answers.register(&ask_id);
        let answer = tokio::select! {
            r = rx => r.map_err(|_| AppError::new(toolcode::EXEC_CANCELLED, "计划评审通道已关闭"))?,
            _ = cancel.cancelled() => {
                (self.ctx.emit)("", "chat:ask_closed", json!({ "id": ask_id }));
                return Err(AppError::new(toolcode::EXEC_CANCELLED, "计划评审已随会话取消"));
            }
        };
        (self.ctx.emit)("", "chat:ask_closed", json!({ "id": ask_id }));
        let rejected = answer
            .get("cancelled")
            .and_then(|v| v.as_bool())
            .unwrap_or(false);
        let note = answer
            .get("value")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .to_string();
        let store2 = self.store.clone();
        let ctx2 = self.ctx.clone();
        let out = tokio::task::spawn_blocking(move || {
            store2.mutate(|slot| {
                if let Some(p) = slot {
                    if rejected {
                        p.plan_status = "rejected".into();
                        p.plan_reject_note = note;
                    } else {
                        p.plan_status = "approved".into();
                        p.plan_reject_note = String::new();
                    }
                    p.plan_updated_at = now_ms();
                }
                Ok(())
            })?;
            match store2.load() {
                Some(p) => {
                    push_widget(&ctx2, &p);
                    Ok(render(&p))
                }
                None => Ok("计划已清空".into()),
            }
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

/// plan_mark_done 工具：执行期勾选步骤；全部勾完状态转 done。
pub struct PlanMarkDoneTool {
    store: Arc<PlanStore>,
    ctx: FeatureCtx,
}

impl PlanMarkDoneTool {
    /// 装配。
    pub fn new(store: Arc<PlanStore>, ctx: FeatureCtx) -> Self {
        Self { store, ctx }
    }
}

#[async_trait]
impl Tool for PlanMarkDoneTool {
    fn name(&self) -> &str {
        "plan_mark_done"
    }

    fn label(&self) -> &str {
        "勾选计划步骤"
    }

    fn description(&self) -> &str {
        "把已完成的计划步骤标记为完成（按提交时的序号，从 1 开始）。全部勾完计划自动收尾为 done。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "done_steps": { "type": "array", "items": { "type": "integer" }, "description": "要标记完成的步骤序号" }
            },
            "required": ["done_steps"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let done: Vec<usize> = args
            .get("done_steps")
            .and_then(|v| v.as_array())
            .map(|a| {
                a.iter()
                    .filter_map(|x| x.as_u64().map(|n| n as usize))
                    .collect()
            })
            .unwrap_or_default();
        if done.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "done_steps 不能为空"));
        }
        let store = self.store.clone();
        let ctx = self.ctx.clone();
        let out = tokio::task::spawn_blocking(move || {
            store.mutate(|slot| {
                let p = slot.as_mut().ok_or_else(|| {
                    AppError::new(toolcode::ARG_INVALID, "当前没有计划，先 plan_submit")
                })?;
                for n in &done {
                    let step = p.plan_steps.get_mut(n.saturating_sub(1)).ok_or_else(|| {
                        AppError::new(toolcode::ARG_INVALID, format!("步骤 {n} 不存在"))
                    })?;
                    step.step_done = true;
                }
                if p.plan_steps.iter().all(|s| s.step_done) {
                    p.plan_status = "done".into();
                }
                p.plan_updated_at = now_ms();
                Ok(())
            })?;
            match store.load() {
                Some(p) => {
                    push_widget(&ctx, &p);
                    Ok(render(&p))
                }
                None => Ok("计划已清空".into()),
            }
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

/// plan_inbox 工具：读当前计划与状态。
pub struct PlanInboxTool {
    store: Arc<PlanStore>,
}

impl PlanInboxTool {
    /// 装配。
    pub fn new(store: Arc<PlanStore>) -> Self {
        Self { store }
    }
}

#[async_trait]
impl Tool for PlanInboxTool {
    fn name(&self) -> &str {
        "plan_inbox"
    }

    fn label(&self) -> &str {
        "查看计划"
    }

    fn description(&self) -> &str {
        "查看当前计划的状态、步骤与驳回意见。恢复会话或每轮推进前先看一眼，避免偏离既定计划。"
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
                .map(|p| render(&p))
                .unwrap_or_else(|| "（当前没有计划）".into()))
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

/// 状态常量给测试用。
pub fn known_statuses() -> [&'static str; 4] {
    STATUSES
}
