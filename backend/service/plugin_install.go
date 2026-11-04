package service

import (
	"strings"

	"WorkBaby/backend/pkg"
	"WorkBaby/backend/sse"
)

// Busy 当前是否有安装任务在跑。
func (p *Plugins) Busy() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.busy != ""
}

// lockBusy 占住安装位；已被占用时返回业务错误。
func (p *Plugins) lockBusy(action string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.busy != "" {
		return pkg.Err(pkg.CodeProvision, "有安装任务正在进行，稍后再试")
	}
	p.busy = action
	return nil
}

// unlockBusy 释放安装位。
func (p *Plugins) unlockBusy() {
	p.mu.Lock()
	p.busy = ""
	p.mu.Unlock()
}

// progress 发一帧安装进度。
func (p *Plugins) progress(action, phase, pluginName, line, message string) {
	p.Hub.Broadcast("app:progress", sse.EngineTag(map[string]any{
		"progress_kind":    "plugin",
		"progress_action":  action,
		"progress_phase":   phase,
		"progress_line":    line,
		"progress_message": message,
		"plugin_name":      pluginName,
	}))
}

// installOne 装一个包：npm 落地 → 声明进 settings.json。已装同版本时跳过 npm。
func (p *Plugins) installOne(source, pluginName, action string) error {
	layout := p.Runtime.Layout
	p.setFailure(pluginName, "")
	version, _ := pluginMeta(layout, pluginName)
	pinned := ""
	if at := strings.LastIndexByte(source, '@'); at > 0 {
		pinned = source[at+1:]
	}
	if version != "" && pinned != "" && version == pinned && pluginInstalled(layout, pluginName) {
		p.declare(pluginName, source, true)
		return nil
	}
	p.progress(action, "start", pluginName, "", "正在安装 "+pluginName)
	res, err := p.Runtime.NpmInstall(layout.PluginRoot(), source, func(line string) {
		p.progress(action, "line", pluginName, line, "")
	})
	if err != nil {
		detail := pkg.As(err).Message
		p.setFailure(pluginName, detail)
		p.progress(action, "fail", pluginName, "", detail)
		return err
	}
	if res.Code != 0 {
		detail := strings.TrimSpace(res.Stderr)
		if detail == "" {
			detail = "npm 退出码非零"
		}
		detail = pkg.Truncate(detail, 2000)
		p.setFailure(pluginName, detail)
		p.progress(action, "fail", pluginName, "", pkg.Truncate(detail, 400))
		return pkg.Err(pkg.CodeProvision, "插件安装失败："+pluginName, detail)
	}
	if !pluginInstalled(layout, pluginName) {
		message := "安装完成但包目录不存在：" + pluginName
		p.setFailure(pluginName, message)
		p.progress(action, "fail", pluginName, "", message)
		return pkg.Err(pkg.CodeProvision, message)
	}
	p.declare(pluginName, source, true)
	installedVersion, _ := pluginMeta(layout, pluginName)
	p.progress(action, "done", pluginName, "", pluginName+"@"+installedVersion+" 就绪")
	pkg.Info("插件安装完成：%s@%s", pluginName, installedVersion)
	return nil
}

// setFailure 记 / 清一个插件的失败原因。
func (p *Plugins) setFailure(pluginName, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if reason == "" {
		delete(p.failures, pluginName)
		return
	}
	p.failures[pluginName] = reason
}

// clearFailures 清空失败表。
func (p *Plugins) clearFailures() {
	p.mu.Lock()
	p.failures = map[string]string{}
	p.mu.Unlock()
}
