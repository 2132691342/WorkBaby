package parser

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// Summarize 给一个会话文件出一行摘要：标题、模型、token、消息数。
func Summarize(file string) domain.SessionRowVO {
	header := ReadHeader(file)
	entries, _ := ReadEntries(file)
	row := domain.SessionRowVO{
		SessionId:    header.Id,
		SessionPath:  file,
		SessionCwd:   header.Cwd,
		UpdatedAt:    header.Timestamp,
		SessionTitle: "（空会话）",
	}
	if row.SessionId == "" {
		row.SessionId = strings.TrimSuffix(filepath.Base(file), ".jsonl")
	}
	if st, err := statSize(file); err == nil {
		row.SizeBytes = st
	}
	for _, entry := range entries {
		if entry.EntryTimestamp != "" {
			if ts := ParseTime(entry.EntryTimestamp); ts > row.UpdatedAt {
				row.UpdatedAt = ts
			}
		}
		if msg := entry.EntryMessage; msg != nil {
			if msg.MessageUsage != nil {
				row.TotalTokens += msg.MessageUsage.UsageTotalTokens
				if msg.MessageUsage.UsageCost != nil {
					row.CostUsd += msg.MessageUsage.UsageCost.CostTotal
				}
			}
			switch {
			case msg.MessageRole == roleUser:
				row.MessageCount++
				if row.SessionTitle == "（空会话）" {
					if text := strings.TrimSpace(ContentText(msg.MessageContent)); text != "" {
						row.SessionTitle = pkg.Truncate(text, 80)
					}
				}
			case msg.MessageRole == roleAssistant:
				row.MessageCount++
				if msg.MessageModel != "" {
					row.SessionModel = msg.MessageModel
				}
			case isToolResult(msg.MessageRole):
				row.MessageCount++
			}
		}
		if entry.EntryUsage != nil {
			row.TotalTokens += entry.EntryUsage.UsageTotalTokens
			if entry.EntryUsage.UsageCost != nil {
				row.CostUsd += entry.EntryUsage.UsageCost.CostTotal
			}
		}
		if entry.EntryType == domain.SessionEntryModelChange &&
			entry.EntryProvider != "" && entry.EntryModelId != "" {
			row.SessionModel = entry.EntryProvider + "/" + entry.EntryModelId
		}
		if entry.EntryType == domain.SessionEntrySessionInfo && entry.EntryName != "" {
			row.SessionTitle = entry.EntryName
		}
	}
	return row
}

// statSize 取文件字节数；读不到给 0。
func statSize(file string) (int64, error) {
	info, err := osStat(file)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
