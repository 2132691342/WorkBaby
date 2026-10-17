// 架构分析的编排：分析结果与可分析入口。
package service

import (
	"strings"

	"WorkBaby/backend/archmap"
	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// ArchMapService 项目架构分析：把 archmap 能力域的结果按工作目录装配（specs/13）。
type ArchMapService struct {
	env *Env
}

// NewArchMapService 构造架构分析服务。
func NewArchMapService(env *Env) *ArchMapService { return &ArchMapService{env: env} }

// Analyze 分析目录；path 为空用全局工作目录。路径越界（..）拒绝——
// 与全项目路径护栏同一纪律。
func (s *ArchMapService) Analyze(path string) (*domain.ArchGraph, error) {
	root := strings.TrimSpace(path)
	if root == "" {
		root = strings.TrimSpace(s.env.Cfg.Workspace)
	}
	if root == "" {
		return nil, pkg.New("1002", "还没有设置工作目录，先在对话页顶部选一个文件夹", "")
	}
	if strings.Contains(root, "..") {
		return nil, pkg.New("1004", "路径里不能有 ..", root)
	}
	return archmap.Analyze(root)
}

// Entrypoints 入口文件清单。
func (s *ArchMapService) Entrypoints(path string) ([]domain.ArchEntrypointVO, error) {
	root := strings.TrimSpace(path)
	if root == "" {
		root = strings.TrimSpace(s.env.Cfg.Workspace)
	}
	if root == "" {
		return nil, pkg.New("1002", "还没有设置工作目录，先在对话页顶部选一个文件夹", "")
	}
	return archmap.Entrypoints(root)
}
