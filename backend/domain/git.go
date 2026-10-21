// 工作区版本状态：读写 Git 仓库的改动、差异、中间状态与提交流程，供界面「看见并收拢改了什么」。
package domain

// 改动文件的状态码取值（与 git status --porcelain 的 XY 两位一致，界面按它选标签）。
const (
	GitChangeModified    = "M"
	GitChangeAdded       = "A"
	GitChangeDeleted     = "D"
	GitChangeRenamed     = "R"
	GitChangeCopied      = "C"
	GitChangeTypeChanged = "T"
	GitChangeUntracked   = "?"
	// GitChangeConflict 合并 / 变基 / 拣选留下的未解决冲突（porcelain 的七种 U* 码归一到这里）
	GitChangeConflict = "U"
)

// 仓库中间状态的取值。非 none 时界面要显示横幅并给出「继续 / 中止」，
// 因为这时 git 拒绝大多数其它写入。
const (
	// GitStateNone 普通状态
	GitStateNone = "none"
	// GitStateMerge 合并未完成（MERGE_HEAD 存在）
	GitStateMerge = "merge"
	// GitStateRebase 变基未完成（rebase-merge / rebase-apply 存在）
	GitStateRebase = "rebase"
	// GitStateCherryPick 拣选未完成
	GitStateCherryPick = "cherry-pick"
	// GitStateRevert 回退提交未完成
	GitStateRevert = "revert"
)

// GitFileVO 一个被改动的文件。
type GitFileVO struct {
	// Path 相对工作目录的路径，分隔符统一为斜杠
	Path string `json:"path"`
	// Status 取 GitChange*：M 改过 / A 新增 / D 删除 / R 改名 / C 复制 / T 类型变化 / ? 未跟踪 / U 冲突
	Status string `json:"status"`
	// Staged 区分「已放进待提交区」与「只改了工作区」，两者在界面上是两种处置动作
	Staged bool `json:"staged"`
	// Conflict 该文件有未解决的冲突；解决后要显式标记，否则 git 不接受提交
	Conflict bool `json:"conflict"`
	// Additions / Deletions 增删行数；二进制文件、未跟踪文件与冲突文件按 0 计
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// GitRemoteVO 一个远端及其地址。
type GitRemoteVO struct {
	// Name 远端名（origin / upstream 等）
	Name string `json:"name"`
	// URL 推送地址
	URL string `json:"url"`
}

// GitStatusVO 工作区的版本状态。不可用时 Available=false 且 Reason 说清怎么办——
// 「没装 Git」与「不是仓库」都该让用户看见原因，而不是一个空列表。
type GitStatusVO struct {
	// Available 系统里有没有可执行的 git
	Available bool `json:"available"`
	// Reason 不可用时的一句话原因；可用为空
	Reason string `json:"reason"`
	// IsRepo 工作目录是否在 Git 仓库里
	IsRepo bool `json:"is_repo"`
	// Workspace 实际读取的那个目录（会话级工作目录优先）
	Workspace string `json:"workspace"`
	// RepoRoot 仓库根目录；工作区是仓库子目录时与 Workspace 不同
	RepoRoot string `json:"repo_root"`
	// Branch 当前分支名；空表示还没有第一次提交
	Branch string `json:"branch"`
	// Upstream 当前分支的上游引用（如 origin/main）；没有上游为空
	Upstream string `json:"upstream"`
	// Ahead / Behind 相对上游的领先 / 落后提交数；没有上游时都是 0
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
	// State 仓库中间状态，取 GitState*
	State string `json:"state"`
	// StateDetail 中间状态的一句话说明（合并来源分支 / 变基分支）
	StateDetail string `json:"state_detail"`
	// Remotes 已配置的远端；没有为空数组
	Remotes []GitRemoteVO `json:"remotes"`
	// Files 改动清单；没有改动为空数组而不是 null
	Files []GitFileVO `json:"files"`
	// Conflict 当前是否有未解决的冲突
	Conflict bool `json:"conflict"`
	// Git 系统里的 git 版本串，不可用为空
	Git string `json:"git"`
}

// 差异行的种类取值。
const (
	// GitLineFile 文件头（整份差异里分隔不同文件）
	GitLineFile = "file"
	// GitLineHunk 段头 `@@ -a,b +c,d @@`
	GitLineHunk = "hunk"
	// GitLineAdd 新增行
	GitLineAdd = "add"
	// GitLineDel 删除行
	GitLineDel = "del"
	// GitLineCtx 未改动的上下文行
	GitLineCtx = "ctx"
)

// GitDiffLineVO 差异里的一行；OldNo / NewNo 为 0 表示该侧没有对应行。
type GitDiffLineVO struct {
	// Kind 取 GitLine*
	Kind string `json:"kind"`
	// OldNo 改动前的行号；新增行为 0
	OldNo int `json:"old_no"`
	// NewNo 改动后的行号；删除行为 0
	NewNo int `json:"new_no"`
	// Text 行内容（不含 diff 的 + / - 前缀）；file 与 hunk 行存原始文本
	Text string `json:"text"`
}

// GitDiffVO 一份差异。Truncated 为真时 Dropped 是被丢掉的行数——
// 模型与用户都得知道「后面还有」，否则截断会被当成文件就这么长。
type GitDiffVO struct {
	// Path 相对工作目录的路径；空表示整个工作区
	Path string `json:"path"`
	// Truncated 是否被截断
	Truncated bool `json:"truncated"`
	// Dropped 被丢掉的行数
	Dropped int `json:"dropped"`
	// Lines 行序列；没有改动为空数组而不是 null
	Lines []GitDiffLineVO `json:"lines"`
}
