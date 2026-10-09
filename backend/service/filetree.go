package service

// 工作区文件浏览：前端 @ 引用文件时的目录列举，只读、只在工作区内。

import (
	"os"
	"sort"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// maxFileEntries 单次列举上限：防手滑把整个盘根甩给前端。
const maxFileEntries = 500

// FileService 列举工作区目录，供附件选择面板逐级浏览。
type FileService struct{ env *Env }

// NewFileService 构造文件列举服务。
func NewFileService(env *Env) *FileService { return &FileService{env: env} }

// List 列出工作区内 rel 目录的条目；目录在前，名称排序。
func (s *FileService) List(rel string) (*domain.FileListVO, error) {
	root := s.env.Cfg.Workspace
	if strings.TrimSpace(root) == "" {
		return nil, pkg.New(1001, "还没有设置工作目录，先在对话页顶部选一个文件夹", "")
	}
	full, err := pkg.SafeJoin(root, rel)
	if err != nil {
		return nil, err
	}
	if !pkg.DirExists(full) {
		return nil, pkg.New(1005, "目录不存在", rel)
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, pkg.Wrap(1005, "读取目录失败", err)
	}
	out := make([]domain.FileEntryVO, 0, len(entries))
	for _, e := range entries {
		// 隐藏条目不进列表：给小白的浏览器里不该出现系统噪音文件
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "$") {
			continue
		}
		out = append(out, domain.FileEntryVO{Name: e.Name(), Dir: e.IsDir()})
		if len(out) >= maxFileEntries {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return &domain.FileListVO{Path: pkg.RelPath(root, full), Entries: out}, nil
}
