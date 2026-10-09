// 知识库完整链路：加文档 → 建索引 → 检索（含短查询子串兜底）→ 删除级联，
// 以及 Office 文档（zip+xml）抽取的页序正确性。
// 单独测任一层都测不出检索失效：文档表、切片表与 FTS5 虚表是协作关系。
package knowledge

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
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
	// 加 → 查 → 删 → 再查是一条链：两字查询在 trigram 下零命中要靠子串兜底，
	// 删除文档必须连带清掉切片，否则 FTS 里会留下查不到来源的孤儿结果。
	t.Run("建索引可检索、短查询有兜底、删除级联", func(t *testing.T) {
		svc := newService(t)
		addFile(t, svc, "季度报告.md", "第三季度销售额增长了 18%，主要来自华东区的渠道拓展。\n客户满意度提升到 92%。")
		flowPath := addFile(t, svc, "流程笔记.txt", "预算审批流程需要三级签字")

		docs, err := svc.List()
		if err != nil || len(docs) != 2 {
			t.Fatalf("应有两个文档: %v", err)
		}
		var flowID string
		for _, d := range docs {
			if d.Status != domain.DocIndexed {
				t.Fatalf("文档应已建索引，实际 %s（路径 %s）", d.Status, d.Path)
			}
			if d.Path == flowPath {
				flowID = d.ID
			}
		}
		if flowID == "" {
			t.Fatal("列表里找不到刚加入的流程笔记")
		}

		if hits, err := svc.Search(context.Background(), "季度销售额", 5); err != nil || len(hits) == 0 {
			t.Fatalf("应能检索到内容: err=%v hits=%d", err, len(hits))
		}
		if short, err := svc.Search(context.Background(), "预算", 5); err != nil || len(short) == 0 {
			t.Fatalf("两字查询应有子串兜底结果: err=%v hits=%d", err, len(short))
		}

		if err := svc.Delete(flowID); err != nil {
			t.Fatal(err)
		}
		if left, err := svc.List(); err != nil || len(left) != 1 {
			t.Fatalf("删除后应只剩一个文档: err=%v n=%d", err, len(left))
		}
		if gone, err := svc.Search(context.Background(), "三级签字", 5); err != nil || len(gone) != 0 {
			t.Fatalf("删除后不应还能检索到内容: err=%v hits=%d", err, len(gone))
		}
	})

	// pptx 的页序按文件序号排，不靠 zip 条目顺序：
	// zip 里 slide10 可能排在 slide2 前面，按条目顺序读会把页序打乱。
	t.Run("pptx 抽取页序正确且可检索", func(t *testing.T) {
		svc := newService(t)
		path := filepath.Join(t.TempDir(), "汇报.pptx")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		slides := map[string]string{
			"ppt/slides/slide2.xml": `<a:p><a:t>第二页讲成本控制</a:t></a:p>`,
			"ppt/slides/slide1.xml": `<a:p><a:t>第一页讲季度增长</a:t></a:p>`,
		}
		// 刻意先写 slide2：抽取结果必须仍按页号排序。
		for _, name := range []string{"ppt/slides/slide2.xml", "ppt/slides/slide1.xml"} {
			w, err := zw.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte(slides[name])); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		if n, err := svc.Add([]string{path}); err != nil || n != 1 {
			t.Fatalf("添加 pptx 失败: n=%d err=%v", n, err)
		}
		text, err := LoadText(path)
		if err != nil {
			t.Fatalf("抽取 pptx 失败: %v", err)
		}
		if i1, i2 := strings.Index(text, "第一页"), strings.Index(text, "第二页"); i1 < 0 || i2 < 0 || i1 > i2 {
			t.Fatalf("页序不对或内容缺失: %q", text)
		}
		if hits, err := svc.Search(context.Background(), "成本控制", 5); err != nil || len(hits) == 0 {
			t.Fatalf("pptx 内容应可检索: err=%v hits=%d", err, len(hits))
		}
	})
}
