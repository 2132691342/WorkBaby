package domain

// 命令来源：内置 / 设置页创建（user_commands 表）/ 用户级文件（{home}/commands/*.md）
// / 工作区级文件（<ws>/.workbaby/commands/*.md）。
const (
	CommandSourceBuiltin   = "builtin"
	CommandSourceUser      = "user"
	CommandSourceFile      = "file"
	CommandSourceWorkspace = "workspace"
)

// SlashCommand 斜杠命令元数据（命令面板）。
//
// 真相源在后端：前端命令面板拉一次即渲染，新增命令不必改前端代码。
// ClientOnly=true 的命令由前端就地执行（开面板/切模型/导出/灌入模板），后端只提供元数据。
type SlashCommand struct {
	Name       string `json:"name"`        // 不含斜杠，如 "compact"
	Args       string `json:"args"`        // 参数提示，无参数为空串
	Desc       string `json:"desc"`        // 一句话说明（面板副标题）
	Group      string `json:"group"`       // 分组：session / model / agent / system / custom
	ClientOnly bool   `json:"client_only"` // 纯前端执行，无需后端接口
	// Prompt 自定义命令的提示词模板（仅 group=custom；选中即灌入输入框）。
	// 模板中 `$ARGUMENTS` 为全部参数、`$1`..`$9` 为位置参数，由发送前展开。
	Prompt string `json:"prompt,omitempty"`
	// Source 命令来源：builtin / user / file，供设置页与面板区分展示。
	Source string `json:"source,omitempty"`
}

// CommandListRESP 命令列表出参。
type CommandListRESP struct {
	Items []SlashCommand `json:"items"`
	Total int            `json:"total"`
}

// CompactREQ 手动压缩入参（/compact）。
//
// Instructions 是「保留指示」：压缩会折叠早期推理与工具结果，用户可指定压缩后仍需
// 留在上下文里的要点（/compact <instructions> 指令）。
// KeepRecent 覆盖默认保留窗口（<=0 用后端默认值）。
type CompactREQ struct {
	Instructions string `json:"instructions,omitempty"`
	KeepRecent   int    `json:"keep_recent,omitempty"`
}

// CompactResultRESP 会话压缩结果出参。
type CompactResultRESP struct {
	SessionID   string `json:"session_id"`
	Compacted   int    `json:"compacted"`   // 被压缩的消息条数
	FreedChars  int    `json:"freed_chars"` // 释放的字符数
	KeptRecent  int    `json:"kept_recent"` // 保留不动的最近条数
	TotalBefore int    `json:"total_before"`
	Pinned      bool   `json:"pinned"`       // 保留指示是否已钉进上下文（Instructions 非空且落库成功）
	FreedTokens int    `json:"freed_tokens"` // 估算释放的 token（rune/4 近似）
	Failed      int    `json:"failed"`       // 单条更新失败数（>0 表示部分成功）
}
