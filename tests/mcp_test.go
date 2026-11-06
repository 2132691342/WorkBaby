package tests

import (
	"encoding/json"
	"os"
	"sync"
	"testing"

	"WorkBaby/backend/domain"
)

// TestMcpConcurrentWrites 断言并发增删改不会互相覆盖。
// mcp.json 是读-改-写：没有锁时后写的那次会拿回改前的旧表，
// 前一次改动凭空消失——用户会看到「刚加的服务器自己没了」。
func TestMcpConcurrentWrites(t *testing.T) {
	app, layout, _ := catalogHarness(t, repoAssetsDir(t))

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
}

// TestMcpRemoveKeepsShadowedUserEntry 断言删项目级条目时不连带删掉被它遮蔽的用户级条目。
// 界面上只显示一行，磁盘上却少两条，是那种「删了一个坏了两处」的问题。
func TestMcpRemoveKeepsShadowedUserEntry(t *testing.T) {
	app, layout, _ := catalogHarness(t, repoAssetsDir(t))
	ws := t.TempDir()
	if _, err := app.Settings.SetWorkspace(ws); err != nil {
		t.Fatalf("切工作区失败：%v", err)
	}

	// 用户级同名条目。
	if _, err := app.Mcp.Save(domain.McpSaveREQ{
		ServerName: "dup", CommandText: "node user.js", McpScope: "user",
	}); err != nil {
		t.Fatalf("写用户级失败：%v", err)
	}
	// 项目级同名条目：它遮蔽用户级。
	if _, err := app.Mcp.Save(domain.McpSaveREQ{
		ServerName: "dup", CommandText: "node project.js", McpScope: "project",
	}); err != nil {
		t.Fatalf("写项目级失败：%v", err)
	}
	if _, err := app.Mcp.Remove("dup"); err != nil {
		t.Fatalf("删除失败：%v", err)
	}

	raw, err := os.ReadFile(layout.McpFile())
	if err != nil {
		t.Fatalf("读用户级 mcp.json 失败：%v", err)
	}
	var parsed struct {
		McpServers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if _, ok := parsed.McpServers["dup"]; !ok {
		t.Error("删项目级条目时把被遮蔽的用户级条目也删了")
	}
}

// TestMcpSaveKeepsTimeout 断言保存时不会顺手把用户配的超时删掉。
func TestMcpSaveKeepsTimeout(t *testing.T) {
	app, _, _ := catalogHarness(t, repoAssetsDir(t))

	if _, err := app.Mcp.Save(domain.McpSaveREQ{
		ServerName: "slow", CommandText: "node slow.js", McpScope: "user", McpTimeout: 45,
	}); err != nil {
		t.Fatalf("首次保存失败：%v", err)
	}
	found := false
	for _, row := range app.Mcp.List().Servers {
		if row.ServerName != "slow" {
			continue
		}
		found = true
		if row.McpTimeout != 45 {
			t.Errorf("超时应回显 45 秒，实际 %d", row.McpTimeout)
		}
	}
	if !found {
		t.Fatal("保存后清单里找不到该服务器")
	}
}
