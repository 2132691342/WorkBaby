package domain

// 会话落盘形态的唯一真相：引擎写、回放读，两边共用这一份结构。
// 格式一旦分叉，界面就会出现「引擎明明写了，历史却是空的」——
// 所以这里只定义一次，agent 与 parser 都按它读写。

// SessionEntryType 是会话条目的种类。
type SessionEntryType string

const (
	// SessionEntryMessage 一条对话消息
	SessionEntryMessage SessionEntryType = "message"
	// SessionEntrySessionInfo 会话改名
	SessionEntrySessionInfo SessionEntryType = "session_info"
	// SessionEntryModelChange 切换模型
	SessionEntryModelChange SessionEntryType = "model_change"
	// SessionEntryThinkingChange 切换思考档
	SessionEntryThinkingChange SessionEntryType = "thinking_level_change"
	// SessionEntryUsage 一次独立计费
	SessionEntryUsage SessionEntryType = "usage"
	// SessionEntryCompaction 上下文压缩
	SessionEntryCompaction SessionEntryType = "compaction"
	// SessionEntryCustom 引擎的自定义条目
	SessionEntryCustom SessionEntryType = "custom"
)

// SessionContentType 是消息内容块的种类。
type SessionContentType string

const (
	// SessionContentText 正文
	SessionContentText SessionContentType = "text"
	// SessionContentThinking 思考过程
	SessionContentThinking SessionContentType = "thinking"
	// SessionContentToolCall 模型要求的工具调用
	SessionContentToolCall SessionContentType = "toolCall"
	// SessionContentImage 图片
	SessionContentImage SessionContentType = "image"
)

// SessionBlockDO 是消息里的一个内容块。
// 一个块只填自己那一族的字段：跨块种类取字段的接口会把它们拆散。
type SessionBlockDO struct {
	// BlockType 块种类
	BlockType SessionContentType `json:"type"`
	// BlockText 正文或思考文本
	BlockText string `json:"text,omitempty"`
	// BlockThinking 思考文本
	BlockThinking string `json:"thinking,omitempty"`
	// BlockId 工具调用 ID
	BlockId string `json:"id,omitempty"`
	// BlockName 工具名
	BlockName string `json:"name,omitempty"`
	// BlockArguments 工具入参
	BlockArguments any `json:"arguments,omitempty"`
	// BlockMime 图片 MIME
	BlockMime string `json:"mime,omitempty"`
	// BlockData 图片内容（base64）
	BlockData string `json:"data,omitempty"`
}

// SessionUsageDO 是一次调用的 token 用量。
type SessionUsageDO struct {
	// UsageInput 输入 token
	UsageInput int `json:"input"`
	// UsageOutput 输出 token
	UsageOutput int `json:"output"`
	// UsageCacheRead 命中缓存的输入 token
	UsageCacheRead int `json:"cacheRead"`
	// UsageCacheWrite 写入缓存的输入 token
	UsageCacheWrite int `json:"cacheWrite"`
	// UsageTotalTokens token 合计
	UsageTotalTokens int `json:"totalTokens"`
	// UsageCost 花费明细（美元）
	UsageCost *SessionCostDO `json:"cost,omitempty"`
}

// SessionCostDO 是花费明细。
type SessionCostDO struct {
	// CostTotal 合计（美元）
	CostTotal float64 `json:"total"`
}

// SessionMessageDO 是一条对话消息。
type SessionMessageDO struct {
	// MessageRole user / assistant / toolResult
	MessageRole string `json:"role"`
	// MessageContent 正文：字符串或内容块数组
	MessageContent any `json:"content"`
	// MessageModel 产生这条的模型
	MessageModel string `json:"model,omitempty"`
	// MessageProvider 产生这条的服务商
	MessageProvider string `json:"provider,omitempty"`
	// MessageUsage 这条的用量
	MessageUsage *SessionUsageDO `json:"usage,omitempty"`
	// MessageStopReason 停止原因；error 表示模型调用失败
	MessageStopReason string `json:"stopReason,omitempty"`
	// MessageErrorMessage 失败原话
	MessageErrorMessage string `json:"errorMessage,omitempty"`
	// MessageToolCallId 工具结果对应的调用 ID
	MessageToolCallId string `json:"toolCallId,omitempty"`
	// MessageToolName 工具结果对应的工具名
	MessageToolName string `json:"toolName,omitempty"`
	// MessageIsError 工具结果是否失败
	MessageIsError bool `json:"isError,omitempty"`
	// MessageDetails 工具结果的附加信息（diff 在这里）
	MessageDetails map[string]any `json:"details,omitempty"`
}

// SessionEntryDO 是会话文件里的一条条目。
type SessionEntryDO struct {
	// EntryType 条目种类
	EntryType SessionEntryType `json:"type"`
	// EntryId 条目 ID（活动分支与隐藏台账的稳定光标）
	EntryId string `json:"id"`
	// EntryParentId 父条目；树形回放靠它
	EntryParentId string `json:"parentId"`
	// EntryTimestamp 条目时刻（ISO 串，UTC 毫秒精度）
	EntryTimestamp string `json:"timestamp"`
	// EntryMessage 消息条目才有
	EntryMessage *SessionMessageDO `json:"message,omitempty"`
	// EntryName 改名条目的新名字
	EntryName string `json:"name,omitempty"`
	// EntryProvider 模型切换条目的服务商
	EntryProvider string `json:"provider,omitempty"`
	// EntryModelId 模型切换条目的模型
	EntryModelId string `json:"modelId,omitempty"`
	// EntryThinkingLevel 思考档条目的新档位
	EntryThinkingLevel string `json:"thinkingLevel,omitempty"`
	// EntryTokensBefore 压缩前的 token 数
	EntryTokensBefore int `json:"tokensBefore,omitempty"`
	// EntryUsage 用量条目
	EntryUsage *SessionUsageDO `json:"usage,omitempty"`
	// EntryCustomType 自定义条目的种类
	EntryCustomType string `json:"customType,omitempty"`
	// EntryData 自定义条目的载荷
	EntryData map[string]any `json:"data,omitempty"`
}

// SessionHeaderDO 是会话文件的首行。
type SessionHeaderDO struct {
	// HeaderType 恒为 session
	HeaderType string `json:"type"`
	// HeaderVersion 格式版本
	HeaderVersion int `json:"version"`
	// HeaderId 会话 ID
	HeaderId string `json:"id"`
	// HeaderTimestamp 创建时刻（ISO 串）
	HeaderTimestamp string `json:"timestamp"`
	// HeaderCwd 绑定的工作目录
	HeaderCwd string `json:"cwd"`
	// HeaderTitle 会话标题
	HeaderTitle string `json:"title,omitempty"`
}
