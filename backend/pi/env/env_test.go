// PI 运行环境的链路：环境闸门判定、PATH 注入、配置互通。
// 坏了的表现：没配模型就去 spawn（错误推到第一次对话）、内置 Node 没生效（用了系统旧版）、
// 用户在 APPEND_SYSTEM.md 里手写的内容被静默覆盖。
package env

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"WorkBaby/backend/runtime"
)

// pathsOf 造一套指向临时目录的路径。
func pathsOf(t *testing.T) runtime.Paths {
	t.Helper()
	root := t.TempDir()
	p, err := runtime.ResolveAt(root)
	if err != nil {
		t.Fatalf("解析路径失败: %v", err)
	}
	return p
}

// TestPiEnvChain 是 env 包唯一的测试入口。
func TestPiEnvChain(t *testing.T) {
	t.Run("环境闸门报出首个不满足项", func(t *testing.T) {
		p := pathsOf(t)
		e := New(p)
		// 三个都没配：阻塞项必须是第一个不满足的，而不是随便一个。
		vo := e.Detect(false)
		if vo.Ready {
			t.Fatal("模型未就绪时不应报告 ready")
		}
		switch {
		case !vo.NodeReady:
			if vo.Blocking != "node" {
				t.Fatalf("node 未就绪时阻塞项应当是 node，实际 %q", vo.Blocking)
			}
			if vo.NodeError == "" {
				t.Fatal("阻塞时必须给出原因，不能只说不可用")
			}
		case !vo.PiReady:
			if vo.Blocking != "pi" {
				t.Fatalf("pi 未就绪时阻塞项应当是 pi，实际 %q", vo.Blocking)
			}
		default:
			if vo.Blocking != "model" {
				t.Fatalf("只有模型缺时阻塞项应当是 model，实际 %q", vo.Blocking)
			}
		}
	})

	t.Run("模型就绪也不等于环境就绪", func(t *testing.T) {
		p := pathsOf(t)
		e := New(p)
		withoutModel := e.Detect(false)
		withModel := e.Detect(true)
		if !withModel.ModelReady {
			t.Fatal("模型就绪标志没透传")
		}
		// 配了模型但缺 node / pi 时仍然不能 ready：没环境就 spawn 不起来。
		if withModel.Ready && (!withModel.NodeReady || !withModel.PiReady) {
			t.Fatal("缺 node / pi 却报告就绪")
		}
		if withoutModel.Ready == withModel.Ready && !withModel.ModelReady {
			t.Fatal("模型状态没影响判定")
		}
	})

	t.Run("PATH 前置注入内置运行时", func(t *testing.T) {
		p := pathsOf(t)
		e := New(p)
		env := e.Env()
		var path string
		var hasAgentDir bool
		for _, kv := range env {
			if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
				path = kv[len("PATH="):]
			}
			if strings.HasPrefix(kv, "PI_CODING_AGENT_DIR=") {
				hasAgentDir = true
				if !strings.HasSuffix(kv, p.PiAgentDir) {
					t.Fatalf("agent 目录不对: %q", kv)
				}
			}
		}
		if path == "" {
			t.Fatal("没有 PATH")
		}
		// 必须前置：后置拼接会被用户 PATH 上的旧版本抢先，内置运行时等于没生效。
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(p.NodeDir)) {
			t.Fatalf("内置 Node 没有前置: %q", path[:min(120, len(path))])
		}
		if !hasAgentDir {
			t.Fatal("必须显式指定 PI 的 agent 目录，否则 PI 的状态不在应用目录里")
		}
	})

	t.Run("agent 目录只出现一次且环境变量不重复", func(t *testing.T) {
		p := pathsOf(t)
		e := New(p)
		n := 0
		for _, kv := range e.Env() {
			if strings.HasPrefix(kv, "PI_CODING_AGENT_DIR=") {
				n++
			}
		}
		// 重复的 PATH / agent-dir 会让「哪个生效」变得不可预测。
		if n != 1 {
			t.Fatalf("agent 目录出现了 %d 次", n)
		}
	})

	t.Run("models.json 写出后是 PI 认识的形状", func(t *testing.T) {
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
		// 字段名必须是 PI 的 camelCase（baseUrl / apiKey / contextWindow）。
		text := string(raw)
		for _, want := range []string{`"baseUrl"`, `"apiKey"`, `"contextWindow"`, `"maxTokens"`, `"openai-completions"`} {
			if !strings.Contains(text, want) {
				t.Fatalf("models.json 缺少 %s\n%s", want, text)
			}
		}
		// 读回来要是同一份。
		back, err := e.ReadModels()
		if err != nil {
			t.Fatalf("读回 models.json 失败: %v", err)
		}
		if back.Providers["my-provider"].Models[0].ContextWindow != 128000 {
			t.Fatalf("读回的值不对: %+v", back.Providers["my-provider"])
		}
	})

	t.Run("settings.json 的 defaultTools 是纯名字列表", func(t *testing.T) {
		p := pathsOf(t)
		e := New(p)
		file := &SettingsFile{
			DefaultProvider: "x", DefaultModel: "y",
			DefaultTools: []string{"read", "grep", "find", "ls"},
			Compaction:   &CompactionSettings{ReserveTokens: 16384, KeepRecentTokens: 20000},
		}
		if err := e.WriteSettings(file); err != nil {
			t.Fatalf("写 settings.json 失败: %v", err)
		}
		back, err := e.ReadSettings()
		if err != nil {
			t.Fatal(err)
		}
		// 纯名字列表替换整个集；加减语法依赖 PI 默认集，会让新工具绕过用户勾选。
		for _, name := range back.DefaultTools {
			if strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
				t.Fatalf("不该出现增减语法: %v", back.DefaultTools)
			}
		}
		if back.Compaction == nil || back.Compaction.ReserveTokens != 16384 {
			t.Fatalf("压缩参数没写进去: %+v", back.Compaction)
		}
	})

	t.Run("mcp.json 内置条目写进去且未知字段保留", func(t *testing.T) {
		p := pathsOf(t)
		e := New(p)
		// 用户已经有一份配置，带着我们不认识的字段。
		original := `{
  "mcpServers": {
    "user-server": {
      "command": "node",
      "args": ["srv.js"],
      "headers": {"X-Custom": "1"},
      "timeout": 60
    }
  },
  "_comment": "我自己写的说明"
}
`
		if err := e.WriteMCPRaw([]byte(original)); err != nil {
			t.Fatal(err)
		}
		if err := e.EnsureKnowledgeMCP(`C:\app\WorkBaby.exe`); err != nil {
			t.Fatalf("写知识库条目失败: %v", err)
		}
		raw, _ := e.ReadMCPRaw()
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("写出来的不是合法 JSON: %v\n%s", err, raw)
		}
		var servers map[string]json.RawMessage
		if err := json.Unmarshal(doc["mcpServers"], &servers); err != nil {
			t.Fatal(err)
		}
		if _, ok := servers["workbaby-knowledge"]; !ok {
			t.Fatalf("知识库条目没写进去: %s", raw)
		}
		// 用户的条目与不认识的字段必须原样留着。
		user, ok := servers["user-server"]
		if !ok {
			t.Fatalf("用户条目被删了: %s", raw)
		}
		if !strings.Contains(string(user), "headers") || !strings.Contains(string(user), "timeout") {
			t.Fatalf("未知字段被裁掉了: %s", user)
		}
		if _, ok := doc["_comment"]; !ok {
			t.Fatalf("顶层未知字段被删了: %s", raw)
		}
	})

	t.Run("mcp.json 被改坏时不覆盖", func(t *testing.T) {
		p := pathsOf(t)
		e := New(p)
		broken := `{"mcpServers": {`
		if err := e.WriteMCPRaw([]byte(broken)); err != nil {
			t.Fatal(err)
		}
		// 用户把文件改坏了：不覆盖它（那会毁掉他的东西），只报错让他自己修。
		if err := e.EnsureKnowledgeMCP("x.exe"); err == nil {
			t.Fatal("坏配置上不该静默改写")
		}
		raw, _ := e.ReadMCPRaw()
		if strings.TrimSpace(string(raw)) != broken {
			t.Fatalf("坏配置被改写了: %q", raw)
		}
	})

	t.Run("配置写入是原子替换", func(t *testing.T) {
		p := pathsOf(t)
		e := New(p)
		if err := e.WriteSettings(&SettingsFile{DefaultModel: "a"}); err != nil {
			t.Fatal(err)
		}
		if err := e.WriteSettings(&SettingsFile{DefaultModel: "b"}); err != nil {
			t.Fatal(err)
		}
		// 临时文件不能留在目录里：每次写入都漏一个 .tmp-* 会持续增长。
		entries, err := os.ReadDir(p.PiAgentDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".tmp-") {
				t.Fatalf("残留了临时文件: %s", entry.Name())
			}
		}
		back, _ := e.ReadSettings()
		if back.DefaultModel != "b" {
			t.Fatalf("最后写入的值没生效: %+v", back)
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
