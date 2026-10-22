// PI 运行环境链路：agent 目录归属、环境闸门判定、配置文件互通。
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
)

// pathsOf 造一套指向临时目录的路径；agent 目录也隔离到临时目录，绝不碰真实配置。
func pathsOf(t *testing.T) runtime.Paths {
	t.Helper()
	root := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(root, "agent"))
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
		if withModel := e.Detect(true); !withModel.ModelReady {
			t.Fatal("模型就绪标志没透传")
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

	// shim 解析：经 cmd.exe 起进程会弹控制台窗口，必须还原成 node + 入口脚本，且绝不吐字面量 node。
	t.Run("PI 入口从 npm shim 还原成 node 与脚本，且绝不吐字面量 node", func(t *testing.T) {
		checkShimNodeFallback(t)
		checkPiShimResolution(t)
	})

	// 命令执行环境：WSL 启动器必须被剔除，工具增量能加能撤，自己写过的 shellPath 才会被撤回。
	t.Run("命令执行环境自愈：剔除 WSL 启动器、工具增量可加可撤", func(t *testing.T) {
		checkWSLStubSkipped(t)
		checkShellSelfHeal(t)
	})
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

// checkShellSelfHeal 验两种形态的落盘：没有 bash 写工具增量，有 bash 时撤回并写 shellPath。
func checkShellSelfHeal(t *testing.T) {
	t.Helper()
	e := New(pathsOf(t))
	if err := e.WriteSettings(&SettingsFile{DefaultModel: "m"}); err != nil {
		t.Fatal(err)
	}

	// 没有 bash：把 bash 工具换成 powershell，其余配置一动不动。
	applied, err := e.applyShell(ShellStatus{Kind: ShellKindPowerShell, Path: `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, Usable: true})
	if err != nil || !applied {
		t.Fatalf("没有 bash 时应当写工具增量: applied=%v err=%v", applied, err)
	}
	raw, err := e.settingsMap()
	if err != nil {
		t.Fatal(err)
	}
	list, ok := toolList(raw)
	if !ok || len(list) != 2 || list[0] != "+powershell" || list[1] != "-bash" {
		t.Fatalf("工具增量写错了: %+v", list)
	}
	if _, err := e.ReadSettings(); err != nil {
		t.Fatal(err)
	}
	file, _ := e.ReadSettings()
	if file.DefaultModel != "m" {
		t.Fatal("写工具增量时抹掉了别的字段")
	}

	// 之后找到了 git bash：撤回增量（列表删空就把键去掉），并写上 shellPath。
	applied, err = e.applyShell(ShellStatus{Kind: ShellKindGitBash, Path: `C:\wb-test\Git\bin\bash.exe`, Usable: true})
	if err != nil || !applied {
		t.Fatalf("找到 bash 后应当撤增量并写 shellPath: applied=%v err=%v", applied, err)
	}
	raw, err = e.settingsMap()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["defaultTools"]; ok {
		t.Fatalf("增量删空后应当把 defaultTools 交回 PI 默认集: %s", raw["defaultTools"])
	}
	if got := e.settingString("shellPath"); got != `C:\wb-test\Git\bin\bash.exe` {
		t.Fatalf("shellPath 没写对: %q", got)
	}

	// bash 又不可用了：撤掉自己写过的 shellPath，并把工具换回 powershell。
	if _, err := e.applyShell(ShellStatus{Kind: ShellKindPowerShell, Usable: true}); err != nil {
		t.Fatal(err)
	}
	if got := e.settingString("shellPath"); got != "" {
		t.Fatalf("自己写的 shellPath 没撤回: %q", got)
	}

	// 用户在工具面板里自己勾过（纯名字列表）之后，找到 bash 也不再替用户改回去。
	mine, err := e.ReadSettings()
	if err != nil {
		t.Fatal(err)
	}
	mine.DefaultTools = []string{"read", "edit", "write", "powershell"}
	if err := e.WriteSettings(mine); err != nil {
		t.Fatal(err)
	}
	if _, err := e.applyShell(ShellStatus{Kind: ShellKindGitBash, Path: `C:\wb-test\Git\bin\bash.exe`, Usable: true}); err != nil {
		t.Fatal(err)
	}
	raw, err = e.settingsMap()
	if err != nil {
		t.Fatal(err)
	}
	list, _ = toolList(raw)
	if len(list) != 4 || list[3] != "powershell" {
		t.Fatalf("用户自己勾的工具被改回去了: %+v", list)
	}
}

// checkShimNodeFallback 验 shim 旁边没有 node.exe 时用调用方给的绝对路径，没有就返回空串。
func checkShimNodeFallback(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "cli.js")
	if err := os.WriteFile(script, []byte("// entry\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "pi.cmd")
	// shim 文本里那行入口路径以 .js 结尾，解析器按行取它（与 npm shim 的形状一致）。
	body := "@ECHO off\r\n%dp0%\\cli.js\r\n"
	if err := os.WriteFile(shim, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// 给的绝对 node 必须被采用（内置 Node 的真实形态：不在 PATH 上）。
	fakeNode := filepath.Join(t.TempDir(), "node.exe")
	if err := os.WriteFile(fakeNode, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, args := resolvePiExe(shim, fakeNode)
	if resolved != fakeNode {
		t.Fatalf("没用上给的 node: %q，期望 %q", resolved, fakeNode)
	}
	if len(args) != 1 || args[0] != script {
		t.Fatalf("入口脚本解析错了: %v", args)
	}

	// 没有 node 时只能报空：把 "node" 交给 exec 就是那句 9705。
	if resolved, _ := resolvePiExe(shim, ""); resolved == "node" {
		t.Fatal("解析结果退化成了字面量 node")
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
