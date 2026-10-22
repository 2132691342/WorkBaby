// 命令执行环境：PI 的 bash 工具在 Windows 上只认真 bash，
// 而 System32 下的 bash.exe 只是 WSL 启动器——没装发行版时每次都失败。
package env

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/pkg"
)

// ShellKind* 是命令执行环境的形态：前三种是真 bash，powershell 是兜底工具。
const (
	ShellKindGitBash    = "git-bash"
	ShellKindMSYS       = "msys"
	ShellKindCygwin     = "cygwin"
	ShellKindPowerShell = "powershell"
	ShellKindNone       = "none"
)

// shellToolMods 是没有 bash 时写进 defaultTools 的增量：把 bash 换成 powershell。
var shellToolMods = []string{"+powershell", "-bash"}

// bashProbeTimeout 是「这份 bash 能不能跑」的试跑上限：坏掉的 WSL 桩会卡在这里。
const bashProbeTimeout = 3 * time.Second

// ShellStatus 是本机命令执行环境：Usable=false 表示 PI 一条命令都跑不了。
type ShellStatus struct {
	Kind   string
	Path   string
	Usable bool
	// Source 是这份 shell 的来源：settings / git / path / system。
	Source string
	Error  string
	// Applied 表示这一次探测顺手改写了 PI 的 settings.json。
	Applied bool
}

var (
	shellMu     sync.Mutex
	shellCached *ShellStatus
)

// ShellStatusOf 返回命令执行环境：探测一次后缓存，界面轮询不会反复起进程。
func (e *Env) ShellStatusOf() ShellStatus {
	shellMu.Lock()
	defer shellMu.Unlock()
	if shellCached == nil {
		st := e.probeShell()
		shellCached = &st
	}
	return *shellCached
}

// RefreshShell 丢掉缓存重新探测（用户刚装了 Git / 刚改了 PATH）。
func (e *Env) RefreshShell() ShellStatus {
	shellMu.Lock()
	shellCached = nil
	shellMu.Unlock()
	return e.ShellStatusOf()
}

// EnsureShell 把探测结果落到 PI 的 settings.json：有真 bash 就写 shellPath，
// 没有就把工具集从 bash 换成 powershell。Applied 表示这次真的改了文件。
func (e *Env) EnsureShell() (ShellStatus, error) {
	st := e.RefreshShell()
	applied, err := e.applyShell(st)
	st.Applied = applied
	return st, err
}

// ShellLabel 把形态翻成给人看的一句话。
func ShellLabel(st ShellStatus) string {
	switch st.Kind {
	case ShellKindGitBash:
		return "Git Bash"
	case ShellKindMSYS:
		return "MSYS2 Bash"
	case ShellKindCygwin:
		return "Cygwin Bash"
	case ShellKindPowerShell:
		return "PowerShell（没有找到可用的 bash）"
	default:
		return "没有可用的命令执行环境"
	}
}

// probeShell 依次看：用户指定的 shellPath → 各处的真 bash → PowerShell。
func (e *Env) probeShell() ShellStatus {
	var broken string
	if custom := strings.TrimSpace(e.settingString("shellPath")); custom != "" {
		path := expandPath(custom)
		if bashWorks(path) {
			return ShellStatus{Kind: shellKindOf(path), Path: path, Usable: true, Source: "settings"}
		}
		broken = "settings.json 里的 shellPath 用不了：" + path
	}
	for _, c := range bashCandidates() {
		if bashWorks(c.path) {
			return ShellStatus{Kind: shellKindOf(c.path), Path: c.path, Usable: true, Source: c.source}
		}
	}
	if ps := powershellExe(); ps != "" {
		return ShellStatus{Kind: ShellKindPowerShell, Path: ps, Usable: true, Source: "system"}
	}
	if broken == "" {
		broken = "这台机器上既没有可用的 bash，也没有 PowerShell"
	}
	return ShellStatus{Kind: ShellKindNone, Error: broken}
}

// shellCandidate 是一份候选 bash 与它的来源。
type shellCandidate struct {
	path   string
	source string
}

// bashCandidates 列出所有可能是真 bash 的路径：Git 安装目录优先，其次 PATH。
func bashCandidates() []shellCandidate {
	seen := map[string]bool{}
	out := []shellCandidate{}
	add := func(path, source string) {
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		key := strings.ToLower(path)
		// WSL 启动器在列表里必须被剔掉：它存在、可执行，但一条命令都跑不了。
		if seen[key] || isWSLStub(path) {
			return
		}
		seen[key] = true
		out = append(out, shellCandidate{path: path, source: source})
	}
	for _, dir := range gitInstallDirs() {
		add(filepath.Join(dir, "bin", "bash.exe"), "git")
	}
	for _, path := range bashOnPath() {
		add(path, "path")
	}
	return out
}

// gitInstallDirs 列出 Git 的安装根：环境变量、注册表与 PATH 上的 git.exe 三路推断。
func gitInstallDirs() []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(dir string) {
		if dir == "" || !pkg.DirExists(dir) {
			return
		}
		key := strings.ToLower(filepath.Clean(dir))
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, filepath.Clean(dir))
	}
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		base := os.Getenv(key)
		if base == "" {
			continue
		}
		add(filepath.Join(base, "Git"))
		if key == "LOCALAPPDATA" {
			add(filepath.Join(base, "Programs", "Git"))
		}
	}
	for _, dir := range gitRegistryDirs() {
		add(dir)
	}
	for _, dir := range gitPathDirs() {
		add(dir)
	}
	return out
}

// gitPathDirs 从 PATH 上的 git.exe 反推安装根：<root>\cmd\git.exe 与 <root>\bin\git.exe 都常见。
func gitPathDirs() []string {
	exe, err := exec.LookPath("git.exe")
	if err != nil {
		return nil
	}
	dir := filepath.Dir(exe)
	if base := strings.ToLower(filepath.Base(dir)); base == "cmd" || base == "bin" {
		return []string{filepath.Dir(dir)}
	}
	return []string{dir}
}

// bashOnPath 按 PATH 顺序挑出存在的 bash.exe，跳过 WSL 启动器。
func bashOnPath() []string {
	out := []string{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "bash.exe")
		if pkg.FileExists(candidate) && !isWSLStub(candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

// isWSLStub 判断这份 bash.exe 是不是 WSL 的启动器（有没有发行版都不该选它）。
func isWSLStub(path string) bool {
	clean := strings.ToLower(filepath.Clean(path))
	if root := os.Getenv("SystemRoot"); root != "" {
		if clean == strings.ToLower(filepath.Join(root, "System32", "bash.exe")) {
			return true
		}
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		if clean == strings.ToLower(filepath.Join(local, "Microsoft", "WindowsApps", "bash.exe")) {
			return true
		}
	}
	return false
}

// bashWorks 真跑一次：文件存在不代表能用，WSL 桩就是这么骗过 PI 的。
func bashWorks(exe string) bool {
	if !pkg.FileExists(exe) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), bashProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-c", "echo wb-shell-ok")
	hideWindow(cmd)
	out, err := cmd.Output()
	return err == nil && strings.Contains(string(out), "wb-shell-ok")
}

// shellKindOf 按路径认出 bash 的家族。
func shellKindOf(path string) string {
	lower := strings.ToLower(filepath.Clean(path))
	switch {
	case strings.Contains(lower, `\msys`):
		return ShellKindMSYS
	case strings.Contains(lower, `\cygwin`):
		return ShellKindCygwin
	default:
		return ShellKindGitBash
	}
}

// powershellExe 找 PowerShell：pwsh 7 优先，系统自带的 5.1 一定在。
func powershellExe() string {
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if exe, err := exec.LookPath(name); err == nil {
			return exe
		}
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		exe := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if pkg.FileExists(exe) {
			return exe
		}
	}
	return ""
}

// expandPath 展开 shellPath 里允许的前导 ~。
func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimLeft(path[1:], `\/`))
		}
	}
	return filepath.Clean(path)
}

// settingString 读 settings.json 的一个字符串字段。
func (e *Env) settingString(key string) string {
	raw, err := e.settingsMap()
	if err != nil {
		return ""
	}
	var value string
	if json.Unmarshal(raw[key], &value) != nil {
		return ""
	}
	return value
}

// shellMarker 记录我们往 settings.json 写过什么：只有自己写的东西才敢撤回。
// ToolList 是写入后的完整列表：用户之后动过工具开关，就跟它对不上，那时不再替用户做主。
type shellMarker struct {
	ShellPath  string   `json:"shell_path,omitempty"`
	ToolMods   []string `json:"tool_mods,omitempty"`
	ToolList   []string `json:"tool_list,omitempty"`
	ToolBefore []string `json:"tool_before,omitempty"`
	ToolHadKey bool     `json:"tool_had_key,omitempty"`
}

// markerPath 是自愈记录的落点；它不进 PI 的配置目录，免得被当成用户配置。
func (e *Env) markerPath() string { return filepath.Join(e.paths.RuntimeDir, "shell.json") }

// readShellMarker 读自愈记录；读不到按「没写过」处理。
func (e *Env) readShellMarker() shellMarker {
	var marker shellMarker
	raw, err := os.ReadFile(e.markerPath())
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		return shellMarker{}
	}
	if json.Unmarshal(raw, &marker) != nil {
		return shellMarker{}
	}
	return marker
}

// writeShellMarker 落盘自愈记录（原子替换）。
func (e *Env) writeShellMarker(marker shellMarker) error {
	body, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return pkg.Wrap("9717", "记录命令执行环境失败", err)
	}
	return WriteFileAtomic(e.markerPath(), append(body, '\n'), 0o600)
}

// applyShell 只改自己该改的键：shellPath 与 defaultTools 里的工具增量。
// 有真 bash 时把之前改过的工具集原样还回去，让 bash 工具回到默认集。
func (e *Env) applyShell(st ShellStatus) (bool, error) {
	if st.Kind == ShellKindNone {
		return false, pkg.New("9717", "这台机器上没有可用的命令执行环境", st.Error)
	}
	raw, err := e.settingsMap()
	if err != nil {
		return false, err
	}
	marker := e.readShellMarker()
	current, listOK := toolList(raw)
	changed, markerDirty := false, false

	if st.Kind == ShellKindPowerShell {
		hadKey := hasDefaultTools(raw)
		if marker.ToolList == nil && listOK {
			if next, added := withToolMods(current, shellToolMods); len(added) > 0 {
				raw["defaultTools"] = encodeToolList(next)
				marker.ToolMods = added
				marker.ToolList = next
				marker.ToolBefore = current
				marker.ToolHadKey = hadKey
				changed = true
			}
		}
		// 我们自己写过、如今已经不能用的 shellPath 必须撤掉，否则 PI 每次起 shell 都报错。
		if marker.ShellPath != "" && strings.EqualFold(e.settingString("shellPath"), marker.ShellPath) {
			delete(raw, "shellPath")
			marker.ShellPath = ""
			changed, markerDirty = true, true
		}
	} else {
		if len(marker.ToolMods) > 0 {
			if marker.ToolList != nil && listOK && equalNames(current, marker.ToolList) {
				if marker.ToolHadKey {
					raw["defaultTools"] = encodeToolList(marker.ToolBefore)
				} else {
					delete(raw, "defaultTools")
				}
				changed = true
			}
			// 记下的一律清掉：列表对不上说明用户自己动过，之后不再替用户做主。
			marker.ToolMods, marker.ToolList, marker.ToolBefore, marker.ToolHadKey = nil, nil, nil, false
			markerDirty = true
		}
		if needsShellPath(st.Path) && !strings.EqualFold(e.settingString("shellPath"), st.Path) {
			encoded, _ := json.Marshal(st.Path)
			raw["shellPath"] = encoded
			marker.ShellPath = st.Path
			changed = true
		}
	}
	if changed {
		if err := writeSettingsMap(e.SettingsPath(), raw); err != nil {
			return false, err
		}
	}
	if changed || markerDirty {
		if err := e.writeShellMarker(marker); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// toolList 读 defaultTools；键不存在返回空表（空表 = 之后可能被增量填上）。
func toolList(raw map[string]json.RawMessage) ([]string, bool) {
	body, ok := raw["defaultTools"]
	if !ok {
		return []string{}, true
	}
	var list []string
	if json.Unmarshal(body, &list) != nil {
		return nil, false
	}
	return list, true
}

// hasDefaultTools 报告 defaultTools 键是否存在。
func hasDefaultTools(raw map[string]json.RawMessage) bool {
	_, ok := raw["defaultTools"]
	return ok
}

// withToolMods 追加增量并返回真正新增的那几项；已有同名项不算新增。
func withToolMods(current, mods []string) ([]string, []string) {
	present := map[string]bool{}
	for _, name := range current {
		present[name] = true
	}
	next := append([]string{}, current...)
	added := []string{}
	for _, mod := range mods {
		if present[mod] {
			continue
		}
		next = append(next, mod)
		present[mod] = true
		added = append(added, mod)
	}
	return next, added
}

// equalNames 比两份工具名列表是否完全一致（顺序敏感：它就是写进文件的那一行）。
func equalNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// encodeToolList 把工具名列表编码回 JSON。
func encodeToolList(list []string) json.RawMessage {
	if list == nil {
		list = []string{}
	}
	body, _ := json.Marshal(list)
	return body
}

// needsShellPath 判断 PI 自己找不找得到这份 bash：找得到就别改用户的配置。
func needsShellPath(path string) bool {
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		base := os.Getenv(key)
		if base == "" {
			continue
		}
		if strings.EqualFold(filepath.Clean(path), filepath.Clean(filepath.Join(base, "Git", "bin", "bash.exe"))) {
			return false
		}
	}
	// PI 取 PATH 上的第一个 bash.exe：只要那份就是我们选的，它也不会跑去用 WSL 启动器。
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, "bash.exe")
		if !pkg.FileExists(candidate) {
			continue
		}
		return !strings.EqualFold(filepath.Clean(candidate), filepath.Clean(path))
	}
	return true
}
