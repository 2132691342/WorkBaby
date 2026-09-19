package functools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// mathEvalTool 求值算术表达式（+ - * / 括号）。
func mathEvalTool() tool.Tool {
	return New(
		"math_eval",
		"Evaluate an arithmetic expression (+ - * / and parentheses).",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["expression"],
			"properties": {
				"expression": {"type": "string", "description": "e.g. '2*(3+4)'"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Expression string `json:"expression"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "math_eval args parse failed", err)
			}
			program, err := expr.Compile(req.Expression)
			if err != nil {
				return nil, pkg.Wrap(4004, "math_eval expression compile failed", err)
			}
			v, err := expr.Run(program, nil)
			if err != nil {
				return nil, pkg.Wrap(4004, "math_eval expression eval failed", err)
			}
			return toNumber(v), nil
		},
	)
}

// mathStatsTool 统计逗号分隔数字的 count/sum/min/max/avg。
func mathStatsTool() tool.Tool {
	return New(
		"math_stats",
		"Compute count/sum/min/max/avg over comma-separated numbers.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["numbers"],
			"properties": {
				"numbers": {"type": "string", "description": "e.g. '1,2,3.5'"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Numbers string `json:"numbers"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "math_stats args parse failed", err)
			}
			var vals []float64
			for _, part := range strings.Split(req.Numbers, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				v, err := strconv.ParseFloat(part, 64)
				if err != nil {
					return nil, pkg.Wrap(4004, "math_stats invalid number: "+part, err)
				}
				vals = append(vals, v)
			}
			if len(vals) == 0 {
				return nil, pkg.New(4004, "math_stats no numbers", "")
			}
			sum := 0.0
			for _, v := range vals {
				sum += v
			}
			minV, maxV := vals[0], vals[0]
			for _, v := range vals[1:] {
				if v < minV {
					minV = v
				}
				if v > maxV {
					maxV = v
				}
			}
			return map[string]any{
				"count": len(vals),
				"sum":   sum,
				"min":   minV,
				"max":   maxV,
				"avg":   sum / float64(len(vals)),
			}, nil
		},
	)
}

// toNumber expr 求值结果归一为数字（int → float64）。
func toNumber(v any) any {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float64:
		return x
	case float32:
		return float64(x)
	case string:
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return f
		}
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
