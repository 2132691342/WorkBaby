package pkg

import "encoding/json"

// UnmarshalJSON 是 json.Unmarshal 的一层薄封装：让调用方不必直接 import encoding/json。
func UnmarshalJSON(raw []byte, out any) error {
	return json.Unmarshal(raw, out)
}

// MarshalJSON 是 json.Marshal 的一层薄封装。
func MarshalJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

// MarshalJSONLine 是 MarshalJSON 的单行形式：会话文件按行追加，不能带缩进。
func MarshalJSONLine(value any) (string, error) {
	raw, err := MarshalJSON(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
