// 版本对账：本地装的是哪一版、npm 上最新是哪一版。引擎页与插件页共用这一份。
package env

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/pkg"
)

// registryTimeout 是单个包的版本查询上限：查不到就报「查不到」，不让界面一直转。
const registryTimeout = 12 * time.Second

// registryTTL 是版本号缓存时长：面板来回切时不必每次都问注册表。
const registryTTL = 5 * time.Minute

type versionHit struct {
	version string
	at      time.Time
}

var (
	latestMu    sync.Mutex
	latestCache = map[string]versionHit{}
)

// LatestVersion 问 npm 注册表要某个包的最新版本号。
// 取 abbreviated 元数据（只有 dist-tags 与版本清单），比完整文档小两个数量级。
func LatestVersion(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", pkg.New("9721", "没有包名，查不了版本", "")
	}
	latestMu.Lock()
	if hit, ok := latestCache[name]; ok && time.Since(hit.at) < registryTTL {
		latestMu.Unlock()
		return hit.version, nil
	}
	latestMu.Unlock()

	url := "https://registry.npmjs.org/" + strings.ReplaceAll(name, "/", "%2F")
	client := &http.Client{Timeout: registryTimeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", pkg.Wrap("9721", "版本查询地址不对", err)
	}
	req.Header.Set("Accept", "application/vnd.npm.install-v1+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", pkg.Wrap("9721", "连不上 npm 注册表，检查网络", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return "", pkg.New("9721", "npm 注册表返回异常", strings.TrimSpace(resp.Status))
	}
	var doc struct {
		DistTags map[string]string `json:"dist-tags"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", pkg.Wrap("9721", "注册表响应读不出来", err)
	}
	version := strings.TrimSpace(doc.DistTags["latest"])
	if version == "" {
		return "", pkg.New("9721", "注册表没有这个包的版本信息", name)
	}
	latestMu.Lock()
	latestCache[name] = versionHit{version: version, at: time.Now()}
	latestMu.Unlock()
	return version, nil
}

// LatestPiVersion 问 PI 自己的最新版本。
func LatestPiVersion() (string, error) { return LatestVersion(piPackage) }

// LocalVersion 读一个包目录里 package.json 的 version；读不到返回空串。
func LocalVersion(dir string) string {
	if dir == "" {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return ""
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &manifest) != nil {
		return ""
	}
	return strings.TrimSpace(manifest.Version)
}
