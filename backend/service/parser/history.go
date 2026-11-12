package parser

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// BuildHistory 把活动分支读成历史轮；hidden 里的条目 ID 会被剔除。
func BuildHistory(file string, hidden map[string]struct{}) []domain.HistoryTurnVO {
	entries, err := ReadEntries(file)
	if err != nil {
		return nil
	}
	return HistoryFromEntries(entries, hidden)
}

// HistoryFromEntries 把任意条目集 + 隐藏集转成历史轮。
// 工具调用挂在产生它的助手轮、结果回填进块——否则「调用」与「结果」被拆成两轮；
// 压缩与自定义消息变成 engine-mark 分隔标记：它们是引擎的表达，不是用户说的话。
func HistoryFromEntries(entries []Entry, hidden map[string]struct{}) []domain.HistoryTurnVO {
	turns := make([]domain.HistoryTurnVO, 0, len(entries))
	var assistant *domain.HistoryTurnVO
	toolIds := map[string]struct{}{}
	reset := func() {
		assistant = nil
		toolIds = map[string]struct{}{}
	}
	for _, entry := range ActivePath(entries) {
		switch entry.EntryType {
		case domain.SessionEntryCompaction:
			reset()
			turns = append(turns, domain.HistoryTurnVO{
				TurnRole: "engine-mark",
				TurnBlocks: []domain.ChatBlockVO{{
					BlockKind: domain.BlockText,
					BlockText: "已压缩上下文（压缩前 " + itoa(entry.EntryTokensBefore) + " tokens）",
				}},
				TurnId: entry.EntryId,
			})
			continue
		case domain.SessionEntryCustom:
			// 自定义条目：引擎用它表达自己的状态。
			// 计划模式这类开关不画进对话流——它不是用户说的话，
			// 画出来会让用户以为对话里多了一条莫名其妙的回复。
			if entry.EntryCustomType == "plan-mode" {
				continue
			}
			reset()
			text := customText(entry)
			if text != "" {
				turns = append(turns, domain.HistoryTurnVO{
					TurnRole:   "engine-mark",
					TurnBlocks: []domain.ChatBlockVO{{BlockKind: domain.BlockText, BlockText: text}},
					TurnId:     entry.EntryId,
				})
			}
			continue
		case domain.SessionEntryModelChange, domain.SessionEntryThinkingChange,
			domain.SessionEntryUsage, domain.SessionEntrySessionInfo:
			// 元信息条目：不进对话流，它们的作用已经体现在消息自身字段上。
			continue
		}
		msg := entry.EntryMessage
		if msg == nil {
			continue
		}
		switch {
		case msg.MessageRole == roleUser:
			reset()
			if text := ContentText(msg.MessageContent); strings.TrimSpace(text) != "" {
				turns = append(turns, domain.HistoryTurnVO{
					TurnRole:   "user",
					TurnBlocks: []domain.ChatBlockVO{{BlockKind: domain.BlockText, BlockText: text}},
					TurnId:     entry.EntryId,
				})
			}
		case msg.MessageRole == roleAssistant:
			blocks, ids := blocksOf(msg)
			turn := domain.HistoryTurnVO{
				TurnRole:   "assistant",
				TurnBlocks: blocks,
				TurnModel:  msg.MessageModel,
				TurnUsage:  usageOf(msg.MessageUsage),
				TurnId:     entry.EntryId,
			}
			turns = append(turns, turn)
			// 上面 append 进切片的是值拷贝，后续回填必须写回切片元素。
			assistant = &turns[len(turns)-1]
			toolIds = ids
		case isToolResult(msg.MessageRole):
			applyToolResult(&turns, assistant, toolIds, msg, entry.EntryId)
		}
	}
	return filterHidden(turns, hidden)
}

// customText 取自定义条目的可读文本。
func customText(entry Entry) string {
	if entry.EntryData == nil {
		return ""
	}
	text, _ := entry.EntryData["text"].(string)
	return strings.TrimSpace(text)
}

// blocksOf 把助手消息拆成内容块，同时收集工具调用 ID。
func blocksOf(msg *domain.SessionMessageDO) ([]domain.ChatBlockVO, map[string]struct{}) {
	ids := map[string]struct{}{}
	source := BlocksOfContent(msg.MessageContent)
	blocks := make([]domain.ChatBlockVO, 0, len(source))
	for _, block := range source {
		switch block.BlockType {
		case domain.SessionContentText:
			blocks = append(blocks, domain.ChatBlockVO{BlockKind: domain.BlockText, BlockText: block.BlockText})
		case domain.SessionContentThinking:
			blocks = append(blocks, domain.ChatBlockVO{BlockKind: domain.BlockThinking, BlockText: block.BlockThinking})
		case domain.SessionContentToolCall:
			ids[block.BlockId] = struct{}{}
			blocks = append(blocks, domain.ChatBlockVO{
				BlockKind:  domain.BlockTool,
				ToolCallId: block.BlockId,
				ToolName:   block.BlockName,
				ToolArgs:   block.BlockArguments,
				ToolStatus: "done",
			})
		}
	}
	return blocks, ids
}

// applyToolResult 把工具结果回填到对应块；找不到宿主就单独成轮。
func applyToolResult(turns *[]domain.HistoryTurnVO, assistant *domain.HistoryTurnVO, toolIds map[string]struct{}, msg *domain.SessionMessageDO, entryId string) {
	if _, ok := toolIds[msg.MessageToolCallId]; ok && assistant != nil {
		for i := range assistant.TurnBlocks {
			block := &assistant.TurnBlocks[i]
			if block.BlockKind == domain.BlockTool && block.ToolCallId == msg.MessageToolCallId {
				fillToolResult(block, msg)
				return
			}
		}
	}
	standalone := domain.ChatBlockVO{
		BlockKind:  domain.BlockTool,
		ToolCallId: msg.MessageToolCallId,
		ToolName:   msg.MessageToolName,
	}
	fillToolResult(&standalone, msg)
	*turns = append(*turns, domain.HistoryTurnVO{
		TurnRole:   "tool",
		TurnBlocks: []domain.ChatBlockVO{standalone},
		TurnId:     entryId,
	})
}

// fillToolResult 把工具输出、错误标记与差异行写进块。
func fillToolResult(block *domain.ChatBlockVO, msg *domain.SessionMessageDO) {
	block.ToolOutput = ContentText(msg.MessageContent)
	block.ToolIsError = msg.MessageIsError
	if msg.MessageIsError {
		block.ToolStatus = "error"
	} else {
		block.ToolStatus = "done"
	}
	if msg.MessageDetails != nil {
		if diff, ok := msg.MessageDetails["diff"].(string); ok && diff != "" {
			block.DiffLines = ParseDiff(diff)
		}
	}
}

// usageOf 把引擎用量转成界面读数。
func usageOf(usage *domain.SessionUsageDO) *domain.TurnUsageVO {
	if usage == nil {
		return nil
	}
	out := &domain.TurnUsageVO{
		InputTokens:      usage.UsageInput,
		OutputTokens:     usage.UsageOutput,
		CacheReadTokens:  usage.UsageCacheRead,
		CacheWriteTokens: usage.UsageCacheWrite,
	}
	if usage.UsageCost != nil {
		out.CostUsd = usage.UsageCost.CostTotal
	}
	return out
}

// filterHidden 剔除被隐藏的轮：隐藏台账在壳里，会话文件不动。
func filterHidden(turns []domain.HistoryTurnVO, hidden map[string]struct{}) []domain.HistoryTurnVO {
	if len(hidden) == 0 {
		return turns
	}
	out := turns[:0]
	for _, turn := range turns {
		if turn.TurnId != "" {
			if _, hide := hidden[turn.TurnId]; hide {
				continue
			}
		}
		out = append(out, turn)
	}
	return out
}

// itoa 是 strconv.Itoa 的本地别名，避免为一个数字转换引入额外 import 块。
func itoa(n int) string {
	if n <= 0 {
		return "?"
	}
	digits := make([]byte, 0, 12)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

// DetailItems 把活动分支读成会话详情的逐条消息。
func DetailItems(file string) []domain.SessionItemVO {
	entries, err := ReadEntries(file)
	if err != nil {
		return nil
	}
	items := make([]domain.SessionItemVO, 0, len(entries))
	for _, entry := range ActivePath(entries) {
		msg := entry.EntryMessage
		if msg == nil {
			continue
		}
		item := domain.SessionItemVO{
			ItemRole:  msg.MessageRole,
			CreatedAt: ParseTime(entry.EntryTimestamp),
		}
		switch {
		case msg.MessageRole == roleUser:
			item.ItemKind = "user"
			item.ItemText = ContentText(msg.MessageContent)
		case msg.MessageRole == roleAssistant:
			item.ItemKind = "assistant"
			item.ItemText = ContentText(msg.MessageContent)
			item.ItemModel = msg.MessageModel
			if msg.MessageUsage != nil {
				item.ItemTokens = msg.MessageUsage.UsageTotalTokens
				if msg.MessageUsage.UsageCost != nil {
					item.CostUsd = msg.MessageUsage.UsageCost.CostTotal
				}
			}
		case isToolResult(msg.MessageRole):
			item.ItemKind = "tool"
			item.ItemText = ContentText(msg.MessageContent)
			item.ToolName = msg.MessageToolName
			item.ToolCallId = msg.MessageToolCallId
			item.ToolIsError = msg.MessageIsError
		default:
			continue
		}
		items = append(items, item)
	}
	return items
}

// GuardSession 校验会话文件落在会话根内。
// 所有拿外部传来的路径去读写的接口都必须先过它，否则 ../ 就能读到任意文件。
func GuardSession(sessionsRoot, file string) (string, error) {
	if !pkg.Exists(file) {
		return "", pkg.Err(pkg.CodeNotFound, "会话文件不存在", file)
	}
	if !pkg.Inside(sessionsRoot, file) {
		return "", pkg.Err(pkg.CodeSessionIO, "会话路径越界", file)
	}
	abs, err := absPath(file)
	if err != nil {
		return "", pkg.WrapErr(pkg.CodeSessionIO, "会话路径非法", err)
	}
	return abs, nil
}

// absPath 取绝对路径；相对路径在 Windows 上没有意义，所以这里只做归一。
func absPath(path string) (string, error) {
	return filepath.Abs(path)
}

// decodeArgs 把工具入参解成通用形状；解不开就原样给字符串。
func decodeArgs(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}
