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
	Packages             []string            `json:"packages,omitempty"`
}

// settingsOwnedKeys 是本结构占据的顶层键：写入前先删掉，零值才真的等于清空。
// packages 不在里面：它归 PI 的包管理（pi install / pi remove）管，我们只在装 / 卸时追加，
// 当成自己的键会把用户用 CLI 装的包抹掉。
var settingsOwnedKeys = []string{
	"defaultProvider", "defaultModel", "defaultThinkingLevel",
	"defaultTools", "compaction", "extensions",
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

// WriteSettings 写 settings.json：只覆盖 WorkBaby 掌管的顶层键，其余原样保留。
// 整体重写会把 PI 自己写的字段（cacheWarming / codemode / prompts…）抹掉，
// 也会抹掉用户用 pi install 加进来的包。
func (e *Env) WriteSettings(file *SettingsFile) error {
	raw, err := e.settingsMap()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(file)
	if err != nil {
		return pkg.Wrap("9712", "配置无法序列化", err)
	}
	owned := map[string]json.RawMessage{}
	if err := json.Unmarshal(encoded, &owned); err != nil {
		return pkg.Wrap("9712", "配置无法序列化", err)
	}
	// 先摘下旧值：既要能清空（零值不在 owned 里），又要能对嵌套对象做字段级合并。
	previous := map[string]json.RawMessage{}
	for _, key := range settingsOwnedKeys {
		if old, ok := raw[key]; ok {
			previous[key] = old
			delete(raw, key)
		}
	}
	for key, value := range owned {
		if old, ok := previous[key]; ok {
			raw[key] = mergeSettingsObjects(old, value)
			continue
		}
		raw[key] = value
	}
	return writeSettingsMap(e.SettingsPath(), raw)
}

// mergeSettingsObjects 把新对象里的键合进旧对象，而不是整个替掉。
// 用于 compaction 这类「我们只改其中几个字段」的嵌套对象：PI 还往里写 enabled。
// 任一侧不是对象时直接取新值。
func mergeSettingsObjects(oldValue, newValue json.RawMessage) json.RawMessage {
	var oldObj, newObj map[string]json.RawMessage
	if json.Unmarshal(oldValue, &oldObj) != nil || json.Unmarshal(newValue, &newObj) != nil {
		return newValue
	}
	for k, v := range newObj {
		oldObj[k] = v
	}
	merged, err := json.Marshal(oldObj)
	if err != nil {
		return newValue
	}
	return merged
}

// settingsMap 读 settings.json 的顶层键值对；文件不存在按空对象处理。
func (e *Env) settingsMap() (map[string]json.RawMessage, error) {
	body, err := ReadFileOrEmpty(e.SettingsPath(), []byte("{}"))
	if err != nil {
		return nil, err
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, pkg.Wrap("9713", "settings.json 格式不对", err)
	}
	return raw, nil
}

// writeSettingsMap 把顶层键值对缩进写回（原子替换）。
func writeSettingsMap(path string, raw map[string]json.RawMessage) error {
	body, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return pkg.Wrap("9712", "配置无法序列化", err)
	}
	return WriteFileAtomic(path, append(body, '\n'), 0o600)
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
