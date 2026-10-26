// 引擎聚合根：PI 自身与内置运行时的状态快照，供设置页「助手引擎」展示与升级。
package domain

import "WorkBaby/backend/pkg"

var (
	// ErrEngineMissing 表示还没装 PI：升级无从谈起，只能先安装。
	ErrEngineMissing = pkg.New("9722", "还没有安装助手引擎，先装再升级", "")
	// ErrEngineBusy 表示正在装 / 升 / 卸：此时发消息会 spawn 到写到一半的文件上。
	ErrEngineBusy = pkg.New("5006", "正在安装或升级引擎，完成后再发消息", "")
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
	// NodeLevel 是 node 目录挂在哪一级 PATH：system / user / both / none。
	NodeLevel string `json:"node_level"`
	// Git* 是当前生效的 git：助手跑命令与界面读状态用的是同一份。
	GitReady   bool   `json:"git_ready"`
	GitVersion string `json:"git_version"`
	GitPath    string `json:"git_path"`
	GitSource  string `json:"git_source"`
	GitLevel   string `json:"git_level"`

	AgentDir   string `json:"agent_dir"`
	ModelReady bool   `json:"model_ready"`
	// Packages 是已声明的插件包数量：插件页才看得见细节，引擎页只给个总数。
	Packages int `json:"packages"`
	// Blocking 是首个不满足项：node / pi / model / 空。
	Blocking string `json:"blocking"`
	// InstallError 是最近一次安装 / 升级失败的原因（空表示没有失败记录）。
	InstallError string `json:"install_error"`

	// 命令执行环境：kind 为 git-bash / msys / cygwin / powershell / none；source 为 settings / git / path / bundled。
	ShellKind   string `json:"shell_kind"`
	ShellPath   string `json:"shell_path"`
	ShellSource string `json:"shell_source"`
	ShellUsable bool   `json:"shell_usable"`
	ShellError  string `json:"shell_error"`
	// 终端入口：终端里 `pi` 能不能直接用。
	TerminalReady  bool   `json:"terminal_ready"`
	TerminalPath   string `json:"terminal_path"`
	TerminalOnPath bool   `json:"terminal_on_path"`
	TerminalError  string `json:"terminal_error"`
	// TerminalShadowedBy 是 PATH 上排在入口目录之前、带着另一份 pi 的目录（空表示没有）。
	TerminalShadowedBy string `json:"terminal_shadowed_by"`
	// OtherPis 是 PATH 上其它装过 pi 的目录：同时存在多份时界面要能说清楚用的是哪份。
	OtherPis []string `json:"other_pis"`
	// TerminalLevel 是入口目录挂在哪一级 PATH：system / user / both / none。
	TerminalLevel string `json:"terminal_level"`
}

// TerminalLevelREQ 切换 PATH 级别入参：Level 为 system / user / none；写系统级经 UAC 提权。
type TerminalLevelREQ struct {
	Level string `json:"level"`
}

// LinkVO 是一个运行时组件（node / git / pi）在 PATH 上的落地情况。
type LinkVO struct {
	Ready   bool   `json:"ready"`
	Source  string `json:"source"`
	Version string `json:"version"`
	Path    string `json:"path"`
	// Level 是目录挂在哪一级 PATH：system / user / both / none。
	Level string `json:"level"`
	// Promotable 表示这个组件有目录可以提级 / 降级。
	Promotable bool `json:"promotable"`
	// Conflict 是提到系统级会遮住的已有安装（空表示不冲突）。
	Conflict string `json:"conflict"`
	Error    string `json:"error"`
}

// ToolchainVO 是 node / git / pi 的工具链快照：装引擎用的是哪份、能不能换、能不能提级。
type ToolchainVO struct {
	Node LinkVO `json:"node"`
	Git  LinkVO `json:"git"`
	Pi   LinkVO `json:"pi"`
	// InstallPrefix 是 PI 安装 / 升级落点的 npm 全局前缀。
	InstallPrefix string `json:"install_prefix"`
	// Bound 表示 node 是装 PI 时钉下来的那份：后续升级与插件都回到它。
	Bound bool `json:"bound"`
	// Alt 是另一份可切换的 node（当前用内置时给出系统那份，反之给出内置那份），空表示没有可换的。
	Alt *NodeOptionVO `json:"alt"`
}

// NodeOptionVO 是一份可切换的 node。
type NodeOptionVO struct {
	Exe     string `json:"exe"`
	Version string `json:"version"`
	Source  string `json:"source"`
}

// PromoteREQ 是运行时提级 / 降级入参：Component 为 node / git / pi，Level 为 system / user / none。
type PromoteREQ struct {
	Component string `json:"component"`
	Level     string `json:"level"`
}

// NodeSwitchREQ 是切换运行时入参：Exe 为目标 node 的绝对路径，空表示回到自动选择。
type NodeSwitchREQ struct {
	Exe string `json:"exe"`
}

// TerminalLevelRESP 是切换后的终端入口读数。
type TerminalLevelRESP struct {
	Level  string `json:"level"`
	OnPath bool   `json:"on_path"`
	Ready  bool   `json:"ready"`
	Error  string `json:"error"`
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
