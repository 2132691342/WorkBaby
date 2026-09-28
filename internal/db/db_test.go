// 数据库初始化：全部业务表与 FTS5 虚表必须能建出来。
// 少一张表对应一条断掉的链路（知识库检索挂在 FTS5 上，统计挂在 token_usages 上）。
package db

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesEveryTable(t *testing.T) {
	gdb, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	pool, _ := gdb.DB()
	defer pool.Close()

	var names []string
	if err := gdb.Raw(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`).Scan(&names).Error; err != nil {
		t.Fatalf("查询表失败: %v", err)
	}
	want := map[string]bool{
		"sessions": true, "entries": true, "approvals": true,
		"providers": true, "knowledge_docs": true, "knowledge_chunks": true,
		"settings": true, "token_usages": true, "knowledge_chunks_fts": true,
	}
	for _, n := range names {
		delete(want, n)
	}
	if len(want) != 0 {
		t.Fatalf("缺少表: %v（已有 %v）", want, names)
	}
}
