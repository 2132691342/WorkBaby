package service

import (
	"context"
	"strings"
	"sync"

	"WorkBaby/backend/agent"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/index"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
)

// Searcher 是工作区的预索引与检索。
// 它在后台把工作区扫一遍建索引，之后模型的搜索就走索引而不是全盘 grep。
type Searcher struct {
	// repo 检索文档仓储
	repo *repo.SearchRepo
	// settings 设置（取当前工作区）
	settings *Settings

	// mu 保护内存索引
	mu sync.RWMutex
	// docs 内存里的索引文档
	docs []index.Doc
	// frecency 频次账
	frecency *index.Frecency
	// workspace 当前索引对应的工作区
	workspace string
	// indexedAt 上次建索引的时刻（毫秒）
	indexedAt int64
	// building 是否正在建索引
	building bool
}

// NewSearcher 造一个检索器。
func NewSearcher(settings *Settings, db *repo.DB) *Searcher {
	return &Searcher{
		repo:     repo.NewSearchRepo(db),
		settings: settings,
		frecency: index.NewFrecency(),
	}
}

// Kit 造给引擎的检索出口。
func (s *Searcher) Kit() agent.Indexer {
	return agent.IndexerFunc(func(ctx context.Context, query string, limit int) ([]agent.IndexHit, error) {
		hits, err := s.Search(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		out := make([]agent.IndexHit, 0, len(hits))
		for _, hit := range hits {
			out = append(out, agent.IndexHit{
				HitPath:   hit.DocPath,
				HitKind:   hit.DocKind,
				HitSymbol: hit.DocSymbol,
				HitLine:   hit.DocLine,
				HitReason: hit.HitReason,
			})
		}
		return out, nil
	})
}

// Status 返回索引状态；界面据此显示「已索引 / 正在建 / 没建」。
func (s *Searcher) Status() domain.IndexStatusVO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count, err := s.repo.Count(s.workspace)
	if err != nil {
		count = 0
	}
	return domain.IndexStatusVO{
		IndexWorkspace: s.workspace,
		IndexDocCount:  int(count),
		IndexBuilt:     s.indexedAt > 0,
		IndexBuilding:  s.building,
		IndexBuiltAt:   s.indexedAt,
	}
}

// RebuildAsync 触发一次后台重建并立即返回当前状态。
// 大仓库全量扫描以分钟计，HTTP 请求与模型工具都不该被它拖住；
// 后台协程用独立的 context，不随任何调用方取消。最终状态靠 /index/status 轮询。
func (s *Searcher) RebuildAsync() (domain.IndexStatusVO, error) {
	if strings.TrimSpace(s.settings.Workspace()) == "" {
		return s.Status(), pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	s.mu.Lock()
	start := !s.building
	if start {
		s.building = true
	}
	s.mu.Unlock()
	if start {
		go func() {
			if _, err := s.rebuildLocked(context.Background()); err != nil {
				pkg.Warn("后台索引失败：%s", pkg.As(err).Message)
			}
		}()
	}
	return s.Status(), nil
}

// Rebuild 为当前工作区同步重建索引（测试与显式等待的场景用）。
// 回的是「收尾之后」的状态：先清 building 再取样，否则应答里 index_building 恒为 true。
func (s *Searcher) Rebuild(ctx context.Context) (domain.IndexStatusVO, error) {
	workspace := s.settings.Workspace()
	if strings.TrimSpace(workspace) == "" {
		return s.Status(), pkg.Err(pkg.CodeSessionBind, "请先选择工作区")
	}
	s.mu.Lock()
	if s.building {
		s.mu.Unlock()
		return s.Status(), nil
	}
	s.building = true
	s.mu.Unlock()
	return s.rebuildLocked(ctx)
}

// rebuildLocked 是已占住 building 位的重建主体：扫描、落库、换内存段。
func (s *Searcher) rebuildLocked(ctx context.Context) (domain.IndexStatusVO, error) {
	workspace := s.settings.Workspace()

	// 失败与成功两条路都要清掉 building：清在这里而不是 defer 里，
	// 是为了让返回值取到「已收尾」的状态。
	finish := func() {
		s.mu.Lock()
		s.building = false
		s.mu.Unlock()
	}

	docs, err := index.Build(ctx, workspace)
	if err != nil {
		finish()
		return s.Status(), pkg.WrapErr(pkg.CodeFileIO, "索引工作区失败", err)
	}
	rows := make([]domain.SearchDocDO, 0, len(docs))
	for _, doc := range docs {
		rows = append(rows, docToDO(doc, workspace))
	}
	if err := s.repo.ReplaceWorkspace(workspace, rows); err != nil {
		finish()
		return s.Status(), err
	}
	s.mu.Lock()
	s.docs = docs
	s.workspace = workspace
	s.indexedAt = pkg.NowMs()
	s.building = false
	s.mu.Unlock()
	pkg.Info("索引完成：工作区=%s 文档=%d", workspace, len(docs))
	return s.Status(), nil
}

// EnsureIndexed 保证当前工作区有可用索引；没有就触发后台构建。
// 不在调用方协程里等：切工作区后第一次搜索不该挂住整个请求几分钟，
// 构建期间 Search 回可读的「正在构建」错误，模型下一步再搜就有结果。
func (s *Searcher) EnsureIndexed(_ context.Context) {
	s.mu.RLock()
	ready := s.indexedAt > 0 && s.workspace == s.settings.Workspace()
	s.mu.RUnlock()
	if ready {
		return
	}
	_, _ = s.RebuildAsync()
}

// Search 检索工作区。检索与记账在同一把锁里：
// 「读 docs → 打分 → 记账」整体与重建互斥，才不会读到半新半旧的索引。
func (s *Searcher) Search(ctx context.Context, query string, limit int) ([]domain.SearchHitVO, error) {
	s.EnsureIndexed(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.docs) == 0 {
		if s.building {
			return nil, pkg.Err(pkg.CodeEngineBusy, "索引正在构建，稍后再搜")
		}
		return []domain.SearchHitVO{}, nil
	}
	hits := index.Search(s.docs, query, s.frecency, limit)
	out := make([]domain.SearchHitVO, 0, len(hits))
	docIDs := make([]string, 0, len(hits))
	for _, hit := range hits {
		out = append(out, domain.SearchHitVO{
			DocPath:       hit.HitDoc.DocPath,
			DocKind:       string(hit.HitDoc.DocKind),
			DocSymbol:     hit.HitDoc.DocSymbol,
			DocSymbolKind: hit.HitDoc.DocSymbolKind,
			DocLine:       hit.HitDoc.DocLine,
			HitScore:      hit.HitScore,
			HitReason:     hit.HitReason,
		})
		docIDs = append(docIDs, hit.HitDoc.DocID)
	}
	if len(docIDs) > 0 {
		// 命中的文件下次排得更前：常用且近用的先出现。
		now := pkg.NowMs()
		for _, docID := range docIDs {
			s.frecency.Record(docID, now)
		}
		_ = s.repo.BumpHit(docIDs)
	}
	return out, nil
}

// docToDO 把索引文档转成持久化行。
func docToDO(doc index.Doc, workspace string) domain.SearchDocDO {
	return domain.SearchDocDO{
		DocID:         doc.DocID,
		WorkspacePath: workspace,
		DocPath:       doc.DocPath,
		DocKind:       string(doc.DocKind),
		DocSymbol:     doc.DocSymbol,
		DocSymbolKind: doc.DocSymbolKind,
		DocLine:       doc.DocLine,
		DocBody:       doc.DocBody,
		DocSize:       doc.DocSize,
		DocMtimeMs:    doc.DocMtimeMs,
	}
}
