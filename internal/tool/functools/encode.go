package functools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"

	"WorkBaby/internal/pkg"
	"WorkBaby/internal/tool"
)

// base64EncodeTool 将文本编码为 Base64。
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

// base64DecodeTool 将 Base64 解码为文本。
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

// urlEncodeTool 对文本做 URL 百分号编码。
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

// urlDecodeTool 对文本做 URL 百分号解码。
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
