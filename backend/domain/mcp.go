// MCP 外部服务聚合根：配置文件里的一项启动声明与它的运行期状态（specs/10）。
package domain

// MCPDefaultTimeout 是单次调用的缺省超时（秒）；配置文件没写就用它。
const MCPDefaultTimeout = 30

// MCPConfigFileName 是配置文件名，落在数据目录下（与 workbaby.db 同级）。
const MCPConfigFileName = "mcp.json"

// MCPEntry 是 mcp.json 里的一项，解析后带上名字（名字即 JSON 的键）。
// 文件是唯一真相源：不落库，改完重载即生效。
type MCPEntry struct {
	Name     string            `json:"name"`
	Command  string            `json:"command"`
	Args     []string          `json:"args"`
	Env      map[string]string `json:"env"`
	Cwd      string            `json:"cwd"`
	Disabled bool              `json:"disabled"`
	Timeout  int               `json:"timeout"`
}

// MCPServerVO 出参：命令与参数给界面展示，环境变量只回键名——值里常写着密钥。
// 只有「声明」没有「连接结果」：连接由 PI 负责，它不回报每个服务的成败，就没有可信的状态可画。
type MCPServerVO struct {
	Name     string   `json:"name"`
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	EnvKeys  []string `json:"env_keys"`
	Cwd      string   `json:"cwd"`
	Disabled bool     `json:"disabled"`
	Timeout  int      `json:"timeout"`
}

// SaveMCPConfigREQ 保存配置原文：后端先校验再落盘，解析不过不写文件。
type SaveMCPConfigREQ struct {
	Raw string `json:"raw"`
}

// MCPConfigRESP 是配置面板要的全部东西：文件在哪、原文是什么、解析出了什么。
type MCPConfigRESP struct {
	Path       string        `json:"path"`
	Raw        string        `json:"raw"`
	Servers    []MCPServerVO `json:"servers"`
	ParseError string        `json:"parse_error"`
}

// MCPTemplateVO 一个内置模板：界面按 kinds 列按钮，点哪个填哪个。
type MCPTemplateVO struct {
	Kind string `json:"kind"`
	// Label 是按钮上的字：模板的用途只有一句话，说清了用户才知道点它。
	Label string `json:"label"`
	Hint  string `json:"hint"`
	Raw   string `json:"raw"`
}

// MCPToolVO 一个已发现的外部工具。
type MCPToolVO struct {
	Name        string         `json:"name"`
	Label       string         `json:"label"`
	Description string         `json:"description"`
	Server      string         `json:"server"`
	Parameters  map[string]any `json:"parameters"`
	Enabled     bool           `json:"enabled"`
}
