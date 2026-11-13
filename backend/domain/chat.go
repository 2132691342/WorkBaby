package domain

// BlockKind 是对话内容块的种类。
type BlockKind string

const (
	// BlockText 正文
	BlockText BlockKind = "text"
	// BlockThinking 思考过程
	BlockThinking BlockKind = "thinking"
	// BlockTool 工具调用
	BlockTool BlockKind = "tool"
)

// ChatBlockVO 是对话里的一个内容块。
type ChatBlockVO struct {
	// BlockKind 块种类
	BlockKind BlockKind `json:"block_kind"`
	// BlockText 文本内容（text / thinking 用）
	BlockText string `json:"block_text,omitempty"`
	// ToolCallId 工具调用 ID
	ToolCallId string `json:"tool_call_id,omitempty"`
	// ToolName 工具名
	ToolName string `json:"tool_name,omitempty"`
	// ToolArgs 工具入参（原样透传，形状由工具决定）
	ToolArgs any `json:"tool_args,omitempty"`
	// ToolOutput 工具输出
	ToolOutput string `json:"tool_output,omitempty"`
	// ToolStatus pending / running / done / error
	ToolStatus string `json:"tool_status,omitempty"`
	// ToolIsError 工具是否失败
	ToolIsError bool `json:"tool_is_error,omitempty"`
	// DurationMs 耗时毫秒
	DurationMs int `json:"duration_ms,omitempty"`
	// FilePath 被改动的文件路径
	FilePath string `json:"file_path,omitempty"`
	// DiffLines 差异行
	DiffLines []DiffLineVO `json:"diff_lines,omitempty"`
}

// DiffLineVO 是一行差异。两种来源（工具结果 / git）归一成这一种。
type DiffLineVO struct {
	// LineKind file / hunk / add / del / ctx
	LineKind string `json:"line_kind"`
	// OldNo 旧文件行号；0 表示不适用
	OldNo int `json:"old_no"`
	// NewNo 新文件行号；0 表示不适用
	NewNo int `json:"new_no"`
	// LineText 行内容（含前缀符号）
	LineText string `json:"line_text"`
}

// TurnUsageVO 是一条回复的用量读数。
type TurnUsageVO struct {
	// InputTokens 输入 token
	InputTokens int `json:"input_tokens"`
	// OutputTokens 输出 token
	OutputTokens int `json:"output_tokens"`
	// CacheReadTokens 缓存命中 token
	CacheReadTokens int `json:"cache_read_tokens"`
	// CacheWriteTokens 写入缓存 token
	CacheWriteTokens int `json:"cache_write_tokens"`
	// CostUsd 本条花费（美元）
	CostUsd float64 `json:"cost_usd"`
}

// HistoryTurnVO 是回放出来的一轮。Role 为 engine-mark 时是壳插的分隔标记。
type HistoryTurnVO struct {
	// TurnRole user / assistant / tool / engine-mark
	TurnRole string `json:"turn_role"`
	// TurnBlocks 这一轮的内容块
	TurnBlocks []ChatBlockVO `json:"turn_blocks"`
	// TurnModel 产生这一轮的模型
	TurnModel string `json:"turn_model,omitempty"`
	// TurnUsage 这一轮的用量
	TurnUsage *TurnUsageVO `json:"turn_usage,omitempty"`
	// TurnId 会话条目 ID（隐藏一轮时回传）
	TurnId string `json:"turn_id,omitempty"`
}

// ContextUsageVO 是上下文窗口占用。
type ContextUsageVO struct {
	// UsageTokens 已用 token；null 表示压缩后还没有可靠读数
	UsageTokens *int `json:"usage_tokens"`
	// ContextWindow 窗口大小
	ContextWindow int `json:"context_window"`
	// UsagePercent 占用百分比；null 同 UsageTokens
	UsagePercent *float64 `json:"usage_percent"`
}

// SessionStatsVO 是会话累计用量。
type SessionStatsVO struct {
	// SessionId 引擎侧会话 ID
	SessionId string `json:"session_id"`
	// UserMessages 用户消息数
	UserMessages int `json:"user_messages"`
	// AssistantMessages 助手消息数
	AssistantMessages int `json:"assistant_messages"`
	// ToolCalls 工具调用数
	ToolCalls int `json:"tool_calls"`
	// TotalMessages 消息总数
	TotalMessages int `json:"total_messages"`
	// InputTokens 输入 token 合计
	InputTokens int `json:"input_tokens"`
	// OutputTokens 输出 token 合计
	OutputTokens int `json:"output_tokens"`
	// CacheReadTokens 缓存命中合计
	CacheReadTokens int `json:"cache_read_tokens"`
	// CacheWriteTokens 缓存写入合计
	CacheWriteTokens int `json:"cache_write_tokens"`
	// TotalTokens token 合计
	TotalTokens int `json:"total_tokens"`
	// CostUsd 花费合计（美元）
	CostUsd float64 `json:"cost_usd"`
	// ContextUsage 上下文占用
	ContextUsage *ContextUsageVO `json:"context_usage,omitempty"`
}

// QueuedMessagesVO 是排队中的两条队。
type QueuedMessagesVO struct {
	// Steering 插话队
	Steering []string `json:"steering"`
	// FollowUp 追加队
	FollowUp []string `json:"follow_up"`
}

// TodoVO 是待办面板上的一条任务。
type TodoVO struct {
	// TodoId 任务号（待办工具侧的自增 ID）
	TodoId int `json:"todo_id"`
	// TodoSubject 一句话说明
	TodoSubject string `json:"todo_subject"`
	// TodoStatus pending / in_progress / completed
	TodoStatus string `json:"todo_status"`
	// TodoActiveForm 进行中的现在进行时描述（如「正在跑测试」）
	TodoActiveForm string `json:"todo_active_form,omitempty"`
}

// ModelInfoVO 是当前模型的定义。
type ModelInfoVO struct {
	// ModelId 模型 ID
	ModelId string `json:"model_id"`
	// ModelName 显示名
	ModelName string `json:"model_name"`
	// ProviderName 服务商
	ProviderName string `json:"provider_name"`
	// ApiProtocol 协议
	ApiProtocol string `json:"api_protocol"`
	// SupportsReasoning 是否支持思考
	SupportsReasoning bool `json:"supports_reasoning"`
	// ContextWindow 上下文窗口
	ContextWindow int `json:"context_window"`
	// MaxTokens 单次输出上限
	MaxTokens int `json:"max_tokens"`
	// InputKinds 支持的输入种类（text / image）
	InputKinds []string `json:"input_kinds,omitempty"`
}

// SlashCommandVO 是一条斜杠命令。
type SlashCommandVO struct {
	// CommandName 命令名（用 /name 触发）
	CommandName string `json:"command_name"`
	// CommandSource builtin / prompt / skill
	CommandSource string `json:"command_source"`
	// CommandDesc 一句话说明
	CommandDesc string `json:"command_desc,omitempty"`
}

// ChatStateRESP 是聊天页的首屏总装：引擎实况 + 会话文件回放。
type ChatStateRESP struct {
	// EngineKind 引擎种类
	EngineKind EngineKind `json:"engine_kind"`
	// SessionFile 当前会话文件
	SessionFile string `json:"session_file"`
	// WorkDir 绑定到的工作区
	WorkDir string `json:"work_dir"`
	// EngineRunning 子进程是否存活
	EngineRunning bool `json:"engine_running"`
	// Streaming 是否正在流式输出
	Streaming bool `json:"streaming"`
	// Model 当前模型
	Model *ModelInfoVO `json:"model,omitempty"`
	// ThinkingLevel 当前思考档
	ThinkingLevel string `json:"thinking_level"`
	// ThinkingLevels 可用思考档
	ThinkingLevels []string `json:"thinking_levels"`
	// ContextUsage 上下文占用
	ContextUsage *ContextUsageVO `json:"context_usage,omitempty"`
	// AutoCompaction 自动压缩是否开启
	AutoCompaction bool `json:"auto_compaction"`
	// Queued 排队消息
	Queued QueuedMessagesVO `json:"queued"`
	// Stats 累计用量
	Stats *SessionStatsVO `json:"stats,omitempty"`
	// History 回放出来的历史轮
	History []HistoryTurnVO `json:"history"`
	// Todos 当前待办（活动分支上最近一次 todo 工具调用派生）
	Todos []TodoVO `json:"todos"`
	// PlanMode 是否处于计划模式（会话文件里的开关状态）
	PlanMode bool `json:"plan_mode"`
	// PlanState 计划模式的细节：被拦下的工具清单
	PlanState *PlanStateVO `json:"plan_state,omitempty"`
	// Commands 可用斜杠命令
	Commands []SlashCommandVO `json:"commands"`
	// Caps 能力表
	Caps EngineCaps `json:"caps"`
	// BindErr 绑定失败原因；空表示绑定成功
	BindErr string `json:"bind_err,omitempty"`
}

// PlanStateVO 是计划模式在界面上的状态。
type PlanStateVO struct {
	// PlanEnabled 是否处于只读计划模式
	PlanEnabled bool `json:"plan_enabled"`
	// BlockedTools 当前被拦下的工具名
	BlockedTools []string `json:"blocked_tools"`
}

// ChatPlanREQ 是开关计划模式的入参。
type ChatPlanREQ struct {
	// SessionFile 目标会话；空表示当前会话
	SessionFile string `json:"session_file"`
	// PlanEnabled 目标状态
	PlanEnabled bool `json:"plan_enabled"`
}

// ChatPlanRESP 是计划模式开关的结果。
type ChatPlanRESP struct {
	// PlanEnabled 生效后的状态
	PlanEnabled bool `json:"plan_enabled"`
	// BlockedTools 被拦下的工具名
	BlockedTools []string `json:"blocked_tools"`
	// PlanMessage 给界面看的一句话
	PlanMessage string `json:"plan_message"`
}

// ChatAttachmentVO 是一个附件。两类走两条完全不同的路：
// file 只把路径折进 prompt 让引擎自己读；image 走 images 字段（唯一的二进制通道）。
type ChatAttachmentVO struct {
	// AttachmentKind file / image
	AttachmentKind string `json:"attachment_kind"`
	// FilePath 文件绝对路径（file 类）
	FilePath string `json:"file_path,omitempty"`
	// FileName 文件名
	FileName string `json:"file_name,omitempty"`
	// ImageBase64 图片内容（image 类）
	ImageBase64 string `json:"image_base64,omitempty"`
	// ImageMime 图片 MIME
	ImageMime string `json:"image_mime,omitempty"`
}

// ChatSendREQ 是发消息 / 插话 / 追加的共用入参。
type ChatSendREQ struct {
	// SessionFile 目标会话；空表示当前会话
	SessionFile string `json:"session_file"`
	// Message 话术
	Message string `json:"message"`
	// Attachments 附件
	Attachments []ChatAttachmentVO `json:"attachments,omitempty"`
}

// ChatSendRESP 是发消息的出参。
type ChatSendRESP struct {
	// RunId 本次运行 ID；没有运行（命令当场消费 / 排队）时为空串
	RunId string `json:"run_id"`
	// Disposition 引擎对这句话的处理方式：started / queued / handled
	Disposition string `json:"disposition"`
}

// 引擎对 prompt 的三种处理方式。started 才真有一次运行；
// queued 表示排队等前一轮结束；handled 表示被斜杠命令当场消费掉，不会有运行。
const (
	// DispositionStarted 引擎受理并起了一轮
	DispositionStarted = "started"
	// DispositionQueued 引擎排队等前一轮结束
	DispositionQueued = "queued"
	// DispositionHandled 被斜杠命令当场处理，没有运行
	DispositionHandled = "handled"
)

// ChatForkREQ 是从某一轮用户消息分叉出新会话的入参。
// 分叉是「以这条用户消息为起点开一份新会话文件」，原会话一个字节不动。
type ChatForkREQ struct {
	// SessionFile 目标会话
	SessionFile string `json:"session_file"`
	// TurnId 作为分叉起点的那条用户消息条目 ID
	TurnId string `json:"turn_id"`
}

// ChatForkRESP 是分叉结果。
type ChatForkRESP struct {
	// SessionFile 新会话文件路径
	SessionFile string `json:"session_file"`
	// ForkText 分叉后预填进输入框的那句话
	ForkText string `json:"fork_text,omitempty"`
}

// ChatSessionREQ 是带会话文件的操作入参。
type ChatSessionREQ struct {
	// SessionFile 目标会话
	SessionFile string `json:"session_file"`
}

// ChatCompactREQ 是手动压缩的入参。
type ChatCompactREQ struct {
	// SessionFile 目标会话
	SessionFile string `json:"session_file"`
	// Instructions 压缩指示
	Instructions string `json:"instructions"`
}

// ChatModelREQ 是切模型的入参。
type ChatModelREQ struct {
	// SessionFile 目标会话
	SessionFile string `json:"session_file"`
	// ProviderName 服务商
	ProviderName string `json:"provider_name"`
	// ModelId 模型 ID
	ModelId string `json:"model_id"`
}

// ChatThinkingREQ 是切思考档的入参。
type ChatThinkingREQ struct {
	// SessionFile 目标会话
	SessionFile string `json:"session_file"`
	// ThinkingLevel off / minimal / low / medium / high
	ThinkingLevel string `json:"thinking_level"`
}

// ChatAutoCompactREQ 是自动压缩开关的入参。
type ChatAutoCompactREQ struct {
	// SessionFile 目标会话
	SessionFile string `json:"session_file"`
	// AutoCompaction 目标状态
	AutoCompaction bool `json:"auto_compaction"`
}

// AskRequestVO 是引擎推上来的一张提问卡。
type AskRequestVO struct {
	// AskId 提问 ID（回答时原样带回）
	AskId string `json:"ask_id"`
	// AskMethod confirm / select / input / editor
	AskMethod string `json:"ask_method"`
	// AskTitle 标题
	AskTitle string `json:"ask_title,omitempty"`
	// AskMessage 正文
	AskMessage string `json:"ask_message,omitempty"`
	// AskPlaceholder 输入占位
	AskPlaceholder string `json:"ask_placeholder,omitempty"`
	// AskPrefill 预填内容
	AskPrefill string `json:"ask_prefill,omitempty"`
	// AskOptions 可选项
	AskOptions []string `json:"ask_options,omitempty"`
	// AskTimeoutMs 超时毫秒
	AskTimeoutMs int `json:"ask_timeout_ms,omitempty"`
}

// AnswerItemVO 是一道结构性问卷的回答。
type AnswerItemVO struct {
	// QuestionId 题目 ID
	QuestionId string `json:"question_id"`
	// SelectedOptions 选中的选项
	SelectedOptions []string `json:"selected_options"`
	// CustomInput 自由输入
	CustomInput string `json:"custom_input,omitempty"`
}

// ChatAnswerREQ 是回答提问板的入参。
type ChatAnswerREQ struct {
	// SessionFile 目标会话
	SessionFile string `json:"session_file"`
	// AskId 提问 ID
	AskId string `json:"ask_id"`
	// Value 输入类回答
	Value string `json:"value,omitempty"`
	// Confirmed 确认类回答
	Confirmed *bool `json:"confirmed,omitempty"`
	// Cancelled 是否取消
	Cancelled bool `json:"cancelled,omitempty"`
	// Answers 问卷类回答
	Answers []AnswerItemVO `json:"answers,omitempty"`
}

// ChatExportRESP 是会话 HTML 导出的出参：自包含单文件，前端用 Blob 下载。
type ChatExportRESP struct {
	// FileName 建议文件名
	FileName string `json:"file_name"`
	// FileContent HTML 全文
	FileContent string `json:"file_content"`
}

// MessageHideREQ 是隐藏 / 取消隐藏一轮的入参。
type MessageHideREQ struct {
	// SessionFile 目标会话
	SessionFile string `json:"session_file"`
	// TurnId 会话条目 ID
	TurnId string `json:"turn_id"`
}

// ChatRemoveQueuedREQ 是单条移除排队消息的入参。
type ChatRemoveQueuedREQ struct {
	// QueueKind steering / followup
	QueueKind string `json:"queue_kind"`
	// QueueIndex 队内位置（从 0 起）
	QueueIndex int `json:"queue_index"`
}
