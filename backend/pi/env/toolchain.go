// 运行时工具链：本机用哪份 node / git / npm 前缀，以及 PI 当初是用哪一份装起来的。
// 装的时候用哪份，后续升级与插件管理就只用那份——换了解释器或前缀等于写出第二份 PI。
package env

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/runtime"
)

// 来源标记：界面上要说清「这份是系统的还是内置的」。
const (
	SourceSystem  = "system"
	SourceBundled = "bundled"
	SourceEnv     = "env"
)

// NodePick 是选定的一份 node 与它同源的 npm 脚本。
type NodePick struct {
	Exe     string
	Version string
	Source  string
	// Npm 是同源的 npm-cli.js（空表示这份 node 旁边没有 npm）。
	Npm string
}

// GitPick 是选定的一份 git；Bash 是它自带的真 bash（没有则为空）。
type GitPick struct {
	Exe     string
	Version string
	Root    string
	Source  string
	Bash    string
}

// Toolchain 是一次解析的结果：node / git 与它们的来源。
type Toolchain struct {
	Node NodePick
	Git  GitPick
	// Bound 表示 node 来自上次装 PI 时钉住的那一份。
	Bound bool
}

// binding 记录装 PI 时用的是哪份 node / npm 前缀 / git：升级与插件管理必须回到同一份。
type binding struct {
	NodeExe     string `json:"node_exe,omitempty"`
	NodeSource  string `json:"node_source,omitempty"`
	NodeVersion string `json:"node_version,omitempty"`
	NpmScript   string `json:"npm_script,omitempty"`
	PiPrefix    string `json:"pi_prefix,omitempty"`
	GitExe      string `json:"git_exe,omitempty"`
	GitSource   string `json:"git_source,omitempty"`
	UpdatedAt   int64  `json:"updated_at,omitempty"`
}

// nodeCandidate 是一份候选 node 与它的来源。
type nodeCandidate struct {
	exe    string
	source string
}

// bindingPath 是绑定文件的落点：放数据目录，不进 PI 的配置目录。
func (e *Env) bindingPath() string { return filepath.Join(e.paths.RuntimeDir, "toolchain.json") }

// readBinding 读绑定记录；读不到按「还没装过」处理。
func (e *Env) readBinding() binding {
	var b binding
	raw, err := os.ReadFile(e.bindingPath())
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		return binding{}
	}
	if ReadJSON(e.bindingPath(), &b) != nil {
		return binding{}
	}
	return b
}

// writeBinding 落盘绑定记录（原子替换）。
func (e *Env) writeBinding(b binding) error {
	b.UpdatedAt = time.Now().UnixMilli()
	body, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return pkg.Wrap("9712", "记录运行时绑定失败", err)
	}
	return WriteFileAtomic(e.bindingPath(), append(body, '\n'), 0o600)
}

// Toolchain 返回本机的运行时组合：进程内只解析一次，装 / 升 / 切之后由 RefreshProbes 作废。
func (e *Env) Toolchain() Toolchain {
	e.tcMu.Lock()
	defer e.tcMu.Unlock()
	if e.tcCache != nil {
		return *e.tcCache
	}
	tc := e.resolveToolchain()
	e.tcCache = &tc
	return tc
}

// resolveToolchain 解析 node 与 git；调用方持锁，这里不得再回调 Toolchain。
func (e *Env) resolveToolchain() Toolchain {
	b := e.readBinding()
	tc := Toolchain{Git: e.pickGit()}
	node, ok := e.pickNode(b)
	if !ok {
		return tc
	}
	tc.Node = node
	tc.Bound = b.NodeExe != "" && samePath(b.NodeExe, node.Exe)
	return tc
}

// pickNode 按「候选 → 内置」的顺序选一份 node：候选都没有能跑的才解压内置那份。
// 绑定优先在 selectNode 里：PI 装在哪份解释器上，升级与插件生态就留在那份上。
func (e *Env) pickNode(b binding) (NodePick, bool) {
	if c, ok := selectNode(e.nodeCandidates(b), b, nodeVersionOf); ok {
		return pickOf(c), true
	}
	// 内置：只在系统里一份能跑的都没有时才解压。
	exe, err := runtime.EnsureNode(e.paths)
	if err != nil {
		return NodePick{}, false
	}
	return NodePick{Exe: exe, Version: nodeVersionOf(exe), Source: SourceBundled, Npm: npmScriptAt(e.paths.NodeDir)}, true
}

// selectNode 在候选里按「绑定 → 系统里最高合规版本 → 任意能跑的」挑一份。
func selectNode(cands []nodeCandidate, b binding, version func(string) string) (nodeCandidate, bool) {
	for _, c := range cands {
		if b.NodeExe != "" && samePath(c.exe, b.NodeExe) && nodeSatisfies(version(c.exe)) {
			return c, true
		}
	}
	// 系统里最合用的那份：合规版本里取最高的，避免 PATH 上先命中一份旧 node。
	best, bestVer := nodeCandidate{}, ""
	for _, c := range cands {
		if c.source == SourceBundled {
			continue
		}
		v := version(c.exe)
		if !nodeSatisfies(v) {
			continue
		}
		if best.exe == "" || compareNodeVersion(v, bestVer) > 0 {
			best, bestVer = c, v
		}
	}
	if best.exe != "" {
		return best, true
	}
	// 都不合规：退回任意一份能跑的，闸门会按版本报错并说清原因。
	for _, c := range cands {
		if version(c.exe) != "" {
			return c, true
		}
	}
	return nodeCandidate{}, false
}

// pickOf 把候选整理成 NodePick，并带上同源的 npm 脚本。
func pickOf(c nodeCandidate) NodePick {
	version := nodeVersionOf(c.exe)
	npm := npmScriptAt(filepath.Dir(c.exe))
	if npm == "" {
		npm = npmShimAt(filepath.Dir(c.exe))
	}
	return NodePick{Exe: c.exe, Version: version, Source: c.source, Npm: npm}
}

// nodeCandidates 列出本机可能是 node 的位置：绑定 → 显式指定 → 系统 PATH
// → 常见安装目录（官方安装包 / nvm / fnm / scoop）→ 已解压的内置运行时。
func (e *Env) nodeCandidates(b binding) []nodeCandidate {
	out := []nodeCandidate{}
	seen := map[string]bool{}
	addFile := func(exe, source string) {
		exe = strings.TrimSpace(exe)
		if exe == "" || !pkg.FileExists(exe) {
			return
		}
		key := strings.ToLower(pkg.RealPath(exe))
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, nodeCandidate{exe: filepath.Clean(exe), source: source})
	}
	addDir := func(dir, source string) { addFile(filepath.Join(dir, "node.exe"), source) }

	addFile(b.NodeExe, e.sourceOfNode(b.NodeExe))
	addFile(os.Getenv("WORKBABY_NODE_EXE"), SourceEnv)
	for _, dir := range effectivePathDirs() {
		addDir(dir, SourceSystem)
	}
	for _, dir := range nodeInstallDirs() {
		addDir(dir, SourceSystem)
	}
	if st := runtime.InspectNode(e.paths); st.Exe != "" {
		addFile(st.Exe, SourceBundled)
	}
	return out
}

// sourceOfNode 按路径判定来源：落在本应用运行时目录里的算内置。
func (e *Env) sourceOfNode(exe string) string {
	if exe == "" {
		return ""
	}
	if pkg.IsInside(e.paths.NodeDir, exe) {
		return SourceBundled
	}
	return SourceSystem
}

// pickGit 选一份 git：用户装的那份优先，一份都没有才用已解压的内置 MinGit。
// 内置不在探测里自动解压：装了 Git 的机器不该为一个状态显示白占几十 MB。
func (e *Env) pickGit() GitPick {
	for _, exe := range e.gitCandidates() {
		pick := GitPick{Exe: exe, Source: SourceSystem, Root: gitRootOf(exe), Version: gitVersionOf(exe)}
		if pick.Version == "" {
			continue
		}
		pick.Bash = bashNearGit(pick.Root)
		return pick
	}
	st := runtime.InspectGitBash(e.paths)
	if st.Git != "" {
		return GitPick{
			Exe: st.Git, Version: gitVersionOf(st.Git), Root: st.Dir,
			Source: SourceBundled, Bash: st.Bash,
		}
	}
	return GitPick{}
}

// gitCandidates 列出系统里的 git.exe：PATH 顺序优先，其次注册表与常见安装目录。
func (e *Env) gitCandidates() []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(exe string) {
		exe = strings.TrimSpace(exe)
		if exe == "" || !pkg.FileExists(exe) {
			return
		}
		key := strings.ToLower(pkg.RealPath(exe))
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, filepath.Clean(exe))
	}
	for _, dir := range effectivePathDirs() {
		add(filepath.Join(dir, "git.exe"))
	}
	for _, dir := range gitInstallDirs() {
		add(filepath.Join(dir, "cmd", "git.exe"))
		add(filepath.Join(dir, "bin", "git.exe"))
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		add(filepath.Join(local, "Microsoft", "WinGet", "Links", "git.exe"))
	}
	return out
}

// GitExe 返回当前生效的 git（内置那份没解压过就是空串，不触发解压）。
func (e *Env) GitExe() string { return e.Toolchain().Git.Exe }

// ToolchainLabel 把 node 来源翻成给人看的一句话（安装日志与界面共用）。
func ToolchainLabel(tc Toolchain) string {
	switch tc.Node.Source {
	case SourceBundled:
		return "内置 Node"
	case SourceEnv:
		return "手动指定"
	default:
		return "系统 Node"
	}
}

// gitRootOf 从 git.exe 反推安装根：<root>\cmd\git.exe 与 <root>\bin\git.exe 都常见。
func gitRootOf(exe string) string {
	dir := filepath.Dir(filepath.Clean(exe))
	if base := strings.ToLower(filepath.Base(dir)); base == "cmd" || base == "bin" {
		return filepath.Dir(dir)
	}
	return dir
}

// bashNearGit 找这份 git 自带的 bash：PI 的 bash 工具要的是真 bash，不是 WSL 启动器。
func bashNearGit(root string) string {
	for _, rel := range []string{`bin\bash.exe`, `usr\bin\bash.exe`} {
		candidate := filepath.Join(root, rel)
		if pkg.FileExists(candidate) && !isWSLStub(candidate) {
			return candidate
		}
	}
	return ""
}

// gitVersionOf 取 git 的版本串（`git --version` 的第一行）；同一个 exe 只跑一次。
func gitVersionOf(exe string) string {
	return cachedVersion("git:"+exe, func() string {
		cmd := exec.Command(exe, "--version")
		hideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	})
}

// installPrefix 返回 PI 该装回哪里：装过就回到同一份前缀，没装过才按当前 node 的 npm 定。
// 系统 node 用它可写的 npm 全局前缀（pi 落在已生效的 PATH 上），写不进去或内置 node 时落回本应用前缀。
func (e *Env) installPrefix() string {
	e.installMu.Lock()
	defer e.installMu.Unlock()
	if e.installDone {
		return e.installPath
	}
	e.installPath, e.installDone = e.resolveInstallPrefix(), true
	return e.installPath
}

// resolveInstallPrefix 是 installPrefix 的实际判定；调用方持锁，这里不得再回调 installPrefix。
func (e *Env) resolveInstallPrefix() string {
	b := e.readBinding()
	if b.PiPrefix != "" && dirWritable(b.PiPrefix) && samePath(b.NodeExe, e.Toolchain().Node.Exe) {
		return b.PiPrefix
	}
	// 已经装在某处（不管是不是我们装的）：升级必须写回原处，否则机器上会多出一份。
	if prefix := e.ownPiPrefix(); prefix != "" && dirWritable(prefix) {
		return prefix
	}
	if e.Toolchain().Node.Source == SourceBundled {
		return e.paths.NpmGlobalDir
	}
	if prefix := e.npmPrefix(); prefix != "" && dirWritable(prefix) {
		return prefix
	}
	return e.paths.NpmGlobalDir
}

// ownPiPrefix 从已装的 PI 反推它所在的 npm 前缀（用户自己装的那份，升级要写回原处）。
func (e *Env) ownPiPrefix() string {
	dir := e.PiPackageDir()
	for i := 0; i < 6 && dir != ""; i++ {
		if strings.EqualFold(filepath.Base(dir), "node_modules") {
			return filepath.Dir(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// npmPrefix 问当前 node 的 npm「全局前缀在哪」；起一次进程，结果按 node + npm 缓存。
func (e *Env) npmPrefix() string {
	tc := e.Toolchain()
	if tc.Node.Exe == "" || tc.Node.Npm == "" {
		return ""
	}
	return cachedVersion("prefix:"+tc.Node.Exe+"|"+tc.Node.Npm, func() string {
		cmd := exec.Command(tc.Node.Exe, tc.Node.Npm, "config", "get", "prefix")
		cmd.Env = procEnv([]string{filepath.Dir(tc.Node.Exe)})
		hideWindow(cmd)
		raw, err := cmd.Output()
		if err != nil {
			return ""
		}
		prefix := strings.TrimSpace(strings.SplitN(string(raw), "\n", 2)[0])
		if prefix == "" || !pkg.DirExists(prefix) {
			return ""
		}
		return prefix
	})
}

// bindInstall 记下这一趟用的 node / npm 前缀 / git：后续升级与插件管理据此回到同一份。
func (e *Env) bindInstall(prefix string) error {
	tc := e.Toolchain()
	if tc.Node.Exe == "" {
		return nil
	}
	b := binding{
		NodeExe: tc.Node.Exe, NodeSource: tc.Node.Source, NodeVersion: tc.Node.Version,
		NpmScript: tc.Node.Npm, PiPrefix: prefix,
		GitExe: tc.Git.Exe, GitSource: tc.Git.Source,
	}
	if err := e.writeBinding(b); err != nil {
		return err
	}
	e.resetProbes()
	return nil
}

// Rebinding 换一份运行时重新绑：用户明确要求「改用系统那份 node」时走它。
// 换完把终端入口重建一次，入口里钉的是绝对解释器路径。
func (e *Env) Rebinding(nodeExe string) error {
	exe := strings.TrimSpace(nodeExe)
	if exe == "" {
		return pkg.New("9726", "没有指定要切换的 node", "")
	}
	if !pkg.FileExists(exe) {
		return pkg.New("9726", "这份 node 已经不在了", exe)
	}
	version := nodeVersionOf(exe)
	if !nodeSatisfies(version) {
		return pkg.New("9708", "Node 版本 "+version+" 低于要求（需要 "+minNodeVersion+" 或更高）", exe)
	}
	b := e.readBinding()
	b.NodeExe, b.NodeSource, b.NodeVersion = exe, e.sourceOfNode(exe), version
	b.NpmScript = npmScriptAt(filepath.Dir(exe))
	// 换解释器就不能再写回旧前缀：那一份属于上一份 node 的生态。
	b.PiPrefix = ""
	if err := e.writeBinding(b); err != nil {
		return err
	}
	e.resetProbes()
	if pkg.FileExists(filepath.Join(e.terminalDir(), "pi.cmd")) {
		if err := e.writeTerminalShim(); err != nil {
			return err
		}
	}
	return nil
}

// NodeOptions 返回所有能跑的 node 候选（界面「切换运行时」用它列选项）。
func (e *Env) NodeOptions() []NodePick {
	cands := e.nodeCandidates(e.readBinding())
	out := make([]NodePick, 0, len(cands))
	for _, c := range cands {
		pick := pickOf(c)
		if pick.Version == "" {
			continue
		}
		out = append(out, pick)
	}
	return out
}

// InstallPrefix 暴露 PI 的安装前缀（引擎页展示「装到哪」）。
func (e *Env) InstallPrefix() string { return e.installPrefix() }

// AlternateNode 挑一份与当前不同的合规 node：当前用内置就推荐系统那份，反之推荐内置。
// 「后来才装上 Node，但引擎还是内置装的」这条路上，界面靠它给出可换的那一份。
func (e *Env) AlternateNode() (NodePick, bool) {
	cur := e.Toolchain().Node
	best := NodePick{}
	for _, o := range e.NodeOptions() {
		if o.Exe == "" || samePath(o.Exe, cur.Exe) || !nodeSatisfies(o.Version) || o.Source == cur.Source {
			continue
		}
		if best.Exe == "" || compareNodeVersion(o.Version, best.Version) > 0 {
			best = o
		}
	}
	return best, best.Exe != ""
}

// compareNodeVersion 按数字段比较版本号：返回 -1 / 0 / 1。
func compareNodeVersion(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

// effectivePathDirs 返回新开终端会看到的那条 PATH：注册表系统级在前、用户级在后，再拼进程 PATH。
// 进程环境是启动那一刻的快照，用户装完 Node 却没重启应用时它还是旧的。
func effectivePathDirs() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range []string{machinePathValue(), userPathValue(), os.Getenv("PATH")} {
		for _, dir := range strings.Split(value, string(os.PathListSeparator)) {
			dir = strings.Trim(strings.TrimSpace(dir), `"`)
			if dir == "" {
				continue
			}
			key := strings.ToLower(filepath.Clean(dir))
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, filepath.Clean(dir))
		}
	}
	return out
}

// nodeInstallDirs 列出 node 的常见安装根：官方安装包写目录与注册表，各版本管理器各有一套。
func nodeInstallDirs() []string {
	out := []string{}
	add := func(dir string) {
		if dir != "" {
			out = append(out, filepath.Clean(dir))
		}
	}
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		base := os.Getenv(key)
		if base == "" {
			continue
		}
		add(filepath.Join(base, "nodejs"))
		if key == "LOCALAPPDATA" {
			add(filepath.Join(base, "Programs", "nodejs"))
		}
	}
	// nvm-windows：NVM_SYMLINK 指向当前版本，NVM_HOME 下按 vX.Y.Z 存各版本。
	add(os.Getenv("NVM_SYMLINK"))
	for _, dir := range versionedDirs(os.Getenv("NVM_HOME")) {
		add(dir)
	}
	// fnm：%APPDATA%\fnm\node-versions\vX.Y.Z\installation
	if appData := os.Getenv("APPDATA"); appData != "" {
		for _, dir := range versionedDirs(filepath.Join(appData, "fnm", "node-versions")) {
			add(filepath.Join(dir, "installation"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "scoop", "apps", "nodejs", "current"))
		add(filepath.Join(home, "scoop", "apps", "nodejs-lts", "current"))
	}
	for _, dir := range nodeRegistryDirs() {
		add(dir)
	}
	return out
}

// versionedDirs 列出 root 下形如 v24.22.0 的子目录（版本管理器按版本分目录）。
func versionedDirs(root string) []string {
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, e := range entries {
		if e.IsDir() && (strings.HasPrefix(e.Name(), "v") || strings.HasPrefix(e.Name(), "V")) {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out
}

// npmScriptAt 找一个目录下的 npm 入口脚本（Windows 标准布局：node.exe 与 node_modules/npm 同目录）。
func npmScriptAt(dir string) string {
	for _, rel := range []string{
		`node_modules\npm\bin\npm-cli.js`,
		`node_modules\npm\bin\npm-cli.mjs`,
	} {
		script := filepath.Join(dir, rel)
		if pkg.FileExists(script) {
			return script
		}
	}
	return ""
}

// npmShimAt 找目录下的 npm.cmd（某些发行版把 npm 放在 node 旁边而不是 node_modules 里）。
func npmShimAt(dir string) string {
	candidate := filepath.Join(dir, "npm.cmd")
	if pkg.FileExists(candidate) {
		return candidate
	}
	return ""
}

// dirWritable 用「能不能在这里建文件」判断前缀可写：Program Files 下的 npm 前缀一装就 EPERM。
func dirWritable(dir string) bool {
	if dir == "" {
		return false
	}
	if err := pkg.EnsureDir(dir); err != nil {
		return false
	}
	f, err := os.CreateTemp(dir, ".wb-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// samePath 判断两个路径指的是同一个文件（短名、符号链接、大小写都不算差异）。
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(pkg.RealPath(a), pkg.RealPath(b))
}

// procEnv 拼子进程环境：把 dirs 前置进 PATH，并清掉继承来的 npm 前缀。
// 前置而不是追加——后置会被用户 PATH 上的旧版本抢先，选中的运行时等于没生效。
func procEnv(dirs []string) []string {
	current := os.Getenv("PATH")
	prefix := strings.Join(dirs, string(os.PathListSeparator))
	out := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		upper := strings.ToUpper(kv)
		if strings.HasPrefix(upper, "PATH=") || strings.HasPrefix(upper, "NPM_CONFIG_PREFIX=") {
			continue
		}
		out = append(out, kv)
	}
	switch {
	case prefix != "" && current != "":
		out = append(out, "PATH="+prefix+string(os.PathListSeparator)+current)
	case prefix != "":
		out = append(out, "PATH="+prefix)
	case current != "":
		out = append(out, "PATH="+current)
	}
	return out
}
