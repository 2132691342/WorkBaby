package functools

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// textReplaceTool 用替换串替换文本中的正则匹配（支持 $1 组引用）。
func textReplaceTool() tool.Tool {
	return New(
		"text_replace",
		"Replaces all matches of a regex in text with a replacement.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["text", "regex"],
			"properties": {
				"text": {"type": "string", "description": "input text"},
				"regex": {"type": "string", "description": "regular expression to match"},
				"replacement": {"type": "string", "description": "replacement string, supports $1 group refs"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Text        string `json:"text"`
				Regex       string `json:"regex"`
				Replacement string `json:"replacement"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "text_replace args parse failed", err)
			}
			re, err := regexp.Compile(req.Regex)
			if err != nil {
				return nil, pkg.Wrap(4004, "text_replace invalid regex: "+req.Regex, err)
			}
			return re.ReplaceAllString(req.Text, req.Replacement), nil
		},
	)
}

// textCountTool 统计文本的字符数、单词数与行数。
func textCountTool() tool.Tool {
	return New(
		"text_count",
		"Counts characters, words and lines in text.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["text"],
			"properties": {
				"text": {"type": "string", "description": "input text"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "text_count args parse failed", err)
			}
			chars := len([]rune(req.Text))
			lines := 0
			if req.Text != "" {
				lines = len(strings.Split(strings.ReplaceAll(req.Text, "\r\n", "\n"), "\n"))
			}
			words := 0
			if trimmed := strings.TrimSpace(req.Text); trimmed != "" {
				words = len(strings.Fields(trimmed))
			}
			return map[string]any{"characters": chars, "words": words, "lines": lines}, nil
		},
	)
}

// textExtractTool 提取文本中所有正则捕获组为 JSON 数组。
func textExtractTool() tool.Tool {
	return New(
		"text_extract",
		"Extracts all regex matches from text, returning a JSON array.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["text", "regex"],
			"properties": {
				"text": {"type": "string", "description": "input text"},
				"regex": {"type": "string", "description": "regular expression with a capture group"},
				"group": {"type": "integer", "description": "capture group index, default 1"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Text  string `json:"text"`
				Regex string `json:"regex"`
				Group int    `json:"group"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "text_extract args parse failed", err)
			}
			if req.Group <= 0 {
				req.Group = 1
			}
			re, err := regexp.Compile(req.Regex)
			if err != nil {
				return nil, pkg.Wrap(4004, "text_extract invalid regex: "+req.Regex, err)
			}
			matches := re.FindAllStringSubmatch(req.Text, -1)
			out := make([]string, 0, len(matches))
			for _, m := range matches {
				if req.Group < len(m) {
					out = append(out, m[req.Group])
				} else if len(m) > 0 {
					out = append(out, m[0])
				}
			}
			return out, nil
		},
	)
}
