// 引擎编排：PI 自身与内置运行时的状态、联网版本对账、升级与插件包更新。
// 升级动作只有两个来源——PI 自己的 `pi update`，以及本应用内置 npm 的 `install -g`。
package service

import (
	"strings"
	"sync"

	"WorkBaby/backend/domain"
	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
)

// EngineService 是引擎页的数据出口，同时记住最近一次安装 / 升级的失败原因。
// 安装是长动作（npm 可能跑几十秒），同一个时刻只允许一趟。
type EngineService struct {
	env *Env

	mu         sync.Mutex
	busy       bool
	installErr string
}

// NewEngineService 构造引擎服务。
func NewEngineService(env *Env) *EngineService { return &EngineService{env: env} }

// Status 汇总本地状态：不联网，点开就能看。
func (s *EngineService) Status() *domain.EngineStatusVO {
	modelReady := s.env.Providers != nil && s.env.Providers.HasUsable()
	det := s.env.PiEnv.Detect(modelReady)
	vo := &domain.EngineStatusVO{
		PiReady: det.PiReady, PiVersion: det.PiVersion,
		PiPath: det.PiPath, PiSource: det.PiSource,
		Managed:     s.env.PiEnv.OwnsInstall(),
		InstallRoot: s.env.PiEnv.PiPackageDir(),
		NodeReady:   det.NodeReady, NodeVersion: det.NodeVersion,
		NodePath: det.NodePath, NodeSource: det.NodeSource, NodeError: det.NodeError,
		PythonReady: det.PythonReady, PythonVersion: det.PythonVersion,
		PythonPath: det.PythonPath, PythonSource: det.PythonSource,
		AgentDir:   det.AgentDir,
		ModelReady: det.ModelReady, Blocking: det.Blocking,
		Packages:     len(resolvePackages(s.env)),
		InstallError: s.LastInstallError(),
	}
	if sup := s.env.PiSupervisor(); sup != nil {
		vo.Running = sup.RunningCount()
	}
	return vo
}

// Latest 联网查最新版本：PI 与全部 npm 来源的插件包。
// 查不到不算失败——网络断了面板仍要能把本地状态说清楚。
func (s *EngineService) Latest() *domain.EngineLatestVO {
	vo := &domain.EngineLatestVO{Packages: []domain.PackageLatestVO{}}
	local := s.env.PiEnv.PiVersion(mustPiExe(s.env))

	if v, err := pienv.LatestPiVersion(); err != nil {
		vo.RegistryError = err.Error()
	} else {
		vo.PiVersion = v
		vo.UpdateAvailable = newerThan(v, local)
	}
	for _, pkg := range resolvePackages(s.env) {
		if pkg.Kind != domain.PackageKindNpm {
			continue
		}
		item := domain.PackageLatestVO{Source: pkg.Source}
		if v, err := pienv.LatestVersion(pkg.Name); err != nil {
			item.Err = err.Error()
		} else {
			item.LatestVersion = v
			item.UpdateAvailable = newerThan(v, pkg.Version)
		}
		vo.Packages = append(vo.Packages, item)
	}
	return vo
}

// Install 装一份 PI（target 为空表示最新版）。忙碌时直接拒绝，避免两次 npm 写坏同一个目录。
func (s *EngineService) Install(target string, onLine func(string)) error {
	if !s.begin() {
		return pkg.New("9710", "正在装，等这一趟结束", "")
	}
	defer s.end()
	err := s.env.PiEnv.InstallPi(target, onLine)
	s.record(err)
	if err == nil {
		s.restartAll()
	}
	return err
}

// Update 升级已装的 PI，升级完重启全部进程让新内核生效。
// 升级前先停进程：Windows 会锁住已加载的 .node 原生模块，覆写失败表现为 npm 的 EPERM。
func (s *EngineService) Update(onLine func(string)) error {
	if !s.begin() {
		return pkg.New("9710", "正在升级，等这一趟结束", "")
	}
	defer s.end()
	if exe := mustPiExe(s.env); exe == "" {
		s.record(domain.ErrEngineMissing)
		return domain.ErrEngineMissing
	}
	s.stopAll()
	defer s.restartAll()
	err := s.env.PiEnv.UpdatePi(onLine)
	s.record(err)
	return err
}

// UpdatePackages 更新插件包：source 为空表示全部（`pi update --extensions`）。
func (s *EngineService) UpdatePackages(source string, onLine func(string)) error {
	if !s.begin() {
		return pkg.New("9719", "正在装，等这一趟结束", "")
	}
	defer s.end()
	args := []string{"update", "--extensions"}
	if src := strings.TrimSpace(source); src != "" {
		args = []string{"update", src}
	}
	// 与升级 PI 同理：先让开被锁的文件，再更新，最后统一重启。
	s.stopAll()
	defer s.restartAll()
	err := s.env.PiEnv.RunCLI(args, onLine)
	s.record(err)
	return err
}

// LastInstallError 返回最近一次失败的原文（成功后清空）。
func (s *EngineService) LastInstallError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.installErr
}

// begin 抢安装锁；返回 false 表示已经有一趟在跑。
func (s *EngineService) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return false
	}
	s.busy = true
	return true
}

// end 放开安装锁。
func (s *EngineService) end() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}

// record 记下这一趟的结果：失败要留原因，成功要把上一次的失败清掉。
func (s *EngineService) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.installErr = ""
		return
	}
	s.installErr = err.Error()
}

// restartAll 重启全部 PI 进程：新装的内核与插件要重新加载才生效。
func (s *EngineService) restartAll() {
	if sup := s.env.PiSupervisor(); sup != nil {
		_ = sup.RestartAll()
	}
	NewSkillService(s.env).Reload(s.env.Workspace())
}

// stopAll 停掉全部 PI 进程（升级前要让开被锁的文件）。
func (s *EngineService) stopAll() {
	if sup := s.env.PiSupervisor(); sup != nil {
		sup.StopAll()
	}
}

// mustPiExe 取 PI 的入口路径（空表示没装）。
func mustPiExe(env *Env) string {
	if env == nil || env.PiEnv == nil {
		return ""
	}
	exe, _ := env.PiEnv.PiExe()
	return exe
}

// newerThan 判断 remote 是否比 local 新；任一版本号认不出就按「不比」处理。
func newerThan(remote, local string) bool {
	remote, local = strings.TrimSpace(remote), strings.TrimSpace(local)
	if remote == "" || local == "" {
		return false
	}
	return compareVersions(remote, local) > 0
}

// compareVersions 按数字段比较版本号：返回 -1 / 0 / 1。
func compareVersions(a, b string) int {
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

// versionParts 把 1.4.0-beta.2 切成 [1 4 0]：预发布后缀不参与大小判定。
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
				break
			}
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out
}
