//! 计划标注：给计划或某一步加一条意见。

use std::path::Path;
use std::sync::Arc;
use std::time::Instant;

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

use crate::plan_types::{now_ms, push_widget, render, PlanAnnotation, PlanStore};
use crate::FeatureCtx;

/// plan_annotate 工具：给计划或某一步加标注（用户意见与助手备注）。
pub struct PlanAnnotateTool {
    store: Arc<PlanStore>,
    ctx: FeatureCtx,
}

impl PlanAnnotateTool {
    /// 装配。
    pub fn new(store: Arc<PlanStore>, ctx: FeatureCtx) -> Self {
        Self { store, ctx }
    }
}

#[async_trait]
impl Tool for PlanAnnotateTool {
    fn name(&self) -> &str {
        "plan_annotate"
    }

    fn label(&self) -> &str {
        "标注计划"
    }

    fn description(&self) -> &str {
        "给当前计划加一条标注：scope=plan 标整个计划，scope=step 标某一步（step_index 从 1 开始）。\
         用户对某一步提出疑问或修改意见时用它，标注会与计划一起呈现，下一轮你会看到。\
         标注只记录意见，不改变步骤状态——要终止计划请用 plan_submit 的驳回。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "scope": { "type": "string", "enum": ["plan", "step"], "description": "标注对象，默认 plan" },
                "step_index": { "type": "integer", "description": "要标注的步骤序号（scope=step 时必填）" },
                "text": { "type": "string", "description": "标注内容" },
                "author": { "type": "string", "enum": ["user", "assistant"], "description": "标注人，默认 assistant" }
            },
            "required": ["text"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        _cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let scope = {
            let s = args.get("scope").and_then(|v| v.as_str()).unwrap_or("plan");
            if !matches!(s, "plan" | "step") {
                return Err(AppError::new(
                    toolcode::ARG_INVALID,
                    "scope 只能是 plan / step",
                ));
            }
            s.to_string()
        };
        let text = args
            .get("text")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        if text.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "text 不能为空"));
        }
        let author = {
            let a = args.get("author").and_then(|v| v.as_str()).unwrap_or("assistant");
            if !matches!(a, "user" | "assistant") {
                return Err(AppError::new(
                    toolcode::ARG_INVALID,
                    "author 只能是 user / assistant",
                ));
            }
            a.to_string()
        };
        let step_index = args.get("step_index").and_then(|v| v.as_i64()).unwrap_or(0);
        if scope == "step" && step_index < 1 {
            return Err(AppError::new(
                toolcode::ARG_INVALID,
                "scope=step 时 step_index 必填（从 1 开始）",
            ));
        }
        let store = self.store.clone();
        let ctx = self.ctx.clone();
        let out = tokio::task::spawn_blocking(move || {
            store.mutate(|slot| {
                let p = slot.as_mut().ok_or_else(|| {
                    AppError::new(toolcode::ARG_INVALID, "当前没有计划，先 plan_submit")
                })?;
                if scope == "step" && p.plan_steps.get((step_index - 1) as usize).is_none() {
                    return Err(AppError::new(
                        toolcode::ARG_INVALID,
                        format!("步骤 {step_index} 不存在"),
                    ));
                }
                p.plan_annotations.push(PlanAnnotation {
                    annotation_id: format!("ANN_{}", wb_pkg::ulid::raw_ulid()),
                    annotation_scope: scope,
                    annotation_step: step_index,
                    annotation_text: text,
                    annotation_author: author,
                    annotation_created_at: now_ms(),
                });
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
