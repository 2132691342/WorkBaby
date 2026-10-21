// 版本历史与写操作：提交查询、分支查询、受控写入动作的入参与回执。
package domain

// GitCommitVO 历史列表里的一次提交。
type GitCommitVO struct {
	Hash      string `json:"hash"`      // 完整 hash
	Short     string `json:"short"`     // 短 hash，界面显示用
	Subject   string `json:"subject"`   // 首行提交信息
	Author    string `json:"author"`    // 作者名
	When      int64  `json:"when"`      // 提交时间，毫秒
	Files     int    `json:"files"`     // 这次提交涉及的文件数
	Additions int    `json:"additions"` // 新增行数
	Deletions int    `json:"deletions"` // 删除行数
}

// GitCommitDetailVO 一次提交的完整内容：元信息 + 每个文件 + 逐行差异。
type GitCommitDetailVO struct {
	Hash    string      `json:"hash"`
	Short   string      `json:"short"`
	Subject string      `json:"subject"`
	Body    string      `json:"body"`
	Author  string      `json:"author"`
	When    int64       `json:"when"`
	Parents []string    `json:"parents"`
	Files   []GitFileVO `json:"files"`
	Diff    *GitDiffVO  `json:"diff,omitempty"`
}

// GitBranchVO 一个分支及其相对上游的领先 / 落后数。
type GitBranchVO struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	// Remote 分支所属远端名；本地分支为空
	Remote string `json:"remote"`
	// Remote 分支名用 Remote=true 区分：同名本地分支与远端分支是两条记录
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`
}

// GitCommitMsgLimit 提交信息的字符上限：git 本身不设限，但一条几万字的信息在历史里读不了。
const GitCommitMsgLimit = 5000

// GitCommitRESP 一次提交的结果：写后重读的状态 + 新提交的 hash。
// 返回重读后的状态是为了让界面用一份数据刷新列表，而不是前端自己推断「刚才那个文件现在在哪」。
type GitCommitRESP struct {
	Status *GitStatusVO `json:"status"`
	Hash   string       `json:"hash"`
}

// GitMessageRESP 助手起草的提交信息。Err 非空表示起草失败，界面回落到手写。
type GitMessageRESP struct {
	Message string `json:"message"`
	Err     string `json:"err"`
}

// GitPathsREQ 受控写入的公共入参：要操作哪些文件。空表示全部改动。
type GitPathsREQ struct {
	Paths []string `json:"paths"`
}

// GitCommitREQ 提交入参：信息由用户填，助手只起草不代填。
type GitCommitREQ struct {
	Message string `json:"message"`
	// Amend 把这次提交改写成上一次提交（未推送时才该用，界面二次确认）
	Amend bool `json:"amend"`
}

// GitBranchREQ 新建 / 切换 / 合并 / 变基分支入参。
type GitBranchREQ struct {
	Name string `json:"name"`
	// Remote 一并取远端分支（切分支 / 合并时才有意义）
	Remote bool `json:"remote"`
	// Rebase 合并时用变基而不是生成合并提交
	Rebase bool `json:"rebase"`
}

// GitPushREQ 推送入参。没有上游时第一次推送要指定远端并建立跟踪。
type GitPushREQ struct {
	// Remote 远端名；为空时用当前分支的上游，再没有就用唯一远端
	Remote string `json:"remote"`
	// Branch 推送的本地分支；为空时用当前分支
	Branch string `json:"branch"`
	// SetUpstream 把这次推送的远端建立为上游（首次推送必须）
	SetUpstream bool `json:"set_upstream"`
	// Force 强制推送级别，取 GitForce*；none 是普通推送
	Force string `json:"force"`
	// Confirm 强制推送时用户输入的分支名，必须与 Branch 完全一致
	Confirm string `json:"confirm"`
}

// 推送的强制级别。默认 none，逐级需要更多确认。
const (
	// GitForceNone 普通推送：远端有本地没有的提交时会被拒绝
	GitForceNone = "none"
	// GitForceLease 带租约的强制推送：远端不再是上次拉取的那个提交就拒绝，只覆盖自己推过的历史
	GitForceLease = "lease"
	// GitForceTrue 无条件强制推送：会静默覆盖别人的提交，必须输分支名确认
	GitForceTrue = "force"
)

// GitPullREQ 拉取入参。
type GitPullREQ struct {
	// Remote 远端名；为空时用上游
	Remote string `json:"remote"`
	// Branch 远端分支；为空时用上游
	Branch string `json:"branch"`
	// Rebase 用变基而不是合并
	Rebase bool `json:"rebase"`
}

// GitInitREQ 初始化仓库入参。
type GitInitREQ struct {
	// Branch 初始分支名；为空用 git 默认
	Branch string `json:"branch"`
}

// GitRemoteREQ 添加远端入参。
type GitRemoteREQ struct {
	// Name 远端名，只允许字母数字与 . _ -
	Name string `json:"name"`
	// URL 远端地址，必须是 git 认得的形态（https / ssh / 本地路径）
	URL string `json:"url"`
}

// GitStateREQ 继续 / 中止一个未完成的合并或变基。
type GitStateREQ struct {
	// Action 取 GitStateAction*
	Action string `json:"action"`
}

// 中间状态的处置动作。
const (
	// GitStateContinue 解决冲突后继续
	GitStateContinue = "continue"
	// GitStateAbort 放弃这次合并 / 变基，回到开始前的状态
	GitStateAbort = "abort"
)

// GitResolveREQ 把冲突文件标记为已解决（等同 git add，但不表态提交）。
type GitResolveREQ struct {
	// Paths 要标记的文件；空表示全部冲突文件
	Paths []string `json:"paths"`
}
