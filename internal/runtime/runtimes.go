// Package runtime 提供运行时的横切设施：数据目录解析、内置运行时管理。
// 不依赖 Wails runtime；可纯单测。
package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// Manifest 描述安装包预置的内置运行时（manifest.json 契约）。
type Manifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	Platform      string          `json:"platform"`
	Arch          string          `json:"arch"`
	Assets        []ManifestAsset `json:"assets"`
}

// ManifestAsset 单个运行时资产：node / python / powershell。
type ManifestAsset struct {
	ID                     string   `json:"id"`
	Version                string   `json:"version"`
	ArchiveFile            string   `json:"archiveFile"`
	ArchiveType            string   `json:"archiveType"`
	StripComponents        int      `json:"stripComponents"`
	SHA256                 string   `json:"sha256"`
	ExecutableRelativePath string   `json:"executableRelativePath"`
	PathDirs               []string `json:"pathDirs"`
	MaxFiles               int      `json:"maxFiles"`
	MaxUncompressedBytes   int64    `json:"maxUncompressedBytes"`
}

// Manager 管理内置运行时的首跑解压与 PATH 前置。
// 解压目标 {home}/runtimes/{id}/{version}；已就绪（可执行文件存在）即跳过。
type Manager struct {
	home       string
	bundledDir string // 显式指定的内置资源目录（dev/测试注入）；空则自动探测
}

// NewManager 构造 Manager；home 为应用数据根目录（runtime.Paths.Home）。
func NewManager(home string) *Manager { return &Manager{home: home} }

// SetBundledDir 显式指定内置资源目录（含 manifest.json + 归档文件）；
// 供 dev 模式 / 测试注入，空值表示回退自动探测。
func (m *Manager) SetBundledDir(dir string) *Manager {
	m.bundledDir = dir
	return m
}

// LocateBundledDir 定位内置资源目录（含 manifest.json + 归档文件）。
// 优先级：显式指定 → 生产 exe 同级 runtimes/ → dev 工作目录下 runtimes/ 或 build/windows/runtimes/。
func (m *Manager) LocateBundledDir() string {
	if m.bundledDir != "" && isValidBundledDir(m.bundledDir) {
		return m.bundledDir
	}
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Join(filepath.Dir(exe), "runtimes"); isValidBundledDir(dir) {
			return dir
		}
	}
	if wd, err := os.Getwd(); err == nil {
		for _, rel := range []string{"runtimes", filepath.Join("build", "windows", "runtimes"), filepath.Join("assets", "runtimes")} {
			if dir := filepath.Join(wd, rel); isValidBundledDir(dir) {
				return dir
			}
		}
	}
	return ""
}

// Ensure 首跑解压全部内置运行时；无内置资源或缺归档时跳过对应资产。
// 单个资产失败返回错误，由调用方降级（仅告警不阻断启动）。
func (m *Manager) Ensure() error {
	dir := m.LocateBundledDir()
	if dir == "" {
		return nil
	}
	mf := m.loadManifest(dir)
	if mf == nil {
		return nil
	}
	for _, a := range mf.Assets {
		if err := m.ensureAsset(dir, a); err != nil {
			return pkg.Wrap(1014, "extract runtime "+a.ID+" failed", err)
		}
	}
	return nil
}

// Status 返回内置运行时检查快照；无资源或 manifest 不可用时给出 error 供设置页排障。
func (m *Manager) Status() domain.RuntimeStatusRESP {
	status := domain.RuntimeStatusRESP{Home: m.home, Assets: []domain.RuntimeAssetStatus{}}
	dir := m.LocateBundledDir()
	if dir == "" {
		status.Error = "bundled runtime dir not found"
		return status
	}
	status.BundledDir = dir
	mf := m.loadManifest(dir)
	if mf == nil {
		status.Error = "invalid runtime manifest"
		return status
	}
	status.Ready = len(mf.Assets) > 0
	for _, a := range mf.Assets {
		target := filepath.Join(m.home, "runtimes", a.ID, a.Version)
		archive := filepath.Join(dir, a.ArchiveFile)
		item := domain.RuntimeAssetStatus{
			ID:           a.ID,
			Version:      a.Version,
			Path:         target,
			Executable:   filepath.Join(target, filepath.FromSlash(a.ExecutableRelativePath)),
			Archive:      archive,
			ArchiveFound: isRegularFile(archive),
			Ready:        isReady(target, a.ExecutableRelativePath),
		}
		if !item.ArchiveFound {
			item.Error = "archive not found"
		}
		if !item.Ready {
			status.Ready = false
		}
		status.Assets = append(status.Assets, item)
	}
	return status
}

// BinDirs 返回已就绪运行时的 bin 目录（动态检查目录存在性，解压完成后自动纳入）。
// 供 exec 工具在执行时前置到子进程 PATH，实现内置环境隔离。
func (m *Manager) BinDirs() []string {
	dir := m.LocateBundledDir()
	if dir == "" {
		return nil
	}
	mf := m.loadManifest(dir)
	if mf == nil {
		return nil
	}
	var dirs []string
	for _, a := range mf.Assets {
		if a.ID == "" || a.Version == "" {
			continue
		}
		base := filepath.Join(m.home, "runtimes", a.ID, a.Version)
		if len(a.PathDirs) == 0 {
			if isDir(base) {
				dirs = append(dirs, base)
			}
			continue
		}
		for _, pd := range a.PathDirs {
			d := base
			if pd != "" && pd != "." {
				d = filepath.Join(base, pd)
			}
			if isDir(d) {
				dirs = append(dirs, d)
			}
		}
	}
	return dirs
}

// EnhancePath 把内置运行时 bin 目录前置到 PATH（Windows ';'，其余 ':'）。
func (m *Manager) EnhancePath(existing string) string {
	dirs := m.BinDirs()
	if len(dirs) == 0 {
		return existing
	}
	sep := string(os.PathListSeparator)
	extra := strings.Join(dirs, sep)
	if existing == "" {
		return extra
	}
	return extra + sep + existing
}

func (m *Manager) ensureAsset(bundled string, a ManifestAsset) error {
	if a.ID == "" || a.Version == "" || a.ArchiveFile == "" {
		return nil
	}
	target := filepath.Join(m.home, "runtimes", a.ID, a.Version)
	if isReady(target, a.ExecutableRelativePath) {
		return nil
	}
	archive := filepath.Join(bundled, a.ArchiveFile)
	if !isRegularFile(archive) {
		return nil // 缺归档文件，跳过该资产
	}
	maxFiles := a.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 5000
	}
	maxBytes := a.MaxUncompressedBytes
	if maxBytes <= 0 {
		maxBytes = 256 << 20
	}
	return Extract(archive, target, a.StripComponents, maxFiles, maxBytes)
}

func (m *Manager) loadManifest(dir string) *Manifest {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil
	}
	var mf Manifest
	if err := json.Unmarshal(data, &mf); err != nil {
		return nil
	}
	return &mf
}

func isValidBundledDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "manifest.json"))
	return err == nil && !info.IsDir()
}

func isReady(target, exeRel string) bool {
	if exeRel != "" {
		return isRegularFile(filepath.Join(target, filepath.FromSlash(exeRel)))
	}
	entries, err := os.ReadDir(target)
	return err == nil && len(entries) > 0
}

func isRegularFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
