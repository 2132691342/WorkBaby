package functools

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// csvReadTool 解析 CSV 文本为 JSON 数组。
func csvReadTool() tool.Tool {
	return New(
		"csv_read",
		"Parses CSV text into a JSON array. With header, each row becomes an object; otherwise an array of cells.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["csv"],
			"properties": {
				"csv": {"type": "string", "description": "CSV text"},
				"hasHeader": {"type": "boolean", "description": "whether the first row is a header, default false"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				CSV       string `json:"csv"`
				HasHeader bool   `json:"hasHeader"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "csv_read args parse failed", err)
			}
			rows, err := csv.NewReader(strings.NewReader(req.CSV)).ReadAll()
			if err != nil {
				return nil, pkg.Wrap(4004, "csv_read parse failed", err)
			}
			if len(rows) == 0 {
				return []any{}, nil
			}
			if req.HasHeader {
				header := rows[0]
				out := make([]map[string]any, 0, len(rows)-1)
				for _, r := range rows[1:] {
					obj := make(map[string]any, len(header))
					for i, h := range header {
						if i < len(r) {
							obj[h] = r[i]
						}
					}
					out = append(out, obj)
				}
				return out, nil
			}
			out := make([][]string, 0, len(rows))
			for _, r := range rows {
				out = append(out, r)
			}
			return out, nil
		},
	)
}

// csvWriteTool 将 JSON 二维数组写为 CSV 文本。
func csvWriteTool() tool.Tool {
	return New(
		"csv_write",
		"Writes a JSON array of arrays (rows) to CSV text.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["json"],
			"properties": {
				"json": {"type": "string", "description": "JSON array of arrays, e.g. [[\"a\",\"b\"],[\"1\",\"2\"]]"},
				"header": {"type": "string", "description": "optional header row as a JSON array, e.g. [\"name\",\"age\"]"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				JSON   string `json:"json"`
				Header string `json:"header"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "csv_write args parse failed", err)
			}
			var rows [][]string
			if err := json.Unmarshal([]byte(req.JSON), &rows); err != nil {
				return nil, pkg.Wrap(4004, "csv_write invalid rows json", err)
			}
			var buf bytes.Buffer
			w := csv.NewWriter(&buf)
			if req.Header != "" {
				var header []string
				if err := json.Unmarshal([]byte(req.Header), &header); err != nil {
					return nil, pkg.Wrap(4004, "csv_write invalid header json", err)
				}
				if err := w.Write(header); err != nil {
					return nil, pkg.Wrap(4004, "csv_write write header failed", err)
				}
			}
			if err := w.WriteAll(rows); err != nil {
				return nil, pkg.Wrap(4004, "csv_write write rows failed", err)
			}
			w.Flush()
			return buf.String(), nil
		},
	)
}
