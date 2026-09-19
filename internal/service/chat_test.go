package service

// 聊天服务长链路测试：会话操作 / 审批 / 插话队列 / 后台任务 / 工作区绑定 / 消息清洗。
// 场景实现为私有函数（testXxx），由文件末尾 6 个按能力域划分的父测试以 t.Run 聚合。

import (
	"WorkBaby/internal/core"
	"WorkBaby/internal/db"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/registry"
	"WorkBaby/internal/memory"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"
	"context"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newChatOpsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db") + "?_pragma=journal_mode(MEMORY)&_pragma=busy_timeout(5000)"
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Migrate(gdb))
	require.NoError(t, db.CreateFTS5(gdb))
	t.Cleanup(func() {
		sqlDB, err := gdb.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return gdb
}

func newChatOpsService(t *testing.T) (*ChatService, *repo.MessageRepo) {
	t.Helper()
	gdb := newChatOpsTestDB(t)
	sessRepo := repo.NewChatSessionRepo(gdb)
	msgRepo := repo.NewMessageRepo(gdb)
	setRepo := repo.NewSystemSettingRepo(gdb)
	mem := memory.NewService(filepath.Join(t.TempDir(), "MEMORY.md"), gdb)
	svc := NewChatService(sessRepo, msgRepo, repo.NewAiProviderRepo(gdb), setRepo, repo.NewTokenUsageRepo(gdb),
		event.New(), registry.New(), NewToolService(tool.NewRegistry(), setRepo), mem)
	return svc, msgRepo
}

func seedSession(t *testing.T, svc *ChatService, msgRepo *repo.MessageRepo, ctx context.Context) (*domain.ChatSessionRESP, int64) {
	t.Helper()
	ses, err := svc.CreateSession(ctx, &domain.ChatSessionREQ{Name: "ops"})
	require.NoError(t, err)
	for i := 1; i <= 5; i++ {
		require.NoError(t, msgRepo.Insert(ctx, &domain.MessageDO{
			ID: "M_" + itoa(i), SessionID: ses.ID, Seq: int64(i),
			Role: domain.MessageRoleUser, Content: "msg" + itoa(i), Status: domain.MessageStatusCompleted,
		}))
	}
	return ses, 5
}

// ===== 会话消息操作 =====

// TestForkSession 从指定消息分叉：新会话复制前段，原会话不变。
func testForkSession(t *testing.T) {
	svc, msgRepo := newChatOpsService(t)
	ctx := context.Background()
	ses, _ := seedSession(t, svc, msgRepo, ctx)

	forked, err := svc.ForkSession(ctx, ses.ID, "M_3", "分叉会话")
	require.NoError(t, err)
	if forked.ID == ses.ID {
		t.Fatalf("forked session must have new id")
	}
	forkedRows, err := msgRepo.ListBySession(ctx, forked.ID, 0, 100)
	require.NoError(t, err)
	if len(forkedRows) != 3 || forkedRows[2].Content != "msg3" {
		t.Fatalf("want 3 copied messages, got %d last=%v", len(forkedRows), forkedRows[2].Content)
	}
	origRows, err := msgRepo.ListBySession(ctx, ses.ID, 0, 100)
	require.NoError(t, err)
	if len(origRows) != 5 {
		t.Fatalf("original session must keep 5 messages, got %d", len(origRows))
	}
}

// ===== 审批 =====

// TestApprovalScopes 审批两档放行：「允许一次」只放行本次，「本会话允许」才免审；
// 不可逆风险即便选了本会话也每次必问（给一次性放行是授权，给永久放行是隐患）。
func testApprovalScopes(t *testing.T) {
	bus := event.New()
	svc := NewApprovalService(bus)
	ctx := core.WithRunContext(context.Background(), "run-1", "ses-1")

	ids := make(chan string, 4)
	var mu sync.Mutex
	remember := map[string]bool{}
	bus.Subscribe(event.MatchExact("chat:approval"), func(_ string, payload any) {
		m, _ := payload.(map[string]any)
		if id, ok := m["id"].(string); ok {
			ids <- id
		}
		cmd, _ := m["command"].(string)
		v, _ := m["can_remember"].(bool)
		mu.Lock()
		remember[cmd] = v
		mu.Unlock()
	})

	// ask 发起一次审批并回填决策，返回是否放行。
	ask := func(t *testing.T, command, risk string, approve bool, scope string) bool {
		t.Helper()
		res := make(chan bool, 1)
		go func() { res <- svc.Approve(ctx, command, risk) }()
		var id string
		select {
		case id = <-ids:
		case <-time.After(2 * time.Second):
			t.Fatalf("审批事件未发布: %s", command)
		}
		if err := svc.Decide(id, approve, scope); err != nil {
			t.Fatalf("decide failed: %v", err)
		}
		select {
		case ok := <-res:
			return ok
		case <-time.After(time.Second):
			t.Fatalf("Approve 未返回")
			return false
		}
	}

	// 「允许一次」：本次放行，但同命令下次仍要问
	if !ask(t, "go build ./...", tool.RiskApprovalNeeds, true, scopeOnce) {
		t.Fatalf("允许一次应放行")
	}
	blocked := make(chan bool, 1)
	go func() { blocked <- svc.Approve(ctx, "go build ./...", tool.RiskApprovalNeeds) }()
	select {
	case <-blocked:
		t.Fatalf("「允许一次」不得免审后续调用")
	case <-time.After(150 * time.Millisecond):
	}
	// 收尾：把挂起的这次拒掉
	select {
	case id := <-ids:
		_ = svc.Decide(id, false, "")
	case <-time.After(time.Second):
		t.Fatalf("第二次审批事件未发布")
	}
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatalf("拒绝后 Approve 应返回")
	}

	// 「本会话允许」：免审（前端拿到的 can_remember 必须为 true）
	if !ask(t, "git status", tool.RiskApprovalNeeds, true, scopeSession) {
		t.Fatalf("本会话允许应放行")
	}
	mu.Lock()
	canRememberGit := remember["git status"]
	mu.Unlock()
	if !canRememberGit {
		t.Fatalf("needs_approval 应允许本会话免审")
	}
	exempt := make(chan bool, 1)
	go func() { exempt <- svc.Approve(ctx, "git status", tool.RiskApprovalNeeds) }()
	select {
	case ok := <-exempt:
		if !ok {
			t.Fatalf("已批准命令应免审放行")
		}
	case <-time.After(time.Second):
		t.Fatalf("免审路径不应阻塞")
	}

	// 不可逆：即便 scope=session 也不免审，且 can_remember=false
	if !ask(t, "rm -rf /tmp/x", tool.RiskApprovalIrrev, true, scopeSession) {
		t.Fatalf("不可逆操作批准后应放行本次")
	}
	mu.Lock()
	canRememberRm := remember["rm -rf /tmp/x"]
	mu.Unlock()
	if canRememberRm {
		t.Fatalf("不可逆操作不得提供「本会话允许」")
	}
	again := make(chan bool, 1)
	go func() { again <- svc.Approve(ctx, "rm -rf /tmp/x", tool.RiskApprovalIrrev) }()
	select {
	case <-again:
		t.Fatalf("不可逆操作必须每次确认")
	case <-time.After(150 * time.Millisecond):
	}
	select {
	case id := <-ids:
		_ = svc.Decide(id, false, "")
	case <-time.After(time.Second):
		t.Fatalf("不可逆第二次审批事件未发布")
	}
	select {
	case <-again:
	case <-time.After(time.Second):
		t.Fatalf("拒绝后 Approve 应返回")
	}
}

func itoa(n int) string {
	return string(rune('0' + n))
}

// TestQueueSteerPersistsAndQueues 有活动 run 时：消息立即落库（前端可见）+ 进入注入队列。
func testQueueSteerPersistsAndQueues(t *testing.T) {
	svc, msgRepo := newChatOpsService(t)
	ctx := context.Background()
	ses, err := svc.CreateSession(ctx, &domain.ChatSessionREQ{Name: "steer2"})
	require.NoError(t, err)

	// 伪造活动 run（真实 run 由 SendStream 注册；此处只验证注入侧契约）
	svc.runs.set(ses.ID, "RUN_STEER", func() {})

	res, err := svc.QueueSteer(ctx, ses.ID, "改成用 Go 写")
	require.NoError(t, err)
	assert.Equal(t, "RUN_STEER", res.RunID)
	assert.True(t, res.Queued)

	queued := svc.steers.drain(ses.ID)
	require.Len(t, queued, 1)
	assert.Equal(t, "改成用 Go 写", queued[0].Content)

	rows, err := msgRepo.ListBySession(ctx, ses.ID, 0, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1, "注入消息应立即落库")
	assert.Equal(t, domain.MessageRoleUser, rows[0].Role)
	assert.Equal(t, "RUN_STEER", rows[0].RunID)
	assert.Equal(t, domain.MessageStatusCompleted, rows[0].Status)
}

// ===== 工作区绑定 =====

// TestWorkspaceBindLifecycle 工作区绑定闭环：
// 创建即绑定（回归：CreateSession 曾丢弃 workspace_path）→ 解析优先绑定目录 →
// 未绑定回落默认根 → 不存在的目录被拒绝。
func testWorkspaceBindLifecycle(t *testing.T) {
	svc, _ := newChatOpsService(t)
	ctx := context.Background()

	// 创建即绑定：路径规范化（正斜杠→系统分隔符）并落库
	dir := t.TempDir()
	ses, err := svc.CreateSession(ctx, &domain.ChatSessionREQ{Name: "ws", WorkspacePath: filepath.ToSlash(dir)})
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(dir), ses.WorkspacePath, "创建时应绑定并规范化工作目录")

	// WorkspaceRoot：绑定会话返回绑定目录；未绑定会话回落默认根
	assert.Equal(t, filepath.Clean(dir), svc.WorkspaceRoot(ctx, ses.ID, "default-root"))
	other, err := svc.CreateSession(ctx, &domain.ChatSessionREQ{Name: "no-ws"})
	require.NoError(t, err)
	assert.Equal(t, "default-root", svc.WorkspaceRoot(ctx, other.ID, "default-root"))

	// 不存在的目录：创建与更新都要拒绝
	_, err = svc.CreateSession(ctx, &domain.ChatSessionREQ{Name: "bad", WorkspacePath: filepath.ToSlash(filepath.Join(dir, "missing"))})
	require.Error(t, err)
	_, err = svc.UpdateWorkspace(ctx, ses.ID, filepath.ToSlash(filepath.Join(dir, "missing")))
	require.Error(t, err)

	// 解绑：空路径清空绑定，回落默认根
	updated, err := svc.UpdateWorkspace(ctx, ses.ID, "")
	require.NoError(t, err)
	assert.Empty(t, updated.WorkspacePath)
	assert.Equal(t, "default-root", svc.WorkspaceRoot(ctx, ses.ID, "default-root"))
}

// TestCompactSessionArchive 复现「摘要+归档」压缩语义：
//
// <p>早期轮次整体标 archived 剔出 LLM 上下文（正文不删改）；归档边界不得切进
// assistant(tool_calls) 与其 tool 结果之间；归档摘要写会话元数据供 buildSystem 注入。
func testCompactSessionArchive(t *testing.T) {
	svc, msgRepo := newChatOpsService(t)
	ctx := context.Background()
	ses, _ := seedSession(t, svc, msgRepo, ctx)
	// 追加工具段 + 收尾轮：M_6 用户 / M_7 assistant(tool_calls) / M_8 tool / M_9 assistant / M_10 用户
	seed := []domain.MessageDO{
		{ID: "M_6", SessionID: ses.ID, Seq: 6, Role: domain.MessageRoleUser, Content: "再来一步", Status: domain.MessageStatusCompleted},
		{ID: "M_7", SessionID: ses.ID, Seq: 7, Role: domain.MessageRoleAssistant,
			ToolCalls: `[{"id":"CALL_X","type":"function","function":{"name":"exec","arguments":"{}"}}]`,
			Status:    domain.MessageStatusCompleted},
		{ID: "M_8", SessionID: ses.ID, Seq: 8, Role: domain.MessageRoleTool, ToolCallID: "CALL_X", Content: "ok", Status: domain.MessageStatusCompleted},
		{ID: "M_9", SessionID: ses.ID, Seq: 9, Role: domain.MessageRoleAssistant, Content: "搞定", Status: domain.MessageStatusCompleted},
		{ID: "M_10", SessionID: ses.ID, Seq: 10, Role: domain.MessageRoleUser, Content: "收尾", Status: domain.MessageStatusCompleted},
	}
	for i := range seed {
		require.NoError(t, msgRepo.Insert(ctx, &seed[i]))
	}

	res, err := svc.CompactSession(ctx, ses.ID, domain.CompactREQ{KeepRecent: 3})
	require.NoError(t, err)
	require.Zero(t, res.Failed)

	rows, err := msgRepo.ListBySession(ctx, ses.ID, 0, 0)
	require.NoError(t, err)
	require.Len(t, rows, 10, "归档不删消息，UI 仍可回看")

	// 边界回退：cut=7 落在 tool 消息上 → 回退到 M_7（assistant），M_1..M_6 归档
	archived := map[string]bool{}
	for _, r := range rows {
		archived[r.ID] = r.Status == domain.MessageStatusArchived
	}
	for _, id := range []string{"M_1", "M_2", "M_3", "M_4", "M_5", "M_6"} {
		assert.True(t, archived[id], "%s 应已归档", id)
	}
	for _, id := range []string{"M_7", "M_8", "M_9", "M_10"} {
		assert.False(t, archived[id], "%s 应保留在上下文", id)
	}

	// 归档后上下文不含归档行；保留侧 assistant(tool_calls)+tool 配对完整
	out, err := svc.toLLMMessages(rows)
	require.NoError(t, err)
	require.Len(t, out, 4)
	keptRoles := map[llm.RoleType]int{}
	for _, m := range out {
		keptRoles[m.Role]++
		if m.Role == llm.RoleTool {
			require.Equal(t, "CALL_X", m.ToolCallID, "保留侧 tool 必须与保留侧 assistant 配对")
		}
	}
	assert.Equal(t, 1, keptRoles[llm.RoleUser])
	assert.Equal(t, 2, keptRoles[llm.RoleAssistant])

	// 归档摘要落会话元数据（buildSystem 每轮注入 system）
	fresh, err := svc.sessions.GetByID(ctx, ses.ID)
	require.NoError(t, err)
	sum := archiveSummary(fresh)
	require.NotEmpty(t, sum)
	assert.Contains(t, sum, "用户：msg1")
	assert.NotContains(t, sum, "收尾", "保留侧轮次不应出现在归档摘要里")
}

// testToLLMMessagesDropsEmptyAssistant 空 assistant 占位（失败/中断残留）必须剥离，
// 否则部分上游以 400 拒绝整轮且会话被持久化毒化；空 content 的 tool 消息兜底为 "(empty)" 而非剥离。
func testToLLMMessagesDropsEmptyAssistant(t *testing.T) {
	svc, _ := newChatOpsService(t)
	toolCallsJSON := `[{"id":"CALL_1","type":"function","function":{"name":"exec","arguments":"{}"}}]`

	hists := []domain.MessageDO{
		{ID: "M1", Role: domain.MessageRoleUser, Content: "开工", Status: domain.MessageStatusCompleted},
		// 上次 run 失败残留的空 assistant 占位（部分 provider 会因空 content 拒绝整轮）
		{ID: "M2", Role: domain.MessageRoleAssistant, Content: "", Status: domain.MessageStatusFailed},
		{ID: "M3", Role: domain.MessageRoleUser, Content: "继续", Status: domain.MessageStatusCompleted},
		// 空 content 的 tool 消息：兜底 "(empty)" 而非剥掉（剥掉会破坏 tool_call 配对）
		{ID: "M4", Role: domain.MessageRoleAssistant, Content: "", ToolCalls: toolCallsJSON, Status: domain.MessageStatusCompleted},
		{ID: "M5", Role: domain.MessageRoleTool, ToolCallID: "CALL_1", Content: "", Status: domain.MessageStatusCompleted},
	}
	out, err := svc.toLLMMessages(hists)
	require.NoError(t, err)
	require.Len(t, out, 4, "空 assistant 占位应被剥掉，其余保留")

	for _, m := range out {
		if m.Role == llm.RoleAssistant && len(m.ToolCalls) == 0 {
			require.NotEmpty(t, m.Content, "无 tool_calls 的 assistant 不允许空 content（GLM 1214）")
		}
		if m.Role == llm.RoleTool {
			require.Equal(t, "(empty)", m.Content, "空 tool 结果应兜底为 (empty)")
		}
	}
}

func testToLLMMessagesDropsOrphanTools(t *testing.T) {
	svc, _ := newChatOpsService(t)
	toolCallsJSON := `[{"id":"CALL_REAL","type":"function","function":{"name":"exec","arguments":"{}"}}]`

	hists := []domain.MessageDO{
		{ID: "M1", Role: domain.MessageRoleUser, Content: "开工", Status: domain.MessageStatusCompleted},
		{ID: "M2", Role: domain.MessageRoleAssistant, Content: "", ToolCalls: toolCallsJSON, Status: domain.MessageStatusCompleted},
		{ID: "M3", Role: domain.MessageRoleTool, ToolCallID: "CALL_REAL", Content: "OK", Status: domain.MessageStatusCompleted},
		// 孤儿：tool_call_id 在上下文中没有匹配的 assistant tool_call（压缩或续跑产生）
		{ID: "M4", Role: domain.MessageRoleTool, ToolCallID: "CALL_GHOST", Content: "stale", Status: domain.MessageStatusCompleted},
		// 防御：tool_call_id 为空的 tool 消息也应被剥掉
		{ID: "M5", Role: domain.MessageRoleTool, ToolCallID: "", Content: "?", Status: domain.MessageStatusCompleted},
		{ID: "M6", Role: domain.MessageRoleUser, Content: "继续", Status: domain.MessageStatusCompleted},
		{ID: "M7", Role: domain.MessageRoleAssistant, Content: "完成", Status: domain.MessageStatusCompleted},
	}
	out, err := svc.toLLMMessages(hists)
	require.NoError(t, err)
	require.Len(t, out, 5, "应剥掉 2 条孤儿 tool 消息，剩 user/asst/tool/user/asst")

	for _, m := range out {
		if m.Role == llm.RoleTool {
			require.NotEmpty(t, m.ToolCallID, "剥除后剩余的 tool 消息必须带有效 tool_call_id")
		}
	}
	// 找到唯一一条幸存 tool 消息，断言它就是 CALL_REAL 那条
	var toolCount int
	for _, m := range out {
		if m.Role == llm.RoleTool {
			toolCount++
			require.Equal(t, "CALL_REAL", m.ToolCallID)
		}
	}
	require.Equal(t, 1, toolCount)
}

// ===== 聚合入口 =====
//
// 场景实现为上面的私有函数（不被 go test 直接发现），由下列 6 个按能力域划分的
// 父测试以 t.Run 聚合；改某个能力只需跑对应的一个父测试。

// TestChatSessionOps 会话操作：分叉 / 压缩归档。
func TestChatSessionOps(t *testing.T) {
	t.Run("fork", testForkSession)
	t.Run("compact_archive", testCompactSessionArchive)
}

// TestChatApproval 危险命令审批：两档放行（一次性 / 本会话）+ 不可逆永不免审。
func TestChatApproval(t *testing.T) {
	t.Run("scopes", testApprovalScopes)
}

// TestChatSteerQueue 中途插话：steer 持久化并入队。
func TestChatSteerQueue(t *testing.T) {
	t.Run("persists_and_queues", testQueueSteerPersistsAndQueues)
}

// TestChatWorkspaceBind 工作区绑定生命周期。
func TestChatWorkspaceBind(t *testing.T) {
	t.Run("lifecycle", testWorkspaceBindLifecycle)
}

// TestChatLLMMessageHygiene 回发上游前的消息清洗（协议硬约束：
// 空 assistant 与孤儿 tool 消息都会让上游 400 拒绝整轮）。
func TestChatLLMMessageHygiene(t *testing.T) {
	t.Run("drops_empty_assistant", testToLLMMessagesDropsEmptyAssistant)
	t.Run("drops_orphan_tools", testToLLMMessagesDropsOrphanTools)
}
