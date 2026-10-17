// MCP 外部服务聚合根：配置文件里的一项启动声明与它的运行期状态（specs/16）。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrMCPServerNotFound = pkg.New("9101", "这个 MCP 服务不存在", "")
	ErrMCPStartFailed    = pkg.New("9102", "MCP 服务起不来", "检查命令路径与参数")
	ErrMCPHandshake      = pkg.New("9103", "MCP 服务握手超时", "它可能不是个正常的 MCP server")
	ErrMCPProtocol       = pkg.New("9104", "MCP 服务返回了看不懂的内容", "协议错误")
	ErrMCPCallFailed     = pkg.New("9105", "MCP 工具调用失败", "")
	ErrMCPConfigParse    = pkg.New("9106", "MCP 配置文件读不通", "按提示改正后再保存")
	ErrMCPConfigIO       = pkg.New("9107", "MCP 配置文件读写失败", "检查数据目录是否可写")
)

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
type MCPServerVO struct {
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	EnvKeys     []string `json:"env_keys"`
	Cwd         string   `json:"cwd"`
	Disabled    bool     `json:"disabled"`
	Timeout     int      `json:"timeout"`
	LastProbeOK bool     `json:"last_probe_ok"`
	LastProbeAt int64    `json:"last_probe_at"`
	LastError   string   `json:"last_error"`
	ToolCount   int      `json:"tool_count"`
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

// ProbeMCPServerREQ 探测入参：服务名走请求体，路由不拼 `/:id/:action`。
type ProbeMCPServerREQ struct {
	Name string `json:"name"`
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
