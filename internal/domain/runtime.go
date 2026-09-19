// Package domain 的 Runtime 状态聚合根：内置运行时（node / python / powershell）的检查快照。
//
// 该聚合根不落库：运行时状态来自安装目录资源 + 用户目录解压结果，用于设置页「关于」排障。
package domain

// RuntimeStatusRESP 内置运行时整体状态；前端按资产逐条展示 ready / error。
type RuntimeStatusRESP struct {
	Home       string               `json:"home"`
	BundledDir string               `json:"bundled_dir"`
	Ready      bool                 `json:"ready"`
	Error      string               `json:"error,omitempty"`
	Assets     []RuntimeAssetStatus `json:"assets"`
}

// RuntimeAssetStatus 单个运行时资产状态：安装目录归档是否存在、用户目录是否已解压出可执行文件。
type RuntimeAssetStatus struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	Ready        bool   `json:"ready"`
	Path         string `json:"path"`
	Executable   string `json:"executable"`
	Archive      string `json:"archive"`
	ArchiveFound bool   `json:"archive_found"`
	Error        string `json:"error,omitempty"`
}
