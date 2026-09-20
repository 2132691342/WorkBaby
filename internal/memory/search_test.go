package memory

// 记忆检索：长查询走 FTS5，短查询（trigram 零命中）必须走子串兜底——
// 中文短词漏结果是记忆能力最容易踩的坑，由本文件守住。

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db") + "?_pragma=journal_mode(MEMORY)&_pragma=busy_timeout(5000)"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, serr := gdb.DB(); serr == nil {
			_ = sqlDB.Close()
		}
	})
	return NewService(filepath.Join(t.TempDir(), "MEMORY.md"), gdb)
}

// TestSearch 检索三条契约：长查询命中、短查询兜底命中、无关查询不误报。
func TestSearch(t *testing.T) {
	svc := newTestService(t)
	require.NoError(t, svc.Append(SectionPreference, "用户偏好简洁的中文回答"))
	require.NoError(t, svc.Append(SectionProject, "项目使用 Wails 桌面壳"))

	t.Run("长查询命中", func(t *testing.T) {
		hits := svc.Search("中文回答", 5)
		require.NotEmpty(t, hits)
		assert.Contains(t, hits[0].Text, "简洁")
	})

	t.Run("短查询走兜底", func(t *testing.T) {
		// 「简洁」只有 2 个字符，trigram 分词必然零命中——必须回退而不是返回空。
		hits := svc.Search("简洁", 5)
		require.NotEmpty(t, hits, "短查询不能因为分词限制而漏结果")
		assert.Contains(t, hits[0].Text, "简洁")
	})

	t.Run("无关查询不误报", func(t *testing.T) {
		assert.Empty(t, svc.Search("完全不相干的词汇xyz", 5))
	})
}
