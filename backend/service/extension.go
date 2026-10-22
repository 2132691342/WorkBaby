// 插件（PI 扩展与包）的编排：目录即真相源，settings.json 的 extensions / packages 是启用清单。
// 装包与卸包走 PI 自己的 CLI，WorkBaby 不重写一遍包的解析规则。
package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"WorkBaby/backend/domain"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
)

// ExtensionService 是插件面板的数据出口。
type ExtensionService struct {
	env *Env
}

// NewExtensionService 构造插件服务。
func NewExtensionService(env *Env) *ExtensionService { return &ExtensionService{env: env} }

// List 列出扩展目录里的条目与已登记的包。
// 登记了但磁盘上找不到的条目也列出来（Kind=missing）：静默消失会让人以为设置没保存。
func (s *ExtensionService) List() (*domain.ExtensionListRESP, error) {
	dir := s.env.PiEnv.ExtensionsDir()
	settings, err := s.env.PiEnv.ReadSettings()
	if err != nil {
		return nil, err
	}
	entries := []domain.ExtensionVO{}
	seen := map[string]bool{}

	appendEntry := func(path string, enabled bool) {
		key := strings.ToLower(filepath.Clean(path))
		if seen[key] {
			return
		}
		seen[key] = true
		entries = append(entries, domain.ExtensionVO{
			Name:    entryName(path),
			Path:    path,
			Kind:    kindOf(path),
			Enabled: enabled,
		})
	}

	if settings != nil {
		for _, raw := range settings.Extensions {
			appendEntry(s.resolve(raw), true)
		}
	}
	children, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, pkg.Wrap("1005", "读扩展目录失败", err)
	}
	for _, child := range children {
		appendEntry(filepath.Join(dir, child.Name()), false)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Enabled != entries[j].Enabled {
			return entries[i].Enabled
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	packages := []domain.PackageVO{}
	for _, pkg := range resolvePackages(s.env) {
		packages = append(packages, domain.PackageVO{
			Source: pkg.Source, Name: pkg.Name, Path: pkg.Path,
			Missing: pkg.Missing, Kind: pkg.Kind, Version: pkg.Version,
		})
	}
	return &domain.ExtensionListRESP{
		Dir:          dir,
		SettingsPath: s.env.PiEnv.SettingsPath(),
		Entries:      entries,
		Packages:     packages,
	}, nil
}

// Toggle 启停一条扩展：登记 / 注销 settings.json 的 extensions。
// 文件不搬走——PI 只读登记表，登记表就是开关。
func (s *ExtensionService) Toggle(path string, enabled bool) error {
	target := filepath.Clean(path)
	if target == "" {
		return domain.ErrExtensionNotFound
	}
	settings, err := s.readSettings()
	if err != nil {
		return err
	}
	list := []string{}
	for _, raw := range settings.Extensions {
		if !samePath(s.resolve(raw), target) {
			list = append(list, raw)
		}
	}
	if enabled {
		if kindOf(target) == domain.ExtensionKindMissing {
			return domain.ErrExtensionNotFound
		}
		list = append(list, target)
	}
	settings.Extensions = list
	return s.saveAndReload(settings)
}

// Import 把磁盘上的 .ts / .js 文件或扩展目录复制进 PI 的扩展目录，并登记启用。
func (s *ExtensionService) Import(paths []string) (*domain.ImportExtensionsRESP, error) {
	dir := s.env.PiEnv.ExtensionsDir()
	if err := pkg.EnsureDir(dir); err != nil {
		return nil, err
	}
	resp := &domain.ImportExtensionsRESP{Skipped: []string{}}
	settings, err := s.readSettings()
	if err != nil {
		return nil, err
	}
	for _, src := range paths {
		name := filepath.Base(src)
		if !isExtensionPath(src) {
			resp.Skipped = append(resp.Skipped, name+"：不是 .ts / .js 文件或目录")
			continue
		}
		dst := filepath.Join(dir, name)
		if pkg.FileExists(dst) || pkg.DirExists(dst) {
			resp.Skipped = append(resp.Skipped, name+"：同名插件已存在")
			continue
		}
		if pkg.DirExists(src) {
			if err := copyTree(src, dst); err != nil {
				return nil, pkg.Wrap("9716", "导入插件失败", err)
			}
		} else if err := pkg.CopyFile(src, dst); err != nil {
			return nil, pkg.Wrap("9716", "导入插件失败", err)
		}
		resp.Imported++
		settings.Extensions = append(settings.Extensions, dst)
	}
	if resp.Imported == 0 {
		return resp, nil
	}
	if err := s.saveAndReload(settings); err != nil {
		return nil, err
	}
	return resp, nil
}

// Delete 删掉一条扩展：磁盘上的文件或目录 + 登记项。
func (s *ExtensionService) Delete(path string) error {
	target := filepath.Clean(path)
	if kindOf(target) == domain.ExtensionKindMissing {
		return domain.ErrExtensionNotFound
	}
	if kindOf(target) == domain.ExtensionKindPattern {
		return pkg.New("9718", "这是设置里的通配条目，请直接编辑 settings.json", target)
	}
	settings, err := s.readSettings()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return pkg.Wrap("9716", "删除插件失败", err)
	}
	list := []string{}
	for _, raw := range settings.Extensions {
		if !samePath(s.resolve(raw), target) {
			list = append(list, raw)
		}
	}
	settings.Extensions = list
	return s.saveAndReload(settings)
}

// Install 用 PI 自己的 CLI 装一个包（npm / git / 本地路径）。
// 输出按行回传：npm 拉包可能几十秒，没有进度界面会被当成卡死。
func (s *ExtensionService) Install(source string, onLine func(string)) error {
	if !validPackageSource(source) {
		return domain.ErrPackageSource
	}
	if err := s.env.PiEnv.RunCLI([]string{"install", source}, onLine); err != nil {
		return err
	}
	return s.reload()
}

// Remove 卸掉一个包：登记项由 PI 自己从 settings.json 里删。
func (s *ExtensionService) Remove(source string, onLine func(string)) error {
	if strings.TrimSpace(source) == "" {
		return domain.ErrPackageSource
	}
	if err := s.env.PiEnv.RunCLI([]string{"remove", source}, onLine); err != nil {
		return err
	}
	return s.reload()
}

// Reload 重启全部 PI 进程并重扫技能：装完包之后技能列表也会变。
func (s *ExtensionService) Reload() error { return s.reload() }

// reload 重启进程 + 重扫技能。
func (s *ExtensionService) reload() error {
	NewSkillService(s.env).Reload(s.env.Workspace())
	sup := s.env.PiSupervisor()
	if sup == nil {
		return nil
	}
	return sup.RestartAll()
}

// readSettings 读 settings.json；不存在时给一份空配置。
func (s *ExtensionService) readSettings() (*pienv.SettingsFile, error) {
	settings, err := s.env.PiEnv.ReadSettings()
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = &pienv.SettingsFile{}
	}
	return settings, nil
}

// saveAndReload 落盘并让 PI 重读：扩展是进程启动期加载的，改登记表不重启等于没改。
func (s *ExtensionService) saveAndReload(settings *pienv.SettingsFile) error {
	if err := s.env.PiEnv.WriteSettings(settings); err != nil {
		return err
	}
	return s.reload()
}

// resolve 把 settings 里的登记项解成绝对路径：相对路径按 agent 目录解析，与 PI 一致。
func (s *ExtensionService) resolve(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if strings.HasPrefix(raw, "!") || strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-") ||
		strings.HasPrefix(raw, "builtin:") {
		return raw
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	return filepath.Join(s.env.PiEnv.AgentDir(), raw)
}

// entryName 给条目一个能显示的名字：文件去扩展名，目录取目录名。
func entryName(path string) string {
	name := filepath.Base(path)
	for _, prefix := range []string{"builtin:", "+builtin:", "-builtin:"} {
		if strings.HasPrefix(name, prefix) {
			return "内置 " + strings.TrimPrefix(name, prefix)
		}
	}
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

// kindOf 判断条目在磁盘上的形态。
func kindOf(path string) string {
	if strings.ContainsAny(path, "*?[]") || strings.HasPrefix(path, "!") ||
		strings.HasPrefix(path, "+") || strings.HasPrefix(path, "-") ||
		strings.HasPrefix(path, "builtin:") {
		return domain.ExtensionKindPattern
	}
	switch {
	case pkg.DirExists(path):
		return domain.ExtensionKindDir
	case pkg.FileExists(path):
		return domain.ExtensionKindFile
	default:
		return domain.ExtensionKindMissing
	}
}

// isExtensionPath 判断导入源像不像一个 PI 扩展。
func isExtensionPath(path string) bool {
	if pkg.DirExists(path) {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".js", ".mjs":
		return pkg.FileExists(path)
	default:
		return false
	}
}

// validPackageSource 放行 PI 认的四种来源写法。
func validPackageSource(source string) bool {
	source = strings.TrimSpace(source)
	switch {
	case source == "":
		return false
	case strings.HasPrefix(source, "npm:"), strings.HasPrefix(source, "git:"),
		strings.HasPrefix(source, "https://"), strings.HasPrefix(source, "http://"),
		strings.HasPrefix(source, "./"), strings.HasPrefix(source, "../"),
		filepath.IsAbs(source):
		return true
	default:
		// 裸包名（@scope/pkg、pkg@1.2）按 npm 处理，PI 也认。
		return !strings.ContainsAny(source, " \t")
	}
}
