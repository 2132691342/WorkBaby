// 命令执行环境：PI 的 bash 工具在 Windows 上只认真 bash，而 System32 下的 bash.exe
// 只是 WSL 启动器——没装发行版时每次都失败。本机一份 bash 都没有时解压内置 Git Bash 兜住。
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
	"WorkBaby/backend/runtime"
)

// ShellKind* 是命令执行环境的形态：前三种是真 bash，powershell 只用于状态展示。
const (
	ShellKindGitBash    = "git-bash"
	ShellKindMSYS       = "msys"
	ShellKindCygwin     = "cygwin"
	ShellKindPowerShell = "powershell"
	ShellKindNone       = "none"
)

// bashProbeTimeout 是「这份 bash 能不能跑」的试跑上限：坏掉的 WSL 桩会卡在这里。
const bashProbeTimeout = 3 * time.Second

// ShellStatus 是本机命令执行环境：Usable=false 表示 PI 一条命令都跑不了。
type ShellStatus struct {
	Kind   string
	Path   string
	Usable bool
	// Source 是这份 shell 的来源：settings / git / path / bundled / system。
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

// EnsureShell 把探测结果落到 PI 的 settings.json：选中 PI 自己找不到的 bash 时写 shellPath
// 引导过去。Applied 表示这次真的改了文件。
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
		if st.Source == "bundled" {
			return "Git Bash（内置）"
		}
		return "Git Bash"
	case ShellKindMSYS:
		return "MSYS2 Bash"
	case ShellKindCygwin:
		return "Cygwin Bash"
	case ShellKindPowerShell:
		return "PowerShell（没有可用的 bash，PI 的 bash 工具跑不了）"
	default:
		return "没有可用的命令执行环境"
	}
}

// probeShell 依次看：用户指定的 shellPath → 选定 git 自带的 bash → 各处的真 bash → 内置 Git Bash → PowerShell。
func (e *Env) probeShell() ShellStatus {
	var broken string
	if custom := strings.TrimSpace(e.settingString("shellPath")); custom != "" {
		path := expandPath(custom)
		if bashWorks(path) {
			return ShellStatus{Kind: shellKindOf(path), Path: path, Usable: true, Source: "settings"}
		}
		broken = "settings.json 里的 shellPath 用不了：" + path
	}
	// 与助手的 git 同源：选定那份 git 自带 bash 时优先用它，别在两处各挑一份。
	if git := e.Toolchain().Git; git.Bash != "" && bashWorks(git.Bash) {
		return ShellStatus{Kind: shellKindOf(git.Bash), Path: git.Bash, Usable: true, Source: gitSourceLabel(git)}
	}
	for _, c := range bashCandidates() {
		if bashWorks(c.path) {
			return ShellStatus{Kind: shellKindOf(c.path), Path: c.path, Usable: true, Source: c.source}
		}
	}
	// 本机一份能跑的 bash 都没有：解压内置 Git Bash（懒解压，只在这里发生）。
	if exe, err := runtime.EnsureGitBash(e.paths); err == nil {
		// 解压完工具链多了内置 git：让它参与后续解析（界面上的 git 状态也要跟着变）。
		e.resetProbes()
		if bashWorks(exe) {
			return ShellStatus{Kind: ShellKindGitBash, Path: exe, Usable: true, Source: "bundled"}
		}
		broken = "内置 Git Bash 解压了但跑不起来：" + exe
	} else if broken == "" {
		broken = "内置 Git Bash 不可用：" + err.Error()
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

// gitSourceLabel 把 git 来源翻成 shell 的 source 口径：git（系统的）/ bundled（内置）。
func gitSourceLabel(git GitPick) string {
	if git.Source == SourceBundled {
		return "bundled"
	}
	return "git"
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

// shellMarker 记录我们往 settings.json 写过的 shellPath：只有自己写的东西才敢撤回。
type shellMarker struct {
	ShellPath string `json:"shell_path,omitempty"`
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

// applyShell 只改 shellPath 这一个键：选中的 bash 是 PI 自己找不到的（非标准位置或内置）
// 才写进去引导；PI 自己找得到就保持用户配置原样。工具集（defaultTools）归工具面板管，这里不碰。
func (e *Env) applyShell(st ShellStatus) (bool, error) {
	if st.Kind == ShellKindNone {
		return false, pkg.New("9717", "这台机器上没有可用的命令执行环境", st.Error)
	}
	raw, err := e.settingsMap()
	if err != nil {
		return false, err
	}
	marker := e.readShellMarker()
	current := e.settingString("shellPath")
	changed := false

	switch {
	case st.Kind == ShellKindPowerShell && marker.ShellPath != "" && strings.EqualFold(current, marker.ShellPath):
		// 我们写过、如今已经不能用的 shellPath 必须撤掉，否则 PI 每次起 shell 都报错。
		delete(raw, "shellPath")
		marker.ShellPath = ""
		changed = true
	case st.Kind != ShellKindPowerShell && needsShellPath(st.Path) && !strings.EqualFold(current, st.Path):
		// 选中了 PI 找不到的 bash（含内置）：写 shellPath 指过去。
		encoded, _ := json.Marshal(st.Path)
		raw["shellPath"] = encoded
		marker.ShellPath = st.Path
		changed = true
	case st.Kind != ShellKindPowerShell && !needsShellPath(st.Path) && marker.ShellPath != "" && strings.EqualFold(current, marker.ShellPath):
		// 我们写过的 shellPath 已被更好的 bash 取代（PI 自己找得到那份）：撤掉，别一直吊在内置上。
		delete(raw, "shellPath")
		marker.ShellPath = ""
		changed = true
	}
	if changed {
		if err := writeSettingsMap(e.SettingsPath(), raw); err != nil {
			return false, err
		}
		if err := e.writeShellMarker(marker); err != nil {
			return false, err
		}
	}
	return changed, nil
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
