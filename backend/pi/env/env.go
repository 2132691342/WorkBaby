// Package env 负责 PI 的运行环境：运行时与 PI 的发现 / 安装、子进程 PATH 注入、
// 与 PI 的配置互通（models / settings / mcp.json）；只服务 pi 包，上层不直接依赖这里。
package env

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/runtime"
)

// minNodeVersion 是 PI 对 Node 的下界。PI 1.4 的 engines 写的是 >=22.19.0：
// 只比主版本会把 22.15 这类“看着够”的版本放进来，装包与运行时会一直EBADENGINE。
const minNodeVersion = "22.19.0"

// 错误码 9700-9799 与 pi 包同段：环境不通与进程不通是同一类故障的不同层次。
var (
	ErrNodeMissing  = pkg.New("9707", "找不到可用的 Node 运行时", "内置运行时不可用，系统里也没有 node")
	ErrNodeTooOld   = pkg.New("9708", "Node 版本太旧，跑不了 PI", "")
	ErrPiMissing    = pkg.New("9709", "还没有安装助手引擎（PI）", "")
	ErrModelMissing = pkg.New("9711", "还没有配置可用的模型服务", "")
)

// piCmdCache 记住 shim 的解析结果：每次调用都读一遍 shim 文本，会把「点一下」放大成几十次磁盘读。
var (
	piCmdMu     sync.Mutex
	piCmdShim   string
	piCmdExe    string
	piCmdArgs   []string
	piCmdCached bool
)

// 版本号探测缓存：node -v / python -V / pi --version 都是真实进程启动（约 0.25s），
// 而闸门与引导页会反复问同一批可执行文件的版本。按被问的那个 exe 索引。
var (
	versionMu    sync.Mutex
	versionCache = map[string]string{}
)

// cachedVersion 按可执行文件路径缓存版本号；拿不到（路径为空或启动失败）就缓存空串。
// 探测过程不持锁：探测会回调到 NodeExe → cachedVersion 自己，持锁即自锁。
func cachedVersion(exe string, probe func() string) string {
	if exe == "" {
		return ""
	}
	versionMu.Lock()
	hit, ok := versionCache[exe]
	versionMu.Unlock()
	if ok {
		return hit
	}
	v := probe()
	versionMu.Lock()
	versionCache[exe] = v
	versionMu.Unlock()
	return v
}

// EnvVO 是环境快照，直接进 BootstrapVO 下发给前端。
type EnvVO struct {
	NodeReady   bool   `json:"node_ready"`
	NodePath    string `json:"node_path"`
	NodeVersion string `json:"node_version"`
	NodeSource  string `json:"node_source"`
	NodeError   string `json:"node_error"`

	PythonReady   bool   `json:"python_ready"`
	PythonPath    string `json:"python_path"`
	PythonVersion string `json:"python_version"`
	PythonSource  string `json:"python_source"`

	PiReady   bool   `json:"pi_ready"`
	PiPath    string `json:"pi_path"`
	PiVersion string `json:"pi_version"`
	PiSource  string `json:"pi_source"`
	PiError   string `json:"pi_error"`

	// AgentDir 是 PI 的配置目录：界面上「我的技能/插件/模型在哪」都以它为准。
	AgentDir string `json:"agent_dir"`

	ModelReady bool `json:"model_ready"`
	Ready      bool `json:"ready"`
	// Blocking 是首个不满足项：node / pi / model / 空。
	Blocking string `json:"blocking"`
}

// Env 是环境解析的持有者：一次解析，全局复用。
type Env struct {
	paths runtime.Paths
	// overridePi 供测试注入假路径，空表示走真实探测。
	overridePi string
}

// New 构造环境解析器。
func New(paths runtime.Paths) *Env { return &Env{paths: paths} }

// Paths 暴露路径解析结果（配置文件读写要用）。
func (e *Env) Paths() runtime.Paths { return e.paths }

// AgentDir 返回 PI 的 agent 目录。
func (e *Env) AgentDir() string { return e.paths.AgentDir }

// NodeExe 返回跑 PI 用的 node：系统已有合规版本就用系统的，否则用内置的。
// 复用系统版本可以省掉一次解压，也让 `pi` 在终端与界面里跑在同一个 Node 上。
func (e *Env) NodeExe() (string, string) {
	if exe := runtime.NodeSystemExe(); exe != "" {
		if nodeSatisfies(nodeVersionOf(exe)) {
			return exe, "system"
		}
	}
	if exe, err := runtime.EnsureNode(e.paths); err == nil {
		return exe, "bundled"
	}
	if exe := runtime.NodeSystemExe(); exe != "" {
		return exe, "system"
	}
	return "", ""
}

// PythonExe 返回给 PI 用的 python：系统优先，其次内置。
func (e *Env) PythonExe() (string, string) {
	if exe := pythonSystemExe(); exe != "" {
		return exe, "system"
	}
	if exe, err := runtime.EnsurePython(e.paths); err == nil {
		return exe, "bundled"
	}
	return "", ""
}

// NpmCmd 返回装包用的 npm：必须与 NodeExe 选中的那份 node 配套。
// 返回的是 npm.cmd 之类的 shim；真要起进程请用 NpmCommand，别把 shim 直接交给 exec。
func (e *Env) NpmCmd() string {
	node, source := e.NodeExe()
	if source == "bundled" {
		return runtime.NpmCmd(e.paths)
	}
	// 系统 node 的旁边通常就是配套的 npm；找不到再退回 PATH 与内置。
	if dir := filepath.Dir(node); node != "" {
		for _, name := range []string{"npm.cmd", "npm"} {
			candidate := filepath.Join(dir, name)
			if pkg.FileExists(candidate) {
				return candidate
			}
		}
	}
	for _, name := range []string{"npm.cmd", "npm"} {
		if exe, err := exec.LookPath(name); err == nil {
			return exe
		}
	}
	return runtime.NpmCmd(e.paths)
}

// NpmCommand 把 npm 的 shim 还原成 node + npm-cli.js，与 PI 的解析同一条路子。
func (e *Env) NpmCommand() (string, []string) {
	shim := e.NpmCmd()
	if shim == "" {
		return "", nil
	}
	dir := filepath.Dir(shim)
	nodeExe, _ := e.NodeExe()
	for _, rel := range []string{
		`node_modules\npm\bin\npm-cli.js`,
		`node_modules\npm\bin\npm-cli.mjs`,
	} {
		for _, base := range []string{dir, filepath.Dir(dir)} {
			candidate := filepath.Join(base, rel)
			if pkg.FileExists(candidate) {
				return shimNode(dir, nodeExe), []string{candidate}
			}
		}
	}
	return shimNode(dir, nodeExe), []string{}
}

// nodeSystemExe 返回系统 PATH 上的 node。
func nodeVersionOf(nodeExe string) string {
	return cachedVersion(nodeExe, func() string {
		if nodeExe == "" {
			return ""
		}
		cmd := exec.Command(nodeExe, "-v")
		hideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	})
}

// nodeSatisfies 检查版本是否满足 PI 的下界（按数字段逐段比）。
func nodeSatisfies(version string) bool {
	if version == "" {
		return false
	}
	cur, min := versionParts(version), versionParts(minNodeVersion)
	for i := 0; i < len(min); i++ {
		got := 0
		if i < len(cur) {
			got = cur[i]
		}
		if got != min[i] {
			return got > min[i]
		}
	}
	return true
}

// versionParts 把 24.21.0 切成 [24 21 0]；预发布后缀不参与比较。
func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	out := []int{}
	for _, seg := range strings.Split(v, ".") {
		n := 0
		for _, r := range seg {
			if r < '0' || r > '9' {
				return out
			}
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out
}

// pythonSystemExe 找系统 python；内置 embeddable 版没有 pip 与 venv，
// 用户机器上已有完整 Python 时没有理由不用它。
func pythonSystemExe() string {
	for _, name := range []string{"python.exe", "python3.exe", "python", "python3"} {
		exe, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		// Windows 商店占位符会返回 9009，跑一次 -V 才能确认它能用。
		cmd := exec.Command(exe, "-V")
		hideWindow(cmd)
		if out, err := cmd.Output(); err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "Python 3") {
			return exe
		}
	}
	return ""
}

// PiExe 定位 PI 的入口：显式指定 > 系统 PATH > 各 npm 全局前缀。
// WORKBABY_PI_EXE 让测试与高级用户指定一份特定的 PI。
func (e *Env) PiExe() (string, string) {
	if e.overridePi != "" {
		return e.overridePi, "override"
	}
	if explicit := strings.TrimSpace(os.Getenv("WORKBABY_PI_EXE")); explicit != "" {
		return explicit, "env"
	}
	for _, name := range []string{"pi.cmd", "pi.exe", "pi"} {
		if exe, err := exec.LookPath(name); err == nil {
			return exe, "system"
		}
	}
	for _, prefix := range e.npmPrefixes() {
		for _, c := range []string{
			filepath.Join(prefix, "pi.cmd"),
			filepath.Join(prefix, "pi.exe"),
			filepath.Join(prefix, "node_modules", ".bin", "pi.cmd"),
		} {
			if pkg.FileExists(c) {
				return c, "npm-global"
			}
		}
	}
	return "", ""
}

// PiCommand 返回可直接交给 os/exec 的 PI 入口与附加参数：shim 还原成 node + 脚本，不经 shell。
// 找不到 PI 报 9709，找不到能跑的 node 报 9707——都在 spawn 之前说清。
func (e *Env) PiCommand() (string, []string, error) {
	exe, _ := e.PiExe()
	if exe == "" {
		return "", nil, ErrPiMissing
	}
	piCmdMu.Lock()
	defer piCmdMu.Unlock()
	if piCmdCached && piCmdShim == exe && piCmdExe != "" {
		return piCmdExe, piCmdArgs, nil
	}
	nodeExe, _ := e.NodeExe()
	resolved, args := resolvePiExe(exe, nodeExe)
	if resolved == "" {
		return "", nil, ErrNodeMissing
	}
	piCmdShim, piCmdExe, piCmdArgs, piCmdCached = exe, resolved, args, true
	return resolved, args, nil
}

// npmPrefixes 列出要找 PI 的 npm 全局前缀：系统 npm 的配置优先，其次应用内那份。
func (e *Env) npmPrefixes() []string {
	out := []string{}
	if npm := e.NpmCmd(); npm != "" {
		cmd := exec.Command(npm, "config", "get", "prefix")
		hideWindow(cmd)
		if raw, err := cmd.Output(); err == nil {
			if p := strings.TrimSpace(string(raw)); p != "" && pkg.DirExists(p) {
				out = append(out, p)
			}
		}
	}
	out = append(out, e.paths.NpmGlobalDir)
	return out
}

// PiVersion 取 PI 的版本号；同一个入口只起一次进程。
func (e *Env) PiVersion(piExe string) string {
	if piExe == "" {
		return ""
	}
	nodeExe, _ := e.NodeExe()
	return cachedVersion(piExe, func() string {
		exe, args := resolvePiExe(piExe, nodeExe)
		if exe == "" {
			return ""
		}
		argv := append(append([]string{}, args...), "--version")
		cmd := exec.Command(exe, argv...)
		cmd.Env = e.Env()
		hideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	})
}

// PiPackageDir 返回 PI 的包根目录（含 package.json），认不出返回空串。
// 版本号与包大小都从这里读，不靠 npm 再跑一次。
func (e *Env) PiPackageDir() string {
	exe, _ := e.PiExe()
	if exe == "" {
		return ""
	}
	nodeExe, _ := e.NodeExe()
	_, args := resolvePiExe(exe, nodeExe)
	if len(args) == 0 {
		return ""
	}
	dir := filepath.Dir(args[0])
	for i := 0; i < 6; i++ {
		if pkg.FileExists(filepath.Join(dir, "package.json")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// OwnsInstall 报告这份 PI 是否由本应用装进自己的前缀（升级只写回自家目录）。
func (e *Env) OwnsInstall() bool {
	exe, _ := e.PiExe()
	if exe == "" || e.paths.NpmGlobalDir == "" {
		return false
	}
	prefix := strings.ToLower(filepath.Clean(e.paths.NpmGlobalDir))
	target := strings.ToLower(filepath.Clean(exe))
	return strings.HasPrefix(target, prefix+string(filepath.Separator))
}

// Detect 跑一遍环境闸门。modelReady 由上层传入（它读过 models.json）。
func (e *Env) Detect(modelReady bool) EnvVO {
	vo := EnvVO{ModelReady: modelReady, AgentDir: e.paths.AgentDir}

	nodeExe, nodeSource := e.NodeExe()
	vo.NodePath = nodeExe
	vo.NodeSource = nodeSource
	vo.NodeVersion = nodeVersionOf(nodeExe)
	vo.NodeReady = nodeExe != "" && nodeSatisfies(vo.NodeVersion)
	if nodeExe == "" {
		vo.NodeError = ErrNodeMissing.Error()
	} else if !vo.NodeReady {
		vo.NodeError = "Node 版本 " + vo.NodeVersion + " 低于要求（需要 " + minNodeVersion + " 或更高）"
	}

	pyExe, pySource := e.PythonExe()
	vo.PythonPath = pyExe
	vo.PythonSource = pySource
	vo.PythonVersion = pythonVersionOf(pyExe)
	vo.PythonReady = pyExe != ""

	piExe, piSource := e.PiExe()
	vo.PiPath = piExe
	vo.PiSource = piSource
	vo.PiVersion = e.PiVersion(piExe)
	vo.PiReady = piExe != ""
	if !vo.PiReady {
		vo.PiError = ErrPiMissing.Error()
	}

	vo.Ready = vo.NodeReady && vo.PiReady && vo.ModelReady
	switch {
	case !vo.NodeReady:
		vo.Blocking = "node"
	case !vo.PiReady:
		vo.Blocking = "pi"
	case !vo.ModelReady:
		vo.Blocking = "model"
	}
	return vo
}

// pythonVersionOf 取 Python 版本号（形如 3.12.8）。
func pythonVersionOf(exe string) string {
	return cachedVersion(exe, func() string {
		if exe == "" {
			return ""
		}
		cmd := exec.Command(exe, "-V")
		hideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "Python "))
	})
}

// Env 返回 PI 子进程要用的完整环境变量：把选中的运行时前置进 PATH。
// 前置拼接是硬要求：后置会被用户 PATH 上的旧版本抢先，选中的运行时等于没生效。
func (e *Env) Env() []string {
	dirs := []string{}
	if node, source := e.NodeExe(); node != "" && source == "bundled" {
		dirs = append(dirs, filepath.Dir(node))
	}
	if py, source := e.PythonExe(); py != "" && source == "bundled" {
		dirs = append(dirs, filepath.Dir(py))
	}
	if d := e.paths.NpmGlobalDir; d != "" && pkg.DirExists(d) {
		dirs = append(dirs, d)
	}

	out := make([]string, 0, len(os.Environ())+4)
	current := os.Getenv("PATH")
	prefix := strings.Join(dirs, string(os.PathListSeparator))
	for _, kv := range os.Environ() {
		upper := strings.ToUpper(kv)
		// PATH 自己重新拼；npm 前缀由本层决定，继承来的会指向别处。
		if strings.HasPrefix(upper, "PATH=") || strings.HasPrefix(upper, "NPM_CONFIG_PREFIX=") {
			continue
		}
		out = append(out, kv)
	}
	if prefix != "" && current != "" {
		out = append(out, "PATH="+prefix+string(os.PathListSeparator)+current)
	} else if prefix != "" {
		out = append(out, "PATH="+prefix)
	} else if current != "" {
		out = append(out, "PATH="+current)
	}
	// PI 的 agent 目录不覆盖：它必须就是用户 pi 命令读写的那一份。
	// Windows 下 Python 的中文输出默认走 GBK，工具结果会变成乱码。
	out = append(out, "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	return out
}

// InstallPi 用 npm 全局安装 PI（target 为空表示最新版），输出按行回调。
// 失败时把最后几行输出带进错误里，否则界面只剩一句「安装失败」，用户无从下手。
func (e *Env) InstallPi(target string, onLine func(string)) error {
	return e.installAt(e.paths.NpmGlobalDir, target, onLine)
}

// installAt 把 PI 装到指定 npm 全局前缀（<prefix>/node_modules/<pkg>）。
func (e *Env) installAt(prefix, target string, onLine func(string)) error {
	spec := piPackage
	if v := strings.TrimSpace(target); v != "" {
		spec = piPackage + "@" + v
	}
	npm, npmArgs := e.NpmCommand()
	if npm == "" {
		return pkg.New("9710", "找不到 npm，无法安装助手引擎", "内置 Node 未就绪且系统也没有 npm")
	}
	args := append(append([]string{}, npmArgs...),
		"install", "-g", "--ignore-scripts", "--no-fund", "--no-audit",
		"--loglevel=http", "--fetch-retries=3")
	if prefix != "" {
		args = append(args, "--prefix", prefix)
	}
	args = append(args, spec)
	cmd := exec.Command(npm, args...)
	cmd.Env = e.npmEnv(prefix)
	hideWindow(cmd)

	tail := newLineTail(24)
	report := func(line string) {
		tail.add(line)
		if onLine != nil {
			onLine(line)
		}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return pkg.Wrap("9710", "无法读取安装进度", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return pkg.Wrap("9710", "无法读取安装进度", err)
	}
	if err := cmd.Start(); err != nil {
		return pkg.Wrap("9710", "装不了助手引擎（npm 起不来）", err)
	}
	pumpPipe(stdout, report)
	pumpPipe(stderr, report)
	waitErr := cmd.Wait()
	// 装完不论成败都要重探：旧版本号与旧入口解析结果都不能留着。
	e.resetProbes()
	if waitErr != nil {
		return pkg.New("9710", "安装助手引擎失败", tail.last())
	}
	if exe, _ := e.PiExe(); exe == "" {
		return pkg.New("9710", "安装好像成功了，但找不到 pi 命令", "检查 npm 全局前缀是否可写")
	}
	return nil
}

// UpdatePi 升级已装的 PI：装回它原来所在的那个前缀，不另起一份。
// 直装失败再交给 PI 自己的 `pi update` 兜底（它会按自己的安装方式选 npm / 前缀）。
func (e *Env) UpdatePi(onLine func(string)) error {
	if exe, _ := e.PiExe(); exe == "" {
		return ErrPiMissing
	}
	prefix := e.installPrefix()
	if prefix != "" {
		if err := e.installAt(prefix, "", onLine); err == nil {
			return nil
		}
		if onLine != nil {
			onLine("直接升级失败，改用 pi update…")
		}
	}
	err := e.runCLI([]string{"update"}, e.Env(), onLine)
	e.resetProbes()
	return err
}

// installPrefix 返回 PI 应该装回哪里：自己装的那份是本应用前缀，
// 系统那份就是它所在包目录里的 node_modules 的父目录。
func (e *Env) installPrefix() string {
	if e.OwnsInstall() {
		return e.paths.NpmGlobalDir
	}
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

// npmEnv 是 npm 子进程的环境：与 PI 同一份 PATH，外加本次要写的前缀与自带的缓存。
func (e *Env) npmEnv(prefix string) []string {
	env := e.Env()
	if prefix == "" {
		prefix = e.paths.NpmGlobalDir
	}
	env = append(env, "npm_config_prefix="+prefix)
	if e.paths.NpmCacheDir != "" {
		env = append(env, "npm_config_cache="+e.paths.NpmCacheDir)
	}
	return env
}

// resetProbes 清掉版本号与入口解析缓存：装完 / 升级完必须重新探测。
func (e *Env) resetProbes() {
	piCmdMu.Lock()
	piCmdShim, piCmdExe, piCmdArgs, piCmdCached = "", "", nil, false
	piCmdMu.Unlock()
	versionMu.Lock()
	versionCache = map[string]string{}
	versionMu.Unlock()
}

// piPackage 是 PI 的 npm 包名。
const piPackage = "@earendil-works/pi-coding-agent"

// WriteFileAtomic 原子替换写入：磁盘写满或进程被杀留下半截配置比写失败更糟。
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := pkg.EnsureDir(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return pkg.Wrap("9712", "写配置文件失败", err)
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return pkg.Wrap("9712", "写配置文件失败", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return pkg.Wrap("9712", "写配置文件失败", err)
	}
	if err := tmp.Close(); err != nil {
		return pkg.Wrap("9712", "写配置文件失败", err)
	}
	_ = os.Chmod(name, perm)
	if err := os.Rename(name, path); err != nil {
		return pkg.Wrap("9712", "写配置文件失败", err)
	}
	return nil
}

// ReadJSON 读一个 JSON 配置文件；文件不存在返回 nil 供调用方走首次生成。
func ReadJSON(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return pkg.Wrap("9713", "读配置文件失败", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return pkg.Wrap("9713", "配置文件格式不对", err)
	}
	return nil
}

// ReadFileOrEmpty 读文件内容；不存在时返回 fallback。
func ReadFileOrEmpty(path string, fallback []byte) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fallback, nil
		}
		return nil, pkg.Wrap("9713", "读配置文件失败", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return fallback, nil
	}
	return raw, nil
}
