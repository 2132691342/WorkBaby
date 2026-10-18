// 扩展聚合根：PI 插件（扩展 / 包）在界面上的形态与操作入参。
package domain

import "WorkBaby/backend/pkg"

var (
	ErrExtensionNotFound = pkg.New("9716", "这个插件不存在", "刷新列表再试")
	ErrExtensionBuiltin  = pkg.New("9717", "审批闸门是 WorkBaby 自带的，不能停用或删除", "")
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
	Builtin bool   `json:"builtin"`
}

// PackageVO 是一条通过 pi install 装进来的包来源（settings.json 的 packages）。
type PackageVO struct {
	Source string `json:"source"`
}

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
