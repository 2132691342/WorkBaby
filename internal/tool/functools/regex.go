package functools

import (
	"context"
	"encoding/json"
	"regexp"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// regexMatchTool 判断文本是否匹配正则（find 语义；任意子串命中即 true）。
func regexMatchTool() tool.Tool {
	return New(
		"regex_match",
		"Test whether the text matches the regular expression.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["pattern", "text"],
			"properties": {
				"pattern": {"type": "string", "description": "regular expression"},
				"text": {"type": "string", "description": "input text"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Pattern string `json:"pattern"`
				Text    string `json:"text"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "regex_match args parse failed", err)
			}
			re, err := regexp.Compile(req.Pattern)
			if err != nil {
				return nil, pkg.Wrap(4004, "regex_match invalid pattern: "+req.Pattern, err)
			}
			return re.MatchString(req.Text), nil
		},
	)
}

// regexExtractTool 提取首个捕获组（无组则返回完整匹配）。
func regexExtractTool() tool.Tool {
	return New(
		"regex_extract",
		"Extract the first capture group (or full match) from the text.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["pattern", "text"],
			"properties": {
				"pattern": {"type": "string", "description": "regular expression with a capture group"},
				"text": {"type": "string", "description": "input text"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Pattern string `json:"pattern"`
				Text    string `json:"text"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "regex_extract args parse failed", err)
			}
			re, err := regexp.Compile(req.Pattern)
			if err != nil {
				return nil, pkg.Wrap(4004, "regex_extract invalid pattern: "+req.Pattern, err)
			}
			m := re.FindStringSubmatch(req.Text)
			if m == nil {
				return nil, nil
			}
			if len(m) > 1 {
				return m[1], nil
			}
			return m[0], nil
		},
	)
}

// regexReplaceTool 替换文本中所有正则匹配。
func regexReplaceTool() tool.Tool {
	return New(
		"regex_replace",
		"Replace all matches of the regex in the text.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["pattern", "replacement", "text"],
			"properties": {
				"pattern": {"type": "string", "description": "regular expression"},
				"replacement": {"type": "string", "description": "replacement string"},
				"text": {"type": "string", "description": "input text"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Pattern     string `json:"pattern"`
				Replacement string `json:"replacement"`
				Text        string `json:"text"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "regex_replace args parse failed", err)
			}
			re, err := regexp.Compile(req.Pattern)
			if err != nil {
				return nil, pkg.Wrap(4004, "regex_replace invalid pattern: "+req.Pattern, err)
			}
			return re.ReplaceAllString(req.Text, req.Replacement), nil
		},
	)
}
