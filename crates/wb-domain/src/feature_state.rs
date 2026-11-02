//! 功能活状态的读取视图：待办 / 目标 / 计划 / 记忆。
//! 这些数据由 wb-features 落盘，service 读成 VO 给界面，界面不再解析 widget 文本。

use serde::{Deserialize, Serialize};

/// 单条待办。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct TodoItemVO {
    /// 条目 id（TODO_{ULID}）
    pub todo_id: String,
    /// 展示序号（清单内 1 起稠密编号）
    pub todo_seq: u32,
    /// 内容
    pub todo_text: String,
    /// pending / in_progress / completed
    pub todo_status: String,
    /// 依赖的条目 id
    pub todo_blocked_by: Vec<String>,
    /// 创建时间（毫秒）
    pub todo_created_at: i64,
    /// 更新时间（毫秒）
    pub todo_updated_at: i64,
}

/// 待办清单响应。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct TodoListRESP {
    /// 全部条目（按序号升序）
    pub resp_items: Vec<TodoItemVO>,
    /// 已完成条数
    pub resp_done_count: usize,
}

/// 目标的一个步骤。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GoalStepVO {
    /// 步骤内容
    pub step_text: String,
    /// 是否已完成
    pub step_done: bool,
}

/// 目标视图。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GoalVO {
    /// 目标 id（GOAL_ 前缀）
    pub goal_id: String,
    /// 目标描述
    pub goal_text: String,
    /// active / blocked / achieved / abandoned
    pub goal_status: String,
    /// 最近进展备注
    pub goal_note: String,
    /// 步骤清单
    pub goal_steps: Vec<GoalStepVO>,
    /// 已完成步骤数
    pub goal_done_steps: usize,
    /// 创建时间（毫秒）
    pub goal_created_at: i64,
    /// 更新时间（毫秒）
    pub goal_updated_at: i64,
}

/// 目标响应：没有目标时 `resp_goal` 为 None（不是错误）。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GoalRESP {
    /// 当前目标
    pub resp_goal: Option<GoalVO>,
    /// 连续自动续跑次数
    pub resp_resume_count: i64,
}

/// 计划的一个步骤。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct PlanStepVO {
    /// 步骤内容
    pub step_text: String,
    /// 是否已完成
    pub step_done: bool,
}

/// 用户对计划的一条标注。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct PlanAnnotationVO {
    /// 标注 id（ANN_ 前缀）
    pub annotation_id: String,
    /// step / plan
    pub annotation_scope: String,
    /// 关联步骤序号（scope=step 时有效，1 起）
    pub annotation_step: i64,
    /// 标注内容
    pub annotation_text: String,
    /// user / assistant
    pub annotation_author: String,
    /// 创建时间（毫秒）
    pub annotation_created_at: i64,
}

/// 计划视图。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct PlanVO {
    /// 计划 id（PLAN_ 前缀）
    pub plan_id: String,
    /// 计划标题
    pub plan_title: String,
    /// draft / approved / rejected / done
    pub plan_status: String,
    /// 驳回意见（approved 后为空）
    pub plan_reject_note: String,
    /// 计划步骤
    pub plan_steps: Vec<PlanStepVO>,
    /// 用户标注
    pub plan_annotations: Vec<PlanAnnotationVO>,
    /// 已完成步骤数
    pub plan_done_steps: usize,
    /// 更新时间（毫秒）
    pub plan_updated_at: i64,
}

/// 计划响应：没有计划时 `resp_plan` 为 None。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct PlanRESP {
    /// 当前计划
    pub resp_plan: Option<PlanVO>,
}

/// 一条持久记忆。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct MemoryEntryVO {
    /// 记忆 id（MEM_ 前缀）
    pub memory_id: String,
    /// memory / user / project / failure
    pub memory_kind: String,
    /// 正文
    pub memory_text: String,
    /// 创建时间（毫秒）
    pub memory_created_at: i64,
    /// 更新时间（毫秒）
    pub memory_updated_at: i64,
}

/// 记忆清单响应。
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct MemoryListRESP {
    /// 条目（按更新时间倒序）
    pub resp_entries: Vec<MemoryEntryVO>,
    /// 四个类别各自的条数（kind → count）
    pub resp_counts: Vec<MemoryKindCountVO>,
    /// 总条数
    pub resp_total: i64,
}

/// 某个类别的记忆条数。
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct MemoryKindCountVO {
    /// 类别名
    pub kind_name: String,
    /// 条数
    pub kind_count: i64,
}

/// 记忆删除请求。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct MemoryDeleteREQ {
    /// 要删除的记忆 id
    pub memory_id: String,
}

/// 待办写入请求（界面上直接增删改，与模型走的 todo 工具同一份落盘）。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct TodoMutateREQ {
    /// 动作：add / update / status / remove
    pub todo_op: String,
    /// 目标条目 id（add 时可空）
    pub todo_id: String,
    /// add / update 的正文
    pub todo_text: String,
    /// status 动作的目标状态：pending / in_progress / completed
    pub todo_status: String,
    /// 依赖条目 id（add / update 时设置）
    pub todo_blocked_by: Vec<String>,
}

/// 目标写入请求：切状态 / 勾选步骤 / 记录进展 / 新建 / 放弃。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct GoalMutateREQ {
    /// 动作：create / status / done_steps / note / abandon
    pub goal_op: String,
    /// create 时的目标描述
    pub goal_text: String,
    /// create 时的步骤清单
    pub goal_steps: Vec<String>,
    /// status 动作的目标状态：active / blocked / achieved / abandoned
    pub goal_status: String,
    /// done_steps 动作要勾选完成的步骤序号（1 起）
    pub goal_done_steps: Vec<i64>,
    /// note 动作的进展备注
    pub goal_note: String,
}

/// 计划写入请求：批准 / 驳回 / 勾选步骤 / 标注。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct PlanMutateREQ {
    /// 动作：approve / reject / step_done / annotate
    pub plan_op: String,
    /// reject 时的驳回意见
    pub plan_reject_note: String,
    /// step_done 动作要勾选完成的步骤序号（1 起）
    pub plan_step_index: i64,
    /// annotate 的标注对象：plan / step
    pub annotation_scope: String,
    /// annotate 且 scope=step 时的步骤序号（1 起）
    pub annotation_step: i64,
    /// annotate 的标注内容
    pub annotation_text: String,
}

/// 记忆写入请求：界面新增一条（模型走 memory_save 工具）。
#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "snake_case")]
pub struct MemorySaveREQ {
    /// 类别：memory / user / project / failure
    pub memory_kind: String,
    /// 正文
    pub memory_text: String,
}
