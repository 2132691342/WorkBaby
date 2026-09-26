package functools

// 数据处理主题：数学 / JSON / CSV / 哈希 / 编码 / 数据聚合 / IP 查询。
// 拆出来便于按主题定位；运行期注册走 base.go 的 All()，新增工具只动本文件。

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// ===== math =====

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

// ===== json =====

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

// ===== csv =====

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
				"has_header": {"type": "boolean", "description": "whether the first row is a header, default false"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				CSV       string `json:"csv"`
				HasHeader bool   `json:"has_header"`
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

// ===== hash =====

func hashMd5Tool() tool.Tool {
	return New(
		"hash_md5",
		"Returns the hex MD5 digest of the input text.",
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
				return nil, pkg.Wrap(4004, "hash_md5 args parse failed", err)
			}
			sum := md5.Sum([]byte(req.Text))
			return hex.EncodeToString(sum[:]), nil
		},
	)
}

func hashSha256Tool() tool.Tool {
	return New(
		"hash_sha256",
		"Returns the hex SHA digest of the input text. Algorithm may be SHA-1/SHA-256/SHA-512 (default SHA-256).",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["text"],
			"properties": {
				"text": {"type": "string", "description": "input text"},
				"algorithm": {"type": "string", "description": "SHA-1/SHA-256/SHA-512, default SHA-256"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Text      string `json:"text"`
				Algorithm string `json:"algorithm"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "hash_sha256 args parse failed", err)
			}
			data := []byte(req.Text)
			switch strings.ToUpper(req.Algorithm) {
			case "SHA-1":
				sum := sha1.Sum(data)
				return hex.EncodeToString(sum[:]), nil
			case "SHA-512":
				sum := sha512.Sum512(data)
				return hex.EncodeToString(sum[:]), nil
			default:
				sum := sha256.Sum256(data)
				return hex.EncodeToString(sum[:]), nil
			}
		},
	)
}

func hashHmacTool() tool.Tool {
	return New(
		"hash_hmac",
		"HMAC-SHA256 hex digest of the text using the given key.",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["key", "text"],
			"properties": {
				"key": {"type": "string", "description": "secret key"},
				"text": {"type": "string", "description": "input text"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Key  string `json:"key"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "hash_hmac args parse failed", err)
			}
			mac := hmac.New(sha256.New, []byte(req.Key))
			mac.Write([]byte(req.Text))
			return hex.EncodeToString(mac.Sum(nil)), nil
		},
	)
}

// ===== encode =====

func base64EncodeTool() tool.Tool {
	return New(
		"base64_encode",
		"Encodes text to Base64 (UTF-8).",
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
				return nil, pkg.Wrap(4004, "base64_encode args parse failed", err)
			}
			return base64.StdEncoding.EncodeToString([]byte(req.Text)), nil
		},
	)
}

func base64DecodeTool() tool.Tool {
	return New(
		"base64_decode",
		"Decodes Base64 back to text (UTF-8).",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["base64"],
			"properties": {
				"base64": {"type": "string", "description": "base64-encoded string"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Base64 string `json:"base64"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "base64_decode args parse failed", err)
			}
			bs, err := base64.StdEncoding.DecodeString(req.Base64)
			if err != nil {
				return nil, pkg.Wrap(4004, "base64_decode failed", err)
			}
			return string(bs), nil
		},
	)
}

func urlEncodeTool() tool.Tool {
	return New(
		"url_encode",
		"Percent-encodes a URL component (UTF-8).",
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
				return nil, pkg.Wrap(4004, "url_encode args parse failed", err)
			}
			return url.QueryEscape(req.Text), nil
		},
	)
}

func urlDecodeTool() tool.Tool {
	return New(
		"url_decode",
		"Percent-decodes a URL component (UTF-8).",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["text"],
			"properties": {
				"text": {"type": "string", "description": "percent-encoded text"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "url_decode args parse failed", err)
			}
			decoded, err := url.QueryUnescape(req.Text)
			if err != nil {
				return nil, pkg.Wrap(4004, "url_decode failed", err)
			}
			return decoded, nil
		},
	)
}

// ===== data =====

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
				"drop_empty": {"type": "boolean", "description": "drop null/empty fields, default true"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				JSON      string `json:"json"`
				DropEmpty bool   `json:"drop_empty"`
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

func dataAggregateTool() tool.Tool {
	return New(
		"data_aggregate",
		"Groups a JSON array by a key field and aggregates another numeric field with op (sum/avg/count/min/max).",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"required": ["json", "group_by", "field"],
			"properties": {
				"json": {"type": "string", "description": "JSON array of objects"},
				"group_by": {"type": "string", "description": "field to group by"},
				"field": {"type": "string", "description": "numeric field to aggregate"},
				"op": {"type": "string", "description": "sum/avg/count/min/max, default sum"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				JSON    string `json:"json"`
				GroupBy string `json:"group_by"`
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

// ===== net =====

func ipLookupTool() tool.Tool {
	return New(
		"ip_lookup",
		"Returns the hostname and IP address of the local machine.",
		tool.RiskReadOnly,
		`{"type": "object", "properties": {}}`,
		func(_ context.Context, _ json.RawMessage) (any, error) {
			host := "unknown"
			if h, err := os.Hostname(); err == nil {
				host = h
			}
			ips := []string{}
			if addrs, err := net.InterfaceAddrs(); err == nil {
				for _, a := range addrs {
					if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
						ips = append(ips, ipnet.IP.String())
					}
				}
			}
			return map[string]any{"hostname": host, "ips": ips}, nil
		},
	)
}