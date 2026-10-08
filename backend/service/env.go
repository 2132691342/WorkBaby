// Package service 是业务编排层：会话、对话、审批、模型服务、技能与设置。
// 它不感知 HTTP 与桌面壳，能力域全部经接口注入。
package service

import (
	"sync"

	"WorkBaby/backend/config"
	"WorkBaby/backend/knowledge"
	"WorkBaby/backend/pkg"
	"WorkBaby/backend/repo"
	"WorkBaby/backend/runtime"
	"WorkBaby/backend/skill"
	"WorkBaby/backend/tool"
)

// Env 是各服务共享的运行环境，构造期一次性注入。
type Env struct {
	Repo      *repo.Repo
	Paths     runtime.Paths
	Cfg       *config.Config
	Emitter   *Emitter
	Registry  *tool.Registry
	Skills    *skill.Registry
	Knowledge *knowledge.Service
	ToolDeps  tool.Deps
	depsMu    sync.Mutex
}

// NewToolDeps 构造工具依赖：内置 Python / PowerShell 优先，知识库检索器直接注入。
func NewToolDeps(p runtime.Paths, ks *knowledge.Service) tool.Deps {
	return tool.Deps{
		PythonExe:     runtime.PythonExe(p),
		PowerShellExe: runtime.PowerShellExe(p),
		Knowledge:     ks,
		TmpDir:        p.TmpDir,
		Reads:         nil,
	}
}

// RefreshToolDeps 重新解析内置运行时路径并写回工具依赖：ToolDeps 是启动期快照，
// 不刷新的话「重新检测」显示已就绪，跑脚本仍然报没有可用的 Python。
func (e *Env) RefreshToolDeps(p runtime.Paths) {
	runtime.ResetProbe()
	e.depsMu.Lock()
	defer e.depsMu.Unlock()
	e.ToolDeps.PythonExe = runtime.PythonExe(p)
	e.ToolDeps.PowerShellExe = runtime.PowerShellExe(p)
}

// ReloadWorkspaceSkills 重新装载当前工作区的技能，切换工作目录后调用。
func (e *Env) ReloadWorkspaceSkills() {
	if e.Skills == nil || e.Cfg == nil {
		return
	}
	skill.LoadWorkspace(e.Skills, e.Cfg.Workspace)
}

// Env 自检：缺核心依赖即启动失败，避免运行期静默失效。
func (e *Env) MissingDeps() error {
	switch {
	case e.Repo == nil:
		return pkg.New(2001, "数据仓储未装配", "")
	case e.Emitter == nil:
		return pkg.New(2001, "事件出口未装配", "")
	case e.Registry == nil:
		return pkg.New(2001, "工具注册表未装配", "")
	case e.Cfg == nil:
		return pkg.New(2001, "配置未装配", "")
	default:
		return nil
	}
}
