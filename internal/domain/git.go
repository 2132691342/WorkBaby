// Package domain 本文件：Git 面板聚合（无独立表；状态源是工作区 .git）。
package domain

import "WorkBaby/internal/pkg"

// GitFileStatusRESP 变更文件行：porcelain 状态 + 暂存标记。
type GitFileStatusRESP struct {
	Path     string `json:"path"`                // 工作区相对路径
	OrigPath string `json:"orig_path,omitempty"` // 重命名/复制的原路径（R/C 时非空）
	Status   string `json:"status"`              // M / A / D / R / C / U / ?
	Staged   bool   `json:"staged"`              // 已进暂存区
}

// GitCommitRESP 提交历史行。
type GitCommitRESP struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
	Refs    string `json:"refs,omitempty"` // --decorate 的引用（HEAD -> main, tag: v1）
}

// GitCommitFileRESP 一次提交里单个文件的变化规模。
type GitCommitFileRESP struct {
	Path     string `json:"path"`
	OrigPath string `json:"orig_path,omitempty"`
	Status   string `json:"status"` // A / M / D / R
	Added    int    `json:"added"`
	Removed  int    `json:"removed"`
	Binary   bool   `json:"binary,omitempty"` // 二进制文件（numstat 为 -）
}

// GitCommitDetailRESP 提交详情：元信息 + 变更文件清单（前端据此再拉单文件 diff）。
type GitCommitDetailRESP struct {
	Hash         string              `json:"hash"`
	ShortHash    string              `json:"short_hash"`
	Author       string              `json:"author"`
	AuthorEmail  string              `json:"author_email,omitempty"`
	Date         string              `json:"date"`
	Subject      string              `json:"subject"`
	Body         string              `json:"body,omitempty"`
	Parents      []string            `json:"parents,omitempty"`
	Refs         string              `json:"refs,omitempty"`
	Files        []GitCommitFileRESP `json:"files"`
	TotalAdded   int                 `json:"total_added"`
	TotalRemoved int                 `json:"total_removed"`
}

// GitLogREQ 提交历史分页入参。
type GitLogREQ struct {
	Limit int  `json:"limit"` // <=0 用 20
	Skip  int  `json:"skip"`  // 跳过条数（分页）
	All   bool `json:"all"`   // true = 含所有分支（--all）
}

// GitOverviewRESP 面板首屏：分支 + 变更 + 最近提交一次取齐。
type GitOverviewRESP struct {
	Root     string              `json:"root"`     // git 仓库根（绝对路径）
	Branch   string              `json:"branch"`   // 当前分支（空 = detached HEAD）
	Upstream string              `json:"upstream"` // 跟踪分支（空 = 未跟踪）
	Ahead    int                 `json:"ahead"`    // 领先 upstream 提交数
	Behind   int                 `json:"behind"`   // 落后 upstream 提交数
	Files    []GitFileStatusRESP `json:"files"`
	Branches []string            `json:"branches"` // 本地分支
	Log      []GitCommitRESP     `json:"log"`      // 最近 20 条
}

// GitSwitchREQ 切换/新建分支入参。
type GitSwitchREQ struct {
	Branch string `json:"branch"` // 目标分支名
	Create bool   `json:"create"` // true = 不存在时新建（checkout -b）
}

// GitDiffREQ 单文件（或全仓）diff 入参。
type GitDiffREQ struct {
	Path   string `json:"path"`   // 空 = 全部变更
	Staged bool   `json:"staged"` // true = 暂存区 vs HEAD
}

// GitCommitREQ 提交入参。
type GitCommitREQ struct {
	Message  string `json:"message"`
	StageAll bool   `json:"stage_all"` // 先 add -A 再提交
}

// GitDiffRESP diff 出参（unified diff 原文，前端 DiffView 渲染）。
type GitDiffRESP struct {
	Diff string `json:"diff"`
}

// GitGraphRESP 提交图谱出参（git log --graph 的原始文本，等宽渲染）。
type GitGraphRESP struct {
	Graph string `json:"graph"`
}

// 包级错误变量；错误码段位 8400（Git 面板）。
var (
	ErrGitNotARepo    = pkg.New(8401, "not a git repository", "")
	ErrGitNothing     = pkg.New(8402, "nothing to commit", "")
	ErrGitBranch      = pkg.New(8403, "branch operation failed", "")
	ErrGitCommitMsg   = pkg.New(8404, "commit message is required", "")
	ErrGitFailed      = pkg.New(8405, "git command failed", "")
	ErrGitBranchInUse = pkg.New(8406, "cannot operate on the current branch", "")
	ErrGitCommitHash  = pkg.New(8407, "commit hash is required", "")
)
