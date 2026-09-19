package service

// 完成度证据（unbacked claim 治理）：回复里声明的产出文件路径必须能在本 run 的 file_changes
// 中找到对应记录，否则判为幻觉（工具被拒绝等同没执行）。

import (
	"regexp"
	"strings"

	"WorkBaby/internal/llm"
)

// artifactClaimRe 匹配「已产出文件」类声明：完成标记 + 文件扩展名同句相邻出现。
// 保留为更宽的版本（向后兼容），路径提取走 artifactPathRe。
var artifactClaimRe = regexp.MustCompile(
	`(?i)((已|已经|成功)[^。\n]{0,12}(创建|生成|保存|写入|写出|导出|输出|制作)|(created|generated|saved|wrote|exported))` +
		`[^。\n]{0,80}\.(pptx|docx|xlsx|pdf|md|txt|csv|html|htm|zip|png|jpe?g|json|mp4|mp3)`)

// artifactPathRe 提取声明中的具体文件名（含后缀）。优先精确匹配文件名本身而非
// 扩展名——这是证据核对的关键：声明 .pdf 不算证据，声明 report.pdf 才是。
var artifactPathRe = regexp.MustCompile(
	`(?i)([\w\-\.\(\)一-龥]+\.(?:pptx|docx|xlsx|pdf|md|txt|csv|html|htm|zip|png|jpe?g|json|mp4|mp3))`)

// claimsArtifact 判断回复是否包含「已产出文件」类声明。
func claimsArtifact(content string) bool {
	return content != "" && artifactClaimRe.MatchString(content)
}

// claimedPaths 从声明里抽取所有提到的文件名（含后缀），小写归一去前后缀空白。
func claimedPaths(content string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range artifactPathRe.FindAllStringSubmatch(content, -1) {
		p := strings.ToLower(strings.TrimSpace(m[1]))
		if p != "" {
			out[p] = struct{}{}
		}
	}
	return out
}

// writeToolNames 写文件类工具（兜底：声明里没抽到路径，但整轮有写工具且产生过变更时也算证据）。
var writeToolNames = map[string]struct{}{
	"file_write":      {},
	"file_edit":       {},
	"archive_manager": {},
}

// evidenceForClaim 核对声明与产物清单的覆盖关系：
//   - claimed 为空：返回 true（无文件声明，无需核对——claimsArtifact 已先筛过含声明）
//   - pathHits > 0：至少一条声明路径在本 run 的 file_changes 命中 → 证据齐备
//   - pathHits == 0 但 writeTools > 0 且 hasChanges：变更在 worktree 但路径未抽到（泛指）→ 接受
//   - 其他：未找到任何产物证据 → false
func evidenceForClaim(claimed map[string]struct{}, calls []llm.ToolCall, changes []changeEvidence) bool {
	if len(claimed) == 0 {
		return true
	}
	hit := 0
	for p := range claimed {
		for _, c := range changes {
			if !c.Refused && strings.EqualFold(c.Path, p) {
				hit++
				break
			}
		}
	}
	if hit > 0 {
		return true
	}
	if len(changes) == 0 {
		return false
	}
	hasWriteTool := false
	for _, c := range calls {
		if _, ok := writeToolNames[c.Function.Name]; ok {
			hasWriteTool = true
			break
		}
	}
	if !hasWriteTool {
		return false
	}
	// 写工具存在且产物落库（即便路径未在声明里精确匹配）：接受「泛指」证据
	realChange := false
	for _, c := range changes {
		if !c.Refused {
			realChange = true
			break
		}
	}
	return realChange
}

// changeEvidence 完成度核对用的最小变更视图（service 装配：file_changes 行 + refused 标记）。
type changeEvidence struct {
	Path    string
	Refused bool
}
