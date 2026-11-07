// Package tests 的会话管线：pi 的 JSONL 会话文件是唯一持久真相，
// 界面每次启动都由它重建。回放的健壮性与壳的写入纪律都在这一条链上。
package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/service/parser"
)

// TestSession 断言会话真相的全部难点，一段一类：
//
//	分支：只走当前分支，不混入被放弃的分支（用户编辑过消息会产生并列分支）。
//	健壮：一条残行不让整份历史读不出来；环状 parentId 必须有界退出。
//	组装：工具结果回填宿主轮、孤立结果单独成轮不丢、压缩成 engine-mark 轮、标题取自 session_info。
//	隐藏：只记壳的台账，绝不改写会话文件。
//	改名：只在末尾追加 session_info；引擎正打开该会话时拒绝——
//	两路同时追加会写出交错行，而会话文件写坏就整份回放不出来。
func TestSession(t *testing.T) {
	// ---- 分支 ----
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

	// ---- 健壮：残行 ----
	corrupt := writeSession(t, t.TempDir(),
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"还行"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timest`,
		`{"type":"message","id":"a2","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"后续答案"}]}}`,
	)
	if !strings.Contains(turnsText(parser.BuildHistory(corrupt, nil)), "后续答案") {
		t.Fatal("一条残行不该让整份历史读不出来：残行之后的有效条目仍应解析")
	}

	// ---- 健壮：环状 parentId ----
	cyclic := writeSession(t, t.TempDir(),
		`{"type":"message","id":"a","parentId":"b","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"环 A"}}`,
		`{"type":"message","id":"b","parentId":"a","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"user","content":"环 B"}}`,
	)
	done := make(chan struct{})
	go func() { parser.BuildHistory(cyclic, nil); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("环状 parentId 让回放卡死：ActivePath 必须用已访问集合兜住")
	}

	// ---- 组装：工具结果回填 ----
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

	// ---- 组装：孤立结果 ----
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

	// ---- 写入纪律：隐藏是壳的台账，文件一个字节都不动 ----
	h := newHarness(t)
	file := writeSession(t, h.layout.SessionsDir(),
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"第一句"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"第一答"}]}}`,
		`{"type":"message","id":"u2","parentId":"a1","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"user","content":"第二句"}}`,
	)
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读会话文件失败：%v", err)
	}
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

	// ---- 写入纪律：引擎正打开时拒绝改名，且不动文件 ----
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

	// ---- 写入纪律：空闲时只在末尾追加 session_info ----
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

// writeSession 在会话根下写一份 pi 会话 JSONL（首行是会话头），返回文件路径。
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
