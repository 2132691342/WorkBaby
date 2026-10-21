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

	// 两份配置同一条回路：写 → 校验 → 落盘 → 读回 → 未知字段保留。
	// 写出去的必须是 PI 认识的 camelCase；整体重写会把 PI 自己写的字段与用户用
	// CLI 装的包抹掉——这是最贵的一类回归，packages / compaction / mcp 都在这条链上验。
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
		// 写入必须是原子替换：残留的 .tmp-* 会持续增长。
		entries, err := os.ReadDir(e.AgentDir())
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".tmp-") {
				t.Fatalf("残留了临时文件: %s", entry.Name())
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

	// shim 解析：经 cmd.exe 起进程会弹控制台窗口，必须还原成 node + 入口脚本。
	// 这是本机体验的断言（没装 PI 就跳过），不是协议断言。
	t.Run("PI 入口从 npm shim 还原成 node 与脚本", func(t *testing.T) {
		checkPiShimResolution(t)
	})
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
	resolved, args := resolvePiExe(shim)
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
