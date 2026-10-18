// PI 运行环境的链路：agent 目录归属、配置文件互通。
// 坏了的表现：界面看不到用户原有的模型/技能、用户手写内容被静默覆盖。
package env

import (
	"encoding/json"
	"os"
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
	// agent 目录是「界面能看到用户已装东西」的前提：跟随环境变量、不被注入覆盖。
	t.Run("agent 目录跟着环境变量走，进程环境不覆盖它", func(t *testing.T) {
		p := pathsOf(t)
		if p.AgentDir != filepath.Join(p.DataDir, "agent") {
			t.Fatalf("agent 目录没有跟随 PI_CODING_AGENT_DIR: %s", p.AgentDir)
		}
		e := New(p)
		seen := 0
		for _, kv := range e.Env() {
			// 只能继承用户自己的设置：换目录会让用户装好的技能、插件、模型全部看不见。
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
	})

	// 闸门决定前端进不进向导：顺序错了用户会被指去错的那一步。
	t.Run("环境闸门按 node → pi → model 的顺序报首个阻塞项", func(t *testing.T) {
		e := New(pathsOf(t))
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

	// 写出去的必须是 PI 认识的 camelCase，且不认识的字段（$schema、compat）不能裁。
	t.Run("models.json 写出 PI 认识的形状，未知字段原样保留", func(t *testing.T) {
		p := pathsOf(t)
		e := New(p)
		authHeader := true
		file := &ModelsFile{Providers: map[string]ProviderConfig{
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
		if err := e.WriteModels(file); err != nil {
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
	})

	// 整体重写会把 PI 自己写的字段与用户用 CLI 装的包抹掉——这是最贵的一类回归。
	// packages 的两种写法（pi install 两种都会写）也在这条链上验。
	t.Run("settings.json 只改自己掌管的键，packages 两种写法都要认", func(t *testing.T) {
		e := New(pathsOf(t))
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
		raw, err := os.ReadFile(e.SettingsPath())
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
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
	})

	// mcp.json 由用户与其它客户端共管：WorkBaby 只做原文往返，不碰内容。
	t.Run("mcp.json 原文往返不改一个字节，项目级路径跟着工作区", func(t *testing.T) {
		e := New(pathsOf(t))
		original := `{"mcpServers":{"user-server":{"command":"node","args":["srv.js"],"timeout":60}},"_comment":"我自己写的说明"}`
		if err := e.WriteMCPRaw(e.MCPPath(), []byte(original)); err != nil {
			t.Fatal(err)
		}
		raw, err := e.MCPRaw(e.MCPPath())
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(raw)) != original {
			t.Fatalf("原文被改写了:\n%s", raw)
		}
		workspace := filepath.Join(t.TempDir(), "proj")
		if got := e.ProjectMCPPath(workspace); got != filepath.Join(workspace, ".pi", "mcp.json") {
			t.Fatalf("项目级 mcp.json 路径不对: %s", got)
		}
		if e.ProjectMCPPath("") != "" {
			t.Fatal("没有工作区时不该给出项目级路径")
		}
	})
}
