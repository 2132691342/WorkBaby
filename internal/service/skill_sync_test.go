package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/skill"
)

// TestSkillServiceTwoLevelDirs 覆盖技能目录两级叠加：全局全会话可用、工作区同名覆盖、
// 切换工作区不串味并恢复全局版本、用户启停状态跨重扫保留、目录删除后摘除陈旧条目。
func TestSkillServiceTwoLevelDirs(t *testing.T) {
	ctx := context.Background()
	gdb := newChatOpsTestDB(t)
	svc := NewSkillService(repo.NewSkillRepo(gdb), skill.NewRegistry())

	globalRoot := filepath.Join(t.TempDir(), "skills")
	writeSkillDir(t, globalRoot, "shared", "全局版本")
	writeSkillDir(t, globalRoot, "global-only", "只有全局")
	svc.WithGlobalDir(globalRoot)
	require.NoError(t, svc.SyncGlobal(ctx))

	got, ok := svc.Get("shared")
	require.True(t, ok, "全局技能应被发现并装载")
	require.Contains(t, got.Body, "全局版本")

	// 工作区 1：同名覆盖 + 工作区独有技能
	ws1 := t.TempDir()
	writeSkillDir(t, filepath.Join(ws1, ".workbaby", "skills"), "shared", "工作区版本")
	writeSkillDir(t, filepath.Join(ws1, ".workbaby", "skills"), "ws1-only", "只有工作区1")
	require.NoError(t, svc.SyncWorkspace(ctx, ws1))

	got, ok = svc.Get("shared")
	require.True(t, ok)
	require.Contains(t, got.Body, "工作区版本", "工作区同名技能必须覆盖全局")
	_, ok = svc.Get("ws1-only")
	require.True(t, ok, "工作区独有技能应可用")

	// 切到工作区 2：ws1 的技能消失，被覆盖掉的全局同名技能恢复
	ws2 := t.TempDir()
	require.NoError(t, svc.SyncWorkspace(ctx, ws2))
	_, ok = svc.Get("ws1-only")
	require.False(t, ok, "切换工作区后上一工作区的技能不得残留")
	got, ok = svc.Get("shared")
	require.True(t, ok)
	require.Contains(t, got.Body, "全局版本", "工作区覆盖撤销后应恢复全局版本")

	// 用户禁用状态跨同步保留（覆盖导入不得把开关重置回 true）
	require.NoError(t, svc.SetEnabled(ctx, "shared", false))
	require.NoError(t, svc.SyncWorkspace(ctx, t.TempDir()))
	rows, err := repo.NewSkillRepo(gdb).List(ctx)
	require.NoError(t, err)
	var enabled bool
	var found bool
	for i := range rows {
		if rows[i].Name == "shared" {
			enabled, found = rows[i].Enabled, true
		}
	}
	require.True(t, found)
	require.False(t, enabled, "用户关闭的技能在目录重扫后必须保持关闭")

	// 目录删除 → 陈旧条目摘除（不能留下「删了却还在列表里」的幽灵技能）
	require.NoError(t, os.RemoveAll(filepath.Join(globalRoot, "global-only")))
	require.NoError(t, svc.SyncGlobal(ctx))
	_, ok = svc.Get("global-only")
	require.False(t, ok, "磁盘已删除的技能应从注册表摘除")
}

// TestSkillDirDiscoveryCompat 守卫目录约定解析：触发词标量与列表写法都支持，
// 损坏技能跳过而非污染整批；陈旧条目判定不影响内置与 UI 自建技能。
func TestSkillDirDiscoveryCompat(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	writeSkillDir(t, root, "scalar", "标量触发词技能")
	writeSkillDir(t, root, "list", "列表触发词技能")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "broken"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "broken", "SKILL.md"),
		[]byte("没有 frontmatter 的正文"), 0o644))

	rows := skill.DiscoverDir(root, domain.SkillSourceKindCustom)
	require.Len(t, rows, 2, "损坏技能必须跳过，其余正常入库")
	names := []string{rows[0].Name, rows[1].Name}
	require.ElementsMatch(t, []string{"scalar", "list"}, names)

	// 陈旧判定：磁盘消失的才摘除，内置（ref=assets/…）与 UI 自建（ref 为空）不动
	require.NoError(t, os.RemoveAll(filepath.Join(root, "list")))
	stale := skill.StaleSkillNames([]domain.SkillDO{
		{Name: "list", SourceRef: filepath.Join(root, "list")},
		{Name: "scalar", SourceRef: filepath.Join(root, "scalar")},
		{Name: "builtin", SourceRef: "assets/skills/builtin"},
		{Name: "ui", SourceRef: ""},
	}, []string{root})
	require.Equal(t, []string{"list"}, stale)
}

// writeSkillDir 写一个符合目录约定的技能（when_to_use 用标量写法，
// 顺带覆盖生态里常见的非列表写法兼容性）。
func writeSkillDir(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	md := "---\nname: " + name + "\ndescription: test skill\nwhen_to_use: " + name + "\n---\n" + body
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644))
}
