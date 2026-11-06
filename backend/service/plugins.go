package service

import (
	"strings"
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/engine"
	"WorkBaby/backend/sse"
)

// Plugins 是插件编排：内置清单落声明、经内置 npm 装进沙箱、启停与升级。
type Plugins struct {
	// Runtime 内置运行时
	Runtime *engine.Runtime
	// Hub 事件广播
	Hub *sse.Hub
	// Chat 引擎门面：动包目录前要先停引擎（Windows 锁文件）
	Chat *Chat

	mu       sync.Mutex
	failures map[string]string
	updates  pluginUpdates
	busy     string
}

// NewPlugins 造一个插件门面。
func NewPlugins(rt *engine.Runtime, hub *sse.Hub, chat *Chat) *Plugins {
	return &Plugins{Runtime: rt, Hub: hub, Chat: chat, failures: map[string]string{}}
}

// DeclareBuiltins 把内置清单写进 settings.json 的 packages 声明。
// 只声明不装包：声明是引擎启动时读的，装包要下几十 MB，必须留在后台。
// 首次声明时按清单默认值落 autoload；用户后续开关只改这一个字段。
func (p *Plugins) DeclareBuiltins() {
	layout := p.Runtime.Layout
	settings := readPiSettings(layout)
	entries := packagesOf(settings)
	changed := false
	seen := map[string]struct{}{}
	for _, item := range entries {
		name := PackageNameOf(parseEntry(item).Source)
		if name != "" {
			seen[name] = struct{}{}
		}
	}
	for _, item := range BuiltinPlugins {
		if _, ok := seen[item.PluginName]; ok {
			continue
		}
		entries = append(entries, entryToJSON(NpmSourceOf(item), item.PluginDefaultOn))
		changed = true
	}
	if !changed {
		return
	}
	settings["packages"] = entries
	if err := writePiSettings(layout, settings); err != nil {
		pkg.Warn("内置插件声明写入失败：%v", err)
	}
}

// ProvisionBuiltins 首启装内置插件（后台跑，失败留给界面上单个重试）。
// 判据是「在且版本对得上钉住的值」：只看目录存在，会让装着别的版本的机器
// 被当成「已安装」，清单钉的版本永远落不下来。
func (p *Plugins) ProvisionBuiltins() (installed, failed int) {
	// 占位不是可选项：首启装配与用户手动点安装是两条入口，
	// 同时跑 npm 会互相覆盖写入，最后得到一个半装的包。
	if err := p.lockBusy("plugin-provision"); err != nil {
		pkg.Warn("内置插件装配跳过：%v", err)
		return 0, 0
	}
	defer p.unlockBusy()

	p.DeclareBuiltins()
	layout := p.Runtime.Layout
	for _, item := range BuiltinPlugins {
		if pluginInstalled(layout, item.PluginName) && !needsVersionFix(layout, item) {
			continue
		}
		if err := p.installOne(NpmSourceOf(item), item.PluginName, "plugin-install"); err != nil {
			failed++
		} else {
			installed++
		}
		p.Hub.Broadcast("plugin:status", sse.EngineTag(map[string]any{"plugin_name": item.PluginName}))
	}
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return installed, failed
}

// needsVersionFix 已装版本与清单钉住的版本不一致时需要重装。
// 清单未钉版本（空串）时一律视为不用管：那是有意跟随 latest。
func needsVersionFix(layout pkg.Layout, item BuiltinPlugin) bool {
	if item.PluginVersion == "" {
		return false
	}
	current, _ := pluginMeta(layout, item.PluginName)
	return current != item.PluginVersion
}

// RetryAll 把缺的内置插件再跑一遍（界面上的「全部重试」）。
func (p *Plugins) RetryAll() (domain.PluginInstallRESP, error) {
	if err := p.lockBusy("plugin-install"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	p.unlockBusy()
	// 先松手再交给 ProvisionBuiltins：它自己会占位，
	// 这里不松就成了自我嵌套，永远拿不到位。
	p.clearFailures()
	installed, failed := p.ProvisionBuiltins()
	return domain.PluginInstallRESP{Installed: installed, Failed: failed}, nil
}

// Install 装一个用户手动给的来源。
func (p *Plugins) Install(source string) (domain.PluginInstallRESP, error) {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeProvision, "插件来源不能为空")
	}
	if err := p.lockBusy("plugin-install"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()
	spec := PluginSourceOf(raw)
	pluginName := PackageNameOf(spec)
	if pluginName == "" {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeProvision, "无法从来源解析出包名", spec)
	}
	if err := p.installOne(spec, pluginName, "plugin-install"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	p.resumeEngine()
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName, Installed: 1}, nil
}

// Retry 重试一个内置插件。
func (p *Plugins) Retry(pluginName string) (domain.PluginInstallRESP, error) {
	item := BuiltinPluginOf(pluginName)
	if item == nil {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeProvision, "不是内置插件："+pluginName)
	}
	if err := p.lockBusy("plugin-install"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()
	if err := p.installOne(NpmSourceOf(*item), item.PluginName, "plugin-install"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName, Installed: 1}, nil
}

// Upgrade 升级：按清单 pin 重装一遍（先清目录，让 npm 真正重下该版本）。
func (p *Plugins) Upgrade(pluginName string) (domain.PluginInstallRESP, error) {
	if err := p.lockBusy("plugin-upgrade"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()
	layout := p.Runtime.Layout
	p.DeclareBuiltins()
	settings := readPiSettings(layout)
	spec := ""
	if item := BuiltinPluginOf(pluginName); item != nil {
		spec = NpmSourceOf(*item)
	} else {
		for _, entry := range packagesOf(settings) {
			parsed := parseEntry(entry)
			if PackageNameOf(parsed.Source) == pluginName {
				spec = parsed.Source
				break
			}
		}
	}
	if spec == "" {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeProvision, "未找到插件："+pluginName)
	}
	// 要动包目录：没在跑的引擎也得先停（锁文件），停完必须重启。
	// 不重启的后果是引擎带着旧版本继续跑，界面上写着新版本号，实际行为是旧的。
	if err := p.stopEngineForDisk(); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.resumeEngine()
	if err := removeAllQuiet(installedPathOf(layout, pluginName)); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	if err := p.installOne(spec, pluginName, "plugin-upgrade"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName, Installed: 1}, nil
}

// UpgradeAll 把版本不符的内置插件升到清单钉住的版本。
// 只处理「版本对不上」的那几个：全部重装会把 12 个包都从网络拉一遍，
// 在包已经对版的机器上白等几分钟且毫无收益。
func (p *Plugins) UpgradeAll() (domain.PluginInstallRESP, error) {
	if err := p.lockBusy("plugin-upgrade-all"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()

	layout := p.Runtime.Layout
	p.DeclareBuiltins()
	if err := p.stopEngineForDisk(); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.resumeEngine()
	installed, failed := 0, 0
	for _, item := range BuiltinPlugins {
		if !pluginInstalled(layout, item.PluginName) || !needsVersionFix(layout, item) {
			continue
		}
		// 先清目录：npm 见到同版本会直接跳过，不清就永远升不到清单的版本。
		if err := removeAllQuiet(installedPathOf(layout, item.PluginName)); err != nil {
			failed++
			p.setFailure(item.PluginName, pkg.As(err).Message)
			continue
		}
		if err := p.installOne(NpmSourceOf(item), item.PluginName, "plugin-upgrade"); err != nil {
			failed++
		} else {
			installed++
		}
		p.Hub.Broadcast("plugin:status", sse.EngineTag(map[string]any{"plugin_name": item.PluginName}))
	}
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{Installed: installed, Failed: failed}, nil
}

// stopEngineForDisk 动包目录前停引擎：Windows 上运行中的 node 锁着包文件，
// 直接删/换目录会失败。正在跑一轮时拒掉——杀引擎等于把用户正在等的回答掐断。
func (p *Plugins) stopEngineForDisk() error {
	if p.Chat == nil {
		return nil
	}
	if p.Chat.Stream.Active() {
		return pkg.Err(pkg.CodeProvision, "引擎正在执行任务，等这一轮结束再动插件")
	}
	// Restart 内部等引擎真退出再返回，锁文件就解了。
	p.Chat.Restart()
	return nil
}

// resumeEngine 把引擎重新拉回当前工作区，让它装载新版本的插件。
// 包只在引擎启动时加载，换完文件不重启就是「界面写着新版本、跑的还是旧行为」。
// 有任务正在跑时不动它：追问一句比打断用户已经等了十分钟的回答更贵。
func (p *Plugins) resumeEngine() {
	if p.Chat == nil || !p.Runtime.PiInstalled() {
		return
	}
	if p.Chat.Stream.Active() {
		pkg.Warn("引擎正忙，新插件下次启动时生效")
		return
	}
	// Restart 对已经停下的引擎是空操作，对还活着的才是真重启。
	p.Chat.Restart()
	if _, err := p.Chat.EnsureRpc(); err != nil {
		pkg.Warn("插件动作后重启引擎失败：%v", err)
	}
}

// Uninstall 卸载：撤声明 + 删包目录。内置插件卸载后仍在清单里（状态回落未安装）。
func (p *Plugins) Uninstall(pluginName string) (domain.PluginInstallRESP, error) {
	if err := p.lockBusy("plugin-uninstall"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()
	layout := p.Runtime.Layout
	if err := p.stopEngineForDisk(); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	p.undeclare(pluginName)
	p.setFailure(pluginName, "")
	if err := removeAllQuiet(installedPathOf(layout, pluginName)); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	pkg.Info("插件已卸载：%s", pluginName)
	p.resumeEngine()
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName}, nil
}

// Toggle 开关：只改 settings.json 的 packages 条目，磁盘不动，秒级生效。
func (p *Plugins) Toggle(pluginName string, enabled bool) (domain.PluginInstallRESP, error) {
	// 首次开关前先补登内置清单：否则在还没装过插件的机器上会拒掉内置项。
	p.DeclareBuiltins()
	layout := p.Runtime.Layout
	settings := readPiSettings(layout)
	entries := packagesOf(settings)
	found := false
	for i, entry := range entries {
		parsed := parseEntry(entry)
		if PackageNameOf(parsed.Source) != pluginName {
			continue
		}
		// 原位替换：保住原有顺序，插件加载次序才不会因为一次开关而变。
		entries[i] = entryToJSON(parsed.Source, enabled)
		found = true
		break
	}
	if !found {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeProvision, "未找到插件声明："+pluginName)
	}
	settings["packages"] = entries
	if err := writePiSettings(layout, settings); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	pkg.Info("插件%s：%s", map[bool]string{true: "启用", false: "停用"}[enabled], pluginName)
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName}, nil
}

// List 插件清单 = 内置清单 ∪ settings 里声明的包。
// 内置项先出（顺序稳定），用户自加的按声明顺序跟在后面。
func (p *Plugins) List() domain.PluginListRESP {
	layout := p.Runtime.Layout
	entries := packagesOf(readPiSettings(layout))
	rows := make([]domain.PluginRowVO, 0, len(entries)+len(BuiltinPlugins))
	seen := map[string]struct{}{}
	for _, item := range BuiltinPlugins {
		source := NpmSourceOf(item)
		enabled := item.PluginDefaultOn
		for _, entry := range entries {
			parsed := parseEntry(entry)
			if PackageNameOf(parsed.Source) == item.PluginName {
				source, enabled = parsed.Source, parsed.Enabled
				break
			}
		}
		rows = append(rows, p.buildRow(item.PluginName, source, enabled))
		seen[item.PluginName] = struct{}{}
	}
	for _, entry := range entries {
		parsed := parseEntry(entry)
		pluginName := PackageNameOf(parsed.Source)
		if pluginName == "" {
			continue
		}
		if _, ok := seen[pluginName]; ok {
			continue
		}
		rows = append(rows, p.buildRow(pluginName, parsed.Source, parsed.Enabled))
		seen[pluginName] = struct{}{}
	}
	builtinInstalled := 0
	for _, row := range rows {
		if row.PluginBuiltin && row.PluginInstalled {
			builtinInstalled++
		}
	}
	return domain.PluginListRESP{
		Plugins:          rows,
		RegistryPath:     layout.SettingsFile(),
		InstallRoot:      layout.PluginRoot(),
		BuiltinTotal:     len(BuiltinPlugins),
		BuiltinInstalled: builtinInstalled,
		Busy:             p.Busy(),
	}
}

// Count 已装且启用的插件数（健康面板用）。
func (p *Plugins) Count() int {
	count := 0
	for _, row := range p.List().Plugins {
		if row.PluginInstalled && row.PluginEnabled {
			count++
		}
	}
	return count
}

// declare 声明一个包：已存在则原位替换（保住顺序），否则追加。
func (p *Plugins) declare(pluginName, source string, enabled bool) {
	layout := p.Runtime.Layout
	settings := readPiSettings(layout)
	entries := packagesOf(settings)
	replaced := false
	for i, entry := range entries {
		if PackageNameOf(parseEntry(entry).Source) == pluginName {
			entries[i] = entryToJSON(source, enabled)
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entryToJSON(source, enabled))
	}
	p.writeEntries(settings, entries)
}

// undeclare 撤掉一个包的声明。
func (p *Plugins) undeclare(pluginName string) {
	layout := p.Runtime.Layout
	settings := readPiSettings(layout)
	entries := packagesOf(settings)
	kept := make([]any, 0, len(entries))
	for _, entry := range entries {
		if PackageNameOf(parseEntry(entry).Source) == pluginName {
			continue
		}
		kept = append(kept, entry)
	}
	p.writeEntries(settings, kept)
}

// writeEntries 把 packages 写回 settings；空数组时删键而不是留 `[]`。
// 留 `[]` 与「没声明过」在引擎眼里等价，但界面上看不出差别，容易被读成「配了但全停用」。
func (p *Plugins) writeEntries(settings map[string]any, entries []any) {
	if len(entries) == 0 {
		delete(settings, "packages")
	} else {
		settings["packages"] = entries
	}
	if err := writePiSettings(p.Runtime.Layout, settings); err != nil {
		pkg.Warn("packages 写入失败：%v", err)
	}
}

// buildRow 组装插件清单的一行。
func (p *Plugins) buildRow(pluginName, source string, enabled bool) domain.PluginRowVO {
	layout := p.Runtime.Layout
	builtin := BuiltinPluginOf(pluginName)
	installed := pluginInstalled(layout, pluginName)
	version, description := pluginMeta(layout, pluginName)
	row := domain.PluginRowVO{
		PluginName:          pluginName,
		PluginSource:        source,
		PluginBuiltin:       builtin != nil,
		PluginEnabled:       enabled && installed,
		PluginInstalled:     installed,
		PluginDesc:          description,
		PluginTargetVersion: "",
		ResourceKinds:       []string{},
	}
	if builtin != nil {
		row.PluginDesc = builtin.PluginDesc
		row.PluginTargetVersion = builtin.PluginVersion
	}
	// 查过更新时把 registry 最新版摆出来：内置项以 pin 为准，用户装的以 latest 为准。
	if latest := p.updatedVersionOf(pluginName); latest != "" {
		row.PluginLatest = latest
	}
	// 可升级 = 已装、有目标版本、两者不一致。
	// 由后端算而不是前端比对：同一条规则不应存在两份实现。
	switch {
	case builtin != nil && builtin.PluginVersion != "":
		row.PluginUpgradable = installed && version != builtin.PluginVersion
	case row.PluginLatest != "":
		row.PluginUpgradable = installed && version != "" && version != row.PluginLatest
	default:
		row.PluginUpgradable = false
	}
	if installed {
		row.PluginVersion = version
		row.PluginPath = installedPathOf(layout, pluginName)
		row.ResourceKinds = resourceKindsOf(layout, pluginName)
	}
	p.mu.Lock()
	failure := p.failures[pluginName]
	p.mu.Unlock()
	row.PluginError = failure
	switch {
	case installed && failure != "":
		row.PluginState = domain.PluginStateFailed
	case installed:
		row.PluginState = domain.PluginStateInstalled
	case failure != "":
		row.PluginState = domain.PluginStateFailed
	default:
		row.PluginState = domain.PluginStateMissing
	}
	return row
}
