package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/internal/tool"
)

// fakeBin 在指定目录建一个同名可执行文件，用于判定 PATH 查找到底命中了哪个目录。
func fakeBin(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	exe := name
	if filepath.Separator == '\\' {
		exe += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dir, exe), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake exe: %v", err)
	}
}

// TestExecResolve builtin 运行时路径查找与白名单闸门：
// 内置目录必须优先于系统 PATH，查找后必须还原 PATH（并发命令不互相污染），白名单外命令被拒。
func TestExecResolve(t *testing.T) {
	t.Run("builtin_takes_priority", func(t *testing.T) {
		builtinDir, systemDir := t.TempDir(), t.TempDir()
		fakeBin(t, builtinDir, "pyfake")
		fakeBin(t, systemDir, "pyfake")
		t.Setenv("PATH", systemDir)

		got, prefix := resolveWithBuiltin("pyfake", []string{builtinDir})
		if len(prefix) != 0 || !strings.HasPrefix(strings.ToLower(got), strings.ToLower(builtinDir)) {
			t.Fatalf("内置目录应优先命中，got=%s prefix=%v", got, prefix)
		}
	})

	t.Run("path_restored_after_lookup", func(t *testing.T) {
		systemDir := t.TempDir()
		fakeBin(t, systemDir, "x")
		origPath := systemDir + string(os.PathListSeparator) + "/usr/bin"
		t.Setenv("PATH", origPath)

		_, _ = resolveWithBuiltin("x", []string{t.TempDir()})
		if after := os.Getenv("PATH"); after != origPath {
			t.Fatalf("查找后必须还原 PATH：before=%q after=%q", origPath, after)
		}
	})

	t.Run("whitelist_gate", func(t *testing.T) {
		policy := tool.ExecPolicy{AllowedBinaries: []string{"pyfake"}}
		if ok, _ := policy.Classify("pyfake --version"); !ok {
			t.Fatal("白名单内命令应放行")
		}
		if ok, _ := policy.Classify("danger --rm -rf /"); ok {
			t.Fatal("白名单外命令应拒绝")
		}
	})
}
