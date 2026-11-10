// 配置治理：MCP 的并发写与作用域遮蔽，以及历史 SQLite 遗留文件的清理边界。
package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
)

// TestMcpConfig 断言 mcp.json 的两条治理纪律：12 路并发读-改-写一条不丢（无锁时后写
// 会拿回旧表，用户刚加的服务器凭空消失）；删项目级条目不连带删被它遮蔽的用户级条目。
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

// sqliteMagic 是 SQLite 库文件头，判据读的就是这 16 字节。
const sqliteMagic = "SQLite format 3\x00"

// TestLegacyCleanup 断言遗留 SQLite 文件的清理边界：
// 该删的删干净、JSON 存储不在时一个都不动、不是 SQLite 的同名文件不碰。
func TestLegacyCleanup(t *testing.T) {
	// ---- 正常路径：JSON 存储已就绪 + 主库确实是 SQLite → 三件套全清 ----
	dir := t.TempDir()
	store := filepath.Join(dir, "workbaby.json")
	if err := os.WriteFile(store, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("写 JSON 存储失败：%v", err)
	}
	for _, name := range []string{"workbaby.db", "workbaby.db-shm", "workbaby.db-wal"} {
		body := []byte("wal-content")
		if name == "workbaby.db" {
			body = []byte(sqliteMagic + "rest")
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatalf("写遗留文件失败：%v", err)
		}
	}
	removed := repo.CleanupLegacyDB(dir, store)
	if len(removed) != 3 {
		t.Fatalf("三件套都应被清掉，实际清了 %d 个：%v", len(removed), removed)
	}
	for _, name := range []string{"workbaby.db", "workbaby.db-shm", "workbaby.db-wal"} {
		if pkg.Exists(filepath.Join(dir, name)) {
			t.Errorf("%s 应已删除", name)
		}
	}
	if !pkg.Exists(store) {
		t.Error("JSON 存储本身不该被动")
	}

	// ---- JSON 存储不在：旧库一个字节都不能动 ----
	orphanDir := t.TempDir()
	legacy := filepath.Join(orphanDir, "workbaby.db")
	if err := os.WriteFile(legacy, []byte(sqliteMagic+"rest"), 0o644); err != nil {
		t.Fatalf("写遗留库失败：%v", err)
	}
	if got := repo.CleanupLegacyDB(orphanDir, filepath.Join(orphanDir, "workbaby.json")); len(got) != 0 {
		t.Fatalf("JSON 存储不在时不该删任何文件，实际删了：%v", got)
	}
	if !pkg.Exists(legacy) {
		t.Fatal("JSON 存储不在时旧库必须留着")
	}

	// ---- 同名但不是 SQLite：那不是我们当初写的库，不碰 ----
	strangerDir := t.TempDir()
	strangerStore := filepath.Join(strangerDir, "workbaby.json")
	if err := os.WriteFile(strangerStore, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("写 JSON 存储失败：%v", err)
	}
	stranger := filepath.Join(strangerDir, "workbaby.db")
	if err := os.WriteFile(stranger, []byte("这不是 SQLite 库"), 0o644); err != nil {
		t.Fatalf("写同名文件失败：%v", err)
	}
	if got := repo.CleanupLegacyDB(strangerDir, strangerStore); len(got) != 0 {
		t.Fatalf("非 SQLite 的同名文件不该被删，实际删了：%v", got)
	}
	if !pkg.Exists(stranger) {
		t.Fatal("非 SQLite 的同名文件必须留着")
	}
}
