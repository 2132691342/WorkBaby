// MCP 能力链路：客户端握手与调用、运行时装配、配置治理。
// 假服务器由测试二进制自起自己（见 mcp_fake_server_test.go），无外部依赖、无网络。
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service"
)

// mcpServerEntry 写一个指向测试二进制自身（假 MCP 服务器模式）的 mcp.json 条目。
func mcpServerEntry(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"command": testExePath(t),
		"args":    []any{"--wb-mcp-server"},
		"enabled": true,
	}
}

// writeMcpConfig 原子写一份 mcp.json。
func writeMcpConfig(t *testing.T, path string, servers map[string]any) {
	t.Helper()
	raw, err := pkg.MarshalJSONLine(map[string]any{"mcpServers": servers})
	if err != nil {
		t.Fatalf("编码 mcp.json 失败：%v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("写 mcp.json 失败：%v", err)
	}
}

// fakeServerDef 引擎形态的服务定义：指向假 MCP 服务器。
func fakeServerDef(t *testing.T, name string) agent.McpServerDef {
	return agent.McpServerDef{
		ServerName: name,
		Command:    testExePath(t),
		Args:       []string{"--wb-mcp-server"},
		Env:        map[string]string{},
	}
}

// testExePath 测试二进制自身。
func testExePath(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("取测试二进制路径失败：%v", err)
	}
	return exe
}

func TestMcp(t *testing.T) {
	// 客户端：握手后拿到工具清单且工具名带服务前缀、tools/call 走通 Registry、
	// 未知方法与死进程各自落到对应错误码。
	t.Run("客户端", func(t *testing.T) {
		t.Run("握手拿到工具清单", func(t *testing.T) {
			tools, client, err := agent.LoadMcpTools(fakeServerDef(t, "fake"), nil)
			if err != nil {
				t.Fatalf("装载失败：%v（detail=%s）", err, pkg.As(err).Detail)
			}
			defer client.Close()
			if len(tools) != 2 {
				t.Fatalf("应注册 2 个工具，实际 %d 个", len(tools))
			}
			names := map[string]bool{}
			for _, tool := range tools {
				spec := tool.Spec()
				names[spec.Name] = true
				if spec.Name != "mcp_fake_echo" && spec.Name != "mcp_fake_fail_always" {
					t.Errorf("工具名形状不对：%q", spec.Name)
				}
				if spec.Schema == nil || spec.Schema["type"] != "object" {
					t.Errorf("工具 %s 的 schema 应原样透传，实际 %v", spec.Name, spec.Schema)
				}
			}
			if !names["mcp_fake_echo"] || !names["mcp_fake_fail_always"] {
				t.Errorf("工具清单不全：%v", names)
			}
		})

		t.Run("调用走通 Registry", func(t *testing.T) {
			tools, client, err := agent.LoadMcpTools(fakeServerDef(t, "fake"), nil)
			if err != nil {
				t.Fatalf("装载失败：%v", err)
			}
			defer client.Close()
			reg := agent.NewRegistry(func() string { return "" })
			reg.ReplaceMcpTools(tools)
			result := reg.Run(t.Context(), "mcp_fake_echo", `{"text":"你好"}`)
			if result.IsError {
				t.Fatalf("echo 调用不应失败：%s", result.Text)
			}
			if result.Text != "echo: 你好" {
				t.Fatalf("echo 应原样说回，实际 %q", result.Text)
			}
			// isError 通道：服务端报错要变成失败结果，而不是成功空文。
			failed := reg.Run(t.Context(), "mcp_fake_fail_always", `{}`)
			if !failed.IsError {
				t.Fatalf("fail_always 应报失败，实际 text=%q", failed.Text)
			}
			if result.Details["mcp_server"] != "fake" {
				t.Errorf("Details 应带上服务名，实际 %v", result.Details)
			}
		})

		t.Run("未知方法走 9102", func(t *testing.T) {
			_, client, err := agent.LoadMcpTools(fakeServerDef(t, "fake"), nil)
			if err != nil {
				t.Fatalf("装载失败：%v", err)
			}
			defer client.Close()
			if _, err := client.Call(t.Context(), "no_such_tool", nil); err == nil {
				t.Fatal("未知工具应报错")
			} else if ae := pkg.As(err); ae.Code != pkg.CodeMcpCall {
				t.Fatalf("未知工具应走 9102（CodeMcpCall），实际 %s", ae.Code)
			}
		})

		t.Run("服务进程死亡立刻报 9103", func(t *testing.T) {
			tools, client, err := agent.LoadMcpTools(fakeServerDef(t, "fake"), nil)
			if err != nil {
				t.Fatalf("装载失败：%v", err)
			}
			if len(tools) == 0 {
				t.Fatal("装载后应有工具")
			}
			client.Close() // 杀掉服务进程，模拟服务端死亡

			done := make(chan agent.Result, 1)
			go func() {
				done <- tools[0].Run(t.Context(), nil, map[string]any{"text": "x"})
			}()
			select {
			case result := <-done:
				if !result.IsError {
					t.Fatalf("死服务的调用应失败，实际 text=%q", result.Text)
				}
				if !strings.Contains(result.Text, pkg.CodeMcpServer) {
					t.Errorf("失败正文应带 9103 码 %s，实际 %q", pkg.CodeMcpServer, result.Text)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("死服务的调用挂起了（超过 5 秒）")
			}
		})
	})

	// 运行时装配：enabled 服务注册进 Registry、disabled 服务不注册；
	// 开关切换与删除后后台重建，工具段跟着清单走；并发重建不会打死注册表。
	t.Run("运行时装配", func(t *testing.T) {
		t.Run("开关与删除驱动重建", func(t *testing.T) {
			app, layout, _ := catalogHarness(t, "")
			entry := mcpServerEntry(t)

			writeMcpConfig(t, layout.McpFile(), map[string]any{
				"fake":  entry,
				"ghost": map[string]any{"command": testExePath(t), "args": []any{"--wb-mcp-server"}, "enabled": false},
			})
			// 用 Save/Toggle 同款入口走一遍 notifyChanged 的同步重建路径。
			app.Mcp.Runtime.Rebuild()

			specs := map[string]bool{}
			for _, spec := range app.Chat.Tools.Specs() {
				specs[spec.Name] = true
			}
			if !specs["mcp_fake_echo"] {
				t.Error("enabled 服务应注册出 mcp_fake_echo")
			}
			if specs["mcp_ghost_echo"] {
				t.Error("disabled 服务不应注册任何工具")
			}

			// 开关切换：关掉 fake，工具应整段消失。
			if _, err := app.Mcp.Toggle("fake", false); err != nil {
				t.Fatalf("开关失败：%v", err)
			}
			waitMcpUntil(t, app, func(names []string) bool { return !hasName(names, "mcp_fake_echo") }, "关闭后工具消失")
			for _, spec := range app.Chat.Tools.Specs() {
				if spec.Name == "mcp_fake_echo" {
					t.Error("关闭后 mcp_fake_echo 仍注册着")
				}
			}

			// 再打开：工具回来。
			if _, err := app.Mcp.Toggle("fake", true); err != nil {
				t.Fatalf("开关失败：%v", err)
			}
			waitMcpUntil(t, app, func(names []string) bool { return hasName(names, "mcp_fake_echo") }, "重新启用后工具回来")

			// 删除：工具段再次清空。
			if _, err := app.Mcp.Remove("fake"); err != nil {
				t.Fatalf("删除失败：%v", err)
			}
			waitMcpUntil(t, app, func(names []string) bool { return !hasName(names, "mcp_fake_echo") }, "删除后工具消失")
			for _, spec := range app.Chat.Tools.Specs() {
				if spec.Name == "mcp_fake_echo" {
					t.Error("删除后 mcp_fake_echo 仍注册着")
				}
			}
		})

		t.Run("并发重建不打死注册表", func(t *testing.T) {
			app, layout, _ := catalogHarness(t, "")
			writeMcpConfig(t, layout.McpFile(), map[string]any{})
			var wg sync.WaitGroup
			for i := range 4 {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					_, _ = app.Mcp.Save(domain.McpSaveREQ{
						ServerName:  "srv" + string(rune('a'+i)),
						CommandText: testExePath(t) + " --wb-mcp-server",
						McpScope:    "user",
					})
				}(i)
			}
			wg.Wait()
			// 四个服务都该注册出来：并发重建不能只装上一半。
			waitMcpUntil(t, app, func(names []string) bool { return len(names) >= 4 }, "并发重建后工具齐全")
		})
	})

	// 配置治理：12 路并发读-改-写一条不丢（无锁时后写会拿回旧表，
	// 用户刚加的服务器凭空消失）；删项目级条目不连带删被它遮蔽的用户级条目。
	t.Run("配置治理", func(t *testing.T) {
		app, layout, _ := catalogHarness(t, "")

		// ---- 并发写 ----
		const total = 12
		var wg sync.WaitGroup
		for i := range total {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				name := "server-" + string(rune('a'+i))
				_, _ = app.Mcp.Save(domain.McpSaveREQ{
					ServerName:  name,
					CommandText: "node server.js",
					McpScope:    "user",
				})
			}(i)
		}
		wg.Wait()

		raw, err := os.ReadFile(layout.McpFile())
		if err != nil {
			t.Fatalf("读 mcp.json 失败：%v", err)
		}
		var parsed struct {
			McpServers map[string]any `json:"mcpServers"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("mcp.json 解析失败（写坏了）：%v", err)
		}
		if len(parsed.McpServers) != total {
			t.Errorf("并发写后应保留 %d 个服务器，实际 %d 个", total, len(parsed.McpServers))
		}

		// ---- 作用域遮蔽 ----
		ws := t.TempDir()
		if _, err := app.Settings.SetWorkspace(ws); err != nil {
			t.Fatalf("切工作区失败：%v", err)
		}
		if _, err := app.Mcp.Save(domain.McpSaveREQ{
			ServerName: "dup", CommandText: "node user.js", McpScope: "user",
		}); err != nil {
			t.Fatalf("写用户级失败：%v", err)
		}
		if _, err := app.Mcp.Save(domain.McpSaveREQ{
			ServerName: "dup", CommandText: "node project.js", McpScope: "project",
		}); err != nil {
			t.Fatalf("写项目级失败：%v", err)
		}
		if _, err := app.Mcp.Remove("dup"); err != nil {
			t.Fatalf("删除失败：%v", err)
		}

		raw, err = os.ReadFile(layout.McpFile())
		if err != nil {
			t.Fatalf("读用户级 mcp.json 失败：%v", err)
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("解析失败：%v", err)
		}
		if _, ok := parsed.McpServers["dup"]; !ok {
			t.Error("删项目级条目时把被遮蔽的用户级条目也删了")
		}
	})
}

// waitMcpUntil 等注册表的 MCP 工具段到达期望状态；重建在后台跑，轮询到为止。
func waitMcpUntil(t *testing.T, app *service.App, want func(names []string) bool, what string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if want(mcpSpecNames(app)) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("后台重建未在 15 秒内落地：%s（当前 %v）", what, mcpSpecNames(app))
}

// mcpSpecNames 当前注册表里的 MCP 工具名清单。
func mcpSpecNames(app *service.App) []string {
	var out []string
	for _, spec := range app.Chat.Tools.Specs() {
		if strings.HasPrefix(spec.Name, "mcp_") {
			out = append(out, spec.Name)
		}
	}
	sort.Strings(out)
	return out
}

// hasName 清单里是否含某个工具名。
func hasName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
