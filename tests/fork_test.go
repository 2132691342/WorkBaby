// 会话分叉：从某一条用户消息开一份新会话，原会话一个字节不动。
// 分叉是「换一种问法重来」不损坏历史的唯一做法，所以原会话不变与分叉点边界都要钉死。
package tests

import (
	"os"
	"testing"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/pkg"
)

// TestSessionFork 断言分叉的三条纪律：原会话一个字节不动、
// 新会话只带到分叉点为止、不存在的起点直接拒。
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
