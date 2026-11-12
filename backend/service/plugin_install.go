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

// installOne 装一个包：交给 pi 的 `install`，由它落盘并写声明。
// 已装同版本时跳过——重装一遍要下几十 MB，界面上看起来一样。
func (p *Plugins) installOne(source, pluginName, action string) error {
	layout := p.Runtime.Layout
	p.setFailure(pluginName, "")
	version, _ := pluginMeta(layout, pluginName)
	if pinned := pinnedVersionOf(source); pinned != "" && version == pinned && pluginInstalled(layout, pluginName) {
		return nil
	}
	p.progress(action, "start", pluginName, "", "正在安装 "+pluginName)
	if err := p.runPiPackage([]string{"install", source}, pluginName, action); err != nil {
		return err
	}
	if !pluginInstalled(layout, pluginName) {
		message := "安装完成但包目录不存在：" + pluginName
		p.setFailure(pluginName, message)
		p.progress(action, "fail", pluginName, "", message)
		return pkg.Err(pkg.CodeProvision, message)
	}
	installedVersion, _ := pluginMeta(layout, pluginName)
	p.progress(action, "done", pluginName, "", pluginName+"@"+installedVersion+" 就绪")
	pkg.Info("插件安装完成：%s@%s", pluginName, installedVersion)
	return nil
}

// runPiPackage 跑一条 pi 的包命令，把输出转成进度帧。
// 装包走网络，失败原因原样留在 detail 里——「npm 报了 404」比「安装失败」有用得多。
func (p *Plugins) runPiPackage(args []string, pluginName, action string) error {
	res, err := p.Runtime.RunCLI(args, func(line string) {
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
			detail = "pi 退出码非零：" + strings.Join(args, " ")
		}
		detail = pkg.Truncate(detail, 2000)
		p.setFailure(pluginName, detail)
		p.progress(action, "fail", pluginName, "", pkg.Truncate(detail, 400))
		return pkg.Err(pkg.CodeProvision, "pi 包命令失败："+strings.Join(args, " "), detail)
	}
	return nil
}

// pinnedVersionOf 从来源串里取钉住的版本；没有给空串。
// 只有 npm 来源能对版本：git 源的 @ref 与本地路径都不是版本号，
// 拿去跟 package.json 的 version 比永远不会相等，只会多余地重装一遍。
func pinnedVersionOf(source string) string {
	if !strings.HasPrefix(source, "npm:") {
		return ""
	}
	if at := strings.LastIndexByte(source, '@'); at > 1 {
		return source[at+1:]
	}
	return ""
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
