package domain

// FileEntryVO 是目录里的一个条目。
type FileEntryVO struct {
	// EntryName 名称
	EntryName string `json:"entry_name"`
	// EntryIsDir 是否目录
	EntryIsDir bool `json:"entry_is_dir"`
	// SizeBytes 字节数（目录为 0）
	SizeBytes int64 `json:"size_bytes"`
	// ModifiedAt 修改时刻（毫秒）
	ModifiedAt int64 `json:"modified_at"`
}

// FileListRESP 是目录清单。
type FileListRESP struct {
	// DirPath 当前目录
	DirPath string `json:"dir_path"`
	// Entries 条目，目录优先
	Entries []FileEntryVO `json:"entries"`
	// Truncated 是否被截断
	Truncated bool `json:"truncated"`
}

// FileSearchRESP 是全工作区搜索的结果。
type FileSearchRESP struct {
	// SearchQuery 查询词
	SearchQuery string `json:"search_query"`
	// Hits 命中文件
	Hits []FileHitVO `json:"hits"`
	// Truncated 是否被截断
	Truncated bool `json:"truncated"`
}

// FileHitVO 是搜索的一个命中。
type FileHitVO struct {
	// FilePath 绝对路径
	FilePath string `json:"file_path"`
	// EntryName 文件名
	EntryName string `json:"entry_name"`
}

// FileReadRESP 是文本读取结果。
type FileReadRESP struct {
	// FilePath 绝对路径
	FilePath string `json:"file_path"`
	// FileText 文本内容
	FileText string `json:"file_text"`
	// SizeBytes 文件字节数
	SizeBytes int64 `json:"size_bytes"`
	// IsBinary 是否是二进制（此时 FileText 是占位说明）
	IsBinary bool `json:"is_binary"`
	// Truncated 是否被截断
	Truncated bool `json:"truncated"`
}

// FilePathREQ 是带路径的入参。
type FilePathREQ struct {
	// FilePath 目标绝对路径
	FilePath string `json:"file_path"`
}

// GitFileVO 是一个改动文件。
type GitFileVO struct {
	// FilePath 相对仓库根的路径
	FilePath string `json:"file_path"`
	// ChangeCode M / A / D / R / T / ? / U
	ChangeCode string `json:"change_code"`
	// IsStaged 是否已进暂存区
	IsStaged bool `json:"is_staged"`
	// HasConflict 是否冲突
	HasConflict bool `json:"has_conflict"`
	// Insertions 新增行数
	Insertions int `json:"insertions"`
	// Deletions 删除行数
	Deletions int `json:"deletions"`
}

// GitRemoteVO 是一条远端。
type GitRemoteVO struct {
	// RemoteName 远端名
	RemoteName string `json:"remote_name"`
	// RemoteUrl 远端地址
	RemoteUrl string `json:"remote_url"`
}

// GitStatusRESP 是仓库状态快照。
type GitStatusRESP struct {
	// GitAvailable 内置 git 是否可用
	GitAvailable bool `json:"git_available"`
	// UnavailableReason 不可用原因
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	// IsRepo 当前目录是否在仓库内
	IsRepo bool `json:"is_repo"`
	// Workspace 工作区
	Workspace string `json:"workspace"`
	// RepoRoot 仓库根
	RepoRoot string `json:"repo_root"`
	// BranchName 当前分支
	BranchName string `json:"branch_name"`
	// UpstreamRef 上游引用
	UpstreamRef string `json:"upstream_ref"`
	// Ahead 领先上游几个提交
	Ahead int `json:"ahead"`
	// Behind 落后上游几个提交
	Behind int `json:"behind"`
	// RepoState normal / merging / rebasing / cherry-picking
	RepoState string `json:"repo_state"`
	// RepoStateDetail 状态补充说明
	RepoStateDetail string `json:"repo_state_detail,omitempty"`
	// Remotes 远端清单
	Remotes []GitRemoteVO `json:"remotes"`
	// ChangedFiles 改动文件
	ChangedFiles []GitFileVO `json:"changed_files"`
	// HasConflict 是否处于冲突中
	HasConflict bool `json:"has_conflict"`
	// GitVersion git 版本
	GitVersion string `json:"git_version"`
}

// GitDiffRESP 是一份差异。
type GitDiffRESP struct {
	// FilePath 目标文件
	FilePath string `json:"file_path"`
	// Truncated 是否被截断
	Truncated bool `json:"truncated"`
	// Dropped 被丢掉的行数
	Dropped int `json:"dropped"`
	// DiffLines 差异行
	DiffLines []DiffLineVO `json:"diff_lines"`
}

// GitCommitVO 是一条提交摘要。
type GitCommitVO struct {
	// CommitHash 完整哈希
	CommitHash string `json:"commit_hash"`
	// ShortHash 短哈希
	ShortHash string `json:"short_hash"`
	// CommitSubject 提交标题
	CommitSubject string `json:"commit_subject"`
	// AuthorName 作者
	AuthorName string `json:"author_name"`
	// CommitTime 提交时刻（毫秒）
	CommitTime int64 `json:"commit_time"`
	// FileCount 改动文件数
	FileCount int `json:"file_count"`
	// Insertions 新增行数
	Insertions int `json:"insertions"`
	// Deletions 删除行数
	Deletions int `json:"deletions"`
}

// GitCommitDetailRESP 是单个提交的完整内容。
type GitCommitDetailRESP struct {
	// Commit 提交摘要
	Commit GitCommitVO `json:"commit"`
	// CommitBody 提交正文
	CommitBody string `json:"commit_body"`
	// ParentHashes 父提交
	ParentHashes []string `json:"parent_hashes"`
	// ChangedFiles 改动文件
	ChangedFiles []GitFileVO `json:"changed_files"`
	// Diff 差异
	Diff GitDiffRESP `json:"diff"`
}

// GitBranchVO 是一条分支。
type GitBranchVO struct {
	// BranchName 分支名
	BranchName string `json:"branch_name"`
	// IsCurrent 是否当前分支
	IsCurrent bool `json:"is_current"`
	// RemoteName 远端名；本地分支为空
	RemoteName string `json:"remote_name,omitempty"`
	// Ahead 领先上游几个提交
	Ahead int `json:"ahead"`
	// Behind 落后上游几个提交
	Behind int `json:"behind"`
}

// GitPathsREQ 是一批文件路径的入参。
type GitPathsREQ struct {
	// FilePaths 相对仓库根的路径
	FilePaths []string `json:"file_paths"`
}

// GitCommitREQ 是提交的入参。
type GitCommitREQ struct {
	// CommitMessage 提交信息
	CommitMessage string `json:"commit_message"`
	// IsAmend 是否 amend 上一个提交
	IsAmend bool `json:"is_amend"`
}

// GitCommitRESP 是提交结果。
type GitCommitRESP struct {
	// Status 提交后的状态
	Status GitStatusRESP `json:"status"`
	// CommitHash 新提交哈希
	CommitHash string `json:"commit_hash"`
}

// GitBranchREQ 是分支操作的入参。
type GitBranchREQ struct {
	// BranchName 分支名
	BranchName string `json:"branch_name"`
	// IsRebase 合并时是否用 rebase
	IsRebase bool `json:"is_rebase"`
}

// GitStateREQ 是冲突收尾的入参。
type GitStateREQ struct {
	// StateAction continue / abort
	StateAction string `json:"state_action"`
}

// GitInitREQ 是初始化的入参。
type GitInitREQ struct {
	// BranchName 初始分支名；空表示 git 默认
	BranchName string `json:"branch_name"`
}

// GitRemoteREQ 是添加远端的入参。
type GitRemoteREQ struct {
	// RemoteName 远端名
	RemoteName string `json:"remote_name"`
	// RemoteUrl 远端地址
	RemoteUrl string `json:"remote_url"`
}

// GitSyncREQ 是推拉取的入参。
type GitSyncREQ struct {
	// RemoteName 远端名
	RemoteName string `json:"remote_name"`
	// BranchName 分支名
	BranchName string `json:"branch_name"`
	// ForceMode none / with-lease
	ForceMode string `json:"force_mode"`
	// SetUpstream 是否同时设置上游
	SetUpstream bool `json:"set_upstream"`
	// Confirm 危险动作的确认词（推 force 时必须等于分支名）
	Confirm string `json:"confirm"`
	// IsRebase 拉取时是否用 rebase
	IsRebase bool `json:"is_rebase"`
}

// GitStashVO 是一条 stash。
type GitStashVO struct {
	// StashRef 引用名（stash@{n}），恢复与丢弃都用它
	StashRef string `json:"stash_ref"`
	// StashSubject 存的时候留的一句话
	StashSubject string `json:"stash_subject"`
}

// GitStashListRESP 是 stash 清单。
type GitStashListRESP struct {
	// Stashes stash 行，最新在前
	Stashes []GitStashVO `json:"stashes"`
}

// GitStashREQ 是 stash 动作的入参。
type GitStashREQ struct {
	// StashMessage 存 stash 时的一句话；可空
	StashMessage string `json:"stash_message"`
	// StashRef 丢弃指定 stash 时的引用名（stash@{n}）
	StashRef string `json:"stash_ref"`
	// Confirm 危险动作的确认词（丢弃 stash 时必须等于引用名）
	Confirm string `json:"confirm"`
}
