// 工具目录：把「助手现在能干什么」变成一份前端可直接渲染的清单。
// 模型每轮都要读工具声明，用户却没有任何入口去看它们；
// 这个接口是那份声明的只读投影，改工具集时清单自动跟着变。
package domain

import "WorkBaby/internal/pkg"

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

// 工具分类的中文名，界面直接用，不在前端再做一次映射。
var CategoryNames = map[string]string{
	CategoryFile:  "文件",
	CategoryShell: "命令",
	CategoryCode:  "代码",
	CategoryWeb:   "联网",
	CategoryData:  "资料",
}

// ToggleToolREQ 启停工具入参。
type ToggleToolREQ struct {
	Enabled bool `json:"enabled"`
}

// RuntimeInfoVO 运行时状态出参：Python 是否就绪 + 失败原因。
// 曾经这里只有「可用 / 不可用」两个值，用户看到不可用却不知道该做什么。
type RuntimeInfoVO struct {
	PythonExe     string `json:"python_exe"`
	PythonSource  string `json:"python_source"`
	PythonVersion string `json:"python_version"`
	PythonError   string `json:"python_error"`
	ArchivePath   string `json:"archive_path"`
}
