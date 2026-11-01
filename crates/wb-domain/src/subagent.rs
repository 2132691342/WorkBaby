//! 子代理视图与请求：隔离子会话的调用契约。

use serde::{Deserialize, Serialize};

/// 子代理角色预设。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SubagentRoleVO {
    /// 角色码（scout / worker / reviewer）
    pub role_code: String,
    /// 显示名
    pub role_name: String,
    /// 职责说明（注入子代理系统提示词）
    pub role_prompt: String,
    /// 允许使用的工具名清单（空 = 继承父会话）
    pub role_tools: Vec<String>,
    /// 最大轮次
    pub role_max_turns: i64,
}

/// 子代理清单响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct SubagentListRESP {
    /// 角色预设清单
    pub resp_roles: Vec<SubagentRoleVO>,
}

/// 子代理运行记录视图。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SubagentRunVO {
    /// 子代理会话 id（SUB_ 前缀）
    pub run_session_id: String,
    /// 所属父会话 id
    pub run_parent_session_id: String,
    /// 角色码
    pub run_role_code: String,
    /// 任务描述
    pub run_task: String,
    /// 结论正文（回灌给父会话的内容）
    pub run_conclusion: String,
    /// 是否失败
    pub run_failed: bool,
    /// 起建毫秒
    pub run_started_at: i64,
    /// 结束毫秒
    pub run_finished_at: i64,
}

/// 子代理清单请求。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct SubagentListREQ {
    /// 父会话 id
    pub req_parent_session_id: String,
}

/// 子代理活跃清单响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct SubagentActiveRESP {
    /// 正在跑的子代理
    pub resp_agents: Vec<SubagentRunVO>,
}

/// 子代理过程正文响应。
#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "snake_case")]
pub struct SubagentTranscriptVO {
    /// 子会话 id
    pub resp_session_id: String,
    /// 已累积的正文（正文与思考的拼接，供结束后回看）
    pub resp_transcript: String,
}
