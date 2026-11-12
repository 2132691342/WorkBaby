package service

import (
	"strings"
	"sync"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/engine"
	"WorkBaby/backend/sse"
)

// Plugins 是插件编排：读 pi 的声明、调 pi 的包命令、把结果映射成界面行。
// 落盘只有一套——pi 自己的 settings.json 与它自己的 npm 根，壳不写第二份。
type Plugins struct {
	// Runtime 引擎入口
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

// Recommend 把清单里的推荐插件装上（只装缺的，已装的不动）。
// 这是用户点了「安装推荐插件」才会走的路：壳不替用户往他自己的配置里写东西。
func (p *Plugins) Recommend() (domain.PluginInstallRESP, error) {
	if err := p.lockBusy("plugin-recommend"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()

	layout := p.Runtime.Layout
	installed, failed := 0, 0
	for _, item := range RecommendedPlugins {
		if pluginInstalled(layout, item.PluginName) {
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
	if installed > 0 {
		p.resumeEngine()
	}
	return domain.PluginInstallRESP{Installed: installed, Failed: failed}, nil
}

// Install 装一个用户手动给的来源。
func (p *Plugins) Install(source string) (domain.PluginInstallRESP, error) {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeBadRequest, "插件来源不能为空")
	}
	if err := p.lockBusy("plugin-install"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()
	spec := PluginSourceOf(raw)
	pluginName := PackageNameOf(spec)
	if pluginName == "" {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeBadRequest, "无法从来源解析出包名", spec)
	}
	if err := p.installOne(spec, pluginName, "plugin-install"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	p.resumeEngine()
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName, Installed: 1}, nil
}

// Retry 重试一个推荐插件的安装。
func (p *Plugins) Retry(pluginName string) (domain.PluginInstallRESP, error) {
	item := RecommendedPluginOf(pluginName)
	if item == nil {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeNotFound, "不是推荐插件："+pluginName)
	}
	if err := p.lockBusy("plugin-install"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()
	if err := p.installOne(NpmSourceOf(*item), item.PluginName, "plugin-install"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	p.resumeEngine()
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName, Installed: 1}, nil
}

// Upgrade 升级一个插件：交给 pi 的包命令，它自己知道该怎么重装。
// 推荐插件按清单钉的版本；用户自装插件按声明的来源串。
func (p *Plugins) Upgrade(pluginName string) (domain.PluginInstallRESP, error) {
	if err := p.lockBusy("plugin-upgrade"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()
	spec := p.specOf(pluginName)
	if spec == "" {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeNotFound, "未找到插件："+pluginName)
	}
	// 要动包目录：引擎没在跑也得先停（Windows 锁文件），停完必须重启。
	if err := p.stopEngineForDisk(); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.resumeEngine()
	if err := p.runPiPackage([]string{"install", spec}, pluginName, "plugin-upgrade"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName, Installed: 1}, nil
}

// UpgradeAll 只升版本与清单不符的推荐插件，外加用户自装的 npm 包升到 latest。
func (p *Plugins) UpgradeAll() (domain.PluginInstallRESP, error) {
	if err := p.lockBusy("plugin-upgrade-all"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()

	layout := p.Runtime.Layout
	if err := p.stopEngineForDisk(); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.resumeEngine()
	installed, failed := 0, 0
	for _, row := range p.List().Plugins {
		if !row.PluginUpgradable {
			continue
		}
		spec := p.specOf(row.PluginName)
		if spec == "" {
			continue
		}
		if err := p.runPiPackage([]string{"install", spec}, row.PluginName, "plugin-upgrade"); err != nil {
			failed++
		} else {
			installed++
		}
		p.Hub.Broadcast("plugin:status", sse.EngineTag(map[string]any{"plugin_name": row.PluginName}))
	}
	// 推荐清单外的用户自装包：交给 pi 自己的 update --extensions。
	if err := p.runPiPackage([]string{"update", "--extensions"}, "", "plugin-upgrade"); err != nil {
		failed++
	}
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	_ = layout
	return domain.PluginInstallRESP{Installed: installed, Failed: failed}, nil
}

// specOf 取一个插件该装的来源串：推荐清单优先按钉住的版本，其余按用户声明。
func (p *Plugins) specOf(pluginName string) string {
	if item := RecommendedPluginOf(pluginName); item != nil {
		return NpmSourceOf(*item)
	}
	for _, entry := range packagesOf(readPiSettings(p.Runtime.Layout)) {
		parsed := parseEntry(entry)
		if PackageNameOf(parsed.Source) == pluginName {
			return parsed.Source
		}
	}
	return ""
}

// stopEngineForDisk 动包目录前停引擎：Windows 上运行中的 node 锁着包文件，
// 直接删/换目录会失败。正在跑一轮时拒掉——杀引擎等于把用户正在等的回答掐断。
func (p *Plugins) stopEngineForDisk() error {
	if p.Chat == nil {
		return nil
	}
	if p.Chat.Stream.Active() {
		return pkg.Err(pkg.CodeEngineBusy, "引擎正在执行任务，等这一轮结束再动插件")
	}
	// Restart 内部等引擎真退出再返回，锁文件就解了。
	p.Chat.Restart()
	return nil
}

// resumeEngine 把引擎重新拉回当前工作区，让它装载新版本的插件。
// 包只在引擎启动时加载，换完文件不重启就是「界面写着新版本、跑的还是旧行为」。
// 有任务正在跑时不动它：追问一句比打断用户已经等了十分钟的回答更贵。
func (p *Plugins) resumeEngine() {
	if p.Chat == nil || !p.Runtime.Installed() {
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

// Uninstall 卸载：走 pi 的 remove，它同时撤声明与删包目录。
func (p *Plugins) Uninstall(pluginName string) (domain.PluginInstallRESP, error) {
	if err := p.lockBusy("plugin-uninstall"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	defer p.unlockBusy()
	spec := p.specOf(pluginName)
	if spec == "" {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeNotFound, "未找到插件："+pluginName)
	}
	if err := p.stopEngineForDisk(); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	p.setFailure(pluginName, "")
	if err := p.runPiPackage([]string{"remove", spec}, pluginName, "plugin-uninstall"); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	pkg.Info("插件已卸载：%s", pluginName)
	p.resumeEngine()
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName}, nil
}

// Toggle 开关：只改 settings.json 的 packages 条目，磁盘不动，重启引擎后生效。
func (p *Plugins) Toggle(pluginName string, enabled bool) (domain.PluginInstallRESP, error) {
	piSettingsMu.Lock()
	defer piSettingsMu.Unlock()
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
		// 用 entryToggle 而不是从零造：用户在这条声明上配的资源过滤不能被开关抹掉。
		entries[i] = entryToggle(entry, enabled)
		found = true
		break
	}
	if !found {
		return domain.PluginInstallRESP{}, pkg.Err(pkg.CodeNotFound, "未找到插件声明："+pluginName)
	}
	settings["packages"] = entries
	if err := writePiSettings(layout, settings); err != nil {
		return domain.PluginInstallRESP{}, err
	}
	pkg.Info("插件%s：%s", map[bool]string{true: "启用", false: "停用"}[enabled], pluginName)
	p.Hub.Broadcast("plugin:status", sse.EngineTag(nil))
	return domain.PluginInstallRESP{PluginName: pluginName}, nil
}

// List 插件清单 = 推荐清单 ∪ settings 里声明的包。
// 推荐项先出（顺序稳定），用户自加的按声明顺序跟在后面。
func (p *Plugins) List() domain.PluginListRESP {
	layout := p.Runtime.Layout
	entries := packagesOf(readPiSettings(layout))
	rows := make([]domain.PluginRowVO, 0, len(entries)+len(RecommendedPlugins))
	seen := map[string]struct{}{}
	for _, item := range RecommendedPlugins {
		source := NpmSourceOf(item)
		enabled := false
		declared := false
		for _, entry := range entries {
			parsed := parseEntry(entry)
			if PackageNameOf(parsed.Source) == item.PluginName {
				source, enabled, declared = parsed.Source, parsed.Enabled, true
				break
			}
		}
		rows = append(rows, p.buildRow(item.PluginName, source, enabled, declared))
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
		rows = append(rows, p.buildRow(pluginName, parsed.Source, parsed.Enabled, true))
		seen[pluginName] = struct{}{}
	}
	recommendedInstalled := 0
	for _, row := range rows {
		if row.PluginRecommended && row.PluginInstalled {
			recommendedInstalled++
		}
	}
	return domain.PluginListRESP{
		Plugins:              rows,
		RegistryPath:         layout.SettingsFile(),
		InstallRoot:          layout.PluginRoot(),
		RecommendedTotal:     len(RecommendedPlugins),
		RecommendedInstalled: recommendedInstalled,
		Busy:                 p.Busy(),
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

// buildRow 组装插件清单的一行。declared 表示 settings 里有没有它这条声明：
// 推荐插件没声明也没装 = 「可安装」，界面上给的就是安装按钮。
func (p *Plugins) buildRow(pluginName, source string, enabled, declared bool) domain.PluginRowVO {
	layout := p.Runtime.Layout
	recommended := RecommendedPluginOf(pluginName)
	installed := pluginInstalled(layout, pluginName)
	version, description := pluginMeta(layout, pluginName)
	row := domain.PluginRowVO{
		PluginName:        pluginName,
		PluginSource:      source,
		PluginRecommended: recommended != nil,
		PluginDeclared:    declared,
		PluginEnabled:     enabled && installed,
		PluginInstalled:   installed,
		PluginDesc:        description,
		ResourceKinds:     []string{},
	}
	if recommended != nil {
		row.PluginDesc = recommended.PluginDesc
		row.PluginTargetVersion = recommended.PluginVersion
	}
	// 查过更新时把 registry 最新版摆出来：推荐项以 pin 为准，用户装的以 latest 为准。
	if latest := p.updatedVersionOf(pluginName); latest != "" {
		row.PluginLatest = latest
	}
	// 可升级 = 已装、有目标版本、两者不一致。
	// 由后端算而不是前端比对：同一条规则不应存在两份实现。
	switch {
	case recommended != nil && recommended.PluginVersion != "":
		row.PluginUpgradable = installed && version != recommended.PluginVersion
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
	case installed && !enabled:
		row.PluginState = domain.PluginStateDisabled
	case installed:
		row.PluginState = domain.PluginStateInstalled
	case failure != "":
		row.PluginState = domain.PluginStateFailed
	default:
		row.PluginState = domain.PluginStateMissing
	}
	return row
}
