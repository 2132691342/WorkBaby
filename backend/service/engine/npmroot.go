package engine

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"WorkBaby/backend/pkg"
)

// npmRootOnce 让全局根只探一次：它在一个进程生命周期里不会变。
var (
	npmRootOnce sync.Once
	npmRootVal  string
)

// npmGlobalRoot 找 npm 的全局安装根，找不到给空串。
// 顺序：npm 自己的环境变量 → node 可执行文件同级 → 各版本管理器约定目录。
// 全程只查目录存在性，不启动任何子进程——启动 npm 要几百毫秒，只为问一个路径不值当。
func npmGlobalRoot() string {
	npmRootOnce.Do(func() { npmRootVal = findNpmGlobalRoot() })
	return npmRootVal
}

// findNpmGlobalRoot 是 npmGlobalRoot 的实际探测逻辑。
func findNpmGlobalRoot() string {
	// npm 装了以后会把前缀写进环境；用户自己设过就用它。
	if prefix := strings.TrimSpace(os.Getenv("npm_config_prefix")); prefix != "" {
		if dir := existingModulesRoot(prefix); dir != "" {
			return dir
		}
	}
	// node 与 npm 通常同级：node.exe 所在目录下就是 node_modules。
	if nodeExe := pkg.LookPath("node"); nodeExe != "" {
		if dir := existingModulesRoot(filepath.Dir(nodeExe)); dir != "" {
			return dir
		}
	}
	// 版本管理器把全局包放在自己的前缀里。
	for _, prefix := range versionManagerPrefixes() {
		if dir := existingModulesRoot(prefix); dir != "" {
			return dir
		}
	}
	return ""
}

// existingModulesRoot 判断一个前缀下是否真有 node_modules，有就回它的路径。
func existingModulesRoot(prefix string) string {
	root := filepath.Join(prefix, "node_modules")
	if pkg.IsDir(root) {
		return root
	}
	return ""
}

// versionManagerPrefixes 常见 node 版本管理器的全局前缀。
func versionManagerPrefixes() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []string{
		os.Getenv("NVM_SYMLINK"),
		os.Getenv("N_PREFIX"),
		os.Getenv("VOLTA_HOME"),
		os.Getenv("FNM_DIR"),
		filepath.Join(home, "AppData", "Roaming", "npm"),
		filepath.Join(home, ".volta"),
		filepath.Join(home, "AppData", "Local", "Volta"),
	}
	out := make([]string, 0, len(candidates))
	for _, item := range candidates {
		if strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return out
}
