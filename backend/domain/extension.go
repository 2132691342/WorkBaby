// 扩展聚合根：PI 插件（扩展 / 包）在界面上的形态与操作入参。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrExtensionNotFound = pkg.New("9716", "这个插件不存在", "刷新列表再试")
	ErrPackageSource     = pkg.New("9718", "插件来源格式不对", "例：npm:@scope/pkg、git:github.com/owner/repo、./本地目录")
)

// 扩展条目的种类。
const (
	ExtensionKindFile    = "file"
	ExtensionKindDir     = "dir"
	ExtensionKindPattern = "pattern" // settings 里的通配条目，界面只读
	ExtensionKindMissing = "missing" // 登记了但磁盘上找不到
)

// ExtensionVO 是扩展目录里的一个条目在界面上的形态。
type ExtensionVO struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}

// PackageVO 是一条通过 pi install 装进来的包：来源 + 解包目录 + 版本；Path 空表示登记了但没落地。
// Version 只有本地包目录里读得到；LatestVersion 由联网检查填，空表示未检查或查不到。
type PackageVO struct {
	Source        string `json:"source"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	Missing       bool   `json:"missing"`
	Kind          string `json:"kind"`
	Version       string `json:"version"`
	LatestVersion string `json:"latest_version"`
	Updateable    bool   `json:"updateable"`
}

// 包来源的种类：只有 npm 与 git 能被 pi update 搬到新版本。
const (
	PackageKindNpm   = "npm"
	PackageKindGit   = "git"
	PackageKindLocal = "local"
)

// ExtensionListRESP 是插件面板一次要的全部东西。
type ExtensionListRESP struct {
	Dir          string        `json:"dir"`
	SettingsPath string        `json:"settings_path"`
	Entries      []ExtensionVO `json:"entries"`
	Packages     []PackageVO   `json:"packages"`
}

// ToggleExtensionREQ 启停入参：Path 是绝对路径，不在路径段里传是因为 Windows 路径带盘符与斜杠。
type ToggleExtensionREQ struct {
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
}

// ImportExtensionsREQ 导入入参：磁盘上的 .ts / .js 文件或目录。
type ImportExtensionsREQ struct {
	Paths []string `json:"paths"`
}

// ImportExtensionsRESP 导入出参。
type ImportExtensionsRESP struct {
	Imported int      `json:"imported"`
	Skipped  []string `json:"skipped"`
}

// DeleteExtensionREQ 删除入参。
type DeleteExtensionREQ struct {
	Path string `json:"path"`
}

// PackageREQ 装包 / 卸包入参：Source 照 pi install 的写法。
type PackageREQ struct {
	Source string `json:"source"`
}

// MarketPackageVO 是插件市场里的一个可装包（npm 关键字 pi-package，PI 官方的发现机制）。
type MarketPackageVO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Publisher   string `json:"publisher"`
	Repo        string `json:"repo"`
	PublishedAt int64  `json:"published_at"`
	// Installed 由本地已装清单对账得出；InstalledVersion 是磁盘上那一份的版本。
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
}

// MarketListRESP 是插件市场一次要的清单；Error 非空表示注册表没连上（不是失败，面板仍要能显示空态）。
type MarketListRESP struct {
	Packages []MarketPackageVO `json:"packages"`
	Error    string            `json:"error"`
}
