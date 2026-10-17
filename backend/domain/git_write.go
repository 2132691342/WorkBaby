// 版本历史与受控写入的域模型：提交元信息、分支、写结果（specs/09）。
package domain

// 版本历史：一次提交的元信息与它改动的文件清单。
// 历史不另建表：实时 git log 是唯一真相源，换分支后历史整块变正是缓存不该存在的原因。

// GitCommitVO 历史列表里的一次提交。
type GitCommitVO struct {
	Hash      string `json:"hash"`      // 完整 hash
	Short     string `json:"short"`     // 短 hash，界面显示用
	Subject   string `json:"subject"`   // 首行提交信息
	Author    string `json:"author"`    //
	When      int64  `json:"when"`      // 提交时间，毫秒
	Files     int    `json:"files"`     // 这次提交涉及的文件数
	Additions int    `json:"additions"` //
	Deletions int    `json:"deletions"` //
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
	Ahead   int    `json:"ahead"`
	Behind  int    `json:"behind"`
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
}

// GitBranchREQ 新建 / 切换分支入参。
type GitBranchREQ struct {
	Name string `json:"name"`
}
