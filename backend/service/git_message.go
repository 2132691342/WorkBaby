// 提交信息起草：起一个不落会话的短命 PI 进程，按当前 diff 请求一句摘要。
// 助手只起草、不提交——提交是用户的决定（specs/08）。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/git"
	"WorkBaby/backend/pi"
)

// draftTimeout 起草超时：一次性短调用，超过这么久就放弃让用户手写。
const draftTimeout = 45 * time.Second

// draftPrompt 是给内核的指令。限定成「一句摘要」而不是自由发挥：
// 提交信息要短、要能放进 git log --oneline，写成长文用户还得自己删。
const draftPrompt = `下面是一个 Git 工作区即将提交的改动。请只输出一行提交信息
（中文，不超过 50 字），说明这次改了什么。不要解释、不要引号、不要换行。

改动文件：
%s

差异片段：
%s`

// DraftCommitMessage 按当前 diff 起草提交信息。
// 失败不报错：结果里带 Err，界面回落到手写——起草不出来不该挡住提交。
func (s *GitService) DraftCommitMessage(sessionID string, paths []string) *domain.GitMessageRESP {
	root, err := s.workspace(sessionID)
	if err != nil {
		return &domain.GitMessageRESP{Err: err.Error()}
	}
	exe, args, err := s.piCommand()
	if err != nil || exe == "" {
		return &domain.GitMessageRESP{Err: "还没有可用的助手引擎，先完成环境引导"}
	}
	// 只送统计与截断后的差异：全文可能几万行，塞进提示词既慢又贵，
	// 而摘要质量在几千字之后不再提升。
	brief := git.Brief(root, paths)
	if brief == "" {
		return &domain.GitMessageRESP{Err: "还没有可提交的改动，先勾选文件入栈"}
	}
	diff := git.DiffText(root, paths)

	ctx, cancel := context.WithTimeout(context.Background(), draftTimeout)
	defer cancel()

	// 短命进程 + 不落会话：这次调用不该在用户的会话列表里留下一行。
	cli, err := pi.Start(pi.Config{
		Exe: exe, Args: args, Workspace: root, NoSession: true,
		Env: s.env.PiEnv.Env(),
	}, draftHandler{})
	if err != nil {
		return &domain.GitMessageRESP{Err: err.Error()}
	}
	defer cli.Stop()

	prompt := fmt.Sprintf(draftPrompt, brief, clipText(diff, 4000))
	if _, err := cli.Call(ctx, "prompt", map[string]any{"message": prompt}); err != nil {
		return &domain.GitMessageRESP{Err: err.Error()}
	}
	// prompt 的响应只确认已入队；正文等内核跑完后取最后一条助手文本。
	raw, err := cli.Call(ctx, "get_last_assistant_text", nil)
	if err != nil {
		return &domain.GitMessageRESP{Err: err.Error()}
	}
	msg := firstLine(strings.TrimSpace(textOf(raw)))
	if msg == "" {
		return &domain.GitMessageRESP{Err: "助手没给出可用的提交信息，自己写一句"}
	}
	return &domain.GitMessageRESP{Message: msg}
}

func (s *GitService) piCommand() (string, []string, error) {
	if s.env == nil || s.env.PiEnv == nil {
		return "", nil, nil
	}
	return s.env.PiEnv.PiCommand()
}

// textOf 从 get_last_assistant_text 的响应里取出正文字段。
func textOf(raw json.RawMessage) string {
	var doc struct {
		Data struct {
			Text string `json:"text"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	return doc.Data.Text
}

// clipText 截断送进提示词的差异：草稿只需要知道「改了哪类东西」。
func clipText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "\n……（差异过长，已截断）"
}

// firstLine 只取第一行：模型偶尔会加一句解释，提交信息框里放不下两行。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'`")
	if len([]rune(s)) > 72 {
		s = string([]rune(s)[:72])
	}
	return s
}

// draftHandler 吞掉短命进程的事件：起草过程不进 SSE，用户不需要看见它在想什么。
type draftHandler struct{}

func (draftHandler) OnEvent(pi.Event) {}

func (draftHandler) OnExit(error) {}
