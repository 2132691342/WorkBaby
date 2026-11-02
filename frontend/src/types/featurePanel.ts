// 功能活状态与配置的类型：待办 / 目标 / 计划 / 记忆、功能配置字段表、内置运行时。
// 从 api.ts 拆出：这些类型随功能面板一起演进，与消息/会话模型是两套生命周期。
// api.ts 末尾重新导出，调用方仍从 ../types/api 取。

// ---- 功能活状态（待办 / 目标 / 计划 / 记忆） ----

export interface TodoItemVO {
  todo_id: string
  todo_seq: number
  todo_text: string
  /** pending / in_progress / completed */
  todo_status: string
  todo_blocked_by: string[]
  todo_created_at: number
  todo_updated_at: number
}

export interface TodoListRESP {
  resp_items: TodoItemVO[]
  resp_done_count: number
}

export interface GoalStepVO {
  step_text: string
  step_done: boolean
}

export interface GoalVO {
  goal_id: string
  goal_text: string
  /** active / blocked / achieved / abandoned */
  goal_status: string
  goal_note: string
  goal_steps: GoalStepVO[]
  goal_done_steps: number
  goal_created_at: number
  goal_updated_at: number
}

export interface GoalRESP {
  /** 没有目标时为 null */
  resp_goal: GoalVO | null
  resp_resume_count: number
}

export interface PlanStepVO {
  step_text: string
  step_done: boolean
}

export interface PlanAnnotationVO {
  annotation_id: string
  /** step / plan */
  annotation_scope: string
  annotation_step: number
  annotation_text: string
  /** user / assistant */
  annotation_author: string
  annotation_created_at: number
}

export interface PlanVO {
  plan_id: string
  plan_title: string
  /** draft / approved / rejected / done */
  plan_status: string
  plan_reject_note: string
  plan_steps: PlanStepVO[]
  plan_annotations: PlanAnnotationVO[]
  plan_done_steps: number
  plan_updated_at: number
}

export interface PlanRESP {
  /** 没有计划时为 null */
  resp_plan: PlanVO | null
}

export interface MemoryEntryVO {
  memory_id: string
  /** memory / user / project / failure */
  memory_kind: string
  memory_text: string
  memory_created_at: number
  memory_updated_at: number
}

export interface MemoryKindCountVO {
  kind_name: string
  kind_count: number
}

export interface MemoryListRESP {
  resp_entries: MemoryEntryVO[]
  resp_counts: MemoryKindCountVO[]
  resp_total: number
}

// ---- 功能配置 ----

export interface FeatureConfigOptionVO {
  option_value: string
  option_label: string
  /** 不可用原因（缺密钥 / 未安装）；空 = 可选 */
  option_reason: string
}

export interface FeatureConfigFieldVO {
  field_name: string
  field_label: string
  field_description: string
  /** text / number / boolean / select / multi_select */
  field_kind: string
  field_value: unknown
  field_options: FeatureConfigOptionVO[]
}

export interface FeatureConfigRESP {
  resp_feature_code: string
  resp_configurable: boolean
  resp_fields: FeatureConfigFieldVO[]
}

// ---- 内置运行时 ----

export interface RuntimeBinaryVO {
  binary_name: string
  binary_ready: boolean
  binary_path: string
}

export interface RuntimeStatusRESP {
  resp_root: string
  resp_binaries: RuntimeBinaryVO[]
  resp_ready: boolean
  resp_failure: string
}
