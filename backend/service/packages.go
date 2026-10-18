// 包清单解析：settings.json 的 packages 是声明，磁盘上的解包目录才是事实。
// 插件页与技能页都要它，放在一处避免两边解析规则分叉。
package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"WorkBaby/backend/pkg"
)

// packageEntry 是一个已声明的 PI 包。
type packageEntry struct {
	Source  string
	Name    string
	Path    string
	Missing bool
}

// resolvePackages 把 settings.json 的 packages 解成磁盘目录。
// npm 包落在 <agent>/npm/node_modules/<name>；git 与本地路径按路径解析。
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
		entry := packageEntry{Source: src, Name: packageNameOf(src)}
		switch {
		case strings.HasPrefix(src, "npm:"):
			name := strings.TrimPrefix(src, "npm:")
			// 去掉 @version；@scope/name 的前导 @ 要保住。
			if idx := strings.LastIndex(name, "@"); idx > 0 {
				name = name[:idx]
			}
			entry.Name = name
			entry.Path = filepath.Join(agent, "npm", "node_modules", filepath.FromSlash(name))
		case strings.HasPrefix(src, "./"), strings.HasPrefix(src, "../"):
			entry.Path = filepath.Join(agent, filepath.FromSlash(src))
		case filepath.IsAbs(src):
			entry.Path = src
		default:
			// git:<url> / https:// / 裸包名：PI 自己管检出目录，这里只知道它存在。
			entry.Path = ""
		}
		if entry.Path == "" {
			entry.Missing = false
			out = append(out, entry)
			continue
		}
		if !pkg.DirExists(entry.Path) {
			entry.Missing = true
		}
		out = append(out, entry)
	}
	return out
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
