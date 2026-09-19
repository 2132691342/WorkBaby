package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/repo"
)

// TestTrustLifecycle 目录信任全生命周期：未登记 ask（fail-closed）→ 登记 allow 且后代继承
// → 显式 deny → 撤销回 ask → Ensure 询问批准后落盘、再次访问不再询问。
func TestTrustLifecycle(t *testing.T) {
	ctx := context.Background()
	roots := []string{filepath.Join(t.TempDir(), "trusted")}
	svc := NewTrustService(repo.NewWorkspaceTrustRepo(newChatOpsTestDB(t)), roots...)

	unknown := filepath.Join(t.TempDir(), "unknown-project")
	res, err := svc.Resolve(ctx, unknown)
	require.NoError(t, err)
	assert.Equal(t, domain.TrustStateAsk, res.State, "未登记目录必须 ask")
	assert.False(t, res.Matched)

	parent := filepath.Join(t.TempDir(), "proj")
	child := filepath.Join(parent, "sub", "deep")
	_, err = svc.Decide(ctx, domain.WorkspaceTrustREQ{Path: parent, State: string(domain.TrustStateAllow)})
	require.NoError(t, err)
	res, err = svc.Resolve(ctx, child)
	require.NoError(t, err)
	assert.Equal(t, domain.TrustStateAllow, res.State, "后代继承祖先 allow")
	assert.Equal(t, parent, res.Source)

	denied := filepath.Join(t.TempDir(), "p")
	_, err = svc.Decide(ctx, domain.WorkspaceTrustREQ{Path: denied, State: string(domain.TrustStateDeny)})
	require.NoError(t, err)
	res, err = svc.Resolve(ctx, denied)
	require.NoError(t, err)
	assert.Equal(t, domain.TrustStateDeny, res.State)

	require.NoError(t, svc.Revoke(ctx, parent))
	res, err = svc.Resolve(ctx, child)
	require.NoError(t, err)
	assert.Equal(t, domain.TrustStateAsk, res.State, "撤销后回到 ask")

	project := filepath.Join(t.TempDir(), "myproj")
	approveCount := 0
	svc.WithApprover(func(context.Context, string) bool { approveCount++; return true })
	ok, reason := svc.Ensure(ctx, project)
	require.True(t, ok, reason)
	assert.Equal(t, 1, approveCount)
	ok, reason = svc.Ensure(ctx, project)
	require.True(t, ok, reason)
	assert.Equal(t, 1, approveCount, "已落盘 allow 不应再次询问")
}

// TestWorkspaceFileSafety 会话工作区文件访问的两条安全边界：
// 读路径拦截 ../ 越界；列表隐藏 local/、.workbaby/、.index/ 等 runtime 数据目录。
func TestWorkspaceFileSafety(t *testing.T) {
	root := t.TempDir()
	const sid = "SESSION_TEST"
	write := func(rel, content string) {
		full := filepath.Join(root, sid, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	write("inside.txt", "ok")
	write("sub/secret.txt", "secret")
	write("sub/normal.txt", "visible")
	write("local/draft.md", "hide")
	write(".workbaby/meta.json", "hide")
	write(".index/cache.bin", "hide")
	svc := NewWorkspaceService(root, nil)

	t.Run("path_traversal_blocked", func(t *testing.T) {
		for _, p := range []string{"../../etc/passwd", "sub/../../etc/hosts", `..\..\windows\system.ini`} {
			_, _, err := svc.ReadFile(context.Background(), sid, p)
			require.Error(t, err, "路径 %q 应被拦截", p)
			require.Contains(t, err.Error(), "escapes")
		}
	})

	t.Run("internal_dirs_hidden", func(t *testing.T) {
		files, err := svc.ListFiles(context.Background(), sid)
		require.NoError(t, err)
		paths := map[string]bool{}
		for _, f := range files {
			paths[f.Path] = true
		}
		require.True(t, paths["inside.txt"])
		require.True(t, paths["sub/normal.txt"])
		require.False(t, paths["local/draft.md"])
		require.False(t, paths[".workbaby/meta.json"])
		require.False(t, paths[".index/cache.bin"])
	})
}
