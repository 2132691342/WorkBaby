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
		switch entry.Type {
		case "compaction":
			reset()
			turns = append(turns, domain.HistoryTurnVO{
				TurnRole: "engine-mark",
				TurnBlocks: []domain.ChatBlockVO{{
					BlockKind: domain.BlockText,
					BlockText: "已压缩上下文（压缩前 " + itoa(entry.TokensBefore) + " tokens）",
				}},
				TurnId: entry.Id,
			})
			continue
		case "custom_message":
			reset()
			text := entry.Content
			if entry.Display && strings.TrimSpace(text) != "" {
				turns = append(turns, domain.HistoryTurnVO{
					TurnRole:   "engine-mark",
					TurnBlocks: []domain.ChatBlockVO{{BlockKind: domain.BlockText, BlockText: text}},
					TurnId:     entry.Id,
				})
			}
			continue
		}
		msg := entry.Message
		if msg == nil {
			continue
		}
		switch msg.Role {
		case "user":
			reset()
			if text := msg.Text(); strings.TrimSpace(text) != "" {
				turns = append(turns, domain.HistoryTurnVO{
					TurnRole:   "user",
					TurnBlocks: []domain.ChatBlockVO{{BlockKind: domain.BlockText, BlockText: text}},
					TurnId:     entry.Id,
				})
			}
		case "assistant":
			blocks, ids := blocksOf(msg)
			turn := domain.HistoryTurnVO{
				TurnRole:   "assistant",
				TurnBlocks: blocks,
				TurnModel:  msg.Model,
				TurnUsage:  usageOf(msg.Usage),
				TurnId:     entry.Id,
			}
			assistant = &turn
			toolIds = ids
			turns = append(turns, turn)
			// 上面 append 进切片的是值拷贝，后续回填必须写回切片元素。
			assistant = &turns[len(turns)-1]
		case "toolResult":
			applyToolResult(&turns, assistant, toolIds, msg, entry.Id)
		}
	}
	return filterHidden(turns, hidden)
}

// blocksOf 把助手消息拆成内容块，同时收集工具调用 ID。
func blocksOf(msg *Message) ([]domain.ChatBlockVO, map[string]struct{}) {
	ids := map[string]struct{}{}
	source := msg.Blocks()
	blocks := make([]domain.ChatBlockVO, 0, len(source))
	for _, block := range source {
		switch block.Type {
		case "text":
			blocks = append(blocks, domain.ChatBlockVO{BlockKind: domain.BlockText, BlockText: block.Text})
		case "thinking":
			blocks = append(blocks, domain.ChatBlockVO{BlockKind: domain.BlockThinking, BlockText: block.Thinking})
		case "toolCall":
			ids[block.Id] = struct{}{}
			blocks = append(blocks, domain.ChatBlockVO{
				BlockKind:  domain.BlockTool,
				ToolCallId: block.Id,
				ToolName:   block.Name,
				ToolArgs:   decodeArgs(block.Arguments),
				ToolStatus: "done",
			})
		}
	}
	return blocks, ids
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

// applyToolResult 把工具结果回填到对应块；找不到宿主就单独成轮。
func applyToolResult(turns *[]domain.HistoryTurnVO, assistant *domain.HistoryTurnVO, toolIds map[string]struct{}, msg *Message, entryId string) {
	if _, ok := toolIds[msg.ToolCallId]; ok && assistant != nil {
		for i := range assistant.TurnBlocks {
			block := &assistant.TurnBlocks[i]
			if block.BlockKind == domain.BlockTool && block.ToolCallId == msg.ToolCallId {
				fillToolResult(block, msg)
				return
			}
		}
	}
	standalone := domain.ChatBlockVO{
		BlockKind:  domain.BlockTool,
		ToolCallId: msg.ToolCallId,
		ToolName:   msg.ToolName,
	}
	fillToolResult(&standalone, msg)
	*turns = append(*turns, domain.HistoryTurnVO{
		TurnRole:   "tool",
		TurnBlocks: []domain.ChatBlockVO{standalone},
		TurnId:     entryId,
	})
}

// fillToolResult 把工具输出、错误标记与差异行写进块。
func fillToolResult(block *domain.ChatBlockVO, msg *Message) {
	block.ToolOutput = msg.Text()
	block.ToolIsError = msg.IsError
	if msg.IsError {
		block.ToolStatus = "error"
	} else {
		block.ToolStatus = "done"
	}
	if msg.Details != nil {
		if diff, ok := msg.Details["diff"].(string); ok && diff != "" {
			block.DiffLines = ParseDiff(diff)
		}
	}
}

// usageOf 把引擎用量转成界面读数。
func usageOf(usage *Usage) *domain.TurnUsageVO {
	if usage == nil {
		return nil
	}
	out := &domain.TurnUsageVO{
		InputTokens:      usage.Input,
		OutputTokens:     usage.Output,
		CacheReadTokens:  usage.CacheRead,
		CacheWriteTokens: usage.CacheWrite,
	}
	if usage.Cost != nil {
		out.CostUsd = usage.Cost.Total
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
		msg := entry.Message
		if msg == nil {
			continue
		}
		item := domain.SessionItemVO{
			ItemRole:  msg.Role,
			CreatedAt: ParseTime(entry.Timestamp),
		}
		switch msg.Role {
		case "user":
			item.ItemKind = "user"
			item.ItemText = msg.Text()
		case "assistant":
			item.ItemKind = "assistant"
			item.ItemText = msg.Text()
			item.ItemModel = msg.Model
			if msg.Usage != nil {
				item.ItemTokens = msg.Usage.TotalTokens
				if msg.Usage.Cost != nil {
					item.CostUsd = msg.Usage.Cost.Total
				}
			}
		case "toolResult":
			item.ItemKind = "tool"
			item.ItemText = msg.Text()
			item.ToolName = msg.ToolName
			item.ToolCallId = msg.ToolCallId
			item.ToolIsError = msg.IsError
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
