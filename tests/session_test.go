// 会话管线：JSONL 会话文件是唯一持久真相，界面重启后由它重建。
// 覆盖回放健壮性、壳的写入纪律、引擎落盘与回放层的格式契约、分叉四件事。
package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/parser"
)

// TestSessionReplay 断言回放层的健壮性与组装规则。
func TestSessionReplay(t *testing.T) {
	// ---- 分支：只回放当前活动分支 ----
	branch := writeSession(t, t.TempDir(),
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"原始问题"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"原始回答"}]}}`,
		`{"type":"message","id":"u2","parentId":"u1","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"user","content":"改过的问题"}}`,
		`{"type":"message","id":"a2","parentId":"u2","timestamp":"2026-01-01T00:00:04.000Z","message":{"role":"assistant","content":[{"type":"text","text":"改过的回答"}]}}`,
	)
	joined := turnsText(parser.BuildHistory(branch, nil))
	if !strings.Contains(joined, "改过的问题") || !strings.Contains(joined, "改过的回答") {
		t.Fatalf("应回放当前分支，实际 %q", joined)
	}
	if strings.Contains(joined, "原始回答") {
		t.Fatalf("不应回放被放弃的分支，实际 %q", joined)
	}

	// ---- 健壮：残行不毁整份历史 ----
	corrupt := writeSession(t, t.TempDir(),
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"还行"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timest`,
		`{"type":"message","id":"a2","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"后续答案"}]}}`,
	)
	if !strings.Contains(turnsText(parser.BuildHistory(corrupt, nil)), "后续答案") {
		t.Fatal("一条残行不该让整份历史读不出来：残行之后的有效条目仍应解析")
	}

	// ---- 健壮：环状父指针必须有界退出 ----
	cyclic := writeSession(t, t.TempDir(),
		`{"type":"message","id":"a","parentId":"b","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"环 A"}}`,
		`{"type":"message","id":"b","parentId":"a","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"user","content":"环 B"}}`,
	)
	done := make(chan struct{})
	go func() { parser.BuildHistory(cyclic, nil); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("环状父指针让回放卡死：活动分支的遍历必须用已访问集合兜住")
	}

	// ---- 组装：工具结果回填宿主轮 ----
	normal := writeSession(t, t.TempDir(),
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"看下文件"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call_1","name":"read_file","arguments":{}}]}}`,
		`{"type":"message","id":"t1","parentId":"a1","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"toolResult","toolCallId":"call_1","toolName":"read_file","content":[{"type":"text","text":"文件内容在此"}]}}`,
	)
	var found bool
	for _, turn := range parser.BuildHistory(normal, nil) {
		if turn.TurnRole == "tool" {
			t.Fatalf("工具结果应挂回助手轮，不该单独成轮")
		}
		for _, block := range turn.TurnBlocks {
			if block.BlockKind == domain.BlockTool && block.ToolCallId == "call_1" {
				found = true
				if block.ToolOutput == "" {
					t.Error("工具块的 output 应已回填，实际为空")
				}
			}
		}
	}
	if !found {
		t.Fatal("应有一个带 call_1 的工具块")
	}

	// ---- 组装：孤立结果单独成 tool 轮 ----
	orphan := writeSession(t, t.TempDir(),
		`{"type":"message","id":"t1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"toolResult","toolCallId":"orphan","toolName":"bash","content":[{"type":"text","text":"孤立结果"}]}}`,
	)
	oturns := parser.BuildHistory(orphan, nil)
	if len(oturns) != 1 || oturns[0].TurnRole != "tool" {
		t.Fatalf("孤立工具结果应单独成 1 个 tool 轮，实际 %d 轮", len(oturns))
	}
	if !strings.Contains(oturns[0].TurnBlocks[0].ToolOutput, "孤立结果") {
		t.Fatal("孤立工具结果的原文不该丢")
	}

	// ---- 组装：压缩标记 ----
	compaction := writeSession(t, t.TempDir(),
		`{"type":"compaction","id":"c1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","tokensBefore":12345,"summary":"之前的摘要"}`,
	)
	marks := parser.BuildHistory(compaction, nil)
	if len(marks) != 1 || marks[0].TurnRole != "engine-mark" {
		t.Fatalf("压缩应是 1 个 engine-mark 轮，实际 %d 轮", len(marks))
	}
	if !strings.Contains(marks[0].TurnBlocks[0].BlockText, "12345") {
		t.Fatalf("压缩标记应带上压缩前 token 数，实际 %q", marks[0].TurnBlocks[0].BlockText)
	}

	// ---- 组装：标题来源 ----
	renamed := writeSession(t, t.TempDir(),
		`{"type":"session_info","id":"s1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","name":"我自己起的名字"}`,
		`{"type":"message","id":"u1","parentId":"s1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"user","content":"内容"}}`,
	)
	if summary := parser.Summarize(renamed); summary.SessionTitle != "我自己起的名字" {
		t.Fatalf("会话名应取自 session_info，实际 %q", summary.SessionTitle)
	}

	// ---- 计划模式：只认活动分支上最后一条 plan-mode 条目 ----
	plan := writeSession(t, t.TempDir(),
		`{"type":"custom","customType":"plan-mode","id":"p1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","data":{"active":true}}`,
		`{"type":"custom","customType":"plan-mode-bash-override","id":"o1","parentId":"p1","timestamp":"2026-01-01T00:00:02.000Z","data":{"command":"rm -rf x"}}`,
	)
	if !parser.PlanModeFromFile(plan) {
		t.Fatal("最后一条 plan-mode 说开着，回放就该是开着")
	}
	if parser.PlanModeFromFile(writeSession(t, t.TempDir(),
		`{"type":"custom","customType":"plan-mode","id":"p1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","data":{"active":true}}`,
		`{"type":"custom","customType":"plan-mode","id":"p2","parentId":"p1","timestamp":"2026-01-01T00:00:02.000Z","data":{"active":false}}`,
	)) {
		t.Fatal("最后一次开关是关，回放就该是关：取到的是第一条而不是最后一条")
	}
	if parser.PlanModeFromFile("") {
		t.Fatal("没有会话文件时计划模式应为关")
	}
}

// TestSessionWriteDiscipline 断言壳对会话文件的写入纪律：
// 隐藏只记台账不动文件、改名只追加、引擎正打开时拒绝改名。
func TestSessionWriteDiscipline(t *testing.T) {
	h := newHarness(t)
	file := writeSession(t, h.layout.SessionsDir,
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"第一句"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"第一答"}]}}`,
		`{"type":"message","id":"u2","parentId":"a1","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"user","content":"第二句"}}`,
	)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读会话文件失败：%v", err)
	}

	// ---- 隐藏是壳的台账，文件一个字节都不动 ----
	hidden := map[string]struct{}{"u2": {}}
	turns := parser.BuildHistory(file, hidden)
	for _, turn := range turns {
		if turn.TurnId == "u2" {
			t.Fatal("被隐藏的轮不该出现在回放结果里")
		}
	}
	if len(turns) == 0 {
		t.Fatal("隐藏一轮不该把整份历史清空")
	}
	afterHide, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("重读会话文件失败：%v", err)
	}
	if string(before) != string(afterHide) {
		t.Fatal("隐藏是壳的台账，绝不能改写会话文件")
	}

	// ---- 引擎正打开时拒绝改名，且不动文件 ----
	h.app.Sessions.Active = func() string { return file }
	env, _ := h.call(http.MethodPost, "/sessions/rename", map[string]string{
		"session_path": file, "session_title": "新名字",
	})
	if env.Code != pkg.CodeEngineBusy {
		t.Fatalf("引擎正打开时改名应回 %q，实际 %q（%s）", pkg.CodeEngineBusy, env.Code, env.Message)
	}
	afterReject, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("重读会话文件失败：%v", err)
	}
	if string(before) != string(afterReject) {
		t.Fatal("被拒的改名不得动会话文件一个字节")
	}

	// ---- 空闲时只在末尾追加 session_info ----
	h.app.Sessions.Active = func() string { return "" }
	env, code := h.call(http.MethodPost, "/sessions/rename", map[string]string{
		"session_path": file, "session_title": "新名字",
	})
	h.mustOK(env, code, "改名", nil)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读改名后的会话文件失败：%v", err)
	}
	if !strings.HasPrefix(string(raw), string(before)) {
		t.Fatal("改名不得重写已有内容，只允许在末尾追加")
	}
	if !strings.Contains(string(raw), `"name":"新名字"`) {
		t.Fatalf("改名应以 session_info 条目追加，实际 %q", string(raw))
	}
}

// TestSessionFormatContract 断言引擎落盘的会话能被回放层读成历史轮。
// 引擎落盘、回放重建是两条链路唯一的交点——两边对格式的理解一旦分叉，
// 界面就看不到历史。
func TestSessionFormatContract(t *testing.T) {
	workspace := t.TempDir()
	store := agent.NewStore(t.TempDir(), func() string { return workspace })
	session, err := store.Create(workspace)
	if err != nil {
		t.Fatalf("建会话失败：%v", err)
	}
	if _, err := session.AppendMessage(agent.Message{
		Role:   agent.RoleUser,
		Blocks: []agent.Block{{Kind: agent.KindText, Text: "契约测试的问题"}},
	}); err != nil {
		t.Fatalf("写用户消息失败：%v", err)
	}
	if _, err := session.AppendMessage(agent.Message{
		Role:   agent.RoleAssistant,
		Blocks: []agent.Block{{Kind: agent.KindText, Text: "契约测试的回答"}},
	}); err != nil {
		t.Fatalf("写助手消息失败：%v", err)
	}

	turns := parser.BuildHistory(session.File(), nil)
	if len(turns) == 0 {
		t.Fatalf("回放层读不出引擎写的会话（引擎用 parent_id/毫秒/blocks，回放层用 parentId/ISO/content）")
	}
	joined := turnsText(turns)
	if !strings.Contains(joined, "契约测试的问题") || !strings.Contains(joined, "契约测试的回答") {
		t.Fatalf("回放层读出的内容不对：%q", joined)
	}

	// 会话目录名也要一致：清单按编码规则扫目录，引擎建会话时用同一个函数，
	// 两边不一致的话新会话建出来了清单里看不见。
	dir := filepath.Base(filepath.Dir(session.File()))
	encoded := pkg.EncodedWorkspace(workspace)
	if dir != encoded {
		t.Fatalf("会话目录名不一致：引擎建的是 %q，回放层扫的是 %q", dir, encoded)
	}
}

// TestSessionFork 断言分叉的三条纪律：原会话一个字节不动、
// 新会话只带到分叉点为止、不存在的起点直接拒。
// 分叉是「换一种问法重来」不损坏历史的唯一做法，所以两条边界都要钉死。
func TestSessionFork(t *testing.T) {
	h := newHarness(t)
	workspace := h.layout.SessionsDir
	if err := pkg.EnsureDir(workspace); err != nil {
		t.Fatalf("建会话目录失败：%v", err)
	}
	// 分叉要落在当前工作区上：没有工作区就没有会话目录。
	if _, err := h.app.Settings.SetWorkspace(workspace); err != nil {
		t.Fatalf("设工作区失败：%v", err)
	}
	store := agent.NewStore(workspace, func() string { return workspace })
	session, err := store.Create(workspace)
	if err != nil {
		t.Fatalf("建会话失败：%v", err)
	}
	// 三轮对话：第一轮、第二轮、第三轮。
	for _, text := range []string{"第一问", "第二问", "第三问"} {
		if _, err := session.AppendMessage(agent.Message{
			Role:   agent.RoleUser,
			Blocks: []agent.Block{{Kind: agent.KindText, Text: text}},
		}); err != nil {
			t.Fatalf("写消息失败：%v", err)
		}
	}
	entries, err := agent.ReadEntries(session.File())
	if err != nil {
		t.Fatalf("读会话失败：%v", err)
	}
	var userIDs []string
	for _, entry := range entries {
		if entry.EntryMessage != nil && entry.EntryMessage.MessageRole == string(agent.RoleUser) {
			userIDs = append(userIDs, entry.EntryId)
		}
	}
	if len(userIDs) != 3 {
		t.Fatalf("应有三轮用户消息，实际 %d", len(userIDs))
	}
	before, err := os.ReadFile(session.File())
	if err != nil {
		t.Fatalf("读原会话失败：%v", err)
	}

	h.app.Chat.BindSession(session.File())

	// ---- 分叉：起点是第二轮 ----
	out, err := h.app.Chat.Fork(session.File(), userIDs[1])
	if err != nil {
		t.Fatalf("分叉失败：%v", err)
	}
	if out.SessionFile == "" {
		t.Fatal("分叉应返回新会话文件路径")
	}
	if out.SessionFile == session.File() {
		t.Fatal("分叉应开一份新会话，不能覆盖原会话")
	}

	// ---- 原会话一个字节不动 ----
	after, err := os.ReadFile(session.File())
	if err != nil {
		t.Fatalf("重读原会话失败：%v", err)
	}
	if string(before) != string(after) {
		t.Fatal("分叉不该动原会话一个字节")
	}

	// ---- 新会话只带到分叉点为止 ----
	forked, err := agent.ReadEntries(out.SessionFile)
	if err != nil {
		t.Fatalf("读新会话失败：%v", err)
	}
	var forkedTexts []string
	for _, entry := range forked {
		if entry.EntryMessage == nil || entry.EntryMessage.MessageRole != string(agent.RoleUser) {
			continue
		}
		forkedTexts = append(forkedTexts, entry.EntryMessage.MessageContent.(string))
	}
	if len(forkedTexts) != 2 {
		t.Fatalf("新会话应只带两轮，实际 %d 轮", len(forkedTexts))
	}
	if forkedTexts[0] != "第一问" || forkedTexts[1] != "第二问" {
		t.Fatalf("新会话内容不对：%v", forkedTexts)
	}
	// 第三问属于另一条思路，不该跟过去。
	for _, text := range forkedTexts {
		if text == "第三问" {
			t.Fatal("分叉点之后的内容不该被复制过去")
		}
	}

	// ---- 起点不存在：直接拒 ----
	_, err = h.app.Chat.Fork(session.File(), "不存在的条目")
	if err == nil {
		t.Fatal("不存在的起点应被拒绝")
	}
	if pkg.As(err).Code != pkg.CodeNotFound {
		t.Fatalf("起点不存在应回 %q，实际 %q", pkg.CodeNotFound, pkg.As(err).Code)
	}

	// ---- 起点为空：直接拒 ----
	_, err = h.app.Chat.Fork(session.File(), "  ")
	if err == nil {
		t.Fatal("空起点应被拒绝")
	}
	if pkg.As(err).Code != pkg.CodeBadRequest {
		t.Fatalf("空起点应回 %q，实际 %q", pkg.CodeBadRequest, pkg.As(err).Code)
	}
}

// writeSession 在会话根下写一份会话 JSONL（首行是会话头），返回文件路径。
// 必须落在会话根内：路径越界本身会先被拦掉，那就测不到后续的写入纪律了。
func writeSession(t *testing.T, root string, lines ...string) string {
	t.Helper()
	dir := filepath.Join(root, "--C--ws--")
	if err := pkg.EnsureDir(dir); err != nil {
		t.Fatalf("建会话目录失败：%v", err)
	}
	file := filepath.Join(dir, "sess.jsonl")
	header := `{"type":"session","version":3,"id":"sess","cwd":"C:\\ws","timestamp":"2026-01-01T00:00:00.000Z"}`
	body := header + "\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatalf("写会话文件失败：%v", err)
	}
	return file
}

// turnsText 把回放结果拼成一段文本，便于断言分支内容。
func turnsText(turns []domain.HistoryTurnVO) string {
	var parts []string
	for _, turn := range turns {
		for _, block := range turn.TurnBlocks {
			parts = append(parts, block.BlockText)
		}
	}
	return strings.Join(parts, "|")
}
