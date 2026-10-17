// 子代理：一次委派 = 一个独立 PI 进程里的隐藏会话，结论回填成工具结果（specs/24）。
// 必须是独立进程：一个 PI 进程只有一个 streaming 状态，同进程起子会话会把父会话的流顶掉。
package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// SubagentService 跑子代理。
type SubagentService struct {
	env      *Env
	sessions *SessionService
	chat     *ChatService
	observer *Observer
	// mu 保证串行：每次一个子代理，跑完即收。并行会把「谁在花钱」变成一团。
	mu sync.Mutex
}

// NewSubagentService 构造子代理服务。
func NewSubagentService(env *Env, sessions *SessionService, chat *ChatService, observer *Observer) *SubagentService {
	return &SubagentService{env: env, sessions: sessions, chat: chat, observer: observer}
}

// subagentWaitTimeout 是一次子代理的等待上限。
const subagentWaitTimeout = 20 * time.Minute

// Run 在父会话下起一个子代理，等它收尾并返回结论。
// 失败也返回结论文本（写明原因）：让模型看到「这一步没做成」比抛错中断整轮更有用。
func (s *SubagentService) Run(ctx context.Context, sessionID string, req domain.SubagentREQ) (string, error) {
	task := strings.TrimSpace(req.Task)
	if task == "" {
		return "", pkg.New("5103", "子代理没说要干什么", "把要它查清 / 做完的事写成一句话")
	}
	sess, err := s.env.Repo.GetSession(sessionID)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	title := "[子代理] " + trimRunes(task, 40)
	if label := strings.TrimSpace(req.Label); label != "" {
		title = "[子代理] " + trimRunes(label, 40)
	}
	child, err := s.sessions.Create(domain.CreateSessionREQ{
		Title: title, Hidden: true,
		// 工作目录跟着父会话：子代理要处理的就是同一摊文件。
		Workspace: sess.Workspace,
		// 权限档继承父会话：子代理不是提权通道。
		Permission: sess.Permission,
		Model:      req.Model,
	})
	if err != nil {
		return "", err
	}
	// 子代理跑完（或超时）都要删掉那条隐藏会话：不删就是每用一次留一条孤儿记录。
	defer s.cleanup(child.ID)

	if _, err := s.chat.Send(child.ID, task, nil); err != nil {
		return "", err
	}

	done := make(chan struct{})
	go func() {
		// 轮询运行态而不是塞回调：PI 的 agent_settled 走翻译层，
		// 这里拿不到那条通道，轮询是唯一不改协议面的做法。
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				if !s.chat.IsRunning(child.ID) {
					close(done)
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	timer := time.NewTimer(subagentWaitTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		_ = s.chat.Stop(child.ID)
		return "子代理超时（" + subagentWaitTimeout.String() + "）没有收尾，这一步没做完。", nil
	case <-ctx.Done():
		_ = s.chat.Stop(child.ID)
		return "", ctx.Err()
	}

	out := strings.TrimSpace(s.chat.TakeResult(child.ID))
	in, output, cached, _ := s.env.Repo.UsageOfSession(child.ID)
	total := in + output + cached
	if out == "" {
		return "子代理失败：没有产出结论", nil
	}
	// 结论里带上用量：父助手能据此判断「这一步贵不贵、要不要再拆」。
	return out + "\n\n（子代理用量 " + itoa(total) + " tokens）", nil
}

// cleanup 删掉子代理的隐藏会话（连同它的 JSONL）。
func (s *SubagentService) cleanup(sessionID string) {
	if err := s.sessions.Delete(sessionID); err != nil {
		pkg.Warnf("subagent: 清理子会话失败 %s: %v", sessionID, err)
	}
}

// itoa 只在子代理回执里拼一个数字。
func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
