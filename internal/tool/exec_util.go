package tool

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// itoa 供平台文件共用，避免各自重复定义。
func itoa(n int) string { return strconv.Itoa(n) }

// runtimePathEnv 在继承环境的基础上，把内置解释器目录插到 PATH 首位：
// 用户机器上可能装着另一个 Python，pip 装到那里等于下次运行就找不到。
func runtimePathEnv(exe string) []string {
	env := append(os.Environ(), "PYTHONIOENCODING=utf-8")
	if exe == "" {
		return env
	}
	dir := filepath.Dir(exe)
	if dir == "" {
		return env
	}
	out := make([]string, 0, len(env)+1)
	out = append(out, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
