package memory

// 记忆层的硬约束：MEMORY.md 是唯一真相源、分节归一化、重复条目幂等、条目上限自收敛、
// 检索在短查询（trigram 零命中）下仍要给出结果。

import (
	"path/filepath"
	"strconv"
	"strings"
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

// TestAppendAndRead 写入与读回的往返。
func TestAppendAndRead(t *testing.T) {
	svc := newTestService(t)

	require.NoError(t, svc.Append(SectionPreference, "回答保持简洁"))
	require.NoError(t, svc.Append(SectionFact, "用户在做 Go 项目"))

	entries := svc.Entries()
	require.Len(t, entries, 2)
	assert.Equal(t, Entry{Section: SectionPreference, Text: "回答保持简洁"}, entries[0], "分节顺序固定：偏好在前")

	raw := svc.Read()
	assert.Contains(t, raw, "## "+SectionPreference)
	assert.Contains(t, raw, "- 回答保持简洁")
}

// TestSectionNormalize 未知分节归入「事实」，避免模型自由发挥把结构写乱。
func TestSectionNormalize(t *testing.T) {
	svc := newTestService(t)

	require.NoError(t, svc.Append("随便写的分节", "内容"))
	require.NoError(t, svc.Append("preference", "别名归一"))

	// 读回顺序按固定分节序（偏好 → 事实 → 流程 → 项目），与写入顺序无关。
	entries := svc.Entries()
	require.Len(t, entries, 2)
	assert.Equal(t, SectionPreference, entries[0].Section, "英文别名应归一到固定分节")
	assert.Equal(t, SectionFact, entries[1].Section)
}

// TestAppendIdempotent 同一条重复写入不制造噪声。
func TestAppendIdempotent(t *testing.T) {
	svc := newTestService(t)

	require.NoError(t, svc.Append(SectionFact, "同一条"))
	require.NoError(t, svc.Append(SectionFact, "同一条"))
	assert.Len(t, svc.Entries(), 1)

	// 空白内容直接忽略，不写空条目。
	require.NoError(t, svc.Append(SectionFact, "   "))
	assert.Len(t, svc.Entries(), 1)
}

// TestRemoveAndReplace 删除按精确匹配；覆盖写入以文件为准。
func TestRemoveAndReplace(t *testing.T) {
	svc := newTestService(t)
	require.NoError(t, svc.Append(SectionFact, "甲"))
	require.NoError(t, svc.Append(SectionFact, "乙"))

	require.NoError(t, svc.Remove(SectionFact, "甲"))
	assert.Len(t, svc.Entries(), 1)

	require.NoError(t, svc.Replace("## 流程\n- 手工编辑的流程\n"))
	entries := svc.Entries()
	require.Len(t, entries, 1)
	assert.Equal(t, SectionProcedure, entries[0].Section)
	assert.Equal(t, "手工编辑的流程", entries[0].Text)
}

// TestEntryLimitDropsOldest 写满后丢最旧一条：长期记忆必须能自我收敛，否则注入段会无限膨胀。
func TestEntryLimitDropsOldest(t *testing.T) {
	svc := newTestService(t)

	for i := 0; i < maxEntries+5; i++ {
		require.NoError(t, svc.Append(SectionFact, "条目"+strings.Repeat("x", i%3)+string(rune('A'+i%26))+strconv.Itoa(i)))
	}

	entries := svc.Entries()
	assert.Len(t, entries, maxEntries)
	assert.NotContains(t, entries[0].Text, "0", "最旧的条目应被丢弃")
}

// TestSearch 检索：长查询走 FTS5，短查询（trigram 零命中）走子串兜底。
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
