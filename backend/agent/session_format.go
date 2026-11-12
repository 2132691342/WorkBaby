package agent

import (
	"encoding/json"
	"strings"

	"WorkBaby/backend/domain"
)

// messageToDO 把引擎消息翻成落盘形态。
// 落盘形态是 domain 定的那一份：引擎写、回放读，中间不隔一层私货。
func messageToDO(msg Message) *domain.SessionMessageDO {
	out := &domain.SessionMessageDO{
		MessageRole:         string(msg.Role),
		MessageContent:      contentToDO(msg),
		MessageModel:        msg.ModelId,
		MessageProvider:     msg.ProviderName,
		MessageStopReason:   msg.StopReason,
		MessageErrorMessage: msg.ErrorMessage,
		MessageToolCallId:   msg.ToolCallId,
		MessageToolName:     msg.ToolName,
		MessageIsError:      msg.IsError,
		MessageDetails:      msg.Details,
	}
	if msg.Usage != nil {
		out.MessageUsage = &domain.SessionUsageDO{
			UsageInput:       msg.Usage.InputTokens,
			UsageOutput:      msg.Usage.OutputTokens,
			UsageCacheRead:   msg.Usage.CacheReadTokens,
			UsageCacheWrite:  msg.Usage.CacheWriteTokens,
			UsageTotalTokens: msg.Usage.TotalTokens(),
		}
	}
	return out
}

// contentToDO 把消息块翻成落盘用的内容块数组。
// 纯文本走字符串形态：那是回放层读用户消息的主路径，
// 写成数组会让每条用户消息都多一层壳。
func contentToDO(msg Message) any {
	parts := make([]domain.SessionBlockDO, 0, len(msg.Blocks))
	for _, block := range msg.Blocks {
		switch block.Kind {
		case KindText:
			parts = append(parts, domain.SessionBlockDO{
				BlockType: domain.SessionContentText,
				BlockText: block.Text,
			})
		case KindThinking:
			parts = append(parts, domain.SessionBlockDO{
				BlockType:     domain.SessionContentThinking,
				BlockThinking: block.Text,
			})
		case KindToolCall:
			parts = append(parts, domain.SessionBlockDO{
				BlockType:      domain.SessionContentToolCall,
				BlockId:        block.CallId,
				BlockName:      block.ToolName,
				BlockArguments: decodeArgs(block.ToolArgs),
			})
		case KindImage:
			parts = append(parts, domain.SessionBlockDO{
				BlockType: domain.SessionContentImage,
				BlockMime: block.ImageMime,
				BlockData: block.ImageBase64,
			})
		}
	}
	if len(parts) == 0 {
		return ""
	}
	// 助手消息与工具结果都是多块形态；用户消息只有文本时走字符串。
	if msg.Role == RoleUser && len(parts) == 1 && parts[0].BlockType == domain.SessionContentText {
		return parts[0].BlockText
	}
	return parts
}

// contentTextOf 取一段落盘正文的纯文本；形状不认得就按原文给。
func contentTextOf(content any) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []any:
		var parts []string
		for _, item := range typed {
			parts = append(parts, contentTextOf(item))
		}
		return strings.Join(parts, "\n")
	case []domain.SessionBlockDO:
		var parts []string
		for _, block := range typed {
			parts = append(parts, blockTextOf(block))
		}
		return strings.Join(parts, "\n")
	case domain.SessionBlockDO:
		return blockTextOf(typed)
	}
	return ""
}

// blockTextOf 取一个内容块的纯文本。
func blockTextOf(block domain.SessionBlockDO) string {
	switch block.BlockType {
	case domain.SessionContentText:
		return block.BlockText
	case domain.SessionContentThinking:
		return block.BlockThinking
	}
	return ""
}

// MessageFromDO 把落盘形态翻回引擎消息：组装上下文时要用回引擎自己的形状。
// 这一步是必须的——引擎的上下文里是块，文件里是块数组，
// 两边各用自己的形状，往返只在边界这一处发生。
func MessageFromDO(msg *domain.SessionMessageDO) Message {
	out := Message{
		Role:         Role(msg.MessageRole),
		ToolCallId:   msg.MessageToolCallId,
		ToolName:     msg.MessageToolName,
		IsError:      msg.MessageIsError,
		StopReason:   msg.MessageStopReason,
		ErrorMessage: msg.MessageErrorMessage,
		ProviderName: msg.MessageProvider,
		ModelId:      msg.MessageModel,
	}
	for _, block := range blocksOfDO(msg.MessageContent) {
		switch block.BlockType {
		case domain.SessionContentText:
			out.Blocks = append(out.Blocks, Block{Kind: KindText, Text: block.BlockText})
		case domain.SessionContentThinking:
			out.Blocks = append(out.Blocks, Block{Kind: KindThinking, Text: block.BlockThinking})
		case domain.SessionContentToolCall:
			out.Blocks = append(out.Blocks, Block{
				Kind:     KindToolCall,
				CallId:   block.BlockId,
				ToolName: block.BlockName,
				ToolArgs: argsToRaw(block.BlockArguments),
			})
		case domain.SessionContentImage:
			out.Blocks = append(out.Blocks, Block{
				Kind:        KindImage,
				ImageMime:   block.BlockMime,
				ImageBase64: block.BlockData,
			})
		}
	}
	if msg.MessageUsage != nil {
		out.Usage = &Usage{
			InputTokens:      msg.MessageUsage.UsageInput,
			OutputTokens:     msg.MessageUsage.UsageOutput,
			CacheReadTokens:  msg.MessageUsage.UsageCacheRead,
			CacheWriteTokens: msg.MessageUsage.UsageCacheWrite,
		}
	}
	return out
}

// blocksOfDO 把落盘正文解成内容块数组；字符串形态转成一个 text 块。
func blocksOfDO(content any) []domain.SessionBlockDO {
	switch typed := content.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []domain.SessionBlockDO{{BlockType: domain.SessionContentText, BlockText: typed}}
	case []any:
		out := make([]domain.SessionBlockDO, 0, len(typed))
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, domain.SessionBlockDO{
				BlockType:      domain.SessionContentType(stringOfField(block, "type")),
				BlockText:      stringOfField(block, "text"),
				BlockThinking:  stringOfField(block, "thinking"),
				BlockId:        stringOfField(block, "id"),
				BlockName:      stringOfField(block, "name"),
				BlockArguments: block["arguments"],
				BlockMime:      stringOfField(block, "mime"),
				BlockData:      stringOfField(block, "data"),
			})
		}
		return out
	case []domain.SessionBlockDO:
		return typed
	}
	return nil
}

// stringOfField 取一个字符串字段；非字符串给空串。
func stringOfField(block map[string]any, key string) string {
	value, _ := block[key].(string)
	return value
}

// argsToRaw 把工具入参翻回 JSON 原文；形状不认得就当空入参。
func argsToRaw(args any) string {
	if args == nil {
		return ""
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return ""
	}
	return string(raw)
}

// decodeArgs 把工具入参原文解成通用形状；解不开就返回原文，让回放层原样显示。
func decodeArgs(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return raw
	}
	return value
}
