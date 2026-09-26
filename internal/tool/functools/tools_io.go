package functools

// IO / 随机性主题：UUID / 随机字符串 / 随机数。
// 拆出来便于按主题定位；运行期注册走 base.go 的 All()，新增工具只动本文件。

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	mrand "math/rand"
	"strings"

	"github.com/google/uuid"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// ===== random =====

func randomUUIDTool() tool.Tool {
	return New(
		"random_uuid",
		"Returns a random UUID v4 string.",
		tool.RiskReadOnly,
		`{"type": "object", "properties": {}}`,
		func(_ context.Context, _ json.RawMessage) (any, error) {
			return uuid.NewString(), nil
		},
	)
}

func randomStringTool() tool.Tool {
	return New(
		"random_string",
		"Returns a random alphanumeric string of the given length (default 16).",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"properties": {
				"length": {"type": "integer", "description": "string length, default 16"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Length int `json:"length"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "random_string args parse failed", err)
			}
			n := req.Length
			if n <= 0 {
				n = 16
			}
			const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
			var sb strings.Builder
			sb.Grow(n)
			for i := 0; i < n; i++ {
				idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
				if err != nil {
					// crypto/rand 故障极端兜底：退化到 math/rand（工具不因熵源失败中断）
					idx = big.NewInt(int64(mrand.Intn(len(alphabet))))
				}
				sb.WriteByte(alphabet[idx.Int64()])
			}
			return sb.String(), nil
		},
	)
}

func randomNumberTool() tool.Tool {
	return New(
		"random_number",
		"Returns a random integer in [min, max] (both inclusive, default 0..100).",
		tool.RiskReadOnly,
		`{
			"type": "object",
			"properties": {
				"min": {"type": "integer", "description": "minimum, default 0"},
				"max": {"type": "integer", "description": "maximum, default 100"}
			}
		}`,
		func(_ context.Context, raw json.RawMessage) (any, error) {
			var req struct {
				Min int `json:"min"`
				Max int `json:"max"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, pkg.Wrap(4004, "random_number args parse failed", err)
			}
			lo, hi := req.Min, req.Max
			if hi == 0 && lo == 0 {
				hi = 100
			}
			if hi < lo {
				lo, hi = hi, lo
			}
			n, err := rand.Int(rand.Reader, big.NewInt(int64(hi-lo+1)))
			if err != nil {
				return nil, pkg.Wrap(4006, "random_number entropy source failed", err)
			}
			return int(n.Int64()) + lo, nil
		},
	)
}