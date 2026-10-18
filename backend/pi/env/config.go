// WorkBaby ↔ PI 的配置互通：读写的就是用户 pi 命令读写的那一份文件。
// 写进去的必须是 PI 认识的字段，否则它忽略而界面以为生效。
package env

import (
	"encoding/json"
	"path/filepath"

	"WorkBaby/backend/pkg"
)

// ModelsPath 返回 PI 的 models.json 路径。
func (e *Env) ModelsPath() string { return filepath.Join(e.paths.AgentDir, "models.json") }

// SettingsPath 返回 PI 的 settings.json 路径。
func (e *Env) SettingsPath() string { return filepath.Join(e.paths.AgentDir, "settings.json") }

// MCPPath 返回用户级 mcp.json 路径。
func (e *Env) MCPPath() string { return filepath.Join(e.paths.AgentDir, "mcp.json") }

// ProjectMCPPath 返回某个工作区的 .pi/mcp.json 路径。
func (e *Env) ProjectMCPPath(workspace string) string {
	if workspace == "" {
		return ""
	}
	return filepath.Join(workspace, ".pi", "mcp.json")
}

// AuthPath 返回 PI 的 auth.json（/login 存下的凭据）。
func (e *Env) AuthPath() string { return filepath.Join(e.paths.AgentDir, "auth.json") }

// ExtensionsDir 返回 PI 的用户扩展目录。
func (e *Env) ExtensionsDir() string { return e.paths.ExtensionsDir }

// SkillsDir 返回 PI 的用户技能目录。
func (e *Env) SkillsDir() string { return e.paths.SkillsDir }

// DisabledSkillsDir 返回停用技能区（不在 PI 的扫描路径里）。
func (e *Env) DisabledSkillsDir() string { return e.paths.DisabledSkillsDir }

// SessionsDir 返回 PI 的会话目录。
func (e *Env) SessionsDir() string { return e.paths.SessionsDir }

// ModelsFile 是 models.json 的最小形态：providers 表 + 其余原样保留。
type ModelsFile struct {
	Providers map[string]ProviderConfig
	// Extra 是不认识的顶层键（$schema 等），写回时原样带上。
	Extra map[string]json.RawMessage
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

// ModelConfig 是 PI 的一个模型条目；未覆盖的字段由 PI 的内置目录提供。
type ModelConfig struct {
	ID            string            `json:"id"`
	Name          string            `json:"name,omitempty"`
	API           string            `json:"api,omitempty"`
	BaseURL       string            `json:"baseUrl,omitempty"`
	Reasoning     bool              `json:"reasoning,omitempty"`
	Input         []string          `json:"input,omitempty"`
	ContextWindow int               `json:"contextWindow,omitempty"`
	MaxTokens     int               `json:"maxTokens,omitempty"`
	Cost          *Cost             `json:"cost,omitempty"`
	ThinkingMap   map[string]string `json:"thinkingLevelMap,omitempty"`
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
	Name          string            `json:"name,omitempty"`
	Reasoning     *bool             `json:"reasoning,omitempty"`
	Input         []string          `json:"input,omitempty"`
	ContextWindow int               `json:"contextWindow,omitempty"`
	MaxTokens     int               `json:"maxTokens,omitempty"`
	ThinkingMap   map[string]string `json:"thinkingLevelMap,omitempty"`
}

// rawModels 是 models.json 的落盘形态。
type rawModels struct {
	Providers map[string]json.RawMessage `json:"providers"`
}

// ReadModelsRaw 读 models.json 的两层原始形态：顶层其余键与服务商表。
// 服务层靠它只改自己掌管的字段，不动别人写的未知字段。
func (e *Env) ReadModelsRaw() (map[string]json.RawMessage, map[string]json.RawMessage, error) {
	extra := map[string]json.RawMessage{}
	providers := map[string]json.RawMessage{}
	body, err := ReadFileOrEmpty(e.ModelsPath(), []byte("{}"))
	if err != nil {
		return nil, nil, err
	}
	top := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, nil, pkg.Wrap("9713", "models.json 格式不对", err)
	}
	for key, raw := range top {
		if key == "providers" {
			if err := json.Unmarshal(raw, &providers); err != nil {
				return nil, nil, pkg.Wrap("9713", "models.json 的 providers 不是对象表", err)
			}
			continue
		}
		extra[key] = raw
	}
	return extra, providers, nil
}

// WriteModelsRaw 写回两层原始形态（原子替换）。
func (e *Env) WriteModelsRaw(extra, providers map[string]json.RawMessage) error {
	providersRaw, err := json.Marshal(providers)
	if err != nil {
		return pkg.Wrap("9712", "models.json 无法序列化", err)
	}
	top := map[string]json.RawMessage{}
	for k, v := range extra {
		top[k] = v
	}
	top["providers"] = providersRaw
	body, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return pkg.Wrap("9712", "models.json 无法序列化", err)
	}
	return WriteFileAtomic(e.ModelsPath(), append(body, '\n'), 0o600)
}

// ReadModels 读 models.json；不存在返回空表（供首次生成）。
func (e *Env) ReadModels() (*ModelsFile, error) {
	file := &ModelsFile{Providers: map[string]ProviderConfig{}, Extra: map[string]json.RawMessage{}}
	body, err := ReadFileOrEmpty(e.ModelsPath(), []byte("{}"))
	if err != nil {
		return nil, err
	}
	top := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &top); err != nil {
		return nil, pkg.Wrap("9713", "models.json 格式不对", err)
	}
	for key, raw := range top {
		if key != "providers" {
			file.Extra[key] = raw
			continue
		}
		providers := map[string]ProviderConfig{}
		if err := json.Unmarshal(raw, &providers); err != nil {
			return nil, pkg.Wrap("9713", "models.json 的 providers 不是对象表", err)
		}
		file.Providers = providers
	}
	return file, nil
}

// WriteModels 写 models.json：已知字段与我们无关的顶层键都原样保留。
func (e *Env) WriteModels(file *ModelsFile) error {
	providers, err := json.Marshal(file.Providers)
	if err != nil {
		return pkg.Wrap("9712", "models.json 无法序列化", err)
	}
	top := map[string]json.RawMessage{}
	for k, v := range file.Extra {
		top[k] = v
	}
	top["providers"] = providers
	body, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return pkg.Wrap("9712", "models.json 无法序列化", err)
	}
	return WriteFileAtomic(e.ModelsPath(), append(body, '\n'), 0o600)
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

// settingsOwnedKeys 是本结构占据的顶层键：写入前先摘下旧值，零值才真的等于清空。
// packages 不在里面：它归 PI 管，当成自己的键会抹掉用户用 CLI 装的包。
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

// ReadSettingsMap 读 settings.json 的顶层键值对，供只读展示与包清单读取。
func (e *Env) ReadSettingsMap() (map[string]json.RawMessage, error) {
	return e.settingsMap()
}

// WriteSettings 写 settings.json：只覆盖 WorkBaby 掌管的顶层键，其余原样保留
// （整体重写会抹掉 PI 自己写的字段与用户用 pi install 装的包）。
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

// ReadPackages 读 settings.json 的 packages 清单（PI 的包声明）。
func (e *Env) ReadPackages() ([]string, error) {
	raw, err := e.settingsMap()
	if err != nil {
		return nil, err
	}
	body, ok := raw["packages"]
	if !ok {
		return []string{}, nil
	}
	// 条目可能是字符串，也可能是 {source, ...} 对象：只取 source。
	var entries []json.RawMessage
	if err := json.Unmarshal(body, &entries); err != nil {
		return []string{}, nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		var s string
		if err := json.Unmarshal(entry, &s); err == nil {
			out = append(out, s)
			continue
		}
		var obj struct {
			Source string `json:"source"`
		}
		if err := json.Unmarshal(entry, &obj); err == nil && obj.Source != "" {
			out = append(out, obj.Source)
		}
	}
	return out, nil
}

// MCPRaw 读一个 mcp.json 原文；不存在返回空对象。
func (e *Env) MCPRaw(path string) ([]byte, error) {
	return ReadFileOrEmpty(path, []byte("{}\n"))
}

// WriteMCPRaw 写 mcp.json 原文。
func (e *Env) WriteMCPRaw(path string, raw []byte) error {
	return WriteFileAtomic(path, raw, 0o600)
}
