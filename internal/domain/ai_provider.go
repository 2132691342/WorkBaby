package domain

import (
	"strings"

	"WorkBaby/internal/pkg"
)

// ProviderKind LLM 协议族；OpenAI 兼容路径覆盖 DeepSeek/Moonshot/智谱/GLM/Ollama 兼容层。
type ProviderKind string

const (
	ProviderKindOpenAI    ProviderKind = "openai"
	ProviderKindAnthropic ProviderKind = "anthropic"
	ProviderKindOllama    ProviderKind = "ollama"
)

// 已知 ProviderKind 列表（用于设置面板 quick-add）。
var AllProviderKinds = []ProviderKind{
	ProviderKindOpenAI,
	ProviderKindAnthropic,
	ProviderKindOllama,
}

// ProviderKindMeta 单个 kind 的展示元数据（前端 Settings 下拉渲染用）。
//
// 关键约束：每加一种 kind，必须在 internal/llm/<kind>/ 实现 Provider 接口，
// 并在 registry.go buildOne 加 case —— 否则前端暴露的选项会让用户保存后
// 立刻撞上 3020「unknown provider kind」（CLAUDE.md §8 反模式「臆造 API 特性」）。
type ProviderKindMeta struct {
	Kind             ProviderKind `json:"kind"`
	Label            string       `json:"label"`             // 用户可见名（前端可 i18n 覆盖）
	BaseURLDefault   string       `json:"base_url_default"`  // 留空 = 走客户端内置默认；非空时新建表单自动填一次
	ModelPlaceholder string       `json:"model_placeholder"` // 前端输入框 placeholder（仅展示，不真填）
}

// ProviderKindsResp 所有已实现 ProviderKind 的元数据列表（前端从后端拉，避免错配）。
type ProviderKindsRESP []ProviderKindMeta

// AllProviderKindMetas 集中维护——新加 ProviderKind 时这里也必须加。
// 一旦在 domain 加新 ProviderKind 但忘了在这里登记或忘了实现 Provider，
// 前端会立刻"看不见这个 kind"，比"前端看得见后端报错"安全得多。
var AllProviderKindMetas = []ProviderKindMeta{
	{Kind: ProviderKindOpenAI, Label: "OpenAI 协议（GPT / DeepSeek / GLM / Kimi 等兼容）", BaseURLDefault: "", ModelPlaceholder: "gpt-4o"},
	{Kind: ProviderKindAnthropic, Label: "Anthropic 协议（Claude）", BaseURLDefault: "", ModelPlaceholder: "claude-sonnet-4-5"},
	{Kind: ProviderKindOllama, Label: "Ollama（本地）", BaseURLDefault: "http://localhost:11434", ModelPlaceholder: "llama3"},
}

// ProviderTier 模型档位：决定默认 Provider 选择顺序与同名模型取哪一个，不做故障转移。
type ProviderTier string

const (
	ProviderTierPrimary ProviderTier = "primary"
	ProviderTierBackup  ProviderTier = "backup"
)

// AllProviderTiers 合法档位（前端设置面板下拉由此驱动，杜绝自造取值）。
var AllProviderTiers = []ProviderTier{ProviderTierPrimary, ProviderTierBackup}

// NormalizeTier 归一化档位：未知值与空值一律回落 primary。
func NormalizeTier(t ProviderTier) ProviderTier {
	if t == ProviderTierPrimary || t == ProviderTierBackup {
		return t
	}
	return ProviderTierPrimary
}

// TierRank 排序权重：primary 在前（仅排序用，见 ProviderTier 语义边界）。
func (p AiProviderDO) TierRank() int {
	if p.Tier == ProviderTierPrimary {
		return 0
	}
	return 1
}

// AiProviderDO LLM Provider 持久化实体；apiKey 用 AES-GCM 加密落库（pkg/crypto.go）。
// 能力字段 SupportsToolCall / SupportsVision / SupportsReasoning 为 *bool：
// nil = 按 kind 推断，true/false = 用户显式声明覆盖推断（同厂不同模型能力差异大，需人工纠偏口）。
type AiProviderDO struct {
	ID              string       `gorm:"primaryKey;size:64" json:"id"`
	Name            string       `gorm:"size:128"           json:"name"`
	Kind            ProviderKind `gorm:"size:32"            json:"kind"`
	APIKey          string       `gorm:"type:text"          json:"-"` // 内部字段，不外露
	BaseURL         string       `gorm:"size:512"           json:"base_url"`
	Model           string       `gorm:"size:128"           json:"model"`
	Alias           string       `gorm:"size:128"           json:"alias"`
	Tier            ProviderTier `gorm:"size:16"            json:"tier"`
	Enabled         bool         `gorm:"default:true"       json:"enabled"`
	ContextWindow   int          `gorm:"default:0"          json:"context_window"`    // 最大输入 token；0 = 走全局默认
	MaxOutputTokens int          `gorm:"default:0"          json:"max_output_tokens"` // 单次最大输出 token；0 = 不限制
	CompressRatio   float64      `gorm:"default:0.75"       json:"compress_ratio"`    // 上下文占比达此比例触发压缩
	Temperature     float64      `gorm:"default:0"          json:"temperature"`       // 0 = 走全局默认
	TopP            float64      `gorm:"default:0"          json:"top_p"`             // 0 = 走全局默认
	ThinkingEffort  string       `gorm:"size:16;default:''" json:"thinking_effort"`   // 空 = 走全局默认
	// ThinkingStyle 思维参数的协议方言；空 = 按 baseURL + 模型名自动探测。
	ThinkingStyle     string `gorm:"size:32;default:''"  json:"thinking_style"`
	SupportsToolCall  *bool  `gorm:"default:null"        json:"supports_tool_call"`
	SupportsVision    *bool  `gorm:"default:null"        json:"supports_vision"`
	SupportsReasoning *bool  `gorm:"default:null"        json:"supports_reasoning"`
	CapabilitiesJSON  string `gorm:"type:text;column:capabilities_json" json:"capabilities_json"`
	PricingJSON       string `gorm:"type:text;column:pricing_json"     json:"pricing_json"`
	CreatedAt         int64  `gorm:"autoCreateTime:milli" json:"created_at"`
	UpdatedAt         int64  `gorm:"autoUpdateTime:milli" json:"updated_at"`
}

// TableName 固定表名。
func (AiProviderDO) TableName() string { return "ai_providers" }

// SupportsToolCallEffective 工具调用能力的最终判定（显式声明 > 协议族推断）。
func (p AiProviderDO) SupportsToolCallEffective() bool {
	if p.SupportsToolCall != nil {
		return *p.SupportsToolCall
	}
	return p.Kind == ProviderKindOpenAI || p.Kind == ProviderKindAnthropic
}

// SupportsVisionEffective 视觉能力的最终判定（显式声明 > 模型名推断）。
func (p AiProviderDO) SupportsVisionEffective() bool {
	if p.SupportsVision != nil {
		return *p.SupportsVision
	}
	return looksVisionCapable(p.Model)
}

// SupportsReasoningEffective 推理能力的最终判定（显式声明 > 方言推断）。
func (p AiProviderDO) SupportsReasoningEffective() bool {
	if p.SupportsReasoning != nil {
		return *p.SupportsReasoning
	}
	return looksReasoningCapable(p.Model)
}

// AiProviderREQ 写入请求（apiKey 可选；空值表示不更新）。
type AiProviderREQ struct {
	Name              string       `json:"name"`
	Kind              ProviderKind `json:"kind"`
	APIKey            string       `json:"api_key,omitempty"`
	BaseURL           string       `json:"base_url"`
	Model             string       `json:"model"`
	Alias             string       `json:"alias"`
	Tier              ProviderTier `json:"tier"`
	Enabled           *bool        `json:"enabled,omitempty"`
	ContextWindow     int          `json:"context_window"`
	MaxOutputTokens   int          `json:"max_output_tokens"`
	CompressRatio     float64      `json:"compress_ratio"`
	Temperature       float64      `json:"temperature"`
	TopP              float64      `json:"top_p"`
	ThinkingEffort    string       `json:"thinking_effort"`
	ThinkingStyle     string       `json:"thinking_style"`
	SupportsToolCall  *bool        `json:"supports_tool_call,omitempty"`
	SupportsVision    *bool        `json:"supports_vision,omitempty"`
	SupportsReasoning *bool        `json:"supports_reasoning,omitempty"`
	CapabilitiesJSON  string       `json:"capabilities_json,omitempty"`
	PricingJSON       string       `json:"pricing_json,omitempty"`
}

// AiProviderRESP 出参（API Key 永远脱敏）。
type AiProviderRESP struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Kind            ProviderKind `json:"kind"`
	APIKeyMasked    string       `json:"api_key_masked"`
	BaseURL         string       `json:"base_url"`
	Model           string       `json:"model"`
	Alias           string       `json:"alias"`
	Tier            ProviderTier `json:"tier"`
	Enabled         bool         `json:"enabled"`
	ContextWindow   int          `json:"context_window"`
	MaxOutputTokens int          `json:"max_output_tokens"`
	CompressRatio   float64      `json:"compress_ratio"`
	Temperature     float64      `json:"temperature"`
	TopP            float64      `json:"top_p"`
	ThinkingEffort  string       `json:"thinking_effort"`
	ThinkingStyle   string       `json:"thinking_style"`
	// ThinkingStyleResolved 实际生效的方言（auto 探测后的结果；前端展示用，不回写）。
	ThinkingStyleResolved string `json:"thinking_style_resolved"`
	SupportsToolCall      *bool  `json:"supports_tool_call"`
	SupportsVision        *bool  `json:"supports_vision"`
	SupportsReasoning     *bool  `json:"supports_reasoning"`
	// 以下为最终生效值（三态解算后的布尔，前端不必自己推断）。
	ToolCallEffective  bool   `json:"tool_call_effective"`
	VisionEffective    bool   `json:"vision_effective"`
	ReasoningEffective bool   `json:"reasoning_effective"`
	CapabilitiesJSON   string `json:"capabilities_json"`
	PricingJSON        string `json:"pricing_json"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
}

// AvailableModelRESP 聊天模型选择器用（扁平化）。
type AvailableModelRESP struct {
	ID              string       `json:"id"`
	Name            string       `json:"name"`
	Kind            ProviderKind `json:"kind"`
	Model           string       `json:"model"`
	Alias           string       `json:"alias"`
	Tier            ProviderTier `json:"tier"`
	Enabled         bool         `json:"enabled"`
	ContextWindow   int          `json:"context_window"`
	MaxOutputTokens int          `json:"max_output_tokens"`
	CompressRatio   float64      `json:"compress_ratio"`
	Temperature     float64      `json:"temperature"`
	ThinkingEffort  string       `json:"thinking_effort"`
	ThinkingStyle   string       `json:"thinking_style"`
	ToolCall        bool         `json:"tool_call"`
	Vision          bool         `json:"vision"`
	Reasoning       bool         `json:"reasoning"`
}

// ModelConfigFile 是 {home}/model.json 的解析容器（配置文件字段 snake_case，与 DB/前端契约一致）。
type ModelConfigFile struct {
	Version   int             `json:"version"`
	Providers []ModelProvider `json:"providers"`
}

// ModelProvider model.json 中的单个 Provider 条目。
type ModelProvider struct {
	ID                string       `json:"id"`
	Name              string       `json:"name"`
	Kind              ProviderKind `json:"kind"`
	BaseURL           string       `json:"base_url"`
	APIKey            string       `json:"api_key"`
	Model             string       `json:"model"`
	Alias             string       `json:"alias"`
	Tier              ProviderTier `json:"tier"`
	Enabled           *bool        `json:"enabled"`
	ContextWindow     int          `json:"context_window"`
	MaxOutputTokens   int          `json:"max_output_tokens"`
	CompressRatio     float64      `json:"compress_ratio"`
	Temperature       float64      `json:"temperature"`
	TopP              float64      `json:"top_p"`
	ThinkingEffort    string       `json:"thinking_effort"`
	ThinkingStyle     string       `json:"thinking_style"`
	SupportsToolCall  *bool        `json:"supports_tool_call,omitempty"`
	SupportsVision    *bool        `json:"supports_vision,omitempty"`
	SupportsReasoning *bool        `json:"supports_reasoning,omitempty"`
}

// visionModelHints 视觉能力模型名关键词（小写匹配）。
var visionModelHints = []string{"vision", "4o", "4.1", "claude-3", "claude-sonnet-4", "claude-opus-4", "gpt-5", "gemini", "qwen-vl", "glm-4v", "llava", "minicpm-v"}

// reasoningModelHints 推理能力模型名关键词（小写匹配）。
var reasoningModelHints = []string{"-reasoner", "r1", "o1", "o3", "o4", "gpt-5", "thinking", "qwen3", "glm-4.5", "glm-4.6", "k2", "-m3", "m2", "sonnet-4", "opus-4", "deepseek-v3.2"}

// looksVisionCapable 按模型名粗判视觉能力；不确定时返回 false（低估不误用）。
func looksVisionCapable(model string) bool {
	m := strings.ToLower(model)
	for _, h := range visionModelHints {
		if strings.Contains(m, h) {
			return true
		}
	}
	return false
}

// looksReasoningCapable 按模型名粗判推理能力；不确定时返回 false。
func looksReasoningCapable(model string) bool {
	m := strings.ToLower(model)
	for _, h := range reasoningModelHints {
		if strings.Contains(m, h) {
			return true
		}
	}
	return false
}

// ProviderCircuitRESP Provider 熔断/就绪状态（前端模型选择器徽标用）。
type ProviderCircuitRESP struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Enabled             bool   `json:"enabled"`
	State               string `json:"state"` // CLOSED / OPEN / HALF_OPEN
	ConsecutiveFailures int    `json:"consecutive_failures"`
	RetryInMs           int64  `json:"retry_in_ms"`
	// Reason 未就绪的原因（apiKey 缺失/解密失败/kind 未知等）；ready 时为空。
	Reason string `json:"reason"`
}

// ProviderReloadRESP model.json 热重载结果。
type ProviderReloadRESP struct {
	Synced int `json:"synced"`
}

// ExecAgentRESP exec 工具二进制白名单（settings/exec/agent）。
type ExecAgentRESP struct {
	Binaries []string `json:"binaries"`
}

// 包级错误变量；错误码段 3000-3999（LLM/Provider）。
var (
	ErrProviderNotFound = pkg.New(3001, "provider not found", "")
	ErrProviderInvalid  = pkg.New(3002, "provider invalid (missing apiKey/baseUrl/model)", "")
	ErrProviderNotReady = pkg.New(3003, "provider not ready (test connect failed)", "")
)
