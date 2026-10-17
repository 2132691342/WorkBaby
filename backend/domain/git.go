// 工作区版本状态：只读读取 Git 仓库的改动与差异，供界面「看见改了什么」。
package domain

// 改动文件的状态码取值（与 git status --porcelain 的 XY 两位一致，界面按它选标签）。
const (
	GitChangeModified  = "M"
	GitChangeAdded     = "A"
	GitChangeDeleted   = "D"
	GitChangeRenamed   = "R"
	GitChangeUntracked = "?"
)

// GitFileVO 一个被改动的文件。
type GitFileVO struct {
	// Path 相对工作目录的路径，分隔符统一为斜杠
	Path string `json:"path"`
	// Status 取 GitChange*：M 改过 / A 新增 / D 删除 / R 改名 / ? 还没被 Git 跟踪
	Status string `json:"status"`
	// Staged 区分「已放进待提交区」与「只改了工作区」，两者在界面上是两种处置动作
	Staged bool `json:"staged"`
	// Additions / Deletions 增删行数；二进制文件与未跟踪文件按 0 计
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
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
	// Branch 当前分支名；空表示还没有第一次提交
	Branch string `json:"branch"`
	// Ahead / Behind 相对上游的领先 / 落后提交数；没有上游时都是 0
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
	// Files 改动清单；没有改动为空数组而不是 null
	Files []GitFileVO `json:"files"`
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
