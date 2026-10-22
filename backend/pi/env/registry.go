// npm 源选择：官方源与国内镜像并联探测，先回来的那个胜出，结果带保鲜期。
package env

import (
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// 两个候选 npm 源：官方源在直连网络下不稳，镜像覆盖同一批包。
const (
	RegistryOfficial = "https://registry.npmjs.org"
	RegistryMirror   = "https://registry.npmmirror.com"
)

// RegistryEnv 允许高级用户钉死源，设了就不再探测。
const RegistryEnv = "WORKBABY_NPM_REGISTRY"

// registryPickTTL 是探测结果的保鲜期：网络环境变了重启应用就会重选。
const registryPickTTL = 10 * time.Minute

// registryProbeTimeout 是单源探测上限：宁可等三秒，也不能让安装按钮一直不说话。
const registryProbeTimeout = 4 * time.Second

var (
	registryMu   sync.Mutex
	registryPick string
	registryAt   time.Time
)

// Registry 返回装包该用的 npm 源。
func Registry() string {
	if v := strings.TrimSpace(os.Getenv(RegistryEnv)); v != "" {
		return normalizeRegistry(v)
	}
	registryMu.Lock()
	pick, at := registryPick, registryAt
	registryMu.Unlock()
	if pick != "" && time.Since(at) < registryPickTTL {
		return pick
	}
	pick = fastestRegistry()
	registryMu.Lock()
	registryPick, registryAt = pick, time.Now()
	registryMu.Unlock()
	return pick
}

// ResetRegistry 丢掉探测结果：一趟安装失败后要求换源时调用。
func ResetRegistry() {
	registryMu.Lock()
	registryPick, registryAt = "", time.Time{}
	registryMu.Unlock()
}

// AlternateRegistry 返回另一个候选源。
func AlternateRegistry(current string) string {
	if normalizeRegistry(current) == RegistryMirror {
		return RegistryOfficial
	}
	return RegistryMirror
}

// RegistryLabel 把源地址翻成给人看的短名。
func RegistryLabel(registry string) string {
	if normalizeRegistry(registry) == RegistryMirror {
		return "国内镜像"
	}
	return "官方源"
}

// normalizeRegistry 去掉尾斜杠：拼接路径时多一条斜杠会 404。
func normalizeRegistry(s string) string { return strings.TrimRight(strings.TrimSpace(s), "/") }

// fastestRegistry 并联请求两个源，先回来的那个就是当前网络下更快的；都失败按官方源返回。
// 不等另一个：无代理时官方源会一直挂到超时，等它等于每次装包都白等三秒。
func fastestRegistry() string {
	ch := make(chan string, 2)
	for _, url := range []string{RegistryOfficial, RegistryMirror} {
		go func(url string) {
			if pingRegistry(url) {
				ch <- url
				return
			}
			ch <- ""
		}(url)
	}
	for i := 0; i < 2; i++ {
		if url := <-ch; url != "" {
			return url
		}
	}
	return RegistryOfficial
}

// pingRegistry 取 PI 的最新版本元数据：比 /-/ping 更能确认「这个源真的有这个包」。
func pingRegistry(base string) bool {
	client := &http.Client{Timeout: registryProbeTimeout}
	url := base + "/" + strings.ReplaceAll(piPackage, "/", "%2F") + "/latest"
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
