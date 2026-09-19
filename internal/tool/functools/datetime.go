package functools

import (
	"context"
	"encoding/json"
	"time"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// currentTimeTool 返回当前日期时间。
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

// dateAddTool 对日期增加（或减少）天数。
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

// dateDiffTool 返回两日期相隔天数（date2 - date1）。
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
