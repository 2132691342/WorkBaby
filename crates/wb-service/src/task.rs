//! task 工具：派一个子代理去干活，只把结论带回来。
//! 子代理在隔离子会话里跑，中间过程不进父上下文——这是把「一次要扫 47 个文件」
//! 的活摘出主上下文的唯一手段。

use std::path::Path;
use std::sync::Arc;
use std::time::Instant;

use async_trait::async_trait;
use serde_json::{json, Value};
use tokio_util::sync::CancellationToken;
use wb_domain::error::tool as toolcode;
use wb_pkg::{AppError, Result};
use wb_tools::{Tool, ToolOutput};

use super::Services;

/// 角色预设：子代理的职责边界与可用工具。
struct RolePreset {
    /// 角色码
    code: &'static str,
    /// 显示名
    name: &'static str,
    /// 职责说明
    prompt: &'static str,
    /// 允许使用的工具（空 = 继承父会话）
    tools: &'static [&'static str],
    /// 最多几轮
    max_turns: i64,
}

/// 角色表：scout 偏只读探查，worker 偏动手执行，reviewer 偏审查。
const ROLES: &[RolePreset] = &[
    RolePreset {
        code: "scout",
        name: "探查",
        prompt: "You explore and report. Read files, search the codebase, and answer \
                 questions about how things work. Do not modify any file.",
        tools: &["read", "grep", "find", "ls"],
        max_turns: 16,
    },
    RolePreset {
        code: "worker",
        name: "执行",
        prompt: "You carry out a concrete task. Read what you need, make the changes, \
                 and verify the result.",
        tools: &[],
        max_turns: 20,
    },
    RolePreset {
        code: "reviewer",
        name: "审查",
        prompt: "You review existing work. Look for defects, inconsistencies, and missing \
                 cases. Report findings without changing files.",
        tools: &["read", "grep", "find", "ls", "bash"],
        max_turns: 12,
    },
];

/// 角色表视图（供 API 与内置提示词引用）。
pub fn roles_vo() -> Vec<wb_domain::subagent::SubagentRoleVO> {
    ROLES
        .iter()
        .map(|r| wb_domain::subagent::SubagentRoleVO {
            role_code: r.code.to_string(),
            role_name: r.name.to_string(),
            role_prompt: r.prompt.to_string(),
            role_tools: r.tools.iter().map(|t| t.to_string()).collect(),
            role_max_turns: r.max_turns,
        })
        .collect()
}

/// 按角色码取预设；未知角色回落到 worker。
fn role_of(code: &str) -> &'static RolePreset {
    ROLES.iter().find(|r| r.code == code).unwrap_or(&ROLES[1])
}

/// task 工具：派子代理干活。
pub struct TaskTool {
    sv: Arc<Services>,
}

impl TaskTool {
    /// 装配。
    pub fn new(sv: Arc<Services>) -> Self {
        Self { sv }
    }

    /// 取父会话的运行参数：没有活跃运行上下文时返回 None（工具报错而不是静默降级）。
    fn parent_cfg(&self) -> Option<(String, Arc<wb_kernel::AgentConfig>)> {
        self.sv
            .scope
            .lock()
            .unwrap()
            .as_ref()
            .map(|s| (s.session_id.clone(), s.cfg.clone()))
    }
}

#[async_trait]
impl Tool for TaskTool {
    fn name(&self) -> &str {
        "task"
    }

    fn label(&self) -> &str {
        "派子代理"
    }

    fn description(&self) -> &str {
        "派一个子代理去完成一段独立任务，子代理在自己的会话里跑，只把结论返回给你。\
         适合：批量阅读大量文件后汇总、在一堆文件里做重复的改动、需要独立视角的审查。\
         子代理看不到你和用户的对话，任务描述要写全。\
         角色：scout（只读探查）/ worker（动手执行）/ reviewer（审查）。"
    }

    fn parameters(&self) -> Value {
        json!({
            "type": "object",
            "properties": {
                "role": {
                    "type": "string",
                    "enum": ["scout", "worker", "reviewer"],
                    "description": "子代理角色，默认 worker"
                },
                "task": {
                    "type": "string",
                    "description": "任务描述（要写清目标与交付物，子代理看不到上下文）"
                },
                "max_turns": {
                    "type": "integer",
                    "description": "最多跑几轮，默认按角色设定",
                    "minimum": 1,
                    "maximum": 64
                }
            },
            "required": ["task"]
        })
    }

    async fn execute(
        &self,
        args: Value,
        cancel: CancellationToken,
        _workdir: &Path,
    ) -> Result<ToolOutput> {
        let started = Instant::now();
        let Some((parent_session_id, parent_cfg)) = self.parent_cfg() else {
            return Err(AppError::new(
                toolcode::NOT_AVAILABLE,
                "task 只能在对话运行时使用",
            ));
        };
        let role_code = args
            .get("role")
            .and_then(|v| v.as_str())
            .unwrap_or("worker")
            .to_string();
        let task = args
            .get("task")
            .and_then(|v| v.as_str())
            .unwrap_or("")
            .trim()
            .to_string();
        if task.is_empty() {
            return Err(AppError::new(toolcode::ARG_INVALID, "task 不能为空"));
        }
        let preset = role_of(&role_code);
        let max_turns = args
            .get("max_turns")
            .and_then(|v| v.as_i64())
            .unwrap_or(preset.max_turns)
            .clamp(1, 64);

        // 角色的工具白名单与父会话工具取交集：子代理不能调用父会话没有的工具。
        let parent_tools = parent_cfg.enabled_tools.clone();
        let tool_names: Vec<String> = if preset.tools.is_empty() {
            parent_tools.clone()
        } else {
            preset
                .tools
                .iter()
                .filter(|t| parent_tools.iter().any(|p| p == *t))
                .map(|t| t.to_string())
                .collect()
        };

        // 观测出口：子代理的流式过程只走 SSE，不进任何上下文
        let board = self.sv.agents.clone();
        let parent_for_obs = parent_session_id.clone();
        let emitter = self.sv.emitter.lock().unwrap().clone();
        let observe: wb_kernel::ObserveFn = std::sync::Arc::new(move |ev| {
            let Some(em) = emitter.as_ref() else { return };
            board.on_event(&parent_for_obs, ev, &|sid, event, data| {
                em.emit(sid, event, data);
            });
        });
        let sc = wb_kernel::SubagentConfig {
            parent_session_id,
            role_code: role_code.clone(),
            role_prompt: preset.prompt.to_string(),
            task: task.clone(),
            tool_names,
            sessions_dir: self.sv.sessions_dir().join("subagents"),
            workspace: self.sv.workspace.lock().unwrap().clone(),
            max_turns,
            observe: Some(observe),
        };
        let outcome = wb_kernel::run_subagent(&parent_cfg, sc, cancel).await?;
        let text = if outcome.failed {
            format!("[子代理未完成]\n{}", outcome.conclusion)
        } else {
            outcome.conclusion.clone()
        };
        Ok(ToolOutput {
            text,
            is_error: outcome.failed,
            duration_ms: started.elapsed().as_millis() as i64,
        })
    }
}
