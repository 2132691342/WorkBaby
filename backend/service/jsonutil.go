package service

import (
	"encoding/json"

	"WorkBaby/backend/domain"
)

// marshalRows 把工作区历史编码进设置值；失败给空数组（历史丢了不影响当前工作区）。
func marshalRows(rows []domain.WorkspaceRowVO) string {
	raw, err := json.Marshal(rows)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// unmarshalRows 解码工作区历史；坏值当空历史。
func unmarshalRows(raw string) []domain.WorkspaceRowVO {
	if raw == "" {
		return nil
	}
	var rows []domain.WorkspaceRowVO
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return nil
	}
	return rows
}

// marshalStrings 编码字符串集合（隐藏轮台账用）。
func marshalStrings(values []string) string {
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// unmarshalStrings 解码字符串集合。
func unmarshalStrings(raw string) []string {
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}
