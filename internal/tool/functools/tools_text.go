package functools

// 文本处理主题：日期时间 / 文本 / 正则。
// 拆出来便于按主题定位；运行期注册走 base.go 的 All()，新增工具只动本文件。

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// ===== datetime =====

func currentTimeTool() tool.Tool {
	return New(
		"current_time",
		"Returns the current date and time.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"properties": {
				"format": {"type": "string", "description": "date format, default '2006-01-02 15:04:05'"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Format string `json:"format"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "current_time args parse failed", err)
			}
			format := req.Format
			if format == "" {
				format = "2006-01-02 15:04:05"
			}
			return time.Now().Format(format), nil
		},
	)
}

func dateAddTool() tool.Tool {
	return New(
		"date_add",
		"Adds (or subtracts) days to a date.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["date"],
			"properties": {
				"date": {"type": "string", "description": "date string, e.g. '2026-08-17'"},
				"days": {"type": "integer", "description": "days to add, negative to subtract; default 0"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Date string `json:"date"`
				Days int    `json:"days"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "date_add args parse failed", err)
			}
			d, err := time.ParseInLocation("2006-01-02", req.Date, time.Local)
			if err != nil {
				return nil, pkg.Wrap(4004, "date_add invalid date: "+req.Date, err)
			}
			return d.AddDate(0, 0, req.Days).Format("2006-01-02"), nil
		},
	)
}

func dateDiffTool() tool.Tool {
	return New(
		"date_diff",
		"Returns the number of days from date1 to date2 (date2 - date1).",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["date1", "date2"],
			"properties": {
				"date1": {"type": "string", "description": "first date, e.g. '2026-08-17'"},
				"date2": {"type": "string", "description": "second date, e.g. '2026-08-20'"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Date1 string `json:"date1"`
				Date2 string `json:"date2"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "date_diff args parse failed", err)
			}
			d1, err := time.ParseInLocation("2006-01-02", req.Date1, time.Local)
			if err != nil {
				return nil, pkg.Wrap(4004, "date_diff invalid date1: "+req.Date1, err)
			}
			d2, err := time.ParseInLocation("2006-01-02", req.Date2, time.Local)
			if err != nil {
				return nil, pkg.Wrap(4004, "date_diff invalid date2: "+req.Date2, err)
			}
			return int(d2.Sub(d1).Hours() / 24), nil
		},
	)
}

// ===== text =====

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

// ===== regex =====

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