package service

import (
	"context"
	"strings"

	"WorkBaby/internal/domain"
	"WorkBaby/internal/repo"
)

// FolderService 文件夹编排：CRUD + 树 + path 一致性维护（防循环 + 级联）。
type FolderService struct {
	repo *repo.FolderRepo
}

func NewFolderService(r *repo.FolderRepo) *FolderService { return &FolderService{repo: r} }

// Create 新建文件夹；name 必填，按父目录拼接 path。
func (s *FolderService) Create(ctx context.Context, req domain.FolderREQ) (domain.FolderRESP, error) {
	if req.Name == "" {
		return domain.FolderRESP{}, domain.ErrFolderNameEmpty
	}
	parentPath := "/"
	var parentID *string
	if req.ParentID != nil && *req.ParentID != "" {
		parent, err := s.repo.GetByID(ctx, *req.ParentID)
		if err != nil {
			return domain.FolderRESP{}, err
		}
		parentPath = parent.Path
		parentID = &parent.ID
	}
	d := &domain.FolderDO{
		Name:        req.Name,
		ParentID:    parentID,
		Path:        joinPath(parentPath, req.Name),
		Description: req.Description,
		WorkspaceID: req.WorkspaceID,
	}
	if err := s.repo.Create(ctx, d); err != nil {
		return domain.FolderRESP{}, err
	}
	return toFolderRESP(d, 0), nil
}

// Update 重命名/移动；含防循环引用与子目录 path 级联更新。
func (s *FolderService) Update(ctx context.Context, id string, req domain.FolderREQ) (domain.FolderRESP, error) {
	orig, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.FolderRESP{}, err
	}
	parentPath := "/"
	var parentID *string
	if req.ParentID != nil && *req.ParentID != "" {
		if *req.ParentID == id {
			return domain.FolderRESP{}, domain.ErrFolderParentCycle // 不能把自己设为父
		}
		parent, err := s.repo.GetByID(ctx, *req.ParentID)
		if err != nil {
			return domain.FolderRESP{}, err
		}
		// 防祖先循环：parent.path 是否以 orig.path 开头
		if parent.Path == orig.Path || strings.HasPrefix(parent.Path, orig.Path+"/") {
			return domain.FolderRESP{}, domain.ErrFolderParentCycle
		}
		parentPath = parent.Path
		parentID = &parent.ID
	}
	newPath := joinPath(parentPath, req.Name)
	// 旧 path → 新 path：级联更新所有子目录的 path 前缀
	if err := s.cascadePath(ctx, orig.Path, newPath, orig.ID); err != nil {
		return domain.FolderRESP{}, err
	}
	upd := &domain.FolderDO{
		ID:          orig.ID,
		Name:        req.Name,
		ParentID:    parentID,
		Path:        newPath,
		Description: req.Description,
		WorkspaceID: orig.WorkspaceID,
		CreatedAt:   orig.CreatedAt,
	}
	if err := s.repo.Update(ctx, upd); err != nil {
		return domain.FolderRESP{}, err
	}
	cc, _ := s.repo.CountChildren(ctx, id)
	return toFolderRESP(upd, cc), nil
}

// Delete 删除文件夹；子目录非空时拒绝。
func (s *FolderService) Delete(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	cc, err := s.repo.CountChildren(ctx, id)
	if err != nil {
		return err
	}
	if cc > 0 {
		return domain.ErrFolderNotEmpty
	}
	return s.repo.Delete(ctx, id)
}

// Get 按 ID 查询。
func (s *FolderService) Get(ctx context.Context, id string) (domain.FolderRESP, error) {
	d, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.FolderRESP{}, err
	}
	cc, _ := s.repo.CountChildren(ctx, id)
	return toFolderRESP(d, cc), nil
}

// ListByParent 列出指定父目录下的子文件夹。
func (s *FolderService) ListByParent(ctx context.Context, parentID *string) ([]domain.FolderRESP, error) {
	rows, err := s.repo.ListByParent(ctx, parentID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.FolderRESP, 0, len(rows))
	for i := range rows {
		cc, _ := s.repo.CountChildren(ctx, rows[i].ID)
		out = append(out, toFolderRESP(&rows[i], cc))
	}
	return out, nil
}

// Tree 整棵树（前端树形展示，全局）。
func (s *FolderService) Tree(ctx context.Context) ([]domain.FolderTreeNode, error) {
	all, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return buildTrees(all), nil
}

// TreeForWorkspace 指定 workspace 的逻辑文件夹树；空 workspaceId = 仅全局（workspace_id 为空）。
func (s *FolderService) TreeForWorkspace(ctx context.Context, workspaceID string) ([]domain.FolderTreeNode, error) {
	all, err := s.repo.ListByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return buildTrees(all), nil
}

// cascadePath 更新旧 path 前缀下的所有子目录 path。
func (s *FolderService) cascadePath(ctx context.Context, oldPath, newPath, skipID string) error {
	all, err := s.repo.ListAll(ctx)
	if err != nil {
		return err
	}
	for i := range all {
		d := &all[i]
		if d.ID == skipID {
			continue
		}
		if d.Path == oldPath || strings.HasPrefix(d.Path, oldPath+"/") {
			d.Path = newPath + d.Path[len(oldPath):]
			if err := s.repo.Update(ctx, d); err != nil {
				return err
			}
		}
	}
	return nil
}

func toFolderRESP(d *domain.FolderDO, childCount int) domain.FolderRESP {
	return domain.FolderRESP{
		ID:          d.ID,
		Name:        d.Name,
		ParentID:    d.ParentID,
		Path:        d.Path,
		Description: d.Description,
		WorkspaceID: d.WorkspaceID,
		ChildCount:  childCount,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

func buildTrees(all []domain.FolderDO) []domain.FolderTreeNode {
	byParent := map[string][]domain.FolderDO{}
	var roots []domain.FolderDO
	for i := range all {
		d := all[i]
		if d.ParentID == nil || *d.ParentID == "" {
			roots = append(roots, d)
		} else {
			byParent[*d.ParentID] = append(byParent[*d.ParentID], d)
		}
	}
	out := make([]domain.FolderTreeNode, 0, len(roots))
	for i := range roots {
		out = append(out, buildTree(&roots[i], byParent))
	}
	return out
}

func buildTree(node *domain.FolderDO, byParent map[string][]domain.FolderDO) domain.FolderTreeNode {
	children := byParent[node.ID]
	cc := len(children)
	kids := make([]domain.FolderTreeNode, 0, cc)
	for i := range children {
		kids = append(kids, buildTree(&children[i], byParent))
	}
	return domain.FolderTreeNode{
		Folder:   toFolderRESP(node, cc),
		Children: kids,
	}
}

func joinPath(parent, name string) string {
	if parent == "" || parent == "/" {
		return name
	}
	if strings.HasSuffix(parent, "/") {
		return parent + name
	}
	return parent + "/" + name
}
