// Package tests 的 MCP 配置治理：mcp.json 是读-改-写，并发锁与作用域遮蔽是全部难点。
package tests

import (
	"encoding/json"
	"os"
	"sync"
	"testing"

	"WorkBaby/backend/domain"
)

// TestMcpConfig 断言 mcp.json 的两条治理纪律：
//
//  1. 并发增删改不互吞：没有锁时后写的那次会拿回改前的旧表，前一次改动凭空消失，
//     用户看到的是「刚加的服务器自己没了」。
//  2. 删项目级条目不连带删被它遮蔽的用户级条目：界面上只显示一行，磁盘上却有两个同名条目。
func TestMcpConfig(t *testing.T) {
	app, layout, _ := catalogHarness(t, "")

	// ---- 并发写 ----
	const total = 12
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
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
}
