// 依赖方向门禁：把 AGENTS.md §2.1 / §2.2 的约束表变成可执行检查。
// 用 go list -json 读编译器视角的真实 import 关系——文本 grep 会被注释与字符串骗过。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type pkgInfo struct {
	ImportPath string
	Imports    []string
}

// layer -> 允许依赖的 backend 包；空切片表示不得依赖任何 backend 业务包。
// api 是依赖注入容器：持有 Repo / Cfg 等句柄是它的职责，禁止的是绕过 service 直接做业务。
var allowed = map[string][]string{
	"server": {"api", "domain", "pkg"},
	"api": {
		"service", "domain", "pkg",
		"repo", "config", "db", "runtime", "tool", "skill", "knowledge",
	},
	"service":        {"repo", "domain", "pkg", "agent", "llm", "tool", "knowledge", "skill", "runtime", "config", "db"},
	"agent":   {"llm", "tool", "domain", "pkg"},
	"llm":     {"domain", "pkg"},
	"tool":    {"domain", "pkg", "llm", "runtime"},
	"knowledge": {"repo", "domain", "pkg", "db"},
	"skill":   {"domain", "pkg"},
	"runtime": {"pkg"},
	"config":  {"domain", "pkg"},
	"db":      {"domain", "pkg"},
	"repo":    {"domain", "pkg"},
	"tray":    {"domain", "pkg"},
	"singleinstance": {"domain", "pkg"},
	"pkg":     {},
}

// 能力域不得回引的上层包。
var upperLayers = []string{"api", "service", "server", "tray", "singleinstance"}

// domainLayers 是能力域全集。
var domainLayers = []string{"agent", "llm", "tool", "knowledge", "skill", "runtime", "config", "db", "repo"}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func main() {
	modOut, err := exec.Command("go", "list", "-m").Output()
	if err != nil {
		fail("go list -m 失败: %v", err)
	}
	module := strings.TrimSpace(string(modOut))
	prefix := module + "/backend/"

	// 循环依赖会让 go list 直接失败（编译器先于本门禁拒绝）。这类违规同样要报出来，
	// 并带上 go 的原始诊断，否则只剩一句无信息量的「失败」。
	listCmd := exec.Command("go", "list", "-json", "./backend/...")
	out, listErr := listCmd.Output()
	if listErr != nil {
		fmt.Fprintln(os.Stderr, "依赖方向门禁失败：go list 无法解析依赖图")
		fmt.Fprintln(os.Stderr, "最常见的原因是引入了循环依赖（编译器会先于本门禁拒绝）")
		if ee, ok := listErr.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			fmt.Fprintf(os.Stderr, "\n--- go list 原始输出 ---\n%s\n", strings.TrimSpace(string(ee.Stderr)))
		}
		os.Exit(1)
	}

	// 流式解码：多条 JSON 记录首尾相接，逐个 Unmarshal 即可正确切分。
	dec := json.NewDecoder(strings.NewReader(string(out)))
	var pkgs []pkgInfo
	for {
		var p pkgInfo
		if err := dec.Decode(&p); err != nil {
			break
		}
		if p.ImportPath != "" {
			pkgs = append(pkgs, p)
		}
	}
	if len(pkgs) == 0 {
		fail("未解析到任何 backend 包")
	}

	var violations []string
	for _, p := range pkgs {
		if !strings.HasPrefix(p.ImportPath, prefix) {
			continue
		}
		layer := strings.Split(strings.TrimPrefix(p.ImportPath, prefix), "/")[0]
		permitted, known := allowed[layer]
		if !known {
			continue
		}
		leaf := len(permitted) == 0

		for _, imp := range p.Imports {
			if !strings.HasPrefix(imp, prefix) {
				continue
			}
			// 同层互引不算越层：llm 的子包（factory / openai / anthropic）都要
			// 引用父包 llm 的 Streamer 与归一化类型。
			if strings.Split(strings.TrimPrefix(imp, prefix), "/")[0] == layer {
				continue
			}
			target := strings.Split(strings.TrimPrefix(imp, prefix), "/")[0]

			switch {
			case leaf:
				violations = append(violations,
					fmt.Sprintf("%s -> %s（叶子包不得依赖任何 backend 业务包）", layer, target))
			case !contains(permitted, target):
				violations = append(violations,
					fmt.Sprintf("%s -> %s（允许：%s）", layer, target, strings.Join(permitted, " / ")))
			case contains(domainLayers, layer) && contains(upperLayers, target):
				violations = append(violations,
					fmt.Sprintf("%s -> %s（能力域不得依赖上层）", layer, target))
			}
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		uniq := violations[:0]
		for i, v := range violations {
			if i == 0 || v != violations[i-1] {
				uniq = append(uniq, v)
			}
		}
		fmt.Fprintf(os.Stderr, "依赖方向门禁失败：%d 处违规\n", len(uniq))
		for _, v := range uniq {
			fmt.Fprintf(os.Stderr, "  - %s\n", v)
		}
		fmt.Fprintln(os.Stderr, "\n约束来源：AGENTS.md §2.1 / §2.2")
		os.Exit(1)
	}

	fmt.Printf("依赖方向门禁通过：%d 个 backend 包，依赖方向全部合规\n", len(pkgs))
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
