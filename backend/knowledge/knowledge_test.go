// 知识库完整链路：加文档 → 建索引 → 检索（含短查询子串兜底）→ 删除级联。
// 单独测任一层都测不出检索失效：文档表、切片表与 FTS5 虚表是协作关系。
package knowledge

import (
	"context"
	"path/filepath"
	"testing"

	"WorkBaby/backend/db"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
)

func newService(t *testing.T) *Service {
	t.Helper()
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

func addFile(t *testing.T, svc *Service, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := pkg.WriteText(path, body); err != nil {
		t.Fatal(err)
	}
	if n, err := svc.Add([]string{path}); err != nil || n != 1 {
		t.Fatalf("添加文档失败: n=%d err=%v", n, err)
	}
	return path
}

func TestKnowledgeChain(t *testing.T) {
	// 检索必须命中内容；两字查询在 trigram 下零命中，要靠子串兜底。
	t.Run("建索引后可检索且短查询有兜底", func(t *testing.T) {
		svc := newService(t)
		addFile(t, svc, "季度报告.md", "第三季度销售额增长了 18%，主要来自华东区的渠道拓展。\n客户满意度提升到 92%。")
		addFile(t, svc, "流程笔记.txt", "预算审批流程需要三级签字")

		docs, err := svc.List()
		if err != nil || len(docs) != 2 {
			t.Fatalf("应有两个文档: %v", err)
		}
		for _, d := range docs {
			if d.Status != domain.DocIndexed {
				t.Fatalf("文档应已建索引，实际 %s", d.Status)
			}
		}

		hits, err := svc.Search(context.Background(), "季度销售额", 5)
		if err != nil {
			t.Fatalf("检索失败: %v", err)
		}
		if len(hits) == 0 {
			t.Fatal("应能检索到内容")
		}
		short, err := svc.Search(context.Background(), "预算", 5)
		if err != nil {
			t.Fatalf("短查询失败: %v", err)
		}
		if len(short) == 0 {
			t.Fatal("短查询应有子串兜底结果")
		}
	})

	// 删除文档必须连带清掉切片，否则 FTS 里会留下查不到来源的孤儿结果。
	t.Run("删除级联清理切片", func(t *testing.T) {
		svc := newService(t)
		addFile(t, svc, "doc.md", "这是一段用于删除测试的内容，会被切成多个切片落库。")

		docs, err := svc.List()
		if err != nil || len(docs) != 1 {
			t.Fatalf("应有一个文档: %v", err)
		}
		if err := svc.Delete(docs[0].ID); err != nil {
			t.Fatal(err)
		}
		left, err := svc.List()
		if err != nil || len(left) != 0 {
			t.Fatalf("删除后不应还有文档: %v", err)
		}
		hits, err := svc.Search(context.Background(), "删除测试", 5)
		if err != nil {
			t.Fatalf("删除后检索失败: %v", err)
		}
		if len(hits) != 0 {
			t.Fatalf("删除后不应还能检索到内容，实际 %d 条", len(hits))
		}
	})
}
