// 引擎编排：PI 自身与内置运行时的状态、联网版本对账、升级与插件包更新。
// 升级动作只有两个来源——PI 自己的 `pi update`，以及本应用内置 npm 的 `install -g`。
package service

import (
	"strings"
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/git"
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
		NodeLevel: det.NodeLevel,
		GitReady:  det.GitReady, GitVersion: det.GitVersion,
		GitPath: det.GitPath, GitSource: det.GitSource, GitLevel: det.GitLevel,
		AgentDir:   det.AgentDir,
		ModelReady: det.ModelReady, Blocking: det.Blocking,
		Packages:     len(resolvePackages(s.env)),
		InstallError: s.LastInstallError(),
	}
	shell := s.env.PiEnv.ShellStatusOf()
	vo.ShellKind, vo.ShellPath = shell.Kind, shell.Path
	vo.ShellSource, vo.ShellUsable, vo.ShellError = shell.Source, shell.Usable, shell.Error
	terminal := s.env.PiEnv.TerminalStatusOf()
	vo.TerminalReady, vo.TerminalPath = terminal.Ready, terminal.Path
	vo.TerminalOnPath, vo.TerminalError = terminal.OnPath, terminal.Error
	vo.TerminalShadowedBy = terminal.ShadowedBy
	vo.TerminalLevel = terminal.Level
	vo.OtherPis = terminal.Others
	if vo.OtherPis == nil {
		vo.OtherPis = []string{}
	}
	if sup := s.env.PiSupervisor(); sup != nil {
		vo.Running = sup.RunningCount()
	}
	return vo
}

// Toolchain 汇总 node / git / pi 的工具链快照：装引擎用的是哪份、能不能换、能不能提级。
func (s *EngineService) Toolchain() *domain.ToolchainVO {
	e := s.env.PiEnv
	tc := e.Toolchain()
	// 读一次工具链就同步一次 git：用户刚装了 Git 时不用重启应用才能用上。
	git.SetExe(tc.Git.Exe)
	vo := &domain.ToolchainVO{InstallPrefix: e.InstallPrefix(), Bound: tc.Bound}
	vo.Node = domain.LinkVO{
		Ready:  tc.Node.Exe != "" && pienv.NodeOK(tc.Node.Version),
		Source: tc.Node.Source, Version: tc.Node.Version, Path: tc.Node.Exe,
	}
	fillLink(&vo.Node, e.LinkOf(pienv.ComponentNode))
	vo.Git = domain.LinkVO{
		Ready: tc.Git.Exe != "", Source: tc.Git.Source, Version: tc.Git.Version, Path: tc.Git.Exe,
	}
	fillLink(&vo.Git, e.LinkOf(pienv.ComponentGit))
	fillLink(&vo.Pi, e.LinkOf(pienv.ComponentPi))
	if exe, src := e.PiExe(); exe != "" {
		vo.Pi.Ready, vo.Pi.Path, vo.Pi.Source, vo.Pi.Version = true, exe, src, e.PiVersion(exe)
	}
	if alt, ok := e.AlternateNode(); ok {
		vo.Alt = &domain.NodeOptionVO{Exe: alt.Exe, Version: alt.Version, Source: alt.Source}
	}
	return vo
}

// fillLink 把 PATH 落地状态并进组件快照。
func fillLink(vo *domain.LinkVO, st pienv.LinkStatus) {
	vo.Level, vo.Promotable, vo.Conflict = st.Level, st.Promotable, st.Conflict
}

// Promote 把运行时的目录提到系统级或降回用户级；写系统级由后端拉起 UAC。
func (s *EngineService) Promote(component, level string) (*domain.ToolchainVO, error) {
	if _, err := s.env.PiEnv.Promote(component, level); err != nil {
		return s.Toolchain(), err
	}
	if component == pienv.ComponentNode || component == pienv.ComponentGit {
		if sup := s.env.PiSupervisor(); sup != nil {
			_ = sup.RestartAll()
		}
	}
	return s.Toolchain(), nil
}

// SwitchNode 换一份 node 重新绑定：后续升级与插件管理都跟着走这份解释器。
func (s *EngineService) SwitchNode(exe string) (*domain.ToolchainVO, error) {
	if err := s.refuseWhileRunning("切换运行时"); err != nil {
		return s.Toolchain(), err
	}
	s.stopAll()
	defer s.restartAll()
	if err := s.env.PiEnv.Rebinding(exe); err != nil {
		return s.Toolchain(), s.recordAnd(err)
	}
	git.SetExe(s.env.PiEnv.GitExe())
	return s.Toolchain(), nil
}

// EnableBundledGit 解压内置 MinGit 并接管 git：机器上一份 git 都没有时用它兜底。
func (s *EngineService) EnableBundledGit() (*domain.ToolchainVO, error) {
	pick, err := s.env.PiEnv.EnableBundledGit()
	if err != nil {
		return s.Toolchain(), s.recordAnd(err)
	}
	git.SetExe(pick.Exe)
	return s.Toolchain(), nil
}

// recordAnd 记下失败原因再原样返回，省掉每个分支各写一次。
func (s *EngineService) recordAnd(err error) error {
	s.record(err)
	return err
}

// FixShell 重新探测命令执行环境并写回 PI；用户刚装了 Git 之后点这个。
func (s *EngineService) FixShell() (pienv.ShellStatus, error) {
	st, err := s.env.PiEnv.EnsureShell()
	if err != nil {
		return st, err
	}
	// shellPath 是进程启动期读的：改了就得让旧进程换新。
	if st.Applied {
		if sup := s.env.PiSupervisor(); sup != nil {
			_ = sup.RestartAll()
		}
	}
	return st, nil
}

// EnableTerminal 生成终端入口并挂到用户 PATH，让终端里也能敲 pi。
func (s *EngineService) EnableTerminal() (pienv.TerminalStatus, error) {
	return s.env.PiEnv.EnsureTerminal()
}

// SwitchTerminalLevel 把终端入口在用户级与系统级 PATH 之间挪：写系统级经 UAC 提权完成。
func (s *EngineService) SwitchTerminalLevel(level string) (pienv.TerminalStatus, error) {
	return s.env.PiEnv.SwitchTerminalLevel(level)
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
	if err := s.refuseWhileRunning("安装引擎"); err != nil {
		s.record(err)
		return err
	}
	err := s.env.PiEnv.InstallPi(target, onLine)
	s.record(err)
	if err == nil {
		s.prepareRuntime(onLine)
		s.restartAll()
	}
	return err
}

// prepareRuntime 装完顺手办掉两件环境事：命令执行环境与终端入口。
// 少了这两步，用户装完只会遇到「命令跑不了」和「终端里没有 pi」。
func (s *EngineService) prepareRuntime(onLine func(string)) {
	if onLine != nil {
		onLine("正在准备命令执行环境…")
	}
	if st, err := s.env.PiEnv.EnsureShell(); err != nil {
		if onLine != nil {
			onLine("命令执行环境未能自动修复：" + err.Error())
		}
	} else if onLine != nil {
		onLine("命令执行环境：" + pienv.ShellLabel(st) + "（" + st.Path + "）")
	}
	// 装回了用户自己的 npm 前缀时 pi 已在 PATH 上：再生成转发入口就是第二个 pi 条目。
	if s.env.PiEnv.PiOnPath() {
		report(onLine, "pi 已在 PATH 上，不再生成额外的终端入口")
		return
	}
	st, err := s.env.PiEnv.EnsureTerminal()
	switch {
	case err != nil:
		if onLine != nil {
			onLine("终端入口生成失败：" + err.Error())
		}
	case onLine != nil:
		onLine("终端入口：" + st.Shim + "（新开的终端里可直接敲 pi）")
	}
}

// Update 升级已装的 PI，升级完重启全部进程让新内核生效。
// 升级前先停进程：Windows 会锁住已加载的 .node 原生模块，覆写失败表现为 npm 的 EPERM。
func (s *EngineService) Update(onLine func(string)) error {
	if !s.begin() {
		return pkg.New("9710", "正在升级，等这一趟结束", "")
	}
	defer s.end()
	if err := s.refuseWhileRunning("升级引擎"); err != nil {
		s.record(err)
		return err
	}
	if exe := mustPiExe(s.env); exe == "" {
		s.record(domain.ErrEngineMissing)
		return domain.ErrEngineMissing
	}
	s.stopAll()
	defer s.restartAll()
	err := s.env.PiEnv.UpdatePi(onLine)
	// 升级会换内核：入口钉的是当前生效 PI，升完刷一遍让终端也用上新版。
	if err == nil {
		if _, termErr := s.env.PiEnv.EnsureTerminal(); termErr != nil {
			report(onLine, "终端入口刷新失败："+termErr.Error())
		}
	}
	s.env.PiEnv.RefreshProbes()
	s.record(err)
	return err
}

// UpdatePackages 更新插件包：source 为空表示全部需要更新的包。
// 不走 `pi update --extensions`：它跳过固定版本，而 pi install 写下的正是固定版本。
func (s *EngineService) UpdatePackages(source string, onLine func(string)) error {
	if !s.begin() {
		return pkg.New("9710", "正在装，等这一趟结束", "")
	}
	defer s.end()
	if err := s.refuseWhileRunning("更新插件"); err != nil {
		s.record(err)
		return err
	}

	targets := []packageEntry{}
	if src := strings.TrimSpace(source); src != "" {
		entry, ok := findPackage(s.env, src)
		if !ok {
			return pkg.New("9719", "settings.json 里没有这个包", src)
		}
		targets = append(targets, entry)
	} else {
		targets = pendingPackages(s.env, onLine)
	}
	if len(targets) == 0 {
		if onLine != nil {
			onLine("所有插件都已是最新版本")
		}
		return nil
	}

	// 与升级 PI 同理：先让开被锁的文件，再更新，最后统一重启。
	s.stopAll()
	defer s.restartAll()
	for _, entry := range targets {
		if err := s.updatePackage(entry, onLine); err != nil {
			s.record(err)
			return err
		}
	}
	s.env.PiEnv.RefreshProbes()
	s.record(nil)
	return nil
}

// updatePackage 更新一个包：npm 装注册表上的最新版，git 交给 PI 自己的 update。
// npm 必须带确切版本号，只写 `npm:<name>` 会按包的根 package.json 里的 ^旧版本 范围装回旧版。
func (s *EngineService) updatePackage(entry packageEntry, onLine func(string)) error {
	switch entry.Kind {
	case domain.PackageKindNpm:
		latest, err := pienv.LatestVersion(entry.Name)
		if err != nil {
			return pkg.Wrap("9721", "查不到 "+entry.Name+" 的最新版本", err)
		}
		if latest == entry.Version {
			report(onLine, entry.Name+" 已是最新版本 "+latest)
			return nil
		}
		spec := "npm:" + entry.Name + "@" + latest
		report(onLine, "正在更新 "+entry.Name+"："+firstNonEmpty(entry.Version, "?")+" → "+latest)
		if err := s.env.PiEnv.RunCLI([]string{"install", spec}, onLine); err != nil {
			return err
		}
		after := pienv.LocalVersion(entry.Path)
		if after != latest {
			return pkg.New("9719", "命令结束了，但磁盘上还是 "+firstNonEmpty(after, "未知版本"), entry.Name)
		}
		return nil
	case domain.PackageKindGit:
		report(onLine, "正在更新 "+entry.Source+"…")
		return s.env.PiEnv.RunCLI([]string{"update", entry.Source}, onLine)
	default:
		report(onLine, "本地目录包不参与更新："+entry.Source)
		return nil
	}
}

// refuseWhileRunning 在更新前确认没有对话在跑：更新会停掉 PI 进程，跑着一半的回合会断。
func (s *EngineService) refuseWhileRunning(action string) error {
	if s.env.Chat == nil || !s.env.Chat.AnyRunning() {
		return nil
	}
	return pkg.New("9719", "有对话正在跑，先等它结束或停止它再"+action, "")
}

// pendingPackages 列出需要更新的包：npm 按注册表最新版本比，git 一律交给 update 判定。
func pendingPackages(env *Env, onLine func(string)) []packageEntry {
	out := []packageEntry{}
	for _, entry := range resolvePackages(env) {
		switch entry.Kind {
		case domain.PackageKindGit:
			out = append(out, entry)
		case domain.PackageKindNpm:
			if entry.Missing {
				out = append(out, entry)
				continue
			}
			latest, err := pienv.LatestVersion(entry.Name)
			if err != nil {
				report(onLine, "查不到 "+entry.Name+" 的最新版本："+err.Error())
				continue
			}
			if newerThan(latest, entry.Version) {
				out = append(out, entry)
			}
		}
	}
	return out
}

// findPackage 按来源或包名在已声明的包里找一个。
func findPackage(env *Env, source string) (packageEntry, bool) {
	target := strings.ToLower(strings.TrimSpace(source))
	name := strings.ToLower(packageNameOf(source))
	for _, entry := range resolvePackages(env) {
		if strings.ToLower(entry.Source) == target || strings.ToLower(entry.Name) == name {
			return entry, true
		}
	}
	return packageEntry{}, false
}

// report 向安装日志投一行；调用方不关心回调有没有接。
func report(onLine func(string), line string) {
	if onLine != nil {
		onLine(line)
	}
}

// LastInstallError 返回最近一次失败的原文（成功后清空）。
func (s *EngineService) LastInstallError() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.installErr
}

// begin 抢安装锁；返回 false 表示已经有一趟在跑。
// 抢到就进入维护态：这期间发消息会 spawn 到写到一半的文件上。
func (s *EngineService) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return false
	}
	s.busy = true
	s.env.SetMaintenance(true)
	return true
}

// end 放开安装锁并退出维护态。
func (s *EngineService) end() {
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
	s.env.SetMaintenance(false)
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
