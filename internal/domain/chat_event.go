package domain

// 聊天事件载荷契约（chat:*）：字段即前后端契约。
// 只定义「形状容易被写歪」的事件；流式增量等极简载荷仍用 map 直传（高频，零转换开销）。

// ChatDoneEvent run 终态载荷（chat:done）。正常收尾与失败收尾共用同一形状——
// 字段缺失一律零值，前端不必再为「同一事件两种形状」写防御式兜底。
type ChatDoneEvent struct {
	Status     string         `json:"status"`                // 完成状态：completed | failed
	Reason     string         `json:"reason"`                // 领域停止原因（MessageStopReason）
	StopReason string         `json:"stop_reason,omitempty"` // 上游归一化 stop 原因
	MessageID  string         `json:"message_id"`
	Usage      *ChatDoneUsage `json:"usage,omitempty"`
}

// ChatDoneUsage 终态用量（与 token_usages 明细同口径）。
type ChatDoneUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CacheRead    int `json:"cache_read_tokens"`
	CacheWrite   int `json:"cache_creation_tokens"`
	Total        int `json:"total_tokens"`
}

// ChatToolCallEvent 工具调用载荷（chat:tool）。
// 形状唯一的价值：arguments 是原始 JSON 字符串（不是对象），activity 由工具自述——
// 两处都曾因「有时是 string 有时是 object」让前端写防御式兜底。
type ChatToolCallEvent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Activity  string `json:"activity,omitempty"` // 工具自述动作（"正在编辑 app.ts"）
	Agent     string `json:"agent,omitempty"`    // 子 Agent 委派来源标签
}

// ChatToolResultEvent 工具结果载荷（chat:tool-result）。
// 三条语义互斥的状态位必须同时可读：error（故障）/ refused（策略拒绝，非故障）/ 其余为成功。
// meta 携带执行期元数据（cwd / same_failure_count / adaptive_hint / truncated_bytes）。
type ChatToolResultEvent struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Content       string            `json:"content,omitempty"`
	Error         string            `json:"error,omitempty"`
	DurationMs    int64             `json:"duration_ms,omitempty"`
	Agent         string            `json:"agent,omitempty"`
	UIHint        string            `json:"ui_hint,omitempty"`
	Data          map[string]any    `json:"data,omitempty"`
	Meta          map[string]string `json:"meta,omitempty"`
	Refused       bool              `json:"refused,omitempty"`
	RefusedReason string            `json:"refused_reason,omitempty"`
}

// ChatApprovalEvent 审批 / 补问载荷（chat:approval）。两种 kind 共用同一形状——
// 补问没有「本会话允许」语义（can_remember 零值 false），前端按此隐藏该选项，
// 不必再为「字段有时缺失」写防御式兜底。
type ChatApprovalEvent struct {
	ID          string `json:"id"`
	Command     string `json:"command"`     // 审批：可读命令；补问：问题正文
	Reason      string `json:"reason,omitempty"`
	Risk        string `json:"risk"`        // needs_approval | irreversible | input_required
	CanRemember bool   `json:"can_remember"` // 不可逆与补问恒 false
}

// ChatStatsEvent 本轮用量（chat:stats）。六个数值字段一旦漏发其一，
// 前端用量行就会出现「局部为 0」的假象——强类型是唯一能挡住它的手段。
type ChatStatsEvent struct {
	Turn                int   `json:"turn"`
	InputTokens         int   `json:"input_tokens"`
	OutputTokens        int   `json:"output_tokens"`
	CacheReadTokens     int   `json:"cache_read_tokens"`
	CacheCreationTokens int   `json:"cache_creation_tokens"`
	TotalTokens         int   `json:"total_tokens"`
	LatencyMs           int64 `json:"latency_ms"`
}

// ChatCompressedEvent 上下文压缩（chat:compressed）。
// FilterKey 是压缩器标识（内核只保留确定性折叠一种："micro"），供前端展示折叠来源。
type ChatCompressedEvent struct {
	RemovedMessages int    `json:"removed_messages"`
	Summary         string `json:"summary,omitempty"`
	Truncated       bool   `json:"truncated,omitempty"`
	FilterKey       string `json:"filter_key,omitempty"`
	CutoffAt        int64  `json:"cutoff_at,omitempty"`
}

// ChatWarnEvent 告警（chat:warn）。三种 kind 的字段集原本各不相同（三处 emit 各写一份），
// 合并为同一形状：缺失即零值，前端按 Kind 分支取用，不再需要防御式兜底。
type ChatWarnEvent struct {
	// Kind: unbacked_claim（声称产出文件但无变更记录）
	//     | file_out_of_sandbox（写到了 .workbaby/ 之外）
	//     | agent_model_override（子智能体覆盖了会话模型）
	Kind    string `json:"kind"`
	Message string `json:"message"`
	// 以下按 Kind 生效，其余为零值。
	RelPath      string `json:"rel_path,omitempty"`      // file_out_of_sandbox
	Path         string `json:"path,omitempty"`          // file_out_of_sandbox
	Workspace    string `json:"workspace,omitempty"`     // file_out_of_sandbox
	Sandbox      string `json:"sandbox,omitempty"`       // file_out_of_sandbox
	Agent        string `json:"agent,omitempty"`         // agent_model_override
	SessionModel string `json:"session_model,omitempty"` // agent_model_override
	Model        string `json:"model,omitempty"`         // agent_model_override
}

// ChatRetryEvent 建流瞬时错误退避重试载荷（chat:retry）。
// Attempt 从 1 起；DelayMs 是本次退避等待毫秒数（前端展示「正在重试 N/3」）。
type ChatRetryEvent struct {
	Attempt int   `json:"attempt"`
	DelayMs int64 `json:"delay_ms"`
}

// ChatErrorEvent run 内错误载荷（chat:error）。与 ChatDoneEvent 共用终态通道，
// 但此处是中途失败（建流失败、上游 chunk 错误等），前端仅展示横幅，不收尾。
type ChatErrorEvent struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ChatSubagentStartEvent 子 Agent 委派起始载荷（chat:subagent-start）。
// SubRunID 是子 run 的 runID（独立 SSE 通道）；Agent 是 Agent 定义名。
type ChatSubagentStartEvent struct {
	SubRunID string `json:"sub_run_id"`
	Agent    string `json:"agent,omitempty"`
}

// ChatSubagentDoneEvent 子 Agent 委派收尾载荷（chat:subagent-done）。
// Reason 走领域停止原因（MessageStopReason），与父 run 同口径。
type ChatSubagentDoneEvent struct {
	SubRunID string `json:"sub_run_id"`
	Agent    string `json:"agent,omitempty"`
	Reason   string `json:"reason"`
}

// ChatSubagentErrorEvent 子 Agent 委派中途错误载荷（chat:subagent-error）。
type ChatSubagentErrorEvent struct {
	SubRunID string `json:"sub_run_id"`
	Agent    string `json:"agent,omitempty"`
	Message  string `json:"message"`
}

// ChatQueueDrainedEvent steering / follow-up 队列取出可见性载荷（chat:queue-drained）。
// PI Phase 3：让前端 / 审计感知「用户消息已入队并被消费」。
// Queue: "steering" | "follow_up"；Count 是本次 drain 取出的消息数；Mode 是 QueueMode。
type ChatQueueDrainedEvent struct {
	Queue string `json:"queue"`
	Count int    `json:"count"`
	Mode  string `json:"mode,omitempty"`
	Turn  int    `json:"turn"`
}
