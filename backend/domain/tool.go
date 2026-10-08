// 工具目录：把「助手现在能干什么」变成一份前端可直接渲染的清单，
// 它是模型工具声明的只读投影，改工具集时清单自动跟着变。
package domain

import "WorkBaby/backend/pkg"

var ErrToolNotFound = pkg.New(4204, "没有这个工具", "")

// ToolVO 是单个工具的视图对象。risk 与 approval 决定界面上的风险标记。
type ToolVO struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Risk        string   `json:"risk"`
	Approval    bool     `json:"approval"`
	Mode        string   `json:"mode"`
	Params      []string `json:"params"`
	Enabled     bool     `json:"enabled"`
	Builtin     bool     `json:"builtin"`
}

// ToolCategory 聚合根：工具分类的稳定取值。
const (
	CategoryFile  = "file"
	CategoryShell = "shell"
	CategoryCode  = "code"
	CategoryWeb   = "web"
	CategoryData  = "data"
)

// ToggleToolREQ 启停工具入参。
type ToggleToolREQ struct {
	Enabled bool `json:"enabled"`
}

// RuntimeInfoVO 运行时状态出参：Python 与 PowerShell 的可用状态 + 失败原因。
// source 取值 bundled / system / ""，让界面能区分「内置就绪」与「用系统里装的那份」。
type RuntimeInfoVO struct {
	PythonExe         string `json:"python_exe"`
	PythonSource      string `json:"python_source"`
	PythonVersion     string `json:"python_version"`
	PythonError       string `json:"python_error"`
	PowerShellExe     string `json:"powershell_exe"`
	PowerShellSource  string `json:"powershell_source"`
	PowerShellVersion string `json:"powershell_version"`
	PowerShellError   string `json:"powershell_error"`
}
