// 起草提交信息用的素材：改动统计概要与截断后的差异文本（specs/08）。
// 用途是「给模型看」，与给人看的结构化 VO 不同，故单独一处实现。
package git

import (
	"sort"
	"strconv"
	"strings"

	"WorkBaby/backend/domain"
)

// maxBriefFiles 概览里最多列几个文件：列满 500 个对摘要没有帮助，只会挤掉差异片段。
const maxBriefFiles = 40

// Brief 给出 paths 的改动统计；paths 为空表示全部改动。
// 返回空串表示没有可提交的改动——调用方据此告诉用户先入栈。
func Brief(dir string, paths []string) string {
	st, err := Status(dir)
	if err != nil {
		return ""
	}
	files := st.Files
	if len(paths) > 0 {
		want := map[string]bool{}
		for _, p := range paths {
			if p = strings.TrimSpace(p); p != "" {
				want[strings.ReplaceAll(p, "\\", "/")] = true
			}
		}
		kept := make([]domain.GitFileVO, 0, len(files))
		for _, f := range files {
			if want[f.Path] {
				kept = append(kept, f)
			}
		}
		files = kept
	}
	if len(files) == 0 {
		return ""
	}
	// 未跟踪的文件单独计：它们还没进索引，摘要里点出来用户才知道「新文件也会进这次提交」。
	var add, del, untracked int
	names := make([]string, 0, len(files))
	for _, f := range files {
		add += f.Additions
		del += f.Deletions
		if f.Status == domain.GitChangeUntracked {
			untracked++
		}
		names = append(names, "  "+f.Path+" ("+statusWord(f.Status)+")")
	}
	sort.Strings(names)
	if len(names) > maxBriefFiles {
		names = append(names[:maxBriefFiles], "  ……（其余文件略）")
	}
	head := "共 " + strconv.Itoa(len(files)) + " 个文件，+" + strconv.Itoa(add) + " / -" + strconv.Itoa(del)
	if untracked > 0 {
		head += "，其中 " + strconv.Itoa(untracked) + " 个是新文件"
	}
	return head + "\n" + strings.Join(names, "\n")
}

// DiffText 取差异的纯文本（不含行号结构），供起草提示词使用。
// 与给人看的 Diff 分开：后者要截断并回报 dropped，前者只要一段能读懂的文字。
func DiffText(dir string, paths []string) string {
	rel := ""
	if len(paths) == 1 {
		rel = strings.TrimSpace(paths[0])
	}
	vo, err := Diff(dir, rel)
	if err != nil {
		return ""
	}
	if len(vo.Lines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, ln := range vo.Lines {
		switch ln.Kind {
		case domain.GitLineAdd:
			b.WriteString("+ " + ln.Text + "\n")
		case domain.GitLineDel:
			b.WriteString("- " + ln.Text + "\n")
		case domain.GitLineHunk:
			b.WriteString("@@ " + ln.Text + "\n")
		}
	}
	return b.String()
}

// statusWord 把状态码翻成一个词，喂给模型比喂单个字母更好懂。
func statusWord(s string) string {
	switch s {
	case domain.GitChangeModified:
		return "修改"
	case domain.GitChangeAdded:
		return "新增"
	case domain.GitChangeDeleted:
		return "删除"
	case domain.GitChangeRenamed:
		return "改名"
	case domain.GitChangeCopied:
		return "复制"
	case domain.GitChangeUntracked:
		return "新文件"
	case domain.GitChangeConflict:
		return "冲突"
	}
	return s
}
