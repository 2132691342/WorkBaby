package parser

import (
	"path/filepath"
	"strings"

	"WorkBaby/backend/domain"
	"WorkBaby/backend/pkg"
)

// EncodedWorkspace 把工作区路径编码成 pi 的会话目录名：
// 剥掉一个头部分隔符，/ \ : 空格换成 -，两端包 --。
// 这个形状由引擎决定，改它等于读不到历史会话。
func EncodedWorkspace(workspace string) string {
	stripped := strings.TrimLeft(workspace, `\/`)
	replacer := strings.NewReplacer("/", "-", `\`, "-", ":", "-", " ", "-")
	return "--" + replacer.Replace(stripped) + "--"
}

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
		if entry.Timestamp != "" {
			if ts := ParseTime(entry.Timestamp); ts > row.UpdatedAt {
				row.UpdatedAt = ts
			}
		}
		if msg := entry.Message; msg != nil {
			if msg.Usage != nil {
				row.TotalTokens += msg.Usage.TotalTokens
				if msg.Usage.Cost != nil {
					row.CostUsd += msg.Usage.Cost.Total
				}
			}
			switch msg.Role {
			case "user":
				row.MessageCount++
				if row.SessionTitle == "（空会话）" {
					if text := strings.TrimSpace(msg.Text()); text != "" {
						row.SessionTitle = pkg.Truncate(text, 80)
					}
				}
			case "assistant":
				row.MessageCount++
				if msg.Model != "" {
					row.SessionModel = msg.Model
				}
			case "toolResult":
				row.MessageCount++
			}
		}
		if entry.Usage != nil {
			row.TotalTokens += entry.Usage.TotalTokens
			if entry.Usage.Cost != nil {
				row.CostUsd += entry.Usage.Cost.Total
			}
		}
		if entry.Type == "model_change" && entry.Provider != "" && entry.ModelId != "" {
			row.SessionModel = entry.Provider + "/" + entry.ModelId
		}
		if entry.Type == "session_info" && entry.Name != "" {
			row.SessionTitle = entry.Name
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
