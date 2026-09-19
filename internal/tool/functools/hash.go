package functools

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"strings"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// hashMd5Tool 返回文本的 MD5 十六进制摘要。
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

// hashSha256Tool 返回文本的 SHA 十六进制摘要（可选算法）。
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

// hashHmacTool 返回文本的 HMAC-SHA256 十六进制摘要。
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
