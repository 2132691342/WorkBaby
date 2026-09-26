package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"WorkBaby/internal/agent"
	"WorkBaby/internal/domain"
	"WorkBaby/internal/llm"
	"WorkBaby/internal/pkg"
)

// sessionMetaKeyGoal 会话目标的元数据键（/goal 落点，run 收尾自动校验与续跑）。
const sessionMetaKeyGoal = "goal"

// goalVerdictTimeoutSec 目标校验是一次独立的小型 LLM 调用：快速失败，
// 校验失败按「暂停目标」处理（盲目续跑比停下来更糟）。
const goalVerdictTimeoutSec = 60

// Goal 查询会话当前目标；无目标时 Goal 为 null。
func (s *ChatService) Goal(ctx context.Context, sessionID string) (domain.GoalRESP, error) {
	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return domain.GoalRESP{}, err
	}
	return domain.GoalRESP{SessionID: ses.ID, Goal: sessionGoal(ses)}, nil
}

// SetGoal 设置/替换/暂停/恢复/清除会话目标（/goal 命令后端实现）。
// 状态变化即时落库并 emit chat:goal（前端目标卡与任务列表同步）。
func (s *ChatService) SetGoal(ctx context.Context, sessionID string, req domain.GoalREQ) (domain.GoalRESP, error) {
	unlock := s.lockSession(sessionID)
	defer unlock()

	ses, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil {
		return domain.GoalRESP{}, err
	}
	meta := readSessionMeta(ses)
	now := time.Now().UnixMilli()
	cur := sessionGoal(ses)

	switch strings.TrimSpace(req.Action) {
	case "set", "replace", "":
		text := strings.TrimSpace(req.Text)
		if text == "" {
			return domain.GoalRESP{}, pkg.New(2101, "目标描述不能为空（/goal <目标描述>）", "")
		}
		// set 语义：已有活动目标时等价 replace，否则新建
		maxRounds := domain.GoalDefaultMaxRounds
		round := 0
		status := domain.GoalStatusActive
		if cur != nil && cur.Status == domain.GoalStatusActive {
			round, maxRounds = cur.Round, cur.MaxRounds
			if maxRounds <= 0 {
				maxRounds = domain.GoalDefaultMaxRounds
			}
		}
		cur = &domain.SessionGoal{Text: text, Status: status, Round: round, MaxRounds: maxRounds, UpdatedAt: now}
	case "pause":
		if cur == nil {
			return domain.GoalRESP{}, pkg.New(2101, "当前会话没有目标", "")
		}
		cur.Status = domain.GoalStatusPaused
		cur.UpdatedAt = now
	case "resume":
		if cur == nil {
			return domain.GoalRESP{}, pkg.New(2101, "当前会话没有目标", "")
		}
		if cur.Status == domain.GoalStatusDone {
			return domain.GoalRESP{}, pkg.New(2101, "目标已完成，不能恢复；用 /goal <新目标> 设定新目标", "")
		}
		cur.Status = domain.GoalStatusActive
		cur.UpdatedAt = now
	case "clear":
		cur = nil
	default:
		return domain.GoalRESP{}, pkg.New(2101, "未知目标操作: "+req.Action, "")
	}

	if cur == nil {
		delete(meta, sessionMetaKeyGoal)
	} else {
		bs, err := json.Marshal(cur)
		if err != nil {
			return domain.GoalRESP{}, pkg.Wrap(2062, "marshal goal failed", err)
		}
		meta[sessionMetaKeyGoal] = string(bs)
	}
	if err := s.writeSessionMeta(ctx, ses, meta); err != nil {
		return domain.GoalRESP{}, err
	}
	s.emit("", ses.ID, "chat:goal", map[string]any{"goal": goalPayload(cur)})
	return domain.GoalRESP{SessionID: ses.ID, Goal: cur}, nil
}

// sessionGoal 读会话目标；未设置/解析失败返回 nil（脏数据不拦聊天主链路）。
func sessionGoal(ses *domain.ChatSessionDO) *domain.SessionGoal {
	v, _ := readSessionMeta(ses)[sessionMetaKeyGoal].(string)
	if strings.TrimSpace(v) == "" {
		return nil
	}
	var g domain.SessionGoal
	if json.Unmarshal([]byte(v), &g) != nil || strings.TrimSpace(g.Text) == "" {
		return nil
	}
	return &g
}

// goalPayload 目标的事件/前端载荷（snake_case 契约）。
func goalPayload(g *domain.SessionGoal) map[string]any {
	if g == nil {
		return nil
	}
	return map[string]any{
		"text": g.Text, "status": g.Status, "round": g.Round,
		"max_rounds": g.MaxRounds, "next_step": g.NextStep,
		"done_because": g.DoneBecause, "updated_at": g.UpdatedAt,
	}
}

// persistGoal 目标状态落库 + 事件广播（emit 失败只告警，不影响主链路）。
func (s *ChatService) persistGoal(ctx context.Context, ses *domain.ChatSessionDO, g *domain.SessionGoal) {
	meta := readSessionMeta(ses)
	if g == nil {
		delete(meta, sessionMetaKeyGoal)
	} else {
		bs, err := json.Marshal(g)
		if err != nil {
			return
		}
		meta[sessionMetaKeyGoal] = string(bs)
	}
	if err := s.writeSessionMeta(ctx, ses, meta); err != nil {
		pkg.L.Warn("persist goal failed", "sessionID", ses.ID, "err", err.Error())
		return
	}
	s.emit("", ses.ID, "chat:goal", map[string]any{"goal": goalPayload(g)})
}

// goalVerdict 单次校验的裁决。
type goalVerdict struct {
	Done     bool   `json:"done"`
	NextStep string `json:"next_step"`
	Because  string `json:"because"`
}

// maybeContinueGoal 目标模式主钩子：一轮 run 成功收尾后调用。未完成 → 校验（LLM 实据裁决）→
// 未达标则携带下一步动作自动续跑下一轮。续跑直接起新 run（runRegistry 保证旧 run 清理不误删新句柄）。
func (s *ChatService) maybeContinueGoal(ctx context.Context, ses *domain.ChatSessionDO, runID string, res *agent.Outcome) {
	g := sessionGoal(ses)
	if g == nil || g.Status != domain.GoalStatusActive {
		return
	}
	// 非正常收尾（用户打断 / 出错 / 限额收尾）一律暂停：无人值守的自动推进
	// 不应在异常状态上继续烧 token；已有轮次与产物都保留，可随时 resume。
	if res.Err != nil {
		g.Status = domain.GoalStatusPaused
		g.NextStep = ""
		g.UpdatedAt = time.Now().UnixMilli()
		s.persistGoal(ctx, ses, g)
		return
	}
	switch res.Reason {
	case agent.ReasonCancelled, agent.ReasonError:
		g.Status = domain.GoalStatusPaused
		g.NextStep = ""
		g.UpdatedAt = time.Now().UnixMilli()
		s.persistGoal(ctx, ses, g)
		return
	}

	maxRounds := g.MaxRounds
	if maxRounds <= 0 {
		maxRounds = domain.GoalDefaultMaxRounds
	}
	if g.Round >= maxRounds {
		g.Status = domain.GoalStatusPaused
		g.NextStep = "已达自动续跑轮数上限（" + fmt.Sprint(maxRounds) + " 轮），确认进度后可 /goal resume 继续"
		g.UpdatedAt = time.Now().UnixMilli()
		s.persistGoal(ctx, ses, g)
		return
	}

	verdict, err := s.goalVerdict(ctx, ses, g, res)
	if err != nil {
		pkg.L.Warn("goal verdict failed; pause", "runID", runID, "sessionID", ses.ID, "err", err.Error())
		g.Status = domain.GoalStatusPaused
		g.NextStep = "校验调用失败，已暂停（/goal resume 可继续）"
		g.UpdatedAt = time.Now().UnixMilli()
		s.persistGoal(ctx, ses, g)
		return
	}
	g.Round++
	g.UpdatedAt = time.Now().UnixMilli()
	if verdict.Done {
		g.Status = domain.GoalStatusDone
		g.NextStep = ""
		g.DoneBecause = verdict.Because
		s.persistGoal(ctx, ses, g)
		return
	}
	g.NextStep = verdict.NextStep
	s.persistGoal(ctx, ses, g)
	// 自动续跑：独立的用户消息驱动下一轮（正常发送链路，历史整体回灌）
	next := "[目标续跑 · 第 " + fmt.Sprint(g.Round+1) + "/" + fmt.Sprint(maxRounds) + " 轮] " +
		"当前目标：" + g.Text + "\n上一步校验给出的下一步动作：" + g.NextStep +
		"\n请直接执行该动作；目标完成后明确汇报达成的实据（改了哪些文件 / 跑了哪些验证 / 结果如何）。"
	go func() {
		_, err := s.SendStream(context.Background(), ses.ID, next, nil, agent.RequestParams{})
		if err != nil {
			pkg.L.Warn("goal auto-continue failed", "sessionID", ses.ID, "err", err.Error())
		}
	}()
}

// goalVerdict 目标校验：只看实据（文件变更 / 命令输出 / 测试结果），
// 计划、清单、口头结论都不算达成。校验失败返回 error（调用方暂停目标）。
func (s *ChatService) goalVerdict(ctx context.Context, ses *domain.ChatSessionDO, g *domain.SessionGoal, res *agent.Outcome) (goalVerdict, error) {
	if s.reg == nil || ses.ProviderID == "" {
		return goalVerdict{}, pkg.New(2001, "registry/provider 未就绪", "")
	}
	prov, err := s.reg.Get(ses.ProviderID)
	if err != nil {
		return goalVerdict{}, err
	}
	// 证据摘要：本轮工具调用计数 + 最终回复（截断），给校验足够的实据面
	toolSummary := fmt.Sprintf("本轮工具调用 %d 次", res.Turns)
	reply := res.Content
	if runeLen := len([]rune(reply)); runeLen > 3000 {
		reply = pkg.TruncateRunes(reply, 3000)
	}
	vctx, cancel := context.WithTimeout(ctx, goalVerdictTimeoutSec*time.Second)
	defer cancel()
	temp := 0.0
	maxTok := 400
	resp, err := prov.Chat(vctx, &llm.ChatRequest{
		Model: ses.Model,
		Messages: []*llm.Message{
			llm.SystemMessage(goalVerdictPrompt),
			llm.UserMessage("目标：" + g.Text + "\n\n本轮最终回复：\n" + reply + "\n\n(" + toolSummary + ")"),
		},
		Temperature: &temp,
		MaxTokens:   &maxTok,
	})
	if err != nil {
		return goalVerdict{}, err
	}
	if resp == nil {
		return goalVerdict{}, pkg.New(2001, "校验响应为空", "")
	}
	return parseGoalVerdict(resp.Message.Content)
}

const goalVerdictPrompt = `你是目标校验器。判断一轮 agent 工作后目标是否已达成，只输出 JSON。

## 评分准则（rubric；按以下清单逐项核查）
1. 产物存在：声称生成/修改的文件必须确实出现或变更（用 ls / file_read 验证）；声称运行的命令必须 exit code=0 且输出与声称一致
2. 可核对：检查项有可观测的证据（文件存在 + 内容匹配 / 命令退出码 + 关键 stdout），口头总结不算
3. 范围匹配：完成的是用户问的事，不是顺手的扩展或绕开
5. 反例触发：出现「我已完成」「已经创建」「已经成功」一类无证据陈述 → done=false 且 because 写明

## 规则
- 只输出 JSON 对象，不要解释、不要 Markdown 代码块
- 校验只看实据：改出来的文件、命令输出、测试结果、可核对的产物。计划、待办清单、听起来像结论的回复都不算达成
- 没达成时必须给出具体、可执行的下一步动作（next_step），像交给另一个工程师的任务卡（含「先验证 X 是否存在 → 若不存在则用 Y 创建 → 完成后跑 Z 验证」这种步骤化结构）

输出结构：
{"done":false,"next_step":"","because":""}`

// parseGoalVerdict 宽松解析校验输出（容忍 ```json 包裹与多余文本）。
func parseGoalVerdict(content string) (goalVerdict, error) {
	var v goalVerdict
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	if err := json.Unmarshal([]byte(strings.TrimSpace(trimmed)), &v); err != nil {
		return goalVerdict{}, pkg.Wrap(2062, "parse goal verdict failed", err)
	}
	if !v.Done && strings.TrimSpace(v.NextStep) == "" {
		return goalVerdict{}, pkg.New(2062, "goal verdict missing next_step", "")
	}
	return v, nil
}
