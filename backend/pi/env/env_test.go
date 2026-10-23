// PI 运行环境链路：agent 目录归属、环境闸门判定、配置文件互通、入口解析与命令环境自愈。
// 坏了的表现：界面看不到用户原有的模型与技能、用户手写内容被静默覆盖、向导指错下一步。
package env

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/runtime"
	"WorkBaby/backend/runtime/runtimetest"
)

// pathsOf 造一套指向临时目录的路径（agent 目录也隔离）；SeedMarkers 让运行时命中「已解压」。
// 本包验的是判定逻辑，归档解压的真实性由 runtime 包守着。
func pathsOf(t *testing.T) runtime.Paths {
	t.Helper()
	root := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(root, "agent"))
	runtimetest.SeedMarkers(t, root)
	p, err := runtime.ResolveAt(root)
	if err != nil {
		t.Fatalf("解析路径失败: %v", err)
	}
	return p
}

// TestPiEnvChain 是 env 包唯一的测试入口。
func TestPiEnvChain(t *testing.T) {
	// 闸门 + agent 目录：两者都是「界面还能不能看见用户已有的东西」的前提。
	// 闸门顺序错了用户会被指去错的那一步；agent 目录被换掉，用户装好的技能插件模型全部消失。
	t.Run("环境闸门按 node → pi → model 报首个阻塞项，agent 目录不被进程环境覆盖", func(t *testing.T) {
		p := pathsOf(t)
		if p.AgentDir != filepath.Join(p.DataDir, "agent") {
			t.Fatalf("agent 目录没有跟随 PI_CODING_AGENT_DIR: %s", p.AgentDir)
		}
		e := New(p)

		// 进程环境里只能出现一次 agent 目录，且必须是用户的那个。
		seen := 0
		for _, kv := range e.Env() {
			if strings.HasPrefix(strings.ToUpper(kv), "PI_CODING_AGENT_DIR=") {
				seen++
				if !strings.HasSuffix(kv, p.AgentDir) {
					t.Fatalf("agent 目录被换掉了: %q，期望 %q", kv, p.AgentDir)
				}
			}
		}
		if seen > 1 {
			t.Fatalf("agent 目录出现了 %d 次", seen)
		}

		// 闸门：按 node → pi → model 的顺序报第一个不满足项，并给得出原因。
		vo := e.Detect(false)
		if vo.Ready {
			t.Fatal("模型未就绪时不应报告 ready")
		}
		switch {
		case !vo.NodeReady:
			if vo.Blocking != "node" || vo.NodeError == "" {
				t.Fatalf("node 未就绪时应报 node 并给原因: %+v", vo)
			}
		case !vo.PiReady:
			if vo.Blocking != "pi" || vo.PiError == "" {
				t.Fatalf("pi 未就绪时应报 pi 并给原因: %+v", vo)
			}
		default:
			if vo.Blocking != "model" {
				t.Fatalf("只有模型缺时阻塞项应当是 model，实际 %q", vo.Blocking)
			}
		}
	})

	// 两份配置同一条回路：写 → 校验 → 落盘 → 读回 → 未知字段保留；整体重写会抹掉 PI 自己写的字段。
	t.Run("配置写入：models.json 形状、settings.json 掌管键与字段级合并、mcp.json 原文往返", func(t *testing.T) {
		e := New(pathsOf(t))
		authHeader := true
		models := &ModelsFile{Providers: map[string]ProviderConfig{
			"my-provider": {
				Name: "我的服务", BaseURL: "https://api.example.com/v1",
				APIKey: "sk-test", API: "openai-completions", AuthHeader: &authHeader,
				Models: []ModelConfig{{
					ID: "my-model", Name: "my-model",
					ContextWindow: 128000, MaxTokens: 16000,
					Input: []string{"text", "image"},
				}},
			},
		}}
		if err := e.WriteModels(models); err != nil {
			t.Fatalf("写 models.json 失败: %v", err)
		}
		raw, err := os.ReadFile(e.ModelsPath())
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, want := range []string{`"baseUrl"`, `"apiKey"`, `"contextWindow"`, `"maxTokens"`, `"openai-completions"`} {
			if !strings.Contains(text, want) {
				t.Fatalf("models.json 缺少 %s\n%s", want, text)
			}
		}
		back, err := e.ReadModels()
		if err != nil {
			t.Fatalf("读回 models.json 失败: %v", err)
		}
		if back.Providers["my-provider"].Models[0].ContextWindow != 128000 {
			t.Fatalf("读回的值不对: %+v", back.Providers["my-provider"])
		}

		// 未知字段（$schema、compat）不能被整体重写裁掉：用户手写的注释字段也是。
		manual := `{"$schema":"x","providers":{"p1":{"baseUrl":"u","compat":{"a":1}}}}`
		if err := WriteFileAtomic(e.ModelsPath(), []byte(manual), 0o600); err != nil {
			t.Fatal(err)
		}
		extra, providers, err := e.ReadModelsRaw()
		if err != nil {
			t.Fatal(err)
		}
		providers["p1"] = json.RawMessage(`{"baseUrl":"u","compat":{"a":1},"apiKey":"k"}`)
		if err := e.WriteModelsRaw(extra, providers); err != nil {
			t.Fatal(err)
		}
		raw, _ = os.ReadFile(e.ModelsPath())
		for _, want := range []string{`"$schema"`, `"compat"`, `"apiKey"`} {
			if !strings.Contains(string(raw), want) {
				t.Fatalf("写入后丢了 %s\n%s", want, raw)
			}
		}

		// settings.json：只改掌管键，其余字段必须原样留着。
		original := `{
  "defaultModel": "old",
  "packages": ["npm:@scope/a", {"source":"git:github.com/x/y"}],
  "cacheWarming": "idle",
  "codemode": {"mode": "only"}
}
`
		if err := WriteFileAtomic(e.SettingsPath(), []byte(original), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := e.WriteSettings(&SettingsFile{DefaultModel: "new"}); err != nil {
			t.Fatalf("写 settings.json 失败: %v", err)
		}
		raw, err = os.ReadFile(e.SettingsPath())
		if err != nil {
			t.Fatal(err)
		}
		text = string(raw)
		for _, want := range []string{`"cacheWarming"`, `"codemode"`, `"packages"`, `"new"`} {
			if !strings.Contains(text, want) {
				t.Fatalf("写入后丢了 %s\n%s", want, text)
			}
		}
		if strings.Contains(text, `"old"`) {
			t.Fatalf("旧值没被覆盖:\n%s", text)
		}

		// packages 的两种写法（pi install 两种都会写）都要认：在下一轮覆盖前读完。
		list, err := e.ReadPackages()
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"npm:@scope/a", "git:github.com/x/y"}
		if len(list) != len(want) {
			t.Fatalf("packages 读到 %d 条，期望 %d: %v", len(list), len(want), list)
		}
		for i := range want {
			if list[i] != want[i] {
				t.Fatalf("packages 第 %d 条是 %q，期望 %q", i, list[i], want[i])
			}
		}

		// 嵌套对象按字段合并：compaction 里的 enabled 归 PI 管，不能被我们写丢。
		withCompaction := `{"compaction": {"enabled": true, "reserveTokens": 1000}}`
		if err := WriteFileAtomic(e.SettingsPath(), []byte(withCompaction), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := e.WriteSettings(&SettingsFile{Compaction: &CompactionSettings{KeepRecentTokens: 20000}}); err != nil {
			t.Fatal(err)
		}
		raw, _ = os.ReadFile(e.SettingsPath())
		text = string(raw)
		for _, want := range []string{`"enabled": true`, `"reserveTokens": 1000`, `"keepRecentTokens": 20000`} {
			if !strings.Contains(text, want) {
				t.Fatalf("写压缩参数时丢了 %s\n%s", want, text)
			}
		}

		// mcp.json 由用户与其它客户端共管：WorkBaby 只做原文往返，不碰内容。
		original = `{"mcpServers":{"user-server":{"command":"node","args":["srv.js"],"timeout":60}},"_comment":"我自己写的说明"}`
		if err := e.WriteMCPRaw(e.MCPPath(), []byte(original)); err != nil {
			t.Fatal(err)
		}
		raw, err = e.MCPRaw(e.MCPPath())
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(raw)) != original {
			t.Fatalf("原文被改写了:\n%s", raw)
		}
	})

	// 入口与路径位置：shim 必须还原成 node + 脚本（经 cmd.exe 会弹窗），入口目录必须前置，
	// 且要能认出排在它前面、也带着旧 pi 的目录——否则终端敲到的是另一份内核。
	t.Run("PI 入口与终端位置：shim 还原、入口目录前置、pi.cmd 与 pi 双份都指向生效份", func(t *testing.T) {
		checkPiShimResolution(t)
		checkTerminalPathOrder(t)
		checkTerminalShimFiles(t)
	})

	// 用户已有 pi 就必须用用户的：探测顺序错会让机器上出现第二份内核，升级也写错地方。
	t.Run("用户 PATH 上的 pi 抢先于自装前缀，自生成的终端转发不算数", func(t *testing.T) {
		checkUserPiFirst(t)
	})

	// 命令执行环境：WSL 启动器必须被剔除，shellPath 才是被引导的那个键，自己写过的才撤回。
	t.Run("命令执行环境：剔除 WSL 启动器、shellPath 引导与撤回、不碰工具集", func(t *testing.T) {
		checkWSLStubSkipped(t)
		checkShellSelfHeal(t)
	})
}

// checkUserPiFirst 验 PiExe 的优先级：用户 PATH > 自装前缀；本应用生成的终端入口
// 只是转发，不算「用户的 pi」，不能因为它在 PATH 前面就取代自装那份被认领。
func checkUserPiFirst(t *testing.T) {
	t.Helper()
	p := pathsOf(t)
	e := New(p)

	mine := filepath.Join(p.NpmGlobalDir, "pi.cmd")
	if err := os.WriteFile(mine, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 只有自装前缀：认到它，来源 npm-global。
	t.Setenv("PATH", t.TempDir())
	if exe, src := e.PiExe(); !strings.EqualFold(exe, mine) || src != "npm-global" {
		t.Fatalf("只有自装前缀时应认到它: %q %q", exe, src)
	}

	// PATH 上有用户那份：用户优先，自装的退居其后（升级时不至于写错地方）。
	userDir := t.TempDir()
	userPi := filepath.Join(userDir, "pi.cmd")
	if err := os.WriteFile(userPi, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", userDir)
	if exe, src := e.PiExe(); !strings.EqualFold(exe, userPi) || src != "system" {
		t.Fatalf("用户 PATH 上的 pi 应当优先: %q %q", exe, src)
	}

	// 本应用生成的终端入口在 PATH 最前面：它是转发不是安装，必须被跳过。
	if err := os.MkdirAll(e.terminalDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", strings.Join([]string{e.terminalDir(), userDir}, ";"))
	if err := os.WriteFile(filepath.Join(e.terminalDir(), "pi.cmd"), []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if exe, src := e.PiExe(); !strings.EqualFold(exe, userPi) || src != "system" {
		t.Fatalf("终端转发不该抢走用户的 pi: %q %q", exe, src)
	}
}

// checkTerminalShimFiles 验终端入口是双份：pi.cmd 给 CMD / PowerShell，
// pi（sh）给 Git Bash；两者都指向当前生效的 node 与入口脚本。
func checkTerminalShimFiles(t *testing.T) {
	t.Helper()
	e := New(pathsOf(t))
	dir := t.TempDir()
	script := filepath.Join(dir, "cli.js")
	if err := os.WriteFile(script, []byte("// entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "pi.cmd")
	if err := os.WriteFile(shim, []byte("@ECHO off\r\n%dp0%\\cli.js\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.overridePi = shim
	if err := e.writeTerminalShim(); err != nil {
		t.Fatalf("生成终端入口失败: %v", err)
	}
	node, _ := e.NodeExe()
	if node == "" {
		t.Skip("本机既没有 node 也没有内置运行时标记")
	}
	cmdBody, err := os.ReadFile(filepath.Join(e.terminalDir(), "pi.cmd"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cmdBody), node) || !strings.Contains(string(cmdBody), script) {
		t.Fatalf("pi.cmd 没有指向当前生效 node 与入口脚本:\n%s", cmdBody)
	}
	shBody, err := os.ReadFile(filepath.Join(e.terminalDir(), "pi"))
	if err != nil {
		t.Fatal("Git Bash 用的 pi 入口没有生成")
	}
	text := string(shBody)
	if !strings.HasPrefix(text, "#!/bin/sh") {
		t.Fatalf("pi 入口不是 sh 脚本:\n%s", text)
	}
	// sh 双引号里反斜杠是转义符，路径必须转成正斜杠。
	if strings.ContainsRune(text, '\\') {
		t.Fatalf("sh 入口里不该有反斜杠路径:\n%s", text)
	}
	if !strings.Contains(text, filepath.ToSlash(node)) || !strings.Contains(text, filepath.ToSlash(script)) {
		t.Fatalf("pi 入口没有指向当前生效 node 与入口脚本:\n%s", text)
	}
}

// checkWSLStubSkipped 验 System32 下的 bash.exe 不会被当成候选 bash。
func checkWSLStubSkipped(t *testing.T) {
	t.Helper()
	root := os.Getenv("SystemRoot")
	if root == "" {
		t.Skip("没有 SystemRoot，跳过 WSL 桩判定")
	}
	stub := filepath.Join(root, "System32", "bash.exe")
	if !isWSLStub(stub) {
		t.Fatalf("WSL 启动器没被认出来: %s", stub)
	}
	for _, c := range bashCandidates() {
		if strings.EqualFold(c.path, stub) {
			t.Fatalf("候选 bash 里混进了 WSL 启动器: %s", c.path)
		}
	}
}

// checkShellSelfHeal 验 shellPath 的引导与撤回；defaultTools 全程不许被碰（工具集归工具面板管）。
func checkShellSelfHeal(t *testing.T) {
	t.Helper()
	e := New(pathsOf(t))
	if err := e.WriteSettings(&SettingsFile{DefaultModel: "m"}); err != nil {
		t.Fatal(err)
	}

	// 选中 PI 找不到的 bash（含内置那份）：写 shellPath 指过去，其余配置一动不动。
	bundledBash := filepath.Join(e.Paths().GitBashDir, "usr", "bin", "bash.exe")
	applied, err := e.applyShell(ShellStatus{Kind: ShellKindGitBash, Path: bundledBash, Usable: true, Source: "bundled"})
	if err != nil || !applied {
		t.Fatalf("PI 找不到的 bash 应当写 shellPath: applied=%v err=%v", applied, err)
	}
	raw, err := e.settingsMap()
	if err != nil {
		t.Fatal(err)
	}
	if got := e.settingString("shellPath"); got != bundledBash {
		t.Fatalf("shellPath 没写对: %q", got)
	}
	if _, has := raw["defaultTools"]; has {
		t.Fatalf("命令执行环境不许碰工具集: %s", raw["defaultTools"])
	}
	file, _ := e.ReadSettings()
	if file.DefaultModel != "m" {
		t.Fatal("写 shellPath 时抹掉了别的字段")
	}

	// 用户后来装了 Git 到标准位置（PI 自己找得到）：我们写的 shellPath 必须撤掉。
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		t.Skip("没有 ProgramFiles，跳过标准位置判定")
	}
	goodBash := filepath.Join(pf, "Git", "bin", "bash.exe")
	if applied, err = e.applyShell(ShellStatus{Kind: ShellKindGitBash, Path: goodBash, Usable: true}); err != nil || !applied {
		t.Fatalf("换成 PI 自己找得到的 bash 时应当撤回 shellPath: applied=%v err=%v", applied, err)
	}
	if got := e.settingString("shellPath"); got != "" {
		t.Fatalf("PI 找得到的那份不需要 shellPath，旧值没撤回: %q", got)
	}

	// 我们写的 shellPath 后来失效了（只剩 PowerShell）：必须撤掉，否则 PI 每条命令都报错。
	e.applyShell(ShellStatus{Kind: ShellKindGitBash, Path: bundledBash, Usable: true, Source: "bundled"})
	if _, err := e.applyShell(ShellStatus{Kind: ShellKindPowerShell, Usable: true}); err != nil {
		t.Fatal(err)
	}
	if got := e.settingString("shellPath"); got != "" {
		t.Fatalf("自己写的 shellPath 没撤回: %q", got)
	}

	// 用户在工具面板里自己勾过：之后无论探测出什么形态都不许动它。
	mine, err := e.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	mine.DefaultTools = []string{"read", "edit", "write", "powershell"}
	if err := e.WriteSettings(mine); err != nil {
		t.Fatal(err)
	}
	if _, err := e.applyShell(ShellStatus{Kind: ShellKindGitBash, Path: bundledBash, Usable: true, Source: "bundled"}); err != nil {
		t.Fatal(err)
	}
	raw, err = e.settingsMap()
	if err != nil {
		t.Fatal(err)
	}
	if body, has := raw["defaultTools"]; !has || !strings.Contains(string(body), "powershell") {
		t.Fatalf("用户自己勾的工具被改了: %s", body)
	}
}

// checkPiShimResolution 验本机 PI 的入口解析。
// 只在装了 PI 的机器上跑；没装就跳过——它是「本机体验」的断言，不是协议断言。
func checkPiShimResolution(t *testing.T) {
	t.Helper()
	shim, lookErr := exec.LookPath("pi.cmd")
	if lookErr != nil {
		shim, lookErr = exec.LookPath("pi.exe")
	}
	if lookErr != nil {
		t.Skip("本机没有 PI")
	}
	resolved, args := resolvePiExe(shim, "")
	if filepath.Base(resolved) == "cmd.exe" {
		t.Fatalf("解析结果仍是 cmd.exe：%s（会弹控制台窗口）", resolved)
	}
	if len(args) == 0 || filepath.Ext(args[0]) != ".js" {
		t.Fatalf("没有解析出入口脚本: %v", args)
	}
	// 解析出来的入口必须真的能跑：跑不起来的话「不弹窗」也没意义。
	cmd := exec.Command(resolved, append(append([]string{}, args...), "--version")...)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("入口跑不起来: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), "1.") {
		t.Fatalf("版本号不像 PI: %q", string(out))
	}
}

// checkTerminalPathOrder 验两条硬约束：入口目录被前置（且去掉重复项），
// 以及排在它前面、也带着 pi 入口的目录会被认出来（否则终端敲到的是别人那份）。
func checkTerminalPathOrder(t *testing.T) {
	t.Helper()
	old := t.TempDir()
	newDir := t.TempDir()
	for _, dir := range []string{old, newDir} {
		if err := os.WriteFile(filepath.Join(dir, "pi.cmd"), []byte("@echo off\r\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pathValue := strings.Join([]string{old, `C:\Windows`, newDir, newDir}, ";")

	next := withPathFirst(pathValue, newDir)
	parts := strings.Split(next, ";")
	if normalizePathKey(parts[0]) != normalizePathKey(newDir) {
		t.Fatalf("入口目录没有被前置: %q", next)
	}
	if strings.Count(strings.ToLower(next), strings.ToLower(newDir)) != 1 {
		t.Fatalf("入口目录重复了: %q", next)
	}

	// 旧 pi 仍排在入口目录之前时，必须报 shadow，不然界面会误报「终端已就绪」。
	shadow, others := scanPiShims(pathValue, newDir)
	if normalizePathKey(shadow) != normalizePathKey(old) {
		t.Fatalf("抢先的旧 pi 没被认出来: %q", shadow)
	}
	if len(others) != 1 {
		t.Fatalf("其它 pi 目录应当只有旧那一份: %v", others)
	}

	// 入口目录在最前面时不应再有 shadow。
	shadow, others = scanPiShims(next, newDir)
	if shadow != "" {
		t.Fatalf("入口已在前，却仍报被抢先: %q", shadow)
	}
	if len(others) != 1 || normalizePathKey(others[0]) != normalizePathKey(old) {
		t.Fatalf("其它 pi 目录识别错了: %v", others)
	}
}
