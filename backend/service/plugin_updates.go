package service

// 插件检查更新：直接问 npm registry 的 latest，不 spawn npm——
// 全部内置包逐个起 npm 进程要十几秒，一次 HTTPS 只要一秒多。
// 结果缓存在内存里，List 组行时并入；没查到的字段留空，界面不猜。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// pluginUpdates是检查更新的内存缓存：包名 → registry 最新版。
// 结构体自持锁：它由 List（多协程读）与 CheckUpdates（网络协程写）共用。
type pluginUpdates struct {
	mu     sync.RWMutex
	latest map[string]string
}

// CheckUpdates 逐包问 npm registry 的最新版，写进缓存后回一份新清单。
// 单包失败不影响其余：离线时表现就是「最新版一列留空」。
func (p *Plugins) CheckUpdates() (domain.PluginListRESP, error) {
	names := p.declaredNpmNames()
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := map[string]string{}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			version, err := npmLatest(client, name)
			if err != nil {
				pkg.Warn("插件最新版查询失败 %s：%v", name, err)
				return
			}
			mu.Lock()
			results[name] = version
			mu.Unlock()
		}(name)
	}
	wg.Wait()
	p.updates.mu.Lock()
	p.updates.latest = results
	p.updates.mu.Unlock()
	pkg.Info("插件最新版检查完成：%d/%d", len(results), len(names))
	return p.List(), nil
}

// updatedVersionOf 查缓存；没查过给空串。
func (p *Plugins) updatedVersionOf(pluginName string) string {
	p.updates.mu.RLock()
	defer p.updates.mu.RUnlock()
	return p.updates.latest[pluginName]
}

// declaredNpmNames 收集走 npm registry 的插件名：内置清单 + settings 里 npm: 声明。
// git: / ./ 来源不在 npm 上，问了也是白问。
func (p *Plugins) declaredNpmNames() []string {
	seen := map[string]struct{}{}
	for _, item := range BuiltinPlugins {
		seen[item.PluginName] = struct{}{}
	}
	for _, entry := range packagesOf(readPiSettings(p.Runtime.Layout)) {
		source := parseEntry(entry).Source
		if !strings.HasPrefix(source, "npm:") {
			continue
		}
		if name := PackageNameOf(source); name != "" {
			seen[name] = struct{}{}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	return names
}

// npmLatest 问 registry 单包的最新版本号。
func npmLatest(client *http.Client, name string) (string, error) {
	url := "https://registry.npmjs.org/" + name + "/latest"
	res, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry 回了 %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return "", err
	}
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return "", err
	}
	if meta.Version == "" {
		return "", fmt.Errorf("应答缺 version 字段")
	}
	return meta.Version, nil
}
