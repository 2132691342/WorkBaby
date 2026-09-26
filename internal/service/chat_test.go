package service

// 聊天服务长链路测试：会话操作（压缩归档）/ 审批两档放行 / 中途插话 /
// 后台任务执行链路与无人值守审批。
// 场景实现为私有函数（testXxx），由文件末尾的父测试以 t.Run 聚合。

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/db"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/event"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/llm/registry"
	"WorkBaby/internal/memory"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
	"WorkBaby/internal/tool"
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
	svc := NewChatService(ChatDeps{
		Sessions: sessRepo, Messages: msgRepo, Providers: repo.NewAiProviderRepo(gdb),
		Settings: setRepo, Usages: repo.NewTokenUsageRepo(gdb),
		Bus: event.New(), Registry: registry.New(), Tools: NewToolService(tool.NewRegistry(), setRepo),
		Memory: mem, Session: NewSessionContext(t.TempDir(), sessRepo),
	})
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

// ===== 审批 =====

// TestApprovalScopes 审批两档放行：「允许一次」只放行本次，「本会话允许」才免审；
// 不可逆风险即便选了本会话也每次必问（给一次性放行是授权，给永久放行是隐患）。
func testApprovalScopes(t *testing.T) {
	bus := event.New()
	svc := NewApprovalService(bus)
	ctx := agent.WithRunContext(context.Background(), "run-1", "ses-1")

	ids := make(chan string, 4)
	var mu sync.Mutex
	remember := map[string]bool{}
	bus.Subscribe(event.MatchExact("chat:approval"), func(_ string, payload any) {
		// chat:approval 由强类型 domain.ChatApprovalEvent 发出，但 Emitter 归一为
		// map（JSON 往返）后广播——订阅端统一按 map 消费，字段与结构体 json tag 一致。
		m, _ := payload.(map[string]any)
		if id, ok := m["id"].(string); ok && id != "" {
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

// TestCompactSessionArchive 复现「摘要+归档」压缩语义：早期轮次整体标 archived 剔出上下文
// （正文不删改）；归档边界不得切进 assistant(tool_calls) 与其 tool 结果之间；
// 归档摘要写会话元数据供 buildSystem 注入。
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

// ===== 聚合入口 =====
// 场景实现为上面的私有函数（不被 go test 直接发现），由下列父测试以 t.Run 聚合；
// 改某个能力只需跑对应的一个父测试。

// TestChatSessionOps 会话操作：分叉 / 压缩归档。
func TestChatSessionOps(t *testing.T) {
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

// waitForTaskState 轮询等待任务进入目标状态（worker 是异步 goroutine，不能同步断言）。
func waitForTaskState(t *testing.T, svc *ChatTaskService, id string, want string) domain.ChatTaskDO {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		row, err := svc.List(context.Background(), 50)
		if err == nil {
			for _, it := range row.Items {
				if it.ID == id && it.State == want {
					return it
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s 未在限时内进入状态 %s", id, want)
	return domain.ChatTaskDO{}
}

// TestChatTaskLifecycle 覆盖任务域的提交校验、执行链路与取消语义。
func TestChatTaskLifecycle(t *testing.T) {
	gdb := newChatOpsTestDB(t)
	bus := event.New()
	sessRepo := repo.NewChatSessionRepo(gdb)
	taskRepo := repo.NewChatTaskRepo(gdb)
	chat := NewChatService(ChatDeps{Bus: bus})
	approval := NewApprovalService(bus)
	svc := NewChatTaskService(taskRepo, bus, chat, approval, sessRepo)

	t.Run("不存在的宿主会话 → 快速失败", func(t *testing.T) {
		row, err := svc.Submit(context.Background(), "SES_missing", "default", "整理文件")
		require.NoError(t, err)
		require.Equal(t, domain.TaskStatePending, row.State, "提交即返回 pending")
		done := waitForTaskState(t, svc, row.ID, domain.TaskStateFailed)
		assert.Contains(t, done.Error, "宿主会话不可用")
	})

	t.Run("启动恢复：遗留未终态任务标记失败", func(t *testing.T) {
		legacy := &domain.ChatTaskDO{
			ID: pkg.NewID("TASK"), SessionID: "SES_x", Agent: "default", Prompt: "p", State: domain.TaskStateRunning,
		}
		require.NoError(t, taskRepo.Create(context.Background(), legacy))
		// 再构造一个服务实例：新实例的 recoverUnfinished 应把 legacy 打成 failed
		NewChatTaskService(taskRepo, bus, chat, approval, sessRepo)
		got, err := taskRepo.GetByID(context.Background(), legacy.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskStateFailed, got.State)
		assert.Contains(t, got.Error, "应用重启")
	})
}

// TestGrantApprover 无人值守审批门：只吃免审授权，不可逆一律拒绝。
func TestGrantApprover(t *testing.T) {
	bus := event.New()
	svc := NewApprovalService(bus)
	g := grantApprover{svc: svc}
	ctx := context.Background()
	call := agent.Call{ID: "1", Name: "exec", Args: []byte(`{"command":"ls"}`)}

	t.Run("未授权拒绝且带可解释原因", func(t *testing.T) {
		assert.False(t, g.Approve(ctx, call, tool.RiskApprovalNeeds))
		assert.Contains(t, g.DenyReason(), "免审授权")
	})

	t.Run("已授权放行", func(t *testing.T) {
		svc.rememberGrant(ctx, "", approvalCommand(call), tool.RiskApprovalNeeds)
		assert.True(t, g.Approve(ctx, call, tool.RiskApprovalNeeds))
	})

	t.Run("不可逆操作永不免审", func(t *testing.T) {
		assert.False(t, g.Approve(ctx, call, tool.RiskApprovalIrrev))
	})

}
