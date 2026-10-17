// 观测面：运行记录、检查点、评估与回归（specs/11 / 15）。字段照后端 json tag。

/** 一次 run 的账；正文不在这里，正文在消息里。 */
export interface RunVO {
  id: string
  session_id: string
  session_title: string
  provider_id: string
  model: string
  /** running / done / failed / aborted */
  status: string
  /** completed / length / max_turns / aborted / error */
  stop_reason: string
  turns: number
  tokens: number
  input: number
  output: number
  cached: number
  context: number
  max_tokens: number
  compressed: number
  started_at: number
  finished_at: number
  duration_ms: number
  can_resume: boolean
}

/** 轮末上下文快照：回答「第几轮时窗口里有多少」。 */
export interface ContextSnapshotDO {
  id: string
  run_id: string
  session_id: string
  turn: number
  used: number
  window: number
  compressed: number
  created_at: number
}

/** 一次运行前打的位点：回到它就是移动会话叶子，不删条目。 */
export interface CheckpointVO {
  id: string
  session_id: string
  run_id: string
  leaf_entry_id: string
  turn: number
  prompt: string
  created_at: number
}

export interface RunDetailVO extends RunVO {
  timeline?: RunTimeline
  snapshots: ContextSnapshotDO[]
  checkpoints: CheckpointVO[]
}

/** 执行时间线：一次 run 里工具动作与正文长度。 */
export interface RunStep {
  kind: string
  name: string
  label: string
  at: number
  ms: number
  ok: boolean
  detail: string
}

export interface RunTimeline {
  chars: number
  steps: RunStep[]
}

/** 归档结果：只丢逐字对话，留结论 / 时间线 / 用量账。 */
export interface ArchiveVO {
  retention_days: number
  sessions: number
  detached_runs: number
}

/** 评估指标：自动采集，界面与回归对比都读它。 */
export interface EvalMetrics {
  input_tokens: number
  output_tokens: number
  cached_tokens: number
  latency_ms: number
  tool_calls: number
  tool_failures: number
  approvals: number
  truncated: boolean
}

export interface EvalVO {
  id: string
  run_id: string
  /** session（对话级） */
  scope: string
  scope_id: string
  score: number
  note: string
  metrics: EvalMetrics
  created_at: number
  score_delta: number
  total_delta: number
  orphaned: boolean
}

export interface BaselineVO {
  id: string
  eval_id: string
  scope: string
  scope_id: string
  score: number
  created_at: number
}

export interface CompareVO {
  baseline_id: string
  run_id: string
  score_delta: number
  total_delta: number
  base_summary: string
  run_summary: string
  base_metrics: EvalMetrics
  run_metrics: EvalMetrics
}
