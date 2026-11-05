// Package tests 是场景测试：只断言「改这里会坏那里」的长链路。
// 本文件覆盖会话回放管线：TestReplayFollowsActiveBranchSafely（分支与健壮性）、
// TestHistoryAssembly（历史轮组装）、TestHiddenTurnsAreFiltered（隐藏台账）。
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/service/parser"
)

// writeSession 把若干条目写成一份 pi 会话 JSONL（首行是会话头）。
// parser 只看文件内容、不看目录约定，所以测试不必复现工作区编码目录命名。
func writeSession(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "sess_01.jsonl")
	header := `{"type":"session","version":3,"id":"sess_01","cwd":"C:\\ws","timestamp":"2026-01-01T00:00:00.000Z"}`
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

// safeRole 取首个轮次角色（空切片时返回空串）。
func safeRole(turns []domain.HistoryTurnVO) string {
	if len(turns) == 0 {
		return ""
	}
	return turns[0].TurnRole
}

// TestReplayFollowsActiveBranchSafely 断言回放的两条遍历不变量：走当前分支
// 而不混入被放弃的分支；残行不让历史读不出来，环状 parentId 不让回放死循环。
func TestReplayFollowsActiveBranchSafely(t *testing.T) {
	// 用户编辑过第一句：当前分支是「改过的问题」那条。
	branch := writeSession(t, []string{
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"原始问题"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"原始回答"}]}}`,
		`{"type":"message","id":"u2","parentId":"u1","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"user","content":"改过的问题"}}`,
		`{"type":"message","id":"a2","parentId":"u2","timestamp":"2026-01-01T00:00:04.000Z","message":{"role":"assistant","content":[{"type":"text","text":"改过的回答"}]}}`,
	})
	joined := turnsText(parser.BuildHistory(branch, nil))
	if !strings.Contains(joined, "改过的问题") || !strings.Contains(joined, "改过的回答") {
		t.Fatalf("应回放当前分支（改过的问题 / 改过的回答），实际 %q", joined)
	}
	if strings.Contains(joined, "原始回答") {
		t.Fatalf("不应回放被放弃的分支（原始回答），实际 %q", joined)
	}

	// 残行之后的有效条目仍要能解析出来。
	corrupt := writeSession(t, []string{
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"还行"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timest`,
		`{"type":"message","id":"a2","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"后续答案"}]}}`,
	})
	cturns := parser.BuildHistory(corrupt, nil)
	if len(cturns) == 0 {
		t.Fatal("一条残行不该让整份历史读不出来")
	}
	if !strings.Contains(turnsText(cturns), "后续答案") {
		t.Fatal("残行之后的有效条目仍应被解析出来")
	}

	// 环：parentId 成环时回放必须有界退出（超时即失败）。
	cyclic := writeSession(t, []string{
		`{"type":"message","id":"a","parentId":"b","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"环 A"}}`,
		`{"type":"message","id":"b","parentId":"a","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"user","content":"环 B"}}`,
	})
	done := make(chan struct{})
	go func() {
		parser.BuildHistory(cyclic, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("环状 parentId 让回放卡死了；ActivePath 的 guard 集合必须兜住")
	}
}

// TestHistoryAssembly 断言历史轮组装的四种形状：工具结果回填宿主轮、
// 孤立结果单独成轮不丢、压缩成带 token 数的引擎标记轮、标题取自 session_info。
func TestHistoryAssembly(t *testing.T) {
	// 正常：工具结果回到调用轮。
	normal := writeSession(t, []string{
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"看下文件"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call_1","name":"read_file","arguments":{}}]}}`,
		`{"type":"message","id":"t1","parentId":"a1","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"toolResult","toolCallId":"call_1","toolName":"read_file","content":[{"type":"text","text":"文件内容在此"}]}}`,
	})
	turns := parser.BuildHistory(normal, nil)
	var toolTurns int
	var found bool
	for _, turn := range turns {
		if turn.TurnRole == "tool" {
			toolTurns++
			continue
		}
		for _, block := range turn.TurnBlocks {
			if block.BlockKind == domain.BlockTool && block.ToolCallId == "call_1" {
				found = true
				if block.ToolOutput == "" {
					t.Errorf("工具块的 output 应已回填，实际为空")
				}
			}
		}
	}
	if !found {
		t.Fatalf("应有一个带 call_1 的工具块；实际轮次=%d", len(turns))
	}
	if toolTurns != 0 {
		t.Fatalf("工具结果应挂回助手轮，不应单独成 %d 轮", toolTurns)
	}

	// 孤立：结果比调用先出现（中断/手改），单独成工具轮且原文保留。
	orphan := writeSession(t, []string{
		`{"type":"message","id":"t1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"toolResult","toolCallId":"orphan","toolName":"bash","content":[{"type":"text","text":"孤立结果"}]}}`,
	})
	oturns := parser.BuildHistory(orphan, nil)
	if len(oturns) != 1 || oturns[0].TurnRole != "tool" {
		t.Fatalf("孤立工具结果应单独成 1 个 tool 轮，实际 %d 轮 role=%q", len(oturns), safeRole(oturns))
	}
	if got := oturns[0].TurnBlocks[0].ToolOutput; !strings.Contains(got, "孤立结果") {
		t.Fatalf("工具输出应保留原文，实际 %q", got)
	}

	// 压缩：engine-mark 轮带上压缩前的 token 数。
	compaction := writeSession(t, []string{
		`{"type":"compaction","id":"c1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","tokensBefore":12345,"summary":"之前的摘要"}`,
	})
	marks := parser.BuildHistory(compaction, nil)
	if len(marks) != 1 || marks[0].TurnRole != "engine-mark" {
		t.Fatalf("压缩应是 1 个 engine-mark 轮，实际 %d 轮 role=%q", len(marks), safeRole(marks))
	}
	if !strings.Contains(marks[0].TurnBlocks[0].BlockText, "12345") {
		t.Fatalf("压缩标记应带上压缩前 token 数，实际 %q", marks[0].TurnBlocks[0].BlockText)
	}

	// 改名：会话名取自 session_info。
	renamed := writeSession(t, []string{
		`{"type":"session_info","id":"s1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","name":"我自己起的名字"}`,
		`{"type":"message","id":"u1","parentId":"s1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"user","content":"内容"}}`,
	})
	if summary := parser.Summarize(renamed); summary.SessionTitle != "我自己起的名字" {
		t.Fatalf("会话名应取自 session_info，实际 %q", summary.SessionTitle)
	}
}

// TestHiddenTurnsAreFiltered 断言隐藏台账生效、且会话文件本身不变（壳的台账不写引擎真相）。
func TestHiddenTurnsAreFiltered(t *testing.T) {
	file := writeSession(t, []string{
		`{"type":"message","id":"u1","parentId":"","timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"user","content":"第一句"}}`,
		`{"type":"message","id":"a1","parentId":"u1","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[{"type":"text","text":"第一答"}]}}`,
		`{"type":"message","id":"u2","parentId":"a1","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"user","content":"第二句"}}`,
	})

	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读会话文件失败：%v", err)
	}

	hidden := map[string]struct{}{"u2": {}}
	turns := parser.BuildHistory(file, hidden)
	for _, turn := range turns {
		if turn.TurnId == "u2" {
			t.Fatal("被隐藏的轮不应出现在回放结果里")
		}
	}
	if len(turns) == 0 {
		t.Fatal("隐藏一轮不该把整份历史清空")
	}

	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("重读会话文件失败：%v", err)
	}
	if string(before) != string(after) {
		t.Fatal("隐藏是壳的台账，绝不能改写会话文件")
	}
}
