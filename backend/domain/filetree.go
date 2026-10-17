// 工作区文件浏览：前端 @ 引用文件的目录列举出参。
package domain

// FileEntryVO 目录里的一个条目。
type FileEntryVO struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
}

// FileListVO 一次目录列举的出参。Path 是相对工作区的目录路径，空串即根目录。
type FileListVO struct {
	Path    string        `json:"path"`
	Entries []FileEntryVO `json:"entries"`
}
