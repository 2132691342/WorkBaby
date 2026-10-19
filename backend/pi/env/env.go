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

// minNodeMajor 是 PI 对 Node 的下界（PI 要求 >= 22）。
const minNodeMajor = 22

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
	piCmdExe    string
	piCmdArgs   []string
	piCmdCached bool
)

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

// NpmCmd 返回装包用的 npm：与 node 保持同一套（系统优先，其次内置）。
// 返回的是 npm.cmd 之类的 shim；真要起进程请用 NpmCommand，别把 shim 直接交给 exec。
func (e *Env) NpmCmd() string {
	if _, err := exec.LookPath("node"); err == nil {
		for _, name := range []string{"npm.cmd", "npm"} {
			if exe, err := exec.LookPath(name); err == nil {
				return exe
			}
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
	for _, rel := range []string{
		`node_modules\npm\bin\npm-cli.js`,
		`node_modules\npm\bin\npm-cli.mjs`,
	} {
		for _, base := range []string{dir, filepath.Dir(dir)} {
			candidate := filepath.Join(base, rel)
			if pkg.FileExists(candidate) {
				return shimNode(dir), []string{candidate}
			}
		}
	}
	return shimNode(dir), []string{}
}

// nodeSystemExe 返回系统 PATH 上的 node。
func nodeVersionOf(nodeExe string) string {
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
}

// nodeSatisfies 检查主版本是否满足 PI 的下界。
func nodeSatisfies(version string) bool {
	if version == "" {
		return false
	}
	major := 0
	for _, r := range version {
		if r < '0' || r > '9' {
			break
		}
		major = major*10 + int(r-'0')
	}
	return major >= minNodeMajor
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
func (e *Env) PiCommand() (string, []string) {
	exe, _ := e.PiExe()
	if exe == "" {
		return "", nil
	}
	piCmdMu.Lock()
	defer piCmdMu.Unlock()
	if piCmdCached && piCmdExe == exe {
		return piCmdExe, piCmdArgs
	}
	resolved, args := resolvePiExe(exe)
	piCmdExe, piCmdArgs, piCmdCached = resolved, args, true
	return resolved, args
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

// PiVersion 跑一次 `pi --version` 拿版本号。
func (e *Env) PiVersion(piExe string) string {
	if piExe == "" {
		return ""
	}
	exe, args := resolvePiExe(piExe)
	argv := append(append([]string{}, args...), "--version")
	cmd := exec.Command(exe, argv...)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
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
		vo.NodeError = "Node 版本 " + vo.NodeVersion + " 低于要求（需要 22 或更高）"
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

// InstallPi 用 npm 全局安装 PI，输出按行回调（前端要看到进度）。
// 安装可能几十秒，没有输出界面会被当成卡死。
func (e *Env) InstallPi(onLine func(string)) error {
	npm, npmArgs := e.NpmCommand()
	if npm == "" {
		return pkg.New("9710", "找不到 npm，无法安装助手引擎", "内置 Node 未就绪且系统也没有 npm")
	}
	args := append(append([]string{}, npmArgs...), "install", "-g", "--ignore-scripts", piPackage)
	cmd := exec.Command(npm, args...)
	cmd.Env = append(e.Env(), "npm_config_prefix="+e.paths.NpmGlobalDir)
	hideWindow(cmd)

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
	pumpPipe(stdout, onLine)
	pumpPipe(stderr, onLine)
	if err := cmd.Wait(); err != nil {
		return pkg.Wrap("9710", "安装助手引擎失败", err)
	}
	if exe, _ := e.PiExe(); exe == "" {
		return pkg.New("9710", "安装好像成功了，但找不到 pi 命令", "检查 npm 全局前缀是否可写")
	}
	return nil
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

// WriteJSON 把值缩进写进配置文件（原子替换）。
func WriteJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return pkg.Wrap("9712", "配置无法序列化", err)
	}
	return WriteFileAtomic(path, append(raw, '\n'), 0o600)
}
