package parser

import (
	"encoding/json"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Entry 是会话文件里的一条条目；字段名与引擎落盘格式一致。
type Entry = domain.SessionEntryDO

// Usage 是引擎的用量对象。
type Usage = domain.SessionUsageDO

// ContentBlock 是消息内容块。
type ContentBlock = domain.SessionBlockDO

// ReadEntries 读全部条目；首行的会话头不算树节点。
// 坏行直接跳过：中途断电会留下残行，一个残行不该让整份历史读不出来。
func ReadEntries(file string) ([]Entry, error) {
	lines, err := pkg.ReadSessionLines(file)
	if err != nil {
		return nil, pkg.WrapErr(pkg.CodeSessionIO, "会话文件读取失败", err)
	}
	out := make([]Entry, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(trimmed), &entry); err != nil {
			continue
		}
		if entry.EntryType == "" || entry.EntryType == "session" {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

// Header 是会话文件的头。
type Header struct {
	// Id 会话 ID
	Id string
	// Cwd 会话绑定的工作目录
	Cwd string
	// Timestamp 创建时刻（毫秒）
	Timestamp int64
}

// ReadHeader 读首行会话头；读不到给零值。
func ReadHeader(file string) Header {
	lines, err := pkg.ReadSessionLines(file)
	if err != nil {
		return Header{}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "{") {
			continue
		}
		var head domain.SessionHeaderDO
		if err := json.Unmarshal([]byte(trimmed), &head); err != nil || head.HeaderType != "session" {
			return Header{}
		}
		return Header{Id: head.HeaderId, Cwd: head.HeaderCwd, Timestamp: ParseTime(head.HeaderTimestamp)}
	}
	return Header{}
}

// ActivePath 取活动分支：从最后一条条目沿父指针走到根，再反转成时间序。
// 环状引用（坏文件）由 guard 集合兜住，绝不无限循环。
func ActivePath(entries []Entry) []Entry {
	if len(entries) == 0 {
		return nil
	}
	byId := make(map[string]Entry, len(entries))
	for _, entry := range entries {
		byId[entry.EntryId] = entry
	}
	chain := make([]Entry, 0, len(entries))
	guard := make(map[string]struct{}, len(entries))
	cur, ok := byId[entries[len(entries)-1].EntryId]
	for ok {
		if _, seen := guard[cur.EntryId]; seen {
			break
		}
		guard[cur.EntryId] = struct{}{}
		chain = append(chain, cur)
		if cur.EntryParentId == "" {
			break
		}
		cur, ok = byId[cur.EntryParentId]
	}
	// 反转成时间序。
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// BlocksOfContent 把落盘正文解成内容块数组；字符串形态转成一个 text 块。
func BlocksOfContent(content any) []ContentBlock {
	switch typed := content.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []ContentBlock{{BlockType: domain.SessionContentText, BlockText: typed}}
	case []any:
		out := make([]ContentBlock, 0, len(typed))
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, ContentBlock{
				BlockType:      domain.SessionContentType(str(block["type"])),
				BlockText:      str(block["text"]),
				BlockThinking:  str(block["thinking"]),
				BlockId:        str(block["id"]),
				BlockName:      str(block["name"]),
				BlockArguments: block["arguments"],
				BlockMime:      str(block["mime"]),
				BlockData:      str(block["data"]),
			})
		}
		return out
	case []ContentBlock:
		return typed
	}
	return nil
}

// ContentText 取一段落盘正文的纯文本。
// 内容块数组的元素是 map：每一项先解成块再取文字，
// 直接递归 map 会得到空串——那正是工具输出“消失”的原因。
func ContentText(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, block := range BlocksOfContent(typed) {
			parts = append(parts, BlockText(block))
		}
		return strings.Join(parts, "\n")
	case []ContentBlock:
		parts := make([]string, 0, len(typed))
		for _, block := range typed {
			parts = append(parts, BlockText(block))
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// BlockText 取一个内容块的纯文本。
func BlockText(block ContentBlock) string {
	switch block.BlockType {
	case domain.SessionContentText:
		return block.BlockText
	case domain.SessionContentThinking:
		return block.BlockThinking
	}
	return ""
}

// str 安全取字符串；非字符串给空串。
func str(value any) string {
	text, _ := value.(string)
	return text
}
