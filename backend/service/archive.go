// 隐藏会话回收：子代理与任务的逐字链按保留期归档，结论、时间线与用量账留下。
package service

import (
	"sync"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// dayMillis 一天的毫秒数（保留期与归档节流共用）。
const dayMillis = 24 * 60 * 60 * 1000

// ArchiveService 回收过期隐藏会话。
type ArchiveService struct {
	env   *Env
	chat  *ChatService
	mu    sync.Mutex
	at    int64 // 上次自动归档时刻：24 小时节流，手动触发不受限
	inFly bool
}

// NewArchiveService 构造归档服务。
func NewArchiveService(env *Env, chat *ChatService) *ArchiveService {
	return &ArchiveService{env: env, chat: chat}
}

// Now 按保留期回收一次：删逐字对话链，留结论、时间线与用量账。
// 保留期为 0 = 永久保存，直接原样返回（这是用户的明确选择，不是错误）。
func (s *ArchiveService) Now() (domain.ArchiveVO, error) {
	raw, err := s.env.Repo.GetSetting(domain.SettingRetentionDays)
	if err != nil {
		return domain.ArchiveVO{}, err
	}
	days := parseRetention(raw)
	res := domain.ArchiveVO{RetentionDays: days}
	if days <= 0 {
		return res, nil
	}
	ids, err := s.env.Repo.ListExpiredHiddenSessions(time.Now().UnixMilli()-int64(days)*dayMillis, 0)
	if err != nil {
		return res, err
	}
	doomed := make([]string, 0, len(ids))
	for _, id := range ids {
		// 还在跑的会话一律不碰：那一轮正要往里面写。
		if s.chat.IsRunning(id) {
			continue
		}
		doomed = append(doomed, id)
	}
	if len(doomed) == 0 {
		return res, nil
	}
	n, err := s.env.Repo.ArchiveSessions(doomed)
	if err != nil {
		return res, err
	}
	res.Sessions = int(n)
	// 运行记录留着，只是「打开原始对话」这个入口没了。
	detached, _ := s.env.Repo.DetachRunSessions(doomed)
	res.DetachedRuns = int(detached)
	pkg.Infof("archive: 按保留期回收 %d 个隐藏会话（保留 %d 天），%d 条运行记录只留结论与时间线",
		res.Sessions, days, res.DetachedRuns)
	return res, nil
}

// Maybe 每次 run 收尾后顺手看看要不要归档：24 小时节流，同一时刻只跑一个。
func (s *ArchiveService) Maybe() {
	s.mu.Lock()
	if s.inFly || time.Now().UnixMilli()-s.at < dayMillis {
		s.mu.Unlock()
		return
	}
	s.inFly, s.at = true, time.Now().UnixMilli()
	s.mu.Unlock()
	if _, err := s.Now(); err != nil {
		pkg.Warnf("archive: 自动归档失败: %v", err)
	}
	s.mu.Lock()
	s.inFly = false
	s.mu.Unlock()
}
