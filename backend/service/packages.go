// 包清单解析：settings.json 的 packages 是声明，磁盘上的解包目录才是事实。
// 插件页与引擎页都要它，放在一处避免两边解析规则分叉。
package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	pienv "WorkBaby/backend/pi/env"
	"WorkBaby/backend/pkg"
)

// packageEntry 是一个已声明的 PI 包：来源、解包目录、种类与本地版本。
type packageEntry struct {
	Source  string
	Name    string
	Path    string
	Kind    string
	Version string
	Missing bool
}

// resolvePackages 把 settings.json 的 packages 解成磁盘目录与版本。
// npm 包落在 <agent>/npm/node_modules/<name>；git 包落在 <agent>/git/<host>/<path>；本地路径原样解析。
func resolvePackages(env *Env) []packageEntry {
	if env == nil || env.PiEnv == nil {
		return []packageEntry{}
	}
	sources, err := env.PiEnv.ReadPackages()
	if err != nil {
		return []packageEntry{}
	}
	agent := env.PiEnv.AgentDir()
	out := make([]packageEntry, 0, len(sources))
	for _, raw := range sources {
		src := strings.TrimSpace(raw)
		if src == "" {
			continue
		}
		entry := packageEntry{Source: src, Name: packageNameOf(src), Kind: packageKindOf(src)}
		switch entry.Kind {
		case "npm":
			name := strings.TrimPrefix(src, "npm:")
			// 去掉 @version；@scope/name 的前导 @ 要保住。
			if idx := strings.LastIndex(name, "@"); idx > 0 {
				name = name[:idx]
			}
			name = strings.TrimSpace(name)
			entry.Name = name
			entry.Path = filepath.Join(agent, "npm", "node_modules", filepath.FromSlash(name))
		case "git":
			entry.Path = gitPackageDir(agent, src)
		default:
			entry.Path = localPackageDir(agent, src)
		}
		if entry.Path == "" {
			out = append(out, entry)
			continue
		}
		if !pkg.DirExists(entry.Path) {
			entry.Missing = true
		} else {
			entry.Version = pienv.LocalVersion(entry.Path)
		}
		out = append(out, entry)
	}
	return out
}

// packageKindOf 判定一条声明属于哪一类来源：只有 npm 与 git 能被 pi update 搬到新版本。
func packageKindOf(src string) string {
	switch {
	case strings.HasPrefix(src, "npm:"):
		return "npm"
	case strings.HasPrefix(src, "git:"), strings.HasPrefix(src, "https://"), strings.HasPrefix(src, "http://"):
		return "git"
	default:
		return "local"
	}
}

// gitPackageDir 还原 git 包的检出目录：<agent>/git/<host>/<owner/repo>。
// 认不出来（scp 写法、带 #ref 的怪形状）返回空串，界面按「只知道它存在」显示。
func gitPackageDir(agent, src string) string {
	raw := src
	for _, prefix := range []string{"git:", "https://", "http://"} {
		raw = strings.TrimPrefix(raw, prefix)
	}
	// 去掉 @ref 与 #ref：检出的目录只由 host/path 决定。
	if idx := strings.LastIndex(raw, "@"); idx > 0 {
		raw = raw[:idx]
	}
	if idx := strings.IndexByte(raw, '#'); idx > 0 {
		raw = raw[:idx]
	}
	parts := strings.SplitN(strings.Trim(raw, "/"), "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return filepath.Join(agent, "git", parts[0], filepath.FromSlash(parts[1]))
}

// localPackageDir 解析本地目录声明：相对路径按 agent 目录算，绝对路径原样。
func localPackageDir(agent, src string) string {
	if filepath.IsAbs(src) {
		return src
	}
	if strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../") {
		return filepath.Join(agent, filepath.FromSlash(src))
	}
	return ""
}

// packageNameOf 从来源写法里取一个能显示的名字。
func packageNameOf(src string) string {
	name := src
	for _, prefix := range []string{"npm:", "git:", "https://", "http://"} {
		name = strings.TrimPrefix(name, prefix)
	}
	if idx := strings.LastIndex(name, "@"); idx > 0 {
		name = name[:idx]
	}
	if strings.HasPrefix(name, "github.com/") {
		parts := strings.Split(strings.TrimPrefix(name, "github.com/"), "/")
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
	}
	return name
}

// packageSkills 列出一个包自带的技能目录（约定目录 <pkg>/skills，或 package.json 的 pi.skills）。
func packageSkills(dir string) []string {
	out := []string{}
	conventional := filepath.Join(dir, "skills")
	if pkg.DirExists(conventional) {
		out = append(out, conventional)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return out
	}
	var manifest struct {
		Pi struct {
			Skills []string `json:"skills"`
		} `json:"pi"`
	}
	if json.Unmarshal(raw, &manifest) != nil {
		return out
	}
	for _, rel := range manifest.Pi.Skills {
		// 清单里允许通配与带 dot 的目录：这里只认普通相对路径，通配交给 PI 自己展开。
		if strings.ContainsAny(rel, "*?[]") {
			continue
		}
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if pkg.DirExists(full) || pkg.FileExists(full) {
			out = append(out, full)
		}
	}
	return out
}
