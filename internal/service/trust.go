package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/pkg"
	"WorkBaby/internal/repo"
)

// TrustService 工作目录信任三态：allow 直行 / ask 执行前询问 / deny 永不执行。
// 判定沿祖先链就近上溯，未登记任何祖先时恒为 ask（fail-closed，不落库）。
// roots 为恒信任的内置根（会话工作区与数据目录），不可撤销。
type TrustService struct {
	repo     *repo.WorkspaceTrustRepo
	roots    []string
	approver func(ctx context.Context, description string) bool // ask 态的人工审批门；nil = 一律拒绝
	mu       sync.Mutex                                         // 串行化「解析 → 询问 → 记账」，避免同一目录重复弹审批
}

// NewTrustService 构造信任服务；roots 为恒信任目录（可为空）。
func NewTrustService(r *repo.WorkspaceTrustRepo, roots ...string) *TrustService {
	norm := make([]string, 0, len(roots))
	for _, p := range roots {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		norm = append(norm, filepath.Clean(abs))
	}
	return &TrustService{repo: r, roots: norm}
}

// WithApprover 注入 ask 态的人工审批门（通常包 ApprovalService.Approve）。
// 未注入时 ask 按拒绝处理（fail-closed：宁可让模型改道，也不静默放行）。
func (s *TrustService) WithApprover(a func(ctx context.Context, description string) bool) *TrustService {
	s.approver = a
	return s
}

// Resolve 解析某目录的信任态（就近上溯；未命中登记返回 ask）。
func (s *TrustService) Resolve(ctx context.Context, path string) (domain.TrustResolveRESP, error) {
	dir, err := NormalizeDir(path)
	if err != nil {
		return domain.TrustResolveRESP{}, err
	}
	if root := s.matchRoot(dir); root != "" {
		return domain.TrustResolveRESP{Path: dir, State: domain.TrustStateAllow, Source: root, Matched: true}, nil
	}
	for cur := dir; ; {
		row, err := s.repo.Get(ctx, cur)
		if err != nil {
			return domain.TrustResolveRESP{Path: dir}, err
		}
		if row != nil {
			return domain.TrustResolveRESP{
				Path: dir, State: row.State, Source: row.Path, Matched: true,
			}, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break // 已到盘符/根目录
		}
		cur = parent
	}
	return domain.TrustResolveRESP{Path: dir, State: domain.TrustStateAsk}, nil
}

// Ensure 工具执行前的信任闸门：解析 → ask 则询问 → 批准后记账为 allow。
//
// 返回 (放行, 拒绝原因)；deny 与「ask 被拒」都返回 false，原因直接进工具回执，
// 模型据此改道（如改用工作区内的目录）而不是把拒绝当故障。
func (s *TrustService) Ensure(ctx context.Context, path string) (bool, string) {
	st, err := s.Resolve(ctx, path)
	if err != nil {
		pkg.L.Warn("resolve workspace trust failed", "path", path, "err", err.Error())
		return false, "directory trust check failed: " + err.Error()
	}
	switch st.State {
	case domain.TrustStateAllow:
		return true, ""
	case domain.TrustStateDeny:
		return false, "directory is not trusted (denied by you): " + st.Path
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// 持锁后重查：并发工具可能已在等待期间把该目录记为 allow
	if again, err := s.Resolve(ctx, st.Path); err == nil && again.State == domain.TrustStateAllow {
		return true, ""
	}
	if s.approver == nil {
		return false, "directory is not trusted and no approver is wired: " + st.Path
	}
	if !s.approver(ctx, "访问未信任目录 "+st.Path) {
		return false, "directory is not trusted (you denied): " + st.Path
	}
	if _, err := s.Decide(ctx, domain.WorkspaceTrustREQ{Path: st.Path, State: string(domain.TrustStateAllow)}); err != nil {
		pkg.L.Warn("persist workspace trust failed", "path", st.Path, "err", err.Error())
	}
	return true, ""
}

// Decide 写入信任决策（原子 upsert，重复决策同一目录只更新状态）。
func (s *TrustService) Decide(ctx context.Context, req domain.WorkspaceTrustREQ) (domain.WorkspaceTrustRESP, error) {
	dir, err := NormalizeDir(req.Path)
	if err != nil {
		return domain.WorkspaceTrustRESP{}, err
	}
	now := time.Now().UnixMilli()
	state := domain.ParseTrustState(req.State)
	if err := s.repo.Upsert(ctx, &domain.WorkspaceTrustDO{
		Path: dir, State: state, UpdatedAt: now,
	}); err != nil {
		return domain.WorkspaceTrustRESP{}, err
	}
	return domain.WorkspaceTrustRESP{Path: dir, State: state, UpdatedAt: now}, nil
}

// List 全部信任登记（设置页数据源）。
func (s *TrustService) List(ctx context.Context) ([]domain.WorkspaceTrustRESP, error) {
	rows, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.WorkspaceTrustRESP, 0, len(rows))
	for i := range rows {
		out = append(out, domain.WorkspaceTrustRESP{
			Path: rows[i].Path, State: rows[i].State, UpdatedAt: rows[i].UpdatedAt,
		})
	}
	return out, nil
}

// Revoke 撤销某目录的信任登记（回到默认 ask）。
func (s *TrustService) Revoke(ctx context.Context, path string) error {
	dir, err := NormalizeDir(path)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, dir)
}

// Roots 恒信任根（前端展示「这些目录默认信任」用）。
func (s *TrustService) Roots() []string {
	out := make([]string, len(s.roots))
	copy(out, s.roots)
	return out
}

// matchRoot 该目录是否落在恒信任根内；返回命中的根（未命中为空串）。
func (s *TrustService) matchRoot(dir string) string {
	for _, r := range s.roots {
		if pkg.Within(r, dir) == nil {
			return r
		}
	}
	return ""
}

// NormalizeDir 目录规范化：绝对化 + Clean；空串报 ErrTrustPathEmpty。
func NormalizeDir(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", domain.ErrTrustPathEmpty
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", pkg.Wrap(1002, "abs path failed", err)
	}
	return filepath.Clean(abs), nil
}

// DirExists 目录是否存在（设置页登记前提示用；不存在不阻止登记——
// 用户可能在 Agent 建目录之前就先授权）。
func DirExists(p string) bool {
	dir, err := NormalizeDir(p)
	if err != nil {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
