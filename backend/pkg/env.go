package pkg

import (
	"os"
	"strings"
)

// ChildEnv 组装子进程环境：继承当前进程的环境，只覆写必须钉住的那几条。
// 不做 PATH 前置，也不用 HOME 遮蔽用户家目录——引擎跑的就是用户自己装的 pi，
// 它该看见用户真实的 shell、git 与 python。
func (l Layout) ChildEnv(extra map[string]string) []string {
	overrides := map[string]string{
		// 引擎的家就是界面上管理的那个目录：缺了这条，界面改的配置对引擎不生效。
		"PI_CODING_AGENT_DIR": l.PiAgentDir,
		// 引擎自己的远端版本探测与本壳无关，留着只是启动时多一次网络等待。
		"PI_SKIP_VERSION_CHECK": "1",
		"NO_COLOR":              "1",
	}
	for k, v := range extra {
		overrides[k] = v
	}
	return mergeEnv(os.Environ(), overrides)
}

// mergeEnv 用 overrides 覆盖 environ 里的同名项。
// 环境变量的键大小写不敏感：Windows 上可能同时存在 PATH 与 Path，
// 只按原样比对会留下一条旧值把新值盖掉，所以统一小写判重。
func mergeEnv(environ []string, overrides map[string]string) []string {
	replace := make(map[string]struct{}, len(overrides))
	for k := range overrides {
		replace[strings.ToLower(k)] = struct{}{}
	}
	out := make([]string, 0, len(environ)+len(overrides))
	for _, item := range environ {
		idx := strings.IndexByte(item, '=')
		if idx <= 0 {
			continue
		}
		if _, hit := replace[strings.ToLower(item[:idx])]; hit {
			continue
		}
		out = append(out, item)
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}
