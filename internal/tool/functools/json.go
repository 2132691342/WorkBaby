package functools

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// jsonParseTool 解析并美化输出 JSON。
func jsonParseTool() tool.Tool {
	return New(
		"json_parse",
		"Parses and pretty-prints a JSON string, validating it.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["json"],
			"properties": {
				"json": {"type": "string", "description": "JSON string to parse"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				JSON string `json:"json"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "json_parse args parse failed", err)
			}
			var v any
			if err := json.Unmarshal([]byte(req.JSON), &v); err != nil {
				return nil, pkg.Wrap(4004, "json_parse invalid json", err)
			}
			bs, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return nil, pkg.Wrap(4004, "json_parse marshal failed", err)
			}
			return string(bs), nil
		},
	)
}

// jsonGetTool 按点分路径从 JSON 中取值（支持 a.b.c 与 [0].name）。
func jsonGetTool() tool.Tool {
	return New(
		"json_get",
		"Extracts a value from JSON by a dotted path, e.g. 'user.name'.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["json", "path"],
			"properties": {
				"json": {"type": "string", "description": "JSON string"},
				"path": {"type": "string", "description": "dotted path, e.g. 'a.b.c' or '[0].name'"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				JSON string `json:"json"`
				Path string `json:"path"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "json_get args parse failed", err)
			}
			var v any
			if err := json.Unmarshal([]byte(req.JSON), &v); err != nil {
				return nil, pkg.Wrap(4004, "json_get invalid json", err)
			}
			cur := v
			for _, seg := range splitPath(req.Path) {
				if strings.HasPrefix(seg, "[") && strings.HasSuffix(seg, "]") {
					idx, err := strconv.Atoi(strings.Trim(seg, "[]"))
					if err != nil {
						return nil, pkg.New(4004, "json_get invalid index segment: "+seg, "")
					}
					arr, ok := cur.([]any)
					if !ok || idx < 0 || idx >= len(arr) {
						return nil, nil
					}
					cur = arr[idx]
					continue
				}
				obj, ok := cur.(map[string]any)
				if !ok {
					return nil, nil
				}
				cur, ok = obj[seg]
				if !ok {
					return nil, nil
				}
			}
			return cur, nil
		},
	)
}

// splitPath 解析点分路径：a.b.c → [a b c]；a[0].name → [a [0] name]。
func splitPath(p string) []string {
	p = strings.ReplaceAll(p, "[", ".[")
	var out []string
	for _, seg := range strings.Split(p, ".") {
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// jsonValidateTool 校验字符串是否为合法 JSON。
func jsonValidateTool() tool.Tool {
	return New(
		"json_validate",
		"Validates whether a string is well-formed JSON.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["json"],
			"properties": {
				"json": {"type": "string", "description": "candidate JSON string"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				JSON string `json:"json"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "json_validate args parse failed", err)
			}
			if strings.TrimSpace(req.JSON) == "" {
				return "invalid: empty input", nil
			}
			if json.Valid([]byte(req.JSON)) {
				return "valid", nil
			}
			return "invalid: malformed json", nil
		},
	)
}
