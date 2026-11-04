package pkg

import (
	"os"
	"strings"
)

// SandboxEnv 组装子进程环境：内置 node / git / python 在前、系统 PATH 在后。
// PATH 前置让 agent 先命中内置副本；HOME / npm 前缀 / PI_CODING_AGENT_DIR 把家搬进沙箱。
func (l Layout) SandboxEnv(extra map[string]string) []string {
	prepend := make([]string, 0, 5)
	for _, dir := range []string{
		l.NodeDir,
		l.GitDir + `\cmd`,
		l.GitDir + `\bin`,
		l.GitDir + `\usr\bin`,
		l.PythonDir,
	} {
		if IsDir(dir) {
			prepend = append(prepend, dir)
		}
	}
	overrides := map[string]string{
		"PATH":                       strings.Join(append(prepend, os.Getenv("PATH")), ";"),
		"npm_config_cache":           l.NpmCacheDir,
		"npm_config_prefix":          l.NpmGlobalDir,
		"npm_config_update_notifier": "false",
		"npm_config_fund":            "false",
		"npm_config_audit":           "false",
		"HOME":                       l.DataDir,
		"USERPROFILE":                l.DataDir,
		"PI_CODING_AGENT_DIR":        l.PiAgentDir,
		"PI_SKIP_VERSION_CHECK":      "1",
		"PI_TELEMETRY":               "0",
		"NODE_NO_WARNINGS":           "1",
	}
	for k, v := range extra {
		overrides[k] = v
	}
	// 环境变量的键大小写不敏感：Windows 上可能同时存在 PATH 与 Path，
	// 只按原样比对会留下一条旧值把新值盖掉，所以统一小写判重。
	keyOf := func(s string) string { return strings.ToLower(s) }
	replace := make(map[string]struct{}, len(overrides))
	for k := range overrides {
		replace[keyOf(k)] = struct{}{}
	}
	out := make([]string, 0, len(os.Environ())+len(overrides))
	for _, item := range os.Environ() {
		idx := strings.IndexByte(item, '=')
		if idx <= 0 {
			continue
		}
		if _, hit := replace[keyOf(item[:idx])]; hit {
			continue
		}
		out = append(out, item)
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}
