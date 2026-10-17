// WorkBaby ↔ PI 的配置互通：WorkBaby 是 models.json / settings.json 的唯一写入者，
// PI 只读。写进去的必须是 PI 认识的字段，否则它忽略而界面以为生效。
package env

import (
	"encoding/json"
	"path/filepath"

	"WorkBaby/backend/pkg"
)

// ModelsPath 返回 PI 的 models.json 路径。
func (e *Env) ModelsPath() string { return filepath.Join(e.paths.PiAgentDir, "models.json") }

// SettingsPath 返回 PI 的 settings.json 路径。
func (e *Env) SettingsPath() string { return filepath.Join(e.paths.PiAgentDir, "settings.json") }

// MCPPath 返回 PI 的 mcp.json 路径。
func (e *Env) MCPPath() string { return filepath.Join(e.paths.PiAgentDir, "mcp.json") }

// AppendSystemPath 返回 PI 的 APPEND_SYSTEM.md 路径。
func (e *Env) AppendSystemPath() string { return filepath.Join(e.paths.PiAgentDir, "APPEND_SYSTEM.md") }

// ExtensionsDir 返回 PI 的扩展目录。
func (e *Env) ExtensionsDir() string { return filepath.Join(e.paths.PiAgentDir, "extensions") }

// PolicyPath 返回审批策略文件路径（由 WorkBaby 的扩展读取）。
func (e *Env) PolicyPath() string { return filepath.Join(e.paths.PiAgentDir, "workbaby-policy.json") }

// ModelsFile 是 models.json 的最小形态：只保留 providers 表。
type ModelsFile struct {
	Providers map[string]ProviderConfig `json:"providers"`
}

// ProviderConfig 是 PI 的一个服务商条目。字段名必须与 PI 一致。
type ProviderConfig struct {
	Name           string                   `json:"name,omitempty"`
	BaseURL        string                   `json:"baseUrl,omitempty"`
	APIKey         string                   `json:"apiKey,omitempty"`
	API            string                   `json:"api,omitempty"`
	Headers        map[string]string        `json:"headers,omitempty"`
	AuthHeader     *bool                    `json:"authHeader,omitempty"`
	Models         []ModelConfig            `json:"models,omitempty"`
	ModelOverrides map[string]ModelOverride `json:"modelOverrides,omitempty"`
}

// ModelConfig 是 PI 的一个模型条目。
type ModelConfig struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	API           string   `json:"api,omitempty"`
	BaseURL       string   `json:"baseUrl,omitempty"`
	Reasoning     bool     `json:"reasoning,omitempty"`
	Input         []string `json:"input,omitempty"`
	ContextWindow int      `json:"contextWindow,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Cost          *Cost    `json:"cost,omitempty"`
}

// Cost 是模型单价（美元 / 百万 token）。
type Cost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}

// ModelOverride 是对内置模型的覆写。
type ModelOverride struct {
	Name          string   `json:"name,omitempty"`
	Reasoning     *bool    `json:"reasoning,omitempty"`
	Input         []string `json:"input,omitempty"`
	ContextWindow int      `json:"contextWindow,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
}

// ReadModels 读 models.json；不存在返回空表（供首次生成）。
func (e *Env) ReadModels() (*ModelsFile, error) {
	file := &ModelsFile{Providers: map[string]ProviderConfig{}}
	if err := ReadJSON(e.ModelsPath(), file); err != nil {
		return nil, err
	}
	if file.Providers == nil {
		file.Providers = map[string]ProviderConfig{}
	}
	return file, nil
}

// WriteModels 写 models.json（原子替换）。
func (e *Env) WriteModels(file *ModelsFile) error {
	if file.Providers == nil {
		file.Providers = map[string]ProviderConfig{}
	}
	return WriteJSON(e.ModelsPath(), file)
}

// SettingsFile 是 settings.json 里 WorkBaby 会写的那部分。
// 没列进来的字段由用户（或 PI 自己）掌管，我们不碰。
type SettingsFile struct {
	DefaultProvider      string              `json:"defaultProvider,omitempty"`
	DefaultModel         string              `json:"defaultModel,omitempty"`
	DefaultThinkingLevel string              `json:"defaultThinkingLevel,omitempty"`
	DefaultTools         []string            `json:"defaultTools,omitempty"`
	Compaction           *CompactionSettings `json:"compaction,omitempty"`
	Extensions           []string            `json:"extensions,omitempty"`
}

// CompactionSettings 是 PI 的压缩参数。
type CompactionSettings struct {
	Enabled          *bool `json:"enabled,omitempty"`
	ReserveTokens    int   `json:"reserveTokens,omitempty"`
	KeepRecentTokens int   `json:"keepRecentTokens,omitempty"`
}

// ReadSettings 读 settings.json。
func (e *Env) ReadSettings() (*SettingsFile, error) {
	file := &SettingsFile{}
	if err := ReadJSON(e.SettingsPath(), file); err != nil {
		return nil, err
	}
	return file, nil
}

// WriteSettings 写 settings.json（原子替换）。
func (e *Env) WriteSettings(file *SettingsFile) error {
	return WriteJSON(e.SettingsPath(), file)
}

// MCPFile 是 mcp.json 的形态。未知字段靠 raw 保留：从别的客户端拷来的配置
// 带 timeout / headers 等，按自己的理解裁剪会让它失效。
type MCPFile struct {
	Servers map[string]json.RawMessage `json:"mcpServers"`
	Extra   map[string]json.RawMessage `json:"-"`
}

// ReadMCPRaw 读 mcp.json 原文；不存在返回空对象。
func (e *Env) ReadMCPRaw() ([]byte, error) {
	return ReadFileOrEmpty(e.MCPPath(), []byte("{}\n"))
}

// WriteMCPRaw 写 mcp.json 原文。
func (e *Env) WriteMCPRaw(raw []byte) error {
	return WriteFileAtomic(e.MCPPath(), raw, 0o600)
}

// KnowledgeMCPEntry 是知识库的 MCP 出口条目：WorkBaby 自动维护，用户不能删。
type KnowledgeMCPEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// EnsureKnowledgeMCP 把知识库的 MCP 条目写进 mcp.json 并保持存在。
// 它由程序管：用户删掉它，知识库工具就消失，而症状是「搜索没结果」不是「配置丢了」。
func (e *Env) EnsureKnowledgeMCP(exePath string) error {
	raw, err := e.ReadMCPRaw()
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		// 用户把文件改坏了：不覆盖它（那会毁掉他的东西），只报错让他自己修。
		return pkg.Wrap("9106", "mcp.json 格式不对，无法写入内置条目", err)
	}
	servers := map[string]json.RawMessage{}
	if existing, ok := doc["mcpServers"]; ok {
		if err := json.Unmarshal(existing, &servers); err != nil {
			return pkg.Wrap("9106", "mcp.json 的 mcpServers 不是对象", err)
		}
	}
	entry, err := json.Marshal(KnowledgeMCPEntry{Command: exePath, Args: []string{"--mcp-knowledge"}})
	if err != nil {
		return pkg.Wrap("9106", "内置 MCP 条目无法序列化", err)
	}
	servers["workbaby-knowledge"] = entry
	serversRaw, err := json.Marshal(servers)
	if err != nil {
		return pkg.Wrap("9106", "内置 MCP 条目无法序列化", err)
	}
	doc["mcpServers"] = serversRaw
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return pkg.Wrap("9106", "mcp.json 无法序列化", err)
	}
	return e.WriteMCPRaw(append(out, '\n'))
}
