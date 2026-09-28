// 覆盖知识库：切分与检索（含短查询子串兜底）。
package knowledge

import (
	"context"
	"path/filepath"
	"testing"

	"WorkBaby/internal/db"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

func newService(t *testing.T) *Service {
	dir := t.TempDir()
	gdb, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() {
		pool, _ := gdb.DB()
		_ = pool.Close()
	})
	return New(repo.New(gdb))
}

func TestAddIndexAndSearch(t *testing.T) {
	svc := newService(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "季度报告.md")
	if err := pkg.WriteText(path, "第三季度销售额增长了 18%，主要来自华东区的渠道拓展。\n客户满意度提升到 92%。"); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.Add([]string{path}); err != nil || n != 1 {
		t.Fatalf("添加文档失败: n=%d err=%v", n, err)
	}
	hits, err := svc.Search(context.Background(), "季度销售额", 5)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("应能检索到内容")
	}
}

func TestShortQueryFallsBackToSubstring(t *testing.T) {
	svc := newService(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "笔记.txt")
	if err := pkg.WriteText(path, "预算审批流程需要三级签字"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add([]string{path}); err != nil {
		t.Fatal(err)
	}
	// 两字查询在 trigram 下零命中，必须靠子串兜底。
	hits, err := svc.Search(context.Background(), "预算", 5)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("短查询应有子串兜底结果")
	}
}

func TestDeleteRemovesChunks(t *testing.T) {
	svc := newService(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := pkg.WriteText(path, "这是一段用于删除测试的内容"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add([]string{path}); err != nil {
		t.Fatal(err)
	}
	docs, err := svc.List()
	if err != nil || len(docs) != 1 {
		t.Fatalf("应有一个文档: %v", err)
	}
	if docs[0].Status != domain.DocIndexed {
		t.Fatalf("文档应已建索引，实际 %s", docs[0].Status)
	}
	if err := svc.Delete(docs[0].ID); err != nil {
		t.Fatal(err)
	}
	left, err := svc.List()
	if err != nil || len(left) != 0 {
		t.Fatalf("删除后不应还有文档: %v", err)
	}
}
