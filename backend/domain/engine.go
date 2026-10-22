// 引擎聚合根：PI 自身与内置运行时的状态快照，供设置页「助手引擎」展示与升级。
package domain

import "WorkBaby/backend/pkg"

var (
	// ErrEngineMissing 表示还没装 PI：升级无从谈起，只能先安装。
	ErrEngineMissing = pkg.New("9722", "还没有安装助手引擎，先装再升级", "")
)

// EngineStatusVO 是引擎页的本地快照：不发网络请求，点开就能看。
type EngineStatusVO struct {
	PiReady   bool   `json:"pi_ready"`
	PiVersion string `json:"pi_version"`
	PiPath    string `json:"pi_path"`
	PiSource  string `json:"pi_source"`
	// Managed 表示这份 PI 是 WorkBaby 自己装的：升级写回本应用的前缀，不动系统的。
	Managed bool `json:"managed"`
	// InstallRoot 是 PI 的包目录（版本文件与升级落点都在这）。
	InstallRoot string `json:"install_root"`
	// Running 是当前活着的 PI 进程数；升级后它们都要重启才会换上新内核。
	Running int `json:"running"`

	NodeReady   bool   `json:"node_ready"`
	NodeVersion string `json:"node_version"`
	NodePath    string `json:"node_path"`
	NodeSource  string `json:"node_source"`
	NodeError   string `json:"node_error"`

	PythonReady   bool   `json:"python_ready"`
	PythonVersion string `json:"python_version"`
	PythonPath    string `json:"python_path"`
	PythonSource  string `json:"python_source"`

	AgentDir   string `json:"agent_dir"`
	ModelReady bool   `json:"model_ready"`
	// Packages 是已声明的插件包数量：插件页才看得见细节，引擎页只给个总数。
	Packages int `json:"packages"`
	// Blocking 是首个不满足项：node / pi / model / 空。
	Blocking string `json:"blocking"`
	// InstallError 是最近一次安装 / 升级失败的原因（空表示没有失败记录）。
	InstallError string `json:"install_error"`

	// 命令执行环境：kind 为 git-bash / msys / cygwin / powershell / none。
	ShellKind   string `json:"shell_kind"`
	ShellPath   string `json:"shell_path"`
	ShellUsable bool   `json:"shell_usable"`
	ShellError  string `json:"shell_error"`
	// 终端入口：终端里 `pi` 能不能直接用。
	TerminalReady  bool   `json:"terminal_ready"`
	TerminalPath   string `json:"terminal_path"`
	TerminalOnPath bool   `json:"terminal_on_path"`
	TerminalError  string `json:"terminal_error"`
}

// EngineLatestVO 是联网查回来的最新版本：与本地快照分开，网络慢不拖累面板首屏。
type EngineLatestVO struct {
	// PiVersion 是 npm 上的最新版本；RegistryError 非空表示没查到。
	PiVersion       string `json:"pi_version"`
	RegistryError   string `json:"registry_error"`
	UpdateAvailable bool   `json:"update_available"`
	// Packages 只覆盖能从注册表问到版本的包（npm 来源）。
	Packages []PackageLatestVO `json:"packages"`
}

// PackageLatestVO 是一个插件包的最新版本。
type PackageLatestVO struct {
	Source          string `json:"source"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	Err             string `json:"err"`
}
