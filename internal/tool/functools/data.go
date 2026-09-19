package functools

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// dataCleanTool 清理 JSON 对象数组（去空白与空字段）。
func dataCleanTool() tool.Tool {
	return New(
		"data_clean",
		"Cleans a JSON array of objects: trims strings and optionally drops null/empty fields and empty objects.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["json"],
			"properties": {
				"json": {"type": "string", "description": "JSON array of objects"},
				"dropEmpty": {"type": "boolean", "description": "drop null/empty fields, default true"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				JSON      string `json:"json"`
				DropEmpty bool   `json:"dropEmpty"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "data_clean args parse failed", err)
			}
			drop := !req.DropEmpty // 默认 true；显式传 false 才不删
			var arr []map[string]any
			if err := json.Unmarshal([]byte(req.JSON), &arr); err != nil {
				return nil, pkg.Wrap(4004, "data_clean invalid json", err)
			}
			out := make([]map[string]any, 0, len(arr))
			for _, obj := range arr {
				cleaned := make(map[string]any, len(obj))
				for k, v := range obj {
					switch x := v.(type) {
					case nil:
						if drop {
							continue
						}
						cleaned[k] = v
					case string:
						trimmed := strings.TrimSpace(x)
						if drop && trimmed == "" {
							continue
						}
						cleaned[k] = trimmed
					case []any:
						if drop && len(x) == 0 {
							continue
						}
						cleaned[k] = v
					default:
						cleaned[k] = v
					}
				}
				if !drop || len(cleaned) > 0 {
					out = append(out, cleaned)
				}
			}
			return out, nil
		},
	)
}

// dataAggregateTool 按字段分组并聚合数值字段。
func dataAggregateTool() tool.Tool {
	return New(
		"data_aggregate",
		"Groups a JSON array by a key field and aggregates another numeric field with op (sum/avg/count/min/max).",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["json", "groupBy", "field"],
			"properties": {
				"json": {"type": "string", "description": "JSON array of objects"},
				"groupBy": {"type": "string", "description": "field to group by"},
				"field": {"type": "string", "description": "numeric field to aggregate"},
				"op": {"type": "string", "description": "sum/avg/count/min/max, default sum"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				JSON    string `json:"json"`
				GroupBy string `json:"groupBy"`
				Field   string `json:"field"`
				Op      string `json:"op"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "data_aggregate args parse failed", err)
			}
			var arr []map[string]any
			if err := json.Unmarshal([]byte(req.JSON), &arr); err != nil {
				return nil, pkg.Wrap(4004, "data_aggregate invalid json", err)
			}
			groups := map[string][]float64{}
			order := []string{}
			for _, obj := range arr {
				key, _ := obj[req.GroupBy].(string)
				if _, ok := groups[key]; !ok {
					order = append(order, key)
				}
				v, _ := toFloat(obj[req.Field])
				groups[key] = append(groups[key], v)
			}
			op := req.Op
			if op == "" {
				op = "sum"
			}
			out := make([]map[string]any, 0, len(order))
			for _, k := range order {
				vals := groups[k]
				out = append(out, map[string]any{
					"group": k,
					"value": aggregate(vals, op),
				})
			}
			return out, nil
		},
	)
}

// dataValidateTool 校验数组内对象是否包含必填字段。
func dataValidateTool() tool.Tool {
	return New(
		"data_validate",
		"Validates that each object in a JSON array has the required fields.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["json", "required"],
			"properties": {
				"json": {"type": "string", "description": "JSON array of objects"},
				"required": {"type": "string", "description": "comma-separated required field names"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				JSON     string `json:"json"`
				Required string `json:"required"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "data_validate args parse failed", err)
			}
			var arr []map[string]any
			if err := json.Unmarshal([]byte(req.JSON), &arr); err != nil {
				return nil, pkg.Wrap(4004, "data_validate invalid json", err)
			}
			var required []string
			for _, f := range strings.Split(req.Required, ",") {
				if f = strings.TrimSpace(f); f != "" {
					required = append(required, f)
				}
			}
			invalid := 0
			errors := []map[string]any{}
			for i, obj := range arr {
				var missing []string
				for _, f := range required {
					if _, ok := obj[f]; !ok {
						missing = append(missing, f)
					}
				}
				if len(missing) > 0 {
					invalid++
					errors = append(errors, map[string]any{"index": i, "missing": missing})
				}
			}
			return map[string]any{
				"valid":   invalid == 0,
				"total":   len(arr),
				"invalid": invalid,
				"errors":  errors,
			}, nil
		},
	)
}

// toFloat 数值字段转为 float64（int/float/字符串数字）。
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

// aggregate 聚合一组数值。
func aggregate(vals []float64, op string) float64 {
	if len(vals) == 0 {
		return 0
	}
	switch strings.ToLower(op) {
	case "avg":
		return round4(sum(vals) / float64(len(vals)))
	case "count":
		return float64(len(vals))
	case "min":
		m := vals[0]
		for _, v := range vals[1:] {
			if v < m {
				m = v
			}
		}
		return m
	case "max":
		m := vals[0]
		for _, v := range vals[1:] {
			if v > m {
				m = v
			}
		}
		return m
	default:
		return sum(vals)
	}
}

func sum(vals []float64) float64 {
	s := 0.0
	for _, v := range vals {
		s += v
	}
	return s
}

func round4(v float64) float64 {
	return float64(int64(v*10000+0.5)) / 10000
}
