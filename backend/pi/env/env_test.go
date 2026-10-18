// PI 运行环境的链路：环境闸门判定、进程环境注入、配置文件互通。
// 坏了的表现：没配模型就 spawn、内置 Node 没生效、用户手写内容被静默覆盖。
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
	t.Run("环境闸门与进程环境：阻塞顺序、Node 前置、agent 目录唯一", func(t *testing.T) {
		e := New(pathsOf(t))
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
		// 配了模型但缺 node / pi 时仍然不能 ready：没环境就 spawn 不起来。
		withModel := e.Detect(true)
		if !withModel.ModelReady {
			t.Fatal("模型就绪标志没透传")
		}
		if withModel.Ready && (!withModel.NodeReady || !withModel.PiReady) {
			t.Fatal("缺 node / pi 却报告就绪")
		}

		// 进程环境：内置 Node 必须前置（后置会被用户 PATH 上的旧版本抢先），
		// agent 目录只能出现一次（重复会让「哪个生效」不可预测）。
		var path string
		agentDirTimes := 0
		for _, kv := range e.Env() {
			switch {
			case strings.HasPrefix(strings.ToUpper(kv), "PATH="):
				path = kv[len("PATH="):]
			case strings.HasPrefix(kv, "PI_CODING_AGENT_DIR="):
				agentDirTimes++
				if !strings.HasSuffix(kv, e.Paths().PiAgentDir) {
					t.Fatalf("agent 目录不对: %q", kv)
				}
			}
		}
		if path == "" {
			t.Fatal("没有 PATH")
		}
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(e.Paths().NodeDir)) {
			t.Fatalf("内置 Node 没有前置: %q", path)
		}
		if agentDirTimes != 1 {
			t.Fatalf("agent 目录出现了 %d 次", agentDirTimes)
		}
	})

	t.Run("models.json 与 settings.json 写出后是 PI 认识的形状", func(t *testing.T) {
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
		back, err := e.ReadModels()
		if err != nil {
			t.Fatalf("读回 models.json 失败: %v", err)
		}
		if back.Providers["my-provider"].Models[0].ContextWindow != 128000 {
			t.Fatalf("读回的值不对: %+v", back.Providers["my-provider"])
		}

		settings := &SettingsFile{
			DefaultProvider: "x", DefaultModel: "y",
			DefaultTools: []string{"read", "grep", "find", "ls"},
			Compaction:   &CompactionSettings{ReserveTokens: 16384, KeepRecentTokens: 20000},
		}
		if err := e.WriteSettings(settings); err != nil {
			t.Fatalf("写 settings.json 失败: %v", err)
		}
		readBack, err := e.ReadSettings()
		if err != nil {
			t.Fatal(err)
		}
		// 纯名字列表替换整个集；加减语法依赖 PI 默认集，会让新工具绕过用户勾选。
		for _, name := range readBack.DefaultTools {
			if strings.HasPrefix(name, "+") || strings.HasPrefix(name, "-") {
				t.Fatalf("不该出现增减语法: %v", readBack.DefaultTools)
			}
		}
		if readBack.Compaction == nil || readBack.Compaction.ReserveTokens != 16384 {
			t.Fatalf("压缩参数没写进去: %+v", readBack.Compaction)
		}

		// 写入必须是原子替换：残留的 .tmp-* 会持续增长，半途文件会让 PI 读到坏 JSON。
		if err := e.WriteSettings(&SettingsFile{DefaultModel: "b"}); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(p.PiAgentDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".tmp-") {
				t.Fatalf("残留了临时文件: %s", entry.Name())
			}
		}
		if again, _ := e.ReadSettings(); again.DefaultModel != "b" {
			t.Fatalf("最后写入的值没生效: %+v", again)
		}
	})

	t.Run("settings.json：只改自己掌管的键，PI 自己写的字段与已装的包不能被抹掉", func(t *testing.T) {
		e := New(pathsOf(t))
		// 用户先用 PI 自己的 CLI 装过包，并调过缓存相关设置。
		original := `{
  "defaultModel": "old",
  "packages": ["npm:@scope/pi-tools"],
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
				t.Fatalf("写入后丢了 %s（整体重写会抹掉用户的设置）\n%s", want, text)
			}
		}
		// 自己掌管的键要被完全接管：旧值不得残留成两份。
		if strings.Contains(text, `"old"`) {
			t.Fatalf("旧值没被覆盖:\n%s", text)
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
	})

	t.Run("mcp.json：补内置条目、未知字段原样保留、坏文件不覆盖", func(t *testing.T) {
		e := New(pathsOf(t))
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
		user, ok := servers["user-server"]
		if !ok {
			t.Fatalf("用户条目被删了: %s", raw)
		}
		// 不认识的字段必须原样留着：裁掉它们等于静默改掉用户的配置。
		if !strings.Contains(string(user), "headers") || !strings.Contains(string(user), "timeout") {
			t.Fatalf("未知字段被裁掉了: %s", user)
		}
		if _, ok := doc["_comment"]; !ok {
			t.Fatalf("顶层未知字段被删了: %s", raw)
		}

		// 用户把文件改坏了：不覆盖它（那会毁掉他的东西），只报错让他自己修。
		broken := `{"mcpServers": {`
		if err := e.WriteMCPRaw([]byte(broken)); err != nil {
			t.Fatal(err)
		}
		if err := e.EnsureKnowledgeMCP("x.exe"); err == nil {
			t.Fatal("坏配置上不该静默改写")
		}
		if raw, _ := e.ReadMCPRaw(); strings.TrimSpace(string(raw)) != broken {
			t.Fatalf("坏配置被改写了: %q", raw)
		}
	})
}
