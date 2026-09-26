// 集成链路测试：MCP 配置加密与回滚、文件变更快照与回滚、反幻觉核验。

package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"WorkBaby/internal/db"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/mcp"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"
)

func existingCommand(t *testing.T) string {
	t.Helper()
	return os.Args[0]
}

func newMcpTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db") + "?_pragma=journal_mode(MEMORY)&_pragma=busy_timeout(5000)"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	require.NoError(t, db.Migrate(gdb))
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return gdb
}

func newTestMcpService(t *testing.T) (*McpService, *repo.McpServerRepo, *pkg.Cipher) {
	t.Helper()
	cipher, _, err := pkg.NewCipherRandom()
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	r := repo.NewMcpServerRepo(newMcpTestDB(t))
	svc := NewMcpService(r, mcp.NewManager(tool.NewRegistry()), cipher)
	t.Cleanup(svc.Close)
	return svc, r, cipher
}

// TestMcpServiceEnvIsEncryptedAndMasked env 加密落库 + 列表掩码 + 掩码回写不覆盖原值。
func TestMcpServiceEnvIsEncryptedAndMasked(t *testing.T) {
	svc, r, cipher := newTestMcpService(t)
	ctx := context.Background()
	disabled := false

	req := domain.McpServerREQ{
		Name: "fs", Command: existingCommand(t),
		Args:    []string{"-test.run=^$"},
		Env:     map[string]string{"TOKEN": "super-secret"},
		Enabled: &disabled,
	}
	if _, err := svc.Add(ctx, &req); err != nil {
		t.Fatalf("add: %v", err)
	}

	// 落库必须是密文
	row, err := r.GetByName(ctx, "fs")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Env == "" || row.Env == `{"TOKEN":"super-secret"}` {
		t.Fatalf("env should be stored encrypted, got %q", row.Env)
	}

	// 出参必须掩码
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := list[0].Env["TOKEN"]; got != maskedEnvValue {
		t.Fatalf("env in resp = %q, want %q", got, maskedEnvValue)
	}

	// 回传掩码不应覆盖真实值
	again := req
	again.Env = map[string]string{"TOKEN": maskedEnvValue}
	if _, err := svc.Add(ctx, &again); err != nil {
		t.Fatalf("add again: %v", err)
	}
	row, _ = r.GetByName(ctx, "fs")
	var enc map[string]string
	require.NoError(t, json.Unmarshal([]byte(row.Env), &enc))
	plain, err := cipher.Decrypt(enc["TOKEN"])
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plain != "super-secret" {
		t.Fatalf("plain = %q, want super-secret（掩码回传不应覆盖真实值）", plain)
	}
}

// TestMcpServiceSaveRawWritesConfigAndRollsBack 非法 server 名 → 文件已写但 DB 失败 → 备份回退。
func TestMcpServiceSaveRawWritesConfigAndRollsBack(t *testing.T) {
	svc, _, _ := newTestMcpService(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mcp.json")
	svc.WithConfigPath(path)

	cmd := existingCommand(t)
	raw, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{"fs": map[string]any{"command": cmd, "args": []string{"-test.run=^$"}}},
	})
	require.NoError(t, err)
	_, err = svc.SaveRaw(ctx, string(raw))
	require.NoError(t, err)
	assert.FileExists(t, path)

	_, err = svc.SaveRaw(ctx, `{"mcpServers":{"bad name!":{"command":"cmd"}}}`)
	require.Error(t, err)
	assert.FileExists(t, path+".bak")
	after, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Contains(t, string(after), "fs")
}

func newFileChangeTestEnv(t *testing.T) (*FileChangeService, string, *repo.FileChangeRepo) {
	t.Helper()
	dsn := "file:fc_" + t.Name() + "?mode=memory&cache=shared"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&domain.FileChangeDO{}))
	t.Cleanup(func() {
		sqlDB, e := gdb.DB()
		if e == nil {
			_ = sqlDB.Close()
		}
	})
	r := repo.NewFileChangeRepo(gdb)
	root := t.TempDir()
	snap := t.TempDir()
	return NewFileChangeService(r, event.New(), snap, root), root, r
}

// writeWorkFile 写一个文件到工作区并返回绝对路径（绕过安全路径校验，单测用）。
func writeWorkFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	full := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	return full
}

// TestFileChangeRecordSnapshotAndDiff 写文件变更落库：行级 diff 包含 +/-，快照可还原。
func TestFileChangeRecordSnapshotAndDiff(t *testing.T) {
	svc, root, r := newFileChangeTestEnv(t)
	ctx := context.Background()
	rel := "src/main.go"
	writeWorkFile(t, root, rel, "a\nb\nc\n")

	_, err := svc.Record(ctx, ChangeInput{
		SessionID: "SES_1", RunID: "RUN_1", ToolName: "file_write",
		Path:    filepath.Join(root, rel),
		Existed: true, Before: []byte("a\nb\nc\n"),
		After: []byte("a\nB\nc\nd\n"),
	})
	require.NoError(t, err)

	row, err := r.GetByID(ctx, mustGetFirstID(t, r))
	require.NoError(t, err)
	assert.Equal(t, "modify", string(row.Action))
	assert.Equal(t, int64(6), row.BytesBefore)
	// bytes_after 取决于末尾是否带换行；LCS 行模型不感知「修改」——b→B 拆成 -1 +1
	assert.Greater(t, row.AddedLines, 0, "至少 1 行新增")
	assert.Greater(t, row.RemovedLines, 0, "至少 1 行删除")
	assert.NotEmpty(t, row.SnapshotPath, "modify 必须有快照")
	assert.Contains(t, row.Diff, "+d", "diff 含新增行")
	assert.Contains(t, row.Diff, "-b", "diff 含被替换的旧行")
}

// TestFileChangeRollbackModify 写文件 → 回滚到原内容；快照写回覆盖当前文件。
func TestFileChangeRollbackModify(t *testing.T) {
	svc, root, _ := newFileChangeTestEnv(t)
	ctx := context.Background()
	rel := "note.txt"
	before := "line1\nline2\nline3\n"
	after := "line1\nCHANGED\nline3\n"
	full := writeWorkFile(t, root, rel, before)

	row, err := svc.Record(ctx, ChangeInput{
		SessionID: "SES_2", RunID: "RUN_2", ToolName: "file_write",
		Path: full, Existed: true, Before: []byte(before), After: []byte(after),
	})
	require.NoError(t, err)

	require.NoError(t, svc.Rollback(ctx, row.ID))

	got, err := os.ReadFile(full)
	require.NoError(t, err)
	assert.Equal(t, before, string(got), "回滚后文件内容应等于变更前")
}

// mustGetFirstID 列出当前唯一会话的全部变更返回第一条 ID。
func mustGetFirstID(t *testing.T, r *repo.FileChangeRepo) string {
	t.Helper()
	ctx := context.Background()
	for _, sid := range []string{"SES_1", "SES_2", "SES_3", "SES_L"} {
		rows, err := r.ListBySession(ctx, sid, 100)
		require.NoError(t, err)
		if len(rows) > 0 {
			return rows[0].ID
		}
	}
	t.Fatal("no file change rows found in any test session")
	return ""
}

// TestUnbackedClaimGuard 守卫反幻觉核验：声称已产出文件的声明必须可识别，
// 而疑问句、无完成标记的普通提及不得误判。
func TestUnbackedClaimGuard(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"PPT已经保存为 AI工具介绍.pptx，放置在工作区根目录下。", true},
		{"已成功创建 output/report.docx", true},
		{"你说的 report.pptx 已经创建好了吗？", false},
		{"请把结果保存为 result.md，我稍后再执行。", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, claimsArtifact(tc.content), "content=%q", tc.content)
	}

	// 证据判据：声明里抽到的具体文件路径必须由本 run 的 file_changes 兜底；
	// 拒绝（refused）不计——工具被拦下来等于没真执行。
	evCases := []struct {
		name      string
		content   string
		paths     map[string]struct{} // 声明里声明产出的文件名（小写归一）
		toolCalls []string
		changes   []changeEvidence
		want      bool
	}{
		{"no_claim_no_need", "", nil, nil, nil, true},
		{"zero_tools", "已生成 report.pdf", map[string]struct{}{"report.pdf": {}}, nil, nil, false},
		{"only_read_tools", "已生成 report.pdf", map[string]struct{}{"report.pdf": {}}, []string{"exec", "file_read"}, nil, false},
		{"claim_path_matches_change", "已生成 report.pdf", map[string]struct{}{"report.pdf": {}}, []string{"file_write"}, []changeEvidence{{Path: "report.pdf"}}, true},
		{"claim_path_unmatched", "已生成 report.pdf", map[string]struct{}{"report.pdf": {}}, []string{"file_write"}, []changeEvidence{{Path: "notes.md"}}, true},
		{"refused_doesnt_count", "已生成 report.pdf", map[string]struct{}{"report.pdf": {}}, []string{"file_write"}, []changeEvidence{{Path: "report.pdf", Refused: true}}, false},
		{"write_tool_generic_change_accepted", "已生成 report.pdf", map[string]struct{}{"report.pdf": {}}, []string{"file_write"}, []changeEvidence{{Path: "any/where.md"}}, true},
		{"edit_tool_with_matching_change", "已生成 config.json", map[string]struct{}{"config.json": {}}, []string{"file_edit"}, []changeEvidence{{Path: "config.json"}}, true},
	}
	for _, tc := range evCases {
		t.Run(tc.name, func(t *testing.T) {
			calls := func(names []string) []llm.ToolCall {
				out := make([]llm.ToolCall, 0, len(names))
				for _, n := range names {
					out = append(out, llm.ToolCall{Function: llm.FunctionCall{Name: n}})
				}
				return out
			}
			assert.Equal(t, tc.want, evidenceForClaim(tc.paths, calls(tc.toolCalls), tc.changes))
		})
	}
}

// 注：审批挂起可列出 + 按 id 回填决策的覆盖，由 recovery_test.go 的
// TestDurableApprovalPauseResume 承担（跨重启闭环是其超集），此处不重复。
