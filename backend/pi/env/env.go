// Package env 负责 PI 的运行环境：内置 Node、PI 的发现与安装、注入的 PATH，
// 以及与 PI 的配置互通（models.json / settings.json / mcp.json）。只有 pi 需要它。
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

// minNodeMajor 是 PI 对 Node 的下界（PI 要求 >= 22.19）。
const minNodeMajor = 22

// 错误码 9700-9799 与 pi 包同段：环境不通与进程不通是同一类故障的不同层次。
var (
	ErrNodeMissing  = pkg.New("9707", "内置 Node 运行时不可用", "归档缺失或解压失败")
	ErrNodeTooOld   = pkg.New("9708", "Node 版本太旧，跑不了 PI", "")
	ErrPiMissing    = pkg.New("9709", "还没有安装助手引擎（PI）", "")
	ErrInstallFail  = pkg.New("9710", "安装助手引擎失败", "")
	ErrModelMissing = pkg.New("9711", "还没有配置可用的模型服务", "")
)

// EnvVO 是环境闸门的快照，直接进 BootstrapVO 下发给前端。
type EnvVO struct {
	NodeReady   bool   `json:"node_ready"`
	NodePath    string `json:"node_path"`
	NodeVersion string `json:"node_version"`
	NodeSource  string `json:"node_source"`
	NodeError   string `json:"node_error"`

	PiReady   bool   `json:"pi_ready"`
	PiPath    string `json:"pi_path"`
	PiVersion string `json:"pi_version"`
	PiSource  string `json:"pi_source"`
	PiError   string `json:"pi_error"`

	ModelReady bool `json:"model_ready"`
	Ready      bool `json:"ready"`
	// Blocking 是首个不满足项：node / pi / model / 空。
	Blocking string `json:"blocking"`
}

// Env 是环境解析的持有者：一次解析，全局复用。
type Env struct {
	paths runtime.Paths
	// override 供测试注入假路径，空表示走真实探测。
	overridePi string
}

// New 构造环境解析器。
func New(paths runtime.Paths) *Env { return &Env{paths: paths} }

// Paths 暴露路径解析结果（配置文件读写要用）。
func (e *Env) Paths() runtime.Paths { return e.paths }

// AgentDir 返回 PI 的 agent 目录。
func (e *Env) AgentDir() string { return e.paths.PiAgentDir }

// NodeExe 返回跑 PI 用的 node：内置优先，其次系统。
func (e *Env) NodeExe() (string, string) {
	if exe, err := runtime.EnsureNode(e.paths); err == nil {
		return exe, "bundled"
	}
	if exe := runtime.NodeSystemExe(); exe != "" {
		return exe, "system"
	}
	return "", ""
}

// NpmCmd 返回装包用的 npm：内置优先，其次系统。
func (e *Env) NpmCmd() string {
	if cmd := runtime.NpmCmd(e.paths); cmd != "" {
		return cmd
	}
	for _, name := range []string{"npm.cmd", "npm"} {
		if exe, err := exec.LookPath(name); err == nil {
			return exe
		}
	}
	return ""
}

// nodeVersionOf 取 node 的版本号（去掉前缀 v）。
func nodeVersionOf(nodeExe string) string {
	if nodeExe == "" {
		return ""
	}
	out, err := exec.Command(nodeExe, "-v").Output()
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

// PiExe 定位 PI 的入口：显式指定 > 内置 npm 前缀 > 系统 PATH。
// WORKBABY_PI_EXE 让测试与高级用户指定一份特定的 PI。
func (e *Env) PiExe() (string, string) {
	if e.overridePi != "" {
		return e.overridePi, "override"
	}
	if explicit := strings.TrimSpace(os.Getenv("WORKBABY_PI_EXE")); explicit != "" {
		return explicit, "env"
	}
	// npm 全局前缀下的布局：<prefix>/node_modules/.bin/pi.cmd 或直接落在 <prefix>/pi.cmd。
	candidates := []string{
		filepath.Join(e.paths.NpmGlobalDir, "pi.cmd"),
		filepath.Join(e.paths.NpmGlobalDir, "node_modules", ".bin", "pi.cmd"),
	}
	for _, c := range candidates {
		if pkg.FileExists(c) {
			return c, "npm-global"
		}
	}
	if exe, err := exec.LookPath("pi.cmd"); err == nil {
		return exe, "system"
	}
	if exe, err := exec.LookPath("pi"); err == nil {
		return exe, "system"
	}
	return "", ""
}

// PiVersion 跑一次 `pi --version` 拿版本号。
func (e *Env) PiVersion(piExe string) string {
	if piExe == "" {
		return ""
	}
	out, err := exec.Command(piExe, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

// Detect 跑一遍环境闸门。modelReady 由上层传入（它知道 provider 配置）。
func (e *Env) Detect(modelReady bool) EnvVO {
	vo := EnvVO{ModelReady: modelReady}

	nodeExe, nodeSource := e.NodeExe()
	vo.NodePath = nodeExe
	vo.NodeSource = nodeSource
	vo.NodeVersion = nodeVersionOf(nodeExe)
	vo.NodeReady = nodeExe != "" && nodeSatisfies(vo.NodeVersion)
	if nodeExe == "" {
		vo.NodeError = ErrNodeMissing.Error()
	} else if !vo.NodeReady {
		vo.NodeError = "Node 版本 " + vo.NodeVersion + " 低于要求（需要 22.19 或更高）"
	}

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

// Env 返回 PI 子进程要用的完整环境变量：注入内置运行时的 PATH（前置拼接）。
// 前置拼接是硬要求：后置会被用户 PATH 上的旧版本抢先，内置运行时等于没生效。
func (e *Env) Env() []string {
	dirs := []string{}
	if d := e.paths.NodeDir; d != "" {
		dirs = append(dirs, d)
	}
	if d := e.paths.NpmGlobalDir; d != "" {
		dirs = append(dirs, d)
	}
	if d := e.paths.PythonDir; d != "" {
		dirs = append(dirs, d)
	}

	out := make([]string, 0, len(os.Environ())+4)
	current := os.Getenv("PATH")
	prefix := strings.Join(dirs, string(os.PathListSeparator))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			continue
		}
		// PI 的 agent 目录必须显式指定：把 PI 的状态收进应用目录，卸载即清，
		// 同时保证它的 skills/ 就是我们的技能目录。
		if strings.HasPrefix(strings.ToUpper(kv), "PI_CODING_AGENT_DIR=") {
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
	out = append(out, "PI_CODING_AGENT_DIR="+e.paths.PiAgentDir)

	// npm 全局前缀落在应用目录：系统前缀要管理员权限，写失败时 npm 报的是权限错。
	out = append(out, "npm_config_prefix="+e.paths.NpmGlobalDir)
	return out
}

// InstallPi 用内置 npm 全局安装 PI，输出按行回调（前端要看到进度）。
// 安装可能几十秒，没有输出界面会被当成卡死。
func (e *Env) InstallPi(onLine func(string)) error {
	npm := e.NpmCmd()
	if npm == "" {
		return pkg.New("9710", "找不到 npm，无法安装助手引擎", "内置 Node 未就绪且系统也没有 npm")
	}
	args := []string{"install", "-g", "--ignore-scripts", piPackage}
	cmd := exec.Command(npm, args...)
	cmd.Env = e.Env()
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

// ModelReady 判断是否至少有一个服务配了密钥与模型。
// 没有可用模型就 spawn，失败点会推到第一次对话，错误还埋在协议流里。
func ModelReady(hasProvider bool) bool { return hasProvider }

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
	if err := os.Chmod(name, perm); err != nil {
		// 权限设置失败在 Windows 上常见（不支持 POSIX 位），不阻断写入。
		_ = err
	}
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
	raw = append(raw, '\n')
	return WriteFileAtomic(path, raw, 0o600)
}

// 编译期确认 time 被引用（版本探测的未来扩展点）。
var _ = time.Second
