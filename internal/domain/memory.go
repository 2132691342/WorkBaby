package domain

// 长期记忆契约：一份 MEMORY.md 的读写视图，不分情景/语义/程序——分类只增加「该往哪写」的决策成本。

// MemoryEntryRESP 单条记忆。
type MemoryEntryRESP struct {
	Section string  `json:"section"`
	Text    string  `json:"text"`
	Score   float64 `json:"score,omitempty"` // 仅检索结果带分数
}

// MemorySectionRESP 分节计数。
type MemorySectionRESP struct {
	Section string `json:"section"`
	Count   int    `json:"count"`
}

// MemoryOverviewRESP 记忆概览（GET /memory）。
type MemoryOverviewRESP struct {
	File     string              `json:"file"`     // MEMORY.md 路径（设置页「打开文件」用）
	Entries  int                 `json:"entries"`  // 条目总数
	Sections []MemorySectionRESP `json:"sections"` // 各分节计数
}

// MemoryListRESP 条目列表。
type MemoryListRESP struct {
	File  string            `json:"file"`
	Items []MemoryEntryRESP `json:"items"`
}

// MemoryWriteREQ 写入入参。
type MemoryWriteREQ struct {
	Section string `json:"section"`
	Content string `json:"content"`
}

// MemoryDeleteREQ 删除入参（按分节 + 正文精确匹配）。
type MemoryDeleteREQ struct {
	Section string `json:"section"`
	Text    string `json:"text"`
}

// MemoryReplaceREQ 整篇覆盖入参。
type MemoryReplaceREQ struct {
	Text string `json:"text"`
}
