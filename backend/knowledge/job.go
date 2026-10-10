package knowledge

import (
	"context"
	"time"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// job 是一趟重建索引的运行状态。重建要逐篇解析再写库，
// 大库跑满一分钟很常见——同步等在一次请求里既占着连接也不可取消。
type job struct {
	running    bool
	total      int
	done       int
	failed     int
	startedAt  int64
	finishedAt int64
	cancel     context.CancelFunc
}

// StartReindex 启动一趟后台重建；已在跑时返回 false，不排队也不重入。
func (s *Service) StartReindex() (bool, error) {
	s.jobMu.Lock()
	if s.cur.running {
		s.jobMu.Unlock()
		return false, nil
	}
	// ctx 必须在起 goroutine 之前建好：晚一步设置 cancel 的话，
	// 用户在这一瞬间点「停止」会拿到一个「没有可停的任务」的假答案。
	ctx, cancel := context.WithCancel(context.Background())
	s.cur = job{running: true, startedAt: nowMillis(), cancel: cancel}
	s.jobMu.Unlock()

	go func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				pkg.Errorf("knowledge: 重建索引 panic: %v", r)
			}
			s.jobMu.Lock()
			s.cur.running = false
			s.cur.finishedAt = nowMillis()
			s.jobMu.Unlock()
		}()
		s.runReindex(ctx)
	}()
	return true, nil
}

// ReindexStatus 取进度快照；没跑过时返回全零（running=false）。
func (s *Service) ReindexStatus() domain.ReindexJobVO {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	return domain.ReindexJobVO{
		Running:    s.cur.running,
		Total:      s.cur.total,
		Done:       s.cur.done,
		Failed:     s.cur.failed,
		StartedAt:  s.cur.startedAt,
		FinishedAt: s.cur.finishedAt,
	}
}

// CancelReindex 请求中断；已结束的任务返回 false。
func (s *Service) CancelReindex() bool {
	s.jobMu.Lock()
	cancel := s.cur.cancel
	running := s.cur.running
	s.jobMu.Unlock()
	if !running || cancel == nil {
		return false
	}
	cancel()
	return true
}

// runReindex 逐篇重建并在每篇之后更新进度。ctx 只在篇与篇之间检查——
// 一篇的解析与写入是原子单位，中途掐断会留下半套分块。
func (s *Service) runReindex(ctx context.Context) {
	docs, err := s.repo.ListDocs()
	if err != nil {
		pkg.Warnf("knowledge: 重建索引取不到文档列表: %v", err)
		return
	}
	s.jobMu.Lock()
	s.cur.total = len(docs)
	s.jobMu.Unlock()

	for i := range docs {
		select {
		case <-ctx.Done():
			pkg.Infof("knowledge: 重建索引已取消（%d/%d）", s.progress().done, len(docs))
			return
		default:
		}
		if err := s.index(&docs[i]); err != nil {
			pkg.Warnf("knowledge: 重建 %s 失败: %v", docs[i].Path, err)
			s.bump(0, 1)
			continue
		}
		s.bump(1, 0)
	}
}

func (s *Service) bump(done, failed int) {
	s.jobMu.Lock()
	s.cur.done += done
	s.cur.failed += failed
	s.jobMu.Unlock()
}

func (s *Service) progress() job {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	return s.cur
}

func nowMillis() int64 { return time.Now().UnixMilli() }
