package rag

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"WorkBaby/internal/db"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
)

// rag_test.go 整合：索引器 + 检索器 + 加载器 三类高价值场景。
// 集成测试单一入口，避免被拆成多个细粒度文件后难以定位。

// ===== 基础设施 =====

func newRagTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db") + "?_pragma=journal_mode(MEMORY)&_pragma=busy_timeout(5000)"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	mustNoErr(t, err)
	mustNoErr(t, db.Migrate(gdb))
	mustNoErr(t, db.CreateFTS5(gdb))
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return gdb
}

func addTextDoc(t *testing.T, gdb *gorm.DB, name, content string) *domain.KnowledgeDocDO {
	t.Helper()
	row := &domain.KnowledgeDocDO{
		ID:         pkg.NewID(domain.IDKnowledgeDoc),
		Name:       name,
		Source:     content,
		SourceType: domain.KnowledgeSourceText,
		MIME:       "text/plain",
		Status:     domain.KnowledgeStatusPending,
	}
	mustNoErr(t, gdb.Create(row).Error)
	return row
}

func repoGetDoc(gdb *gorm.DB, id string) (*domain.KnowledgeDocDO, error) {
	var row domain.KnowledgeDocDO
	if err := gdb.First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// 极简断言包装：避免为每个测试引入 testify/require 依赖。
func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ===== 索引器 =====

// TestIndexAndSearchChinese 索引→FTS5 检索端到端：覆盖分块、入库、虚拟表同步、BM25 排序。
func TestIndexAndSearchChinese(t *testing.T) {
	gdb := newRagTestDB(t)
	doc := addTextDoc(t, gdb, "部署手册", "本手册说明知识库检索服务的部署方式。\n\n第二步：配置 SQLite 并启动 FTS5 全文索引。")

	mustNoErr(t, NewIndexer(gdb, DefaultChunker()).Index(context.Background(), doc.ID))

	row, err := repoGetDoc(gdb, doc.ID)
	mustNoErr(t, err)
	if row.Status != domain.KnowledgeStatusIndexed {
		t.Fatalf("status = %q (%s), want indexed", row.Status, row.ErrorMsg)
	}
	if row.ChunkCount <= 0 {
		t.Fatalf("chunk count = %d", row.ChunkCount)
	}

	hits, err := NewFTS5Retriever(gdb).Search(context.Background(), "知识库检索", 5)
	mustNoErr(t, err)
	if len(hits) == 0 {
		t.Fatalf("want hits for chinese query")
	}
	if hits[0].DocName != "部署手册" {
		t.Fatalf("doc name = %q", hits[0].DocName)
	}
	if hits[0].Score <= 0 {
		t.Fatalf("score should be positive after negation: %f", hits[0].Score)
	}
	if !strings.Contains(hits[0].Content, "部署方式") {
		t.Fatalf("content mismatch: %q", hits[0].Content)
	}
}

// TestSearchShortChineseQuery 回归：「部署」「报错」这类 2 字中文查询是中文场景最高频的
// 查询长度，必须能召回相关内容（unicode61 逐字分词下走 FTS，否则 LIKE 兜底），
// 而不是静默空结果（模型会据此编造答案）或按 created_at 返回无关内容。
func TestSearchShortChineseQuery(t *testing.T) {
	gdb := newRagTestDB(t)
	ctx := context.Background()
	docA := addTextDoc(t, gdb, "部署手册", "本手册说明服务的部署方式与回滚步骤。")
	docB := addTextDoc(t, gdb, "无关文档", "这个文档讲的是烹饪与园艺，内容与主题完全无关。")
	mustNoErr(t, NewIndexer(gdb, DefaultChunker()).Index(ctx, docA.ID))
	mustNoErr(t, NewIndexer(gdb, DefaultChunker()).Index(ctx, docB.ID))

	hits, err := NewFTS5Retriever(gdb).Search(ctx, "部署", 5)
	mustNoErr(t, err)
	if len(hits) == 0 {
		t.Fatalf("2 字中文查询必须兜底召回")
	}
	if hits[0].DocName != "部署手册" {
		t.Fatalf("top hit = %q, want 部署手册（兜底结果必须按相关性排序）", hits[0].DocName)
	}
	if !strings.Contains(hits[0].Content, "部署") {
		t.Fatalf("top content mismatch: %q", hits[0].Content)
	}
}

// TestReindexReplacesChunksAndFTS 重建索引必须替换（而非追加）chunks 与 FTS5 行：
// 否则同一文档会出现历史脏数据 + 重复命中。
//
// 设计要点：标记 token 必须选「trigram 无公共前缀」的两组，否则 FTS5 trigram
// 分词器会因共享子串误匹配（query "alpha_old" 也能命中含 "alpha_new" 的内容）。
func TestReindexReplacesChunksAndFTS(t *testing.T) {
	gdb := newRagTestDB(t)
	ctx := context.Background()
	idx := NewIndexer(gdb, DefaultChunker())

	doc := addTextDoc(t, gdb, "架构笔记 v1",
		"第一段：golang 微服务的第一版设计。\n\n第二段：zzzalpha_old 关键字命中点。\n\n第三段：通用说明段段段。")
	mustNoErr(t, idx.Index(ctx, doc.ID))

	doc, err := repoGetDoc(gdb, doc.ID)
	mustNoErr(t, err)
	oldChunkCount := doc.ChunkCount
	if oldChunkCount <= 0 {
		t.Fatalf("v1 索引必须产出至少一个 chunk")
	}

	doc.Source = "第一段：golang 微服务的第二版设计。\n\n第二段：zzqgamma_new 关键字命中点。\n\n第三段：通用说明段段段。"
	mustNoErr(t, gdb.Save(doc).Error)
	mustNoErr(t, idx.Index(ctx, doc.ID))

	doc, err = repoGetDoc(gdb, doc.ID)
	mustNoErr(t, err)
	if doc.ChunkCount != oldChunkCount {
		t.Fatalf("chunks 数量应保持恒定（替换而非追加）: %d → %d", oldChunkCount, doc.ChunkCount)
	}

	// 旧 token 必须不再召回
	hits, err := NewFTS5Retriever(gdb).Search(ctx, "zzzalpha_old", 10)
	mustNoErr(t, err)
	if len(hits) != 0 {
		t.Fatalf("旧关键字必须被 FTS5 同步清理")
	}

	// 新 token 必须召回
	hits, err = NewFTS5Retriever(gdb).Search(ctx, "zzqgamma_new", 10)
	mustNoErr(t, err)
	if len(hits) == 0 {
		t.Fatalf("新关键字必须能召回")
	}
}

// ===== 检索器 =====
