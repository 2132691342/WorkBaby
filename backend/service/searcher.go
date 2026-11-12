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

// Rebuild 为当前工作区重建索引。
// 索引是派生数据：重建失败只影响检索速度，不影响任何功能。
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

	defer func() {
		s.mu.Lock()
		s.building = false
		s.mu.Unlock()
	}()

	docs, err := index.Build(workspace)
	if err != nil {
		return s.Status(), pkg.WrapErr(pkg.CodeFileIO, "索引工作区失败", err)
	}
	rows := make([]domain.SearchDocDO, 0, len(docs))
	for _, doc := range docs {
		rows = append(rows, docToDO(doc, workspace))
	}
	if err := s.repo.ReplaceWorkspace(workspace, rows); err != nil {
		return s.Status(), err
	}
	s.mu.Lock()
	s.docs = docs
	s.workspace = workspace
	s.indexedAt = pkg.NowMs()
	s.mu.Unlock()
	pkg.Info("索引完成：工作区=%s 文档=%d", workspace, len(docs))
	return s.Status(), nil
}

// EnsureIndexed 保证当前工作区有可用索引；没有就现建。
// 切工作区后第一次搜索会走到这里：用户不该为了搜个文件先点一次「建索引」。
func (s *Searcher) EnsureIndexed(ctx context.Context) {
	s.mu.RLock()
	ready := s.indexedAt > 0 && s.workspace == s.settings.Workspace()
	s.mu.RUnlock()
	if ready {
		return
	}
	_, _ = s.Rebuild(ctx)
}

// Search 检索工作区。
func (s *Searcher) Search(ctx context.Context, query string, limit int) ([]domain.SearchHitVO, error) {
	s.EnsureIndexed(ctx)
	s.mu.RLock()
	docs := s.docs
	frecency := s.frecency
	s.mu.RUnlock()
	if len(docs) == 0 {
		return []domain.SearchHitVO{}, nil
	}
	hits := index.Search(docs, query, frecency, limit)
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
	// 一个都没命中时不要碰 docIDs：空的切片取下标会直接崩。
	if len(docIDs) > 0 {
		// 命中的文件下次排得更前：常用且近用的先出现。
		now := pkg.NowMs()
		for _, docID := range docIDs {
			frecency.Record(docID, now)
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
